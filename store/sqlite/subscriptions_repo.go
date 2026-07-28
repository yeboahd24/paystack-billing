package sqlite

import (
	"context"
	"database/sql"

	"github.com/yeboahd24/paystack-billing/store"
	"github.com/yeboahd24/paystack-billing/store/db"
)

type SubscriptionRepository struct {
	q *db.Queries
}

func NewSubscriptionRepository(d *DB) *SubscriptionRepository {
	return &SubscriptionRepository{q: db.New(d.DB)}
}

var _ store.SubscriptionRepository = (*SubscriptionRepository)(nil)

func (r *SubscriptionRepository) GetActiveSubscription(ctx context.Context, userID int64) (*db.Subscription, error) {
	s, err := r.q.GetActiveSubscription(ctx, userID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SubscriptionRepository) ListUserSubscriptions(ctx context.Context, userID int64) ([]db.Subscription, error) {
	return r.q.ListUserSubscriptions(ctx, userID)
}

func (r *SubscriptionRepository) InsertSubscriptionIfNew(ctx context.Context, userID int64, plan, reference, periodEnd string) (int64, error) {
	res, err := r.q.InsertSubscriptionIfNew(ctx, db.InsertSubscriptionIfNewParams{
		UserID:            userID,
		Plan:              plan,
		ProviderReference: reference,
		CurrentPeriodEnd:  &periodEnd,
	})
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *SubscriptionRepository) CancelActiveSubscription(ctx context.Context, userID int64) error {
	return r.q.CancelActiveSubscription(ctx, userID)
}
