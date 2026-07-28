package sqlite

import (
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/yeboahd24/paystack-billing/migrations"
)

type DB struct {
	*sql.DB
}

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
