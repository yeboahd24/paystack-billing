package store

import (
	"context"

	"github.com/yeboahd24/paystack-billing/store/db"
)

type PlanRepository interface {
	ListPlans(ctx context.Context) ([]db.Plan, error)
	GetPlan(ctx context.Context, name string) (*db.Plan, error)
}

type SubscriptionRepository interface {
	GetActiveSubscription(ctx context.Context, userID int64) (*db.Subscription, error)
	ListUserSubscriptions(ctx context.Context, userID int64) ([]db.Subscription, error)
	InsertSubscriptionIfNew(ctx context.Context, userID int64, plan, reference, periodEnd string) (int64, error)
	CancelActiveSubscription(ctx context.Context, userID int64) error
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
