package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/yeboahd24/paystack-billing/store"
	"github.com/yeboahd24/paystack-billing/store/db"
)

// SubscriptionRepository implements store.SubscriptionRepository for SQLite.
type SubscriptionRepository struct {
	q *db.Queries
}

// NewSubscriptionRepository creates a subscription repository backed by the given DB.
func NewSubscriptionRepository(d *DB) *SubscriptionRepository {
	return &SubscriptionRepository{q: db.New(d.DB)}
}

// NewSubscriptionRepositoryWithQueries creates a subscription repository using a specific queries instance.
func NewSubscriptionRepositoryWithQueries(q *db.Queries) *SubscriptionRepository {
	return &SubscriptionRepository{q: q}
}

var _ store.SubscriptionRepository = (*SubscriptionRepository)(nil)

func oneOrNil(s db.Subscription, err error) (*db.Subscription, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func affected(res sql.Result, err error) (int64, error) {
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *SubscriptionRepository) GetCurrentSubscription(ctx context.Context, userID int64) (*db.Subscription, error) {
	return oneOrNil(r.q.GetCurrentSubscription(ctx, userID))
}

func (r *SubscriptionRepository) GetSubscriptionByReference(ctx context.Context, reference string) (*db.Subscription, error) {
	return oneOrNil(r.q.GetSubscriptionByReference(ctx, reference))
}

func (r *SubscriptionRepository) ListUserSubscriptions(ctx context.Context, userID int64) ([]db.Subscription, error) {
	return r.q.ListUserSubscriptions(ctx, userID)
}

func (r *SubscriptionRepository) CreatePendingSubscription(ctx context.Context, userID int64, plan, reference string, amount int64) error {
	return r.q.CreatePendingSubscription(ctx, db.CreatePendingSubscriptionParams{
		UserID:            userID,
		Plan:              plan,
		ProviderReference: reference,
		Amount:            &amount,
	})
}

func (r *SubscriptionRepository) ActivatePendingSubscription(ctx context.Context, reference, periodStart, periodEnd string) (int64, error) {
	return affected(r.q.ActivatePendingSubscription(ctx, db.ActivatePendingSubscriptionParams{
		CurrentPeriodStart: &periodStart,
		CurrentPeriodEnd:   &periodEnd,
		ProviderReference:  reference,
	}))
}

func (r *SubscriptionRepository) InsertActiveSubscription(ctx context.Context, userID int64, plan, reference string, amount int64, periodStart, periodEnd string) (int64, error) {
	return affected(r.q.InsertActiveSubscription(ctx, db.InsertActiveSubscriptionParams{
		UserID:             userID,
		Plan:               plan,
		ProviderReference:  reference,
		Amount:             &amount,
		CurrentPeriodStart: &periodStart,
		CurrentPeriodEnd:   &periodEnd,
	}))
}

func (r *SubscriptionRepository) SupersedeSubscription(ctx context.Context, id int64) error {
	return r.q.SupersedeSubscription(ctx, id)
}

func (r *SubscriptionRepository) CancelCurrentSubscription(ctx context.Context, userID int64) (int64, error) {
	return affected(r.q.CancelCurrentSubscription(ctx, userID))
}

func (r *SubscriptionRepository) ExpireSubscription(ctx context.Context, id int64) (int64, error) {
	return affected(r.q.ExpireSubscription(ctx, id))
}

func (r *SubscriptionRepository) MarkSubscriptionRefunded(ctx context.Context, id int64) error {
	return r.q.MarkSubscriptionRefunded(ctx, id)
}

func (r *SubscriptionRepository) ListExpiredSubscriptions(ctx context.Context, now string, limit int64) ([]db.Subscription, error) {
	return r.q.ListExpiredSubscriptions(ctx, db.ListExpiredSubscriptionsParams{CurrentPeriodEnd: &now, Limit: limit})
}

func (r *SubscriptionRepository) SetSubscriptionPeriodEnd(ctx context.Context, id int64, periodEnd string) error {
	return r.q.SetSubscriptionPeriodEnd(ctx, db.SetSubscriptionPeriodEndParams{CurrentPeriodEnd: &periodEnd, ID: id})
}

func (r *SubscriptionRepository) RestoreSubscription(ctx context.Context, id int64, periodEnd string) (int64, error) {
	return affected(r.q.RestoreSubscription(ctx, db.RestoreSubscriptionParams{CurrentPeriodEnd: &periodEnd, ID: id}))
}
