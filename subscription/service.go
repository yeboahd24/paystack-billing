// Package subscription manages the subscription lifecycle: checkout, activation, cancellation, and expiry.
package subscription

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/yeboahd24/paystack-billing/billing"
	"github.com/yeboahd24/paystack-billing/email"
	"github.com/yeboahd24/paystack-billing/plan"
	"github.com/yeboahd24/paystack-billing/store"
)

var (
	ErrInvalidPlan         = fmt.Errorf("invalid plan")
	ErrNoSubscription      = fmt.Errorf("no active subscription")
	ErrVerificationFailed  = fmt.Errorf("transaction verification failed")
)

// Service orchestrates the subscription lifecycle.
type Service struct {
	subs   store.SubscriptionRepository
	users  store.UserRepository
	txp    store.TxProvider
	pay    billing.PaymentProvider
	plans  *plan.Service
	sender email.Sender
}

// NewService creates a subscription service with its dependencies.
func NewService(subs store.SubscriptionRepository, users store.UserRepository, txp store.TxProvider, pay billing.PaymentProvider, plans *plan.Service, sender email.Sender) *Service {
	return &Service{subs: subs, users: users, txp: txp, pay: pay, plans: plans, sender: sender}
}

// InitCheckout starts a payment checkout for the given user and plan.
func (s *Service) InitCheckout(ctx context.Context, userID int64, planName, callbackURL string) (checkoutURL, reference string, err error) {
	p := s.plans.Get(planName)
	if p == nil {
		return "", "", ErrInvalidPlan
	}
	user, err := s.users.GetUser(ctx, userID)
	if err != nil {
		return "", "", err
	}
	if user == nil {
		return "", "", fmt.Errorf("user not found")
	}
	return s.pay.InitTransaction(ctx, user.Email, int(p.Amount), callbackURL, map[string]any{
		"user_id": userID,
		"plan":    planName,
	})
}

// HandleChargeSuccess processes a successful payment: verifies the transaction server-side,
// then atomically inserts the subscription and upgrades the user's tier.
func (s *Service) HandleChargeSuccess(ctx context.Context, userID int64, planName, reference string) error {
	p := s.plans.Get(planName)
	if p == nil {
		return ErrInvalidPlan
	}

	ok, err := s.pay.VerifyTransaction(ctx, reference, int(p.Amount))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrVerificationFailed, err)
	}
	if !ok {
		return ErrVerificationFailed
	}

	periodEnd := time.Now().AddDate(0, 1, 0).UTC().Format("2006-01-02 15:04:05")

	return s.txp.WithTx(ctx, func(subs store.SubscriptionRepository, users store.UserRepository) error {
		rows, err := subs.InsertSubscriptionIfNew(ctx, userID, planName, reference, periodEnd)
		if err != nil {
			return err
		}
		if rows == 0 {
			return nil
		}
		return users.UpdateUserTier(ctx, userID, planName, p.DailyLimit)
	})
}

// Cancel marks the user's active subscription as cancelled.
func (s *Service) Cancel(ctx context.Context, userID int64) error {
	return s.subs.CancelActiveSubscription(ctx, userID)
}

// DowngradeExpired checks if a user's subscription has expired and downgrades to free.
func (s *Service) DowngradeExpired(ctx context.Context, userID int64) (bool, error) {
	sub, err := s.subs.GetActiveSubscription(ctx, userID)
	if err != nil {
		return false, err
	}
	if sub == nil {
		return false, nil
	}
	if sub.CurrentPeriodEnd == nil || *sub.CurrentPeriodEnd == "" {
		return false, nil
	}
	periodEnd, err := time.Parse("2006-01-02 15:04:05", *sub.CurrentPeriodEnd)
	if err != nil {
		return false, nil
	}
	if time.Now().UTC().Before(periodEnd) {
		return false, nil
	}

	free := s.plans.Get("free")
	if free == nil {
		return false, fmt.Errorf("free plan not found")
	}
	return true, s.users.UpdateUserTier(ctx, userID, "free", free.DailyLimit)
}

// SweepExpired iterates all paid-tier users and downgrades any with expired subscriptions.
func (s *Service) SweepExpired(ctx context.Context) {
	users, err := s.users.ListPaidTierUsers(ctx)
	if err != nil {
		log.Printf("sweep: list paid users: %v", err)
		return
	}
	for _, u := range users {
		downgraded, err := s.DowngradeExpired(ctx, u.ID)
		if err != nil {
			log.Printf("sweep: downgrade user %d: %v", u.ID, err)
			continue
		}
		if downgraded {
			if err := s.sender.Send(ctx, u.Email, u.Name, "Subscription Expired", "<p>Your subscription has ended. You've been moved to the free tier.</p>"); err != nil {
				log.Printf("sweep: send expiry email to %s: %v", u.Email, err)
			}
		}
	}
}
