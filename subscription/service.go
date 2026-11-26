// Package subscription manages the subscription lifecycle: checkout, activation,
// renewal, plan changes, cancellation, expiry and refunds.
//
// Every payment is one subscription row keyed by its provider reference. A user
// has at most one current row (active, or cancelled but still inside its paid
// period); older rows are kept as history with status superseded, expired or
// refunded.
package subscription

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/yeboahd24/paystack-billing/billing"
	"github.com/yeboahd24/paystack-billing/email"
	"github.com/yeboahd24/paystack-billing/plan"
	"github.com/yeboahd24/paystack-billing/store"
	"github.com/yeboahd24/paystack-billing/store/db"
)

// Subscription statuses.
const (
	StatusPending    = "pending"
	StatusActive     = "active"
	StatusCancelled  = "cancelled"
	StatusSuperseded = "superseded"
	StatusExpired    = "expired"
	StatusRefunded   = "refunded"
)

// TimeLayout is how period times are stored (always UTC).
const TimeLayout = "2006-01-02 15:04:05"

var (
	ErrInvalidPlan        = errors.New("invalid plan")
	ErrNoSubscription     = errors.New("no active subscription")
	ErrVerificationFailed = errors.New("transaction verification failed")

	// ErrPaymentRejected: the provider answered and the payment is not good
	// (not found, not successful, or wrong amount/currency). Wrapped with
	// ErrVerificationFailed.
	ErrPaymentRejected = errors.New("payment rejected by provider")
	// ErrReferenceMismatch: the webhook's user or plan differs from what the
	// checkout recorded for this reference.
	ErrReferenceMismatch = errors.New("payment does not match its checkout")
	// ErrUnknownPayment: no checkout was recorded for this reference and the
	// event carries no usable user/plan.
	ErrUnknownPayment = errors.New("unknown payment reference")
	// ErrPartialRefund: a refund smaller than the payment; access is left
	// unchanged for someone to review.
	ErrPartialRefund = errors.New("partial refund: access unchanged")
)

// IsPermanent reports whether err will fail the same way on every retry, so a
// webhook handler should answer 200 (and log it) rather than 5xx. Any other
// error (database, network, provider outage) is worth a retry: answer 5xx so the
// provider redelivers.
func IsPermanent(err error) bool {
	return errors.Is(err, ErrPaymentRejected) ||
		errors.Is(err, ErrInvalidPlan) ||
		errors.Is(err, ErrReferenceMismatch) ||
		errors.Is(err, ErrUnknownPayment) ||
		errors.Is(err, ErrPartialRefund)
}

// Timeouts for one unit of sweep work, independent of how many users expire.
const (
	sweepBatch        = 100
	sweepStepTimeout  = 10 * time.Second
	sweepEmailTimeout = 15 * time.Second
)

// Service orchestrates the subscription lifecycle.
type Service struct {
	subs   store.SubscriptionRepository
	users  store.UserRepository
	txp    store.TxProvider
	pay    billing.PaymentProvider
	plans  *plan.Service
	sender email.Sender
	now    func() time.Time
}

// NewService creates a subscription service with its dependencies.
func NewService(subs store.SubscriptionRepository, users store.UserRepository, txp store.TxProvider, pay billing.PaymentProvider, plans *plan.Service, sender email.Sender) *Service {
	return &Service{subs: subs, users: users, txp: txp, pay: pay, plans: plans, sender: sender, now: time.Now}
}

// InitCheckout starts a payment checkout for the given user and plan. It records
// a pending subscription with the price charged, so the payment is later checked
// against that amount even if the plan's price changes in between.
func (s *Service) InitCheckout(ctx context.Context, userID int64, planName, callbackURL string) (checkoutURL, reference string, err error) {
	p := s.plans.Get(planName)
	if p == nil || p.Amount <= 0 {
		return "", "", ErrInvalidPlan
	}
	user, err := s.users.GetUser(ctx, userID)
	if err != nil {
		return "", "", err
	}
	if user == nil {
		return "", "", fmt.Errorf("user not found")
	}
	checkoutURL, reference, err = s.pay.InitTransaction(ctx, user.Email, int(p.Amount), callbackURL, map[string]any{
		"user_id": userID,
		"plan":    planName,
	})
	if err != nil {
		return "", "", err
	}
	if err := s.subs.CreatePendingSubscription(ctx, userID, planName, reference, p.Amount); err != nil {
		return "", "", fmt.Errorf("record checkout: %w", err)
	}
	return checkoutURL, reference, nil
}

