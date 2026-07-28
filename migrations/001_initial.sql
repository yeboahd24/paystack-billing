-- +goose Up

CREATE TABLE users (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    email       TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    tier        TEXT NOT NULL DEFAULT 'free',
    daily_limit INTEGER NOT NULL DEFAULT 20,
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE plans (
    name           TEXT PRIMARY KEY,
    display_name   TEXT NOT NULL,
    amount         INTEGER NOT NULL DEFAULT 0,
    daily_limit    INTEGER NOT NULL DEFAULT 20,
    retention_days INTEGER,
    description    TEXT NOT NULL DEFAULT ''
);

CREATE TABLE subscriptions (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id             INTEGER NOT NULL REFERENCES users(id),
    plan                TEXT NOT NULL,
    provider_reference  TEXT NOT NULL UNIQUE,
    status              TEXT NOT NULL DEFAULT 'pending',
    current_period_end  TEXT,
    created_at          TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX subscriptions_user_idx ON subscriptions (user_id);

CREATE TABLE usage_history (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id),
    date    TEXT NOT NULL,
    count   INTEGER NOT NULL DEFAULT 0,
    UNIQUE(user_id, date)
);

-- +goose Down
DROP TABLE IF EXISTS usage_history;
DROP TABLE IF EXISTS subscriptions;
DROP TABLE IF EXISTS plans;
DROP TABLE IF EXISTS users;
