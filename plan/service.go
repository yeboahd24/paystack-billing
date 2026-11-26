// Package plan provides plan management and tier comparison.
package plan

import (
	"context"
	"fmt"
	"sync"

	"github.com/yeboahd24/paystack-billing/store"
	"github.com/yeboahd24/paystack-billing/store/db"
)

var ErrInvalidPlan = fmt.Errorf("unknown plan")

// Service loads and caches plans from the database. It is safe for concurrent
// use; call Refresh (or run worker.RunPlanRefresh) to pick up price changes
// without a restart.
type Service struct {
	repo   store.PlanRepository
	mu     sync.RWMutex
	list   []db.Plan
	byName map[string]db.Plan
}

// NewService loads all plans from the repository into memory.
func NewService(ctx context.Context, repo store.PlanRepository) (*Service, error) {
	list, err := repo.ListPlans(ctx)
	if err != nil {
		return nil, fmt.Errorf("load plans: %w", err)
	}
	return &Service{repo: repo, list: list, byName: index(list)}, nil
}

// List returns all known plans in order.
func (s *Service) List() []db.Plan {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]db.Plan(nil), s.list...)
}

func index(list []db.Plan) map[string]db.Plan {
	byName := make(map[string]db.Plan, len(list))
	for _, p := range list {
		byName[p.Name] = p
	}
	return byName
}

// Get returns the plan with the given name, or nil.
func (s *Service) Get(name string) *db.Plan {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.byName[name]
	if !ok {
		return nil
	}
	return &p
}

// DailyLimit returns the daily request limit for the named plan.
func (s *Service) DailyLimit(name string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.byName[name]
	if !ok {
		return 0, ErrInvalidPlan
	}
	return p.DailyLimit, nil
}

// Amount returns the price (in smallest currency unit) for the named plan.
func (s *Service) Amount(name string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.byName[name]
	if !ok {
		return 0, ErrInvalidPlan
	}
	return p.Amount, nil
}

// Refresh reloads all plans from the repository, replacing the in-memory cache.
func (s *Service) Refresh(ctx context.Context) error {
	list, err := s.repo.ListPlans(ctx)
	if err != nil {
		return fmt.Errorf("refresh plans: %w", err)
	}
	s.mu.Lock()
	s.list, s.byName = list, index(list)
	s.mu.Unlock()
	return nil
}

// TierAtLeast checks whether userTier is at least minTier in the plan ordering.
func (s *Service) TierAtLeast(userTier, minTier string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
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