// errAlreadyProcessed rolls back a fulfilment that lost a race with a duplicate.
var errAlreadyProcessed = errors.New("already processed")

// HandleChargeSuccess processes a successful payment. It is safe to call more
// than once for the same reference (duplicate webhooks, webhook + callback).
//
// When the checkout was recorded by InitCheckout, its user, plan and amount are
// used and userID/planName (from webhook metadata) may be zero; if they are
// given and disagree, ErrReferenceMismatch is returned. Without a recorded
// checkout, userID and planName are required and the plan's current price is
// expected.
//
// The payment is verified with the provider, then, in one transaction:
//   - same plan as the current subscription (renewal): the new period starts
//     where the current one ends, so paying early loses nothing;
//   - different plan (upgrade/downgrade): starts now for one month, plus the
//     unused time of the old plan converted at the price ratio;
//   - no current subscription: starts now for one month.
//
// The previous current row becomes superseded, so only one row is ever current.
func (s *Service) HandleChargeSuccess(ctx context.Context, userID int64, planName, reference string) error {
	rec, err := s.subs.GetSubscriptionByReference(ctx, reference)
	if err != nil {
		return err
	}
	if rec != nil {
		if rec.Status != StatusPending {
			return nil // already fulfilled (or refunded): nothing to do
		}
		if (userID != 0 && userID != rec.UserID) || (planName != "" && planName != rec.Plan) {
			return fmt.Errorf("%w: reference %s", ErrReferenceMismatch, reference)
		}
		userID, planName = rec.UserID, rec.Plan
	}
	if userID == 0 || planName == "" {
		return fmt.Errorf("%w: %s", ErrUnknownPayment, reference)
	}
	p := s.plans.Get(planName)
	if p == nil || p.Amount <= 0 {
		return ErrInvalidPlan
	}
	amount := p.Amount
	if rec != nil && rec.Amount != nil && *rec.Amount > 0 {
		amount = *rec.Amount
	}

	ok, err := s.pay.VerifyTransaction(ctx, reference, int(amount))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrVerificationFailed, err)
	}
	if !ok {
		return fmt.Errorf("%w: %w", ErrVerificationFailed, ErrPaymentRejected)
	}

	now := s.now().UTC()
	err = s.txp.WithTx(ctx, func(subs store.SubscriptionRepository, users store.UserRepository) error {
		again, err := subs.GetSubscriptionByReference(ctx, reference)
		if err != nil {
			return err
		}
		if again != nil && again.Status != StatusPending {
			return errAlreadyProcessed
		}
		cur, err := subs.GetCurrentSubscription(ctx, userID)
		if err != nil {
			return err
		}
		end := s.periodEnd(cur, planName, amount, now)
		if cur != nil {
			if err := subs.SupersedeSubscription(ctx, cur.ID); err != nil {
				return err
			}
		}
		start, endStr := now.Format(TimeLayout), end.Format(TimeLayout)
		var n int64
		if again != nil {
			n, err = subs.ActivatePendingSubscription(ctx, reference, start, endStr)
		} else {
			n, err = subs.InsertActiveSubscription(ctx, userID, planName, reference, amount, start, endStr)
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return errAlreadyProcessed
		}
		return users.UpdateUserTier(ctx, userID, planName, p.DailyLimit)
	})
	if errors.Is(err, errAlreadyProcessed) {
		return nil
	}
	return err
}

// periodEnd decides when a new payment's access ends, given the user's current
// subscription (or nil).
func (s *Service) periodEnd(cur *db.Subscription, planName string, amount int64, now time.Time) time.Time {
	if cur == nil {
		return AddMonths(now, 1)
	}
	curEnd, ok := parseTime(cur.CurrentPeriodEnd)
	if !ok || !curEnd.After(now) {
		return AddMonths(now, 1)
	}
	if cur.Plan == planName {
		return AddMonths(curEnd, 1)
	}
	// Plan change: credit the unused time of the old plan at the price ratio
	// (e.g. 10 days left on a plan half the price → 5 extra days).
	end := AddMonths(now, 1)
	curAmount := s.amountOf(cur)
	if curAmount > 0 && amount > 0 {
		remaining := int64(curEnd.Sub(now) / time.Second)
		end = end.Add(time.Duration(remaining*curAmount/amount) * time.Second)
	}
	return end
}

