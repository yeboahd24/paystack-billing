package subscription

import (
	"context"
	"fmt"
	"time"

	"github.com/dominicyeboah/paystack-billing/internal/billing"
	"github.com/dominicyeboah/paystack-billing/internal/email"
	"github.com/dominicyeboah/paystack-billing/internal/plan"
	"github.com/dominicyeboah/paystack-billing/store"
)

var (
	ErrInvalidPlan    = fmt.Errorf("invalid plan")
	ErrNoSubscription = fmt.Errorf("no active subscription")
)

type Service struct {
	subs   store.SubscriptionRepository
	users  store.UserRepository
	pay    billing.PaymentProvider
	plans  *plan.Service
	sender email.Sender
}

func NewService(subs store.SubscriptionRepository, users store.UserRepository, pay billing.PaymentProvider, plans *plan.Service, sender email.Sender) *Service {
	return &Service{subs: subs, users: users, pay: pay, plans: plans, sender: sender}
}

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

func (s *Service) HandleChargeSuccess(ctx context.Context, userID int64, planName, reference string) error {
	p := s.plans.Get(planName)
	if p == nil {
		return ErrInvalidPlan
	}

	periodEnd := time.Now().AddDate(0, 1, 0).UTC().Format("2006-01-02 15:04:05")

	rows, err := s.subs.InsertSubscriptionIfNew(ctx, userID, planName, reference, periodEnd)
	if err != nil {
		return err
	}
	if rows == 0 {
		return nil
	}

	return s.users.UpdateUserTier(ctx, userID, planName, p.DailyLimit)
}

func (s *Service) Cancel(ctx context.Context, userID int64) error {
	return s.subs.CancelActiveSubscription(ctx, userID)
}

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

func (s *Service) SweepExpired(ctx context.Context) {
	users, err := s.users.ListPaidTierUsers(ctx)
	if err != nil {
		return
	}
	for _, u := range users {
		downgraded, err := s.DowngradeExpired(ctx, u.ID)
		if err != nil {
			continue
		}
		if downgraded {
			s.sender.Send(ctx, u.Email, u.Name, "Subscription Expired", "<p>Your subscription has ended. You've been moved to the free tier.</p>")
		}
	}
}
