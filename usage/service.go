// Package usage tracks daily API usage per user.
package usage

import (
	"context"
	"time"

	"github.com/yeboahd24/paystack-billing/plan"
	"github.com/yeboahd24/paystack-billing/store"
	"github.com/yeboahd24/paystack-billing/store/db"
)

// Service records and reports usage history.
type Service struct {
	repo  store.UsageRepository
	plans *plan.Service
}

// NewService creates a usage service with its dependencies.
func NewService(repo store.UsageRepository, plans *plan.Service) *Service {
	return &Service{repo: repo, plans: plans}
}

// History contains the user's daily usage breakdown.
type History struct {
	Daily   []db.UsageHistory
	Today   int
	Allowed int64
}

// Record increments the usage counter for the given user for today.
func (s *Service) Record(ctx context.Context, userID int64) error {
	return s.repo.RecordUsage(ctx, userID)
}

// GetHistory returns the usage history for the given user and tier over the last N days.
func (s *Service) GetHistory(ctx context.Context, userID int64, tier string, days int) (History, error) {
	daily, err := s.repo.GetUsageHistory(ctx, userID, days)
	if err != nil {
		return History{}, err
	}

	var today int
	if len(daily) > 0 && daily[0].Date == todayStr() {
		today = int(daily[0].Count)
	}

	p := s.plans.Get(tier)
	var allowed int64
	if p != nil {
		allowed = p.DailyLimit
	}

	return History{Daily: daily, Today: today, Allowed: allowed}, nil
}

func todayStr() string {
	return time.Now().UTC().Format("2006-01-02")
}