func (s *Service) amountOf(sub *db.Subscription) int64 {
	if sub.Amount != nil && *sub.Amount > 0 {
		return *sub.Amount
	}
	if p := s.plans.Get(sub.Plan); p != nil {
		return p.Amount
	}
	return 0
}

// Cancel stops the user's subscription from renewing. Access continues until
// the end of the period already paid for, when the expiry sweep moves the user
// to free.
func (s *Service) Cancel(ctx context.Context, userID int64) error {
	n, err := s.subs.CancelCurrentSubscription(ctx, userID)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNoSubscription
	}
	return nil
}

// DowngradeExpired ends the user's subscription and moves them to free if its
// period is over. It reports whether it did.
func (s *Service) DowngradeExpired(ctx context.Context, userID int64) (bool, error) {
	cur, err := s.subs.GetCurrentSubscription(ctx, userID)
	if err != nil || cur == nil {
		return false, err
	}
	end, ok := parseTime(cur.CurrentPeriodEnd)
	if !ok || s.now().UTC().Before(end) {
		return false, nil
	}
	return s.expire(ctx, cur)
}

// expire marks sub expired and moves its user to free, atomically. It returns
// false when sub was no longer current (renewed or refunded meanwhile).
func (s *Service) expire(ctx context.Context, sub *db.Subscription) (bool, error) {
	free := s.plans.Get("free")
	if free == nil {
		return false, fmt.Errorf("free plan not found")
	}
	done := false
	err := s.txp.WithTx(ctx, func(subs store.SubscriptionRepository, users store.UserRepository) error {
		n, err := subs.ExpireSubscription(ctx, sub.ID)
		if err != nil || n == 0 {
			return err
		}
		done = true
		return users.UpdateUserTier(ctx, sub.UserID, "free", free.DailyLimit)
	})
	return done && err == nil, err
}

// SweepExpired ends every subscription whose period is over (active or
// cancelled), moves those users to free and emails them. It works in batches
// with a timeout per step, so it doesn't fail as a whole when many users expire
// at once. It returns how many users were downgraded.
func (s *Service) SweepExpired(ctx context.Context) int {
	total := 0
	for ctx.Err() == nil {
		stepCtx, cancel := context.WithTimeout(ctx, sweepStepTimeout)
		subs, err := s.subs.ListExpiredSubscriptions(stepCtx, s.now().UTC().Format(TimeLayout), sweepBatch)
		cancel()
		if err != nil {
			log.Printf("sweep: list expired: %v", err)
			return total
		}
		handled := 0
		for i := range subs {
			downgraded, ok := s.expireAndNotify(ctx, &subs[i])
			if ok {
				handled++
			}
			if downgraded {
				total++
			}
		}
		// Stop when nothing is left, or when a whole batch failed (those rows
		// would come back first every time; the next sweep retries them).
		if len(subs) < sweepBatch || handled == 0 {
			return total
		}
	}
	return total
}

// expireAndNotify expires one subscription and emails the user. ok is false
// only when expiring failed.
func (s *Service) expireAndNotify(ctx context.Context, sub *db.Subscription) (downgraded, ok bool) {
	stepCtx, cancel := context.WithTimeout(ctx, sweepStepTimeout)
	defer cancel()
	downgraded, err := s.expire(stepCtx, sub)
	if err != nil {
		log.Printf("sweep: expire subscription %d (user %d): %v", sub.ID, sub.UserID, err)
		return false, false
	}
	if !downgraded {
		return false, true
	}
	user, err := s.users.GetUser(stepCtx, sub.UserID)
	if err != nil || user == nil {
		log.Printf("sweep: user %d downgraded; no email (lookup: %v)", sub.UserID, err)
		return true, true
	}
	mailCtx, cancelMail := context.WithTimeout(ctx, sweepEmailTimeout)
	defer cancelMail()
	if err := s.sender.Send(mailCtx, user.Email, user.Name, "Subscription Expired", "<p>Your subscription has ended. You've been moved to the free tier.</p>"); err != nil {
		log.Printf("sweep: send expiry email to user %d: %v", user.ID, err)
	}
	return true, true
}

