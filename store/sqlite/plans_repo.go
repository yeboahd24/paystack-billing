package sqlite

import (
	"context"
	"database/sql"

	"github.com/yeboahd24/paystack-billing/store"
	"github.com/yeboahd24/paystack-billing/store/db"
)

// PlanRepository implements store.PlanRepository for SQLite.
type PlanRepository struct {
	q *db.Queries
}

// NewPlanRepository creates a plan repository backed by the given DB.
func NewPlanRepository(d *DB) *PlanRepository {
	return &PlanRepository{q: db.New(d.DB)}
}

var _ store.PlanRepository = (*PlanRepository)(nil)

func (r *PlanRepository) ListPlans(ctx context.Context) ([]db.Plan, error) {
	return r.q.ListPlans(ctx)
}

func (r *PlanRepository) GetPlan(ctx context.Context, name string) (*db.Plan, error) {
	p, err := r.q.GetPlan(ctx, name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}
