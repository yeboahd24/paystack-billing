package sqlite

import (
	"context"
	"fmt"

	"github.com/yeboahd24/paystack-billing/store"
	"github.com/yeboahd24/paystack-billing/store/db"
)

// UsageRepository implements store.UsageRepository for SQLite.
type UsageRepository struct {
	q *db.Queries
}

// NewUsageRepository creates a usage repository backed by the given DB.
func NewUsageRepository(d *DB) *UsageRepository {
	return &UsageRepository{q: db.New(d.DB)}
}

var _ store.UsageRepository = (*UsageRepository)(nil)

func (r *UsageRepository) RecordUsage(ctx context.Context, userID int64) error {
	return r.q.RecordUsage(ctx, userID)
}

func (r *UsageRepository) GetUsageHistory(ctx context.Context, userID int64, days int) ([]db.UsageHistory, error) {
	return r.q.GetUsageHistory(ctx, db.GetUsageHistoryParams{
		UserID: userID,
		Date:   fmt.Sprintf("-%d days", days),
	})
}