// HandleRefund removes the access a refunded payment bought. refundedAmount is
// in the smallest currency unit; 0 means unknown and is treated as a full
// refund. A refund smaller than the payment returns ErrPartialRefund and changes
// nothing, for someone to review. Calling it again for the same reference does
// nothing.
//
// The refunded payment's month comes off the user's current subscription when
// that month is part of it:
//   - refunded payment is the current one: if it renewed a same-plan payment,
//     that earlier payment becomes current again with the time it paid for;
//     otherwise the user moves to free;
//   - refunded payment was renewed by the current one (same plan): the current
//     period loses a month;
//   - anything else (expired, pending, a different plan): marked refunded only.
//
// When no paid time is left the user moves to free.
func (s *Service) HandleRefund(ctx context.Context, reference string, refundedAmount int64) error {
	sub, err := s.subs.GetSubscriptionByReference(ctx, reference)
	if err != nil {
		return err
	}
	if sub == nil {
		return fmt.Errorf("%w: %s", ErrUnknownPayment, reference)
	}
	if sub.Status == StatusRefunded {
		return nil
	}
	if paid := s.amountOf(sub); refundedAmount > 0 && paid > 0 && refundedAmount < paid {
		return fmt.Errorf("%w: refunded %d of %d on %s", ErrPartialRefund, refundedAmount, paid, reference)
	}
	free := s.plans.Get("free")
	if free == nil {
		return fmt.Errorf("free plan not found")
	}
	now := s.now().UTC()
	toFree := func(users store.UserRepository) error {
		return users.UpdateUserTier(ctx, sub.UserID, "free", free.DailyLimit)
	}
	return s.txp.WithTx(ctx, func(subs store.SubscriptionRepository, users store.UserRepository) error {
		cur, err := subs.GetCurrentSubscription(ctx, sub.UserID)
		if err != nil {
			return err
		}
		if err := subs.MarkSubscriptionRefunded(ctx, sub.ID); err != nil {
			return err
		}
		if cur == nil {
			return nil
		}
		end, ok := parseTime(cur.CurrentPeriodEnd)
		if ok {
			end = AddMonths(end, -1)
		}
		left := ok && end.After(now)

		switch {
		case cur.ID == sub.ID:
			if !left {
				return toFree(users)
			}
			prev, err := previousSamePlan(ctx, subs, sub)
			if err != nil {
				return err
			}
			if prev == nil {
				return toFree(users)
			}
			_, err = subs.RestoreSubscription(ctx, prev.ID, end.Format(TimeLayout))
			return err
		case sub.Status == StatusSuperseded && cur.Plan == sub.Plan:
			if !left {
				if _, err := subs.ExpireSubscription(ctx, cur.ID); err != nil {
					return err
				}
				return toFree(users)
			}
			return subs.SetSubscriptionPeriodEnd(ctx, cur.ID, end.Format(TimeLayout))
		}
		return nil
	})
}

// previousSamePlan returns the newest superseded row of the same plan created
// before sub: the payment sub renewed from.
func previousSamePlan(ctx context.Context, subs store.SubscriptionRepository, sub *db.Subscription) (*db.Subscription, error) {
	history, err := subs.ListUserSubscriptions(ctx, sub.UserID)
	if err != nil {
		return nil, err
	}
	for i := range history {
		h := &history[i]
		if h.ID < sub.ID && h.Status == StatusSuperseded && h.Plan == sub.Plan {
			return h, nil
		}
	}
	return nil, nil
}

func parseTime(v *string) (time.Time, bool) {
	if v == nil || *v == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(TimeLayout, *v)
	return t, err == nil
}

// AddMonths adds n calendar months, clamping to the last day of the target
// month: 31 Jan + 1 month = 28 Feb (29 in a leap year), not 3 March.
func AddMonths(t time.Time, n int) time.Time {
	y, m, d := t.Date()
	first := time.Date(y, m+time.Month(n), 1, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
	last := first.AddDate(0, 1, -1).Day()
	return first.AddDate(0, 0, min(d, last)-1)
}
