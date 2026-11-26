package store

import (
	"context"

	"github.com/yeboahd24/paystack-billing/store/db"
)

type PlanRepository interface {
	ListPlans(ctx context.Context) ([]db.Plan, error)
	GetPlan(ctx context.Context, name string) (*db.Plan, error)
}

// SubscriptionRepository stores subscriptions. Each payment is one row, keyed by
// its provider reference; a user has at most one current row (status active or
// cancelled). Times are UTC "2006-01-02 15:04:05".
type SubscriptionRepository interface {
	// GetCurrentSubscription returns the active or cancelled-but-running
	// subscription, or nil.
	GetCurrentSubscription(ctx context.Context, userID int64) (*db.Subscription, error)
	GetSubscriptionByReference(ctx context.Context, reference string) (*db.Subscription, error)
	ListUserSubscriptions(ctx context.Context, userID int64) ([]db.Subscription, error)
	// CreatePendingSubscription records a checkout and the amount it charges.
	CreatePendingSubscription(ctx context.Context, userID int64, plan, reference string, amount int64) error
	// ActivatePendingSubscription turns a pending row active; it returns 0 when
	// the row is not pending (already processed, or never created).
	ActivatePendingSubscription(ctx context.Context, reference, periodStart, periodEnd string) (int64, error)
	// InsertActiveSubscription records a paid subscription that had no pending
	// row; it returns 0 when the reference is already recorded.
	InsertActiveSubscription(ctx context.Context, userID int64, plan, reference string, amount int64, periodStart, periodEnd string) (int64, error)
	SupersedeSubscription(ctx context.Context, id int64) error
	// CancelCurrentSubscription stops renewal; access runs to the period end.
	CancelCurrentSubscription(ctx context.Context, userID int64) (int64, error)
	// ExpireSubscription ends a current subscription; it returns 0 when it was
	// no longer current.
	ExpireSubscription(ctx context.Context, id int64) (int64, error)
	MarkSubscriptionRefunded(ctx context.Context, id int64) error
	SetSubscriptionPeriodEnd(ctx context.Context, id int64, periodEnd string) error
	// RestoreSubscription makes a superseded row current again with the given
	// period end; it returns 0 when the row was not superseded.
	RestoreSubscription(ctx context.Context, id int64, periodEnd string) (int64, error)
	// ListExpiredSubscriptions returns current subscriptions whose period ended
	// at or before now, oldest first.
	ListExpiredSubscriptions(ctx context.Context, now string, limit int64) ([]db.Subscription, error)
}

type UsageRepository interface {
	RecordUsage(ctx context.Context, userID int64) error
	GetUsageHistory(ctx context.Context, userID int64, days int) ([]db.UsageHistory, error)
}

type UserRepository interface {
	GetUser(ctx context.Context, id int64) (*db.User, error)
	GetUserByEmail(ctx context.Context, email string) (*db.User, error)
	CreateUser(ctx context.Context, email, name, tier string, dailyLimit int64) (*db.User, error)
	UpdateUserTier(ctx context.Context, id int64, tier string, dailyLimit int64) error
	ListPaidTierUsers(ctx context.Context) ([]db.User, error)
}

type TxProvider interface {
	WithTx(ctx context.Context, fn func(SubscriptionRepository, UserRepository) error) error
}
