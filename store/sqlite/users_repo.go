package sqlite

import (
	"context"
	"database/sql"

	"github.com/yeboahd24/paystack-billing/store"
	"github.com/yeboahd24/paystack-billing/store/db"
)

// UserRepository implements store.UserRepository for SQLite.
type UserRepository struct {
	q *db.Queries
}

// NewUserRepository creates a user repository backed by the given DB.
func NewUserRepository(d *DB) *UserRepository {
	return &UserRepository{q: db.New(d.DB)}
}

// NewUserRepositoryWithQueries creates a user repository using a specific queries instance.
func NewUserRepositoryWithQueries(q *db.Queries) *UserRepository {
	return &UserRepository{q: q}
}

var _ store.UserRepository = (*UserRepository)(nil)

func (r *UserRepository) GetUser(ctx context.Context, id int64) (*db.User, error) {
	u, err := r.q.GetUser(ctx, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) GetUserByEmail(ctx context.Context, email string) (*db.User, error) {
	u, err := r.q.GetUserByEmail(ctx, email)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) CreateUser(ctx context.Context, email, name, tier string, dailyLimit int64) (*db.User, error) {
	res, err := r.q.CreateUser(ctx, db.CreateUserParams{
		Email:      email,
		Name:       name,
		Tier:       tier,
		DailyLimit: dailyLimit,
	})
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return r.GetUser(ctx, id)
}

func (r *UserRepository) UpdateUserTier(ctx context.Context, id int64, tier string, dailyLimit int64) error {
	return r.q.UpdateUserTier(ctx, db.UpdateUserTierParams{
		ID:         id,
		Tier:       tier,
		DailyLimit: dailyLimit,
	})
}

func (r *UserRepository) ListPaidTierUsers(ctx context.Context) ([]db.User, error) {
	return r.q.ListPaidTierUsers(ctx)
}
