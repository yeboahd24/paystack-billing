// Package sqlite provides SQLite-backed implementations of the store interfaces.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/yeboahd24/paystack-billing/migrations"
	"github.com/yeboahd24/paystack-billing/store"
	"github.com/yeboahd24/paystack-billing/store/db"
)

// DB wraps a SQLite connection with auto-migration support.
type DB struct {
	*sql.DB
}

// Open opens a SQLite database, runs migrations, and returns a DB handle.
func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	conn.SetMaxOpenConns(1)

	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return nil, fmt.Errorf("goose dialect: %w", err)
	}
	if err := goose.Up(conn, "."); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &DB{conn}, nil
}

// WithTx runs fn inside a database transaction, committing on success or rolling back on error.
func (d *DB) WithTx(ctx context.Context, fn func(store.SubscriptionRepository, store.UserRepository) error) error {
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	q := db.New(tx)
	subRepo := NewSubscriptionRepositoryWithQueries(q)
	userRepo := NewUserRepositoryWithQueries(q)
	if err := fn(subRepo, userRepo); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
