package plan

import (
	"context"
	"fmt"

	"github.com/yeboahd24/paystack-billing/store"
	"github.com/yeboahd24/paystack-billing/store/db"
)

var ErrInvalidPlan = fmt.Errorf("unknown plan")

type Service struct {
	repo   store.PlanRepository
	list   []db.Plan
	byName map[string]db.Plan
}

func NewService(ctx context.Context, repo store.PlanRepository) (*Service, error) {
	list, err := repo.ListPlans(ctx)
	if err != nil {
		return nil, fmt.Errorf("load plans: %w", err)
	}
	byName := make(map[string]db.Plan, len(list))
	for _, p := range list {
		byName[p.Name] = p
	}
	return &Service{repo: repo, list: list, byName: byName}, nil
}

func (s *Service) List() []db.Plan {
	return s.list
}

func (s *Service) Get(name string) *db.Plan {
	p, ok := s.byName[name]
	if !ok {
		return nil
	}
	return &p
}

func (s *Service) DailyLimit(name string) (int64, error) {
	p, ok := s.byName[name]
	if !ok {
		return 0, ErrInvalidPlan
	}
	return p.DailyLimit, nil
}

func (s *Service) Amount(name string) (int64, error) {
	p, ok := s.byName[name]
	if !ok {
		return 0, ErrInvalidPlan
	}
	return p.Amount, nil
}

func (s *Service) TierAtLeast(userTier, minTier string) bool {
	userRank := -1
	minRank := -1
	for i, p := range s.list {
		if p.Name == userTier {
			userRank = i
		}
		if p.Name == minTier {
			minRank = i
		}
	}
	return userRank >= minRank
}
