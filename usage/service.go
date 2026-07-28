package usage

import (
	"context"
	"time"

	"github.com/yeboahd24/paystack-billing/plan"
	"github.com/yeboahd24/paystack-billing/store"
	"github.com/yeboahd24/paystack-billing/store/db"
)

type Service struct {
	repo  store.UsageRepository
	plans *plan.Service
}

func NewService(repo store.UsageRepository, plans *plan.Service) *Service {
	return &Service{repo: repo, plans: plans}
}

type History struct {
	Daily   []db.UsageHistory
	Today   int
	Allowed int64
}

func (s *Service) Record(ctx context.Context, userID int64) error {
	return s.repo.RecordUsage(ctx, userID)
}

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
