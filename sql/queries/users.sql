-- name: GetUser :one
SELECT id, email, name, tier, daily_limit, created_at
FROM users
WHERE id = ?;

-- name: GetUserByEmail :one
SELECT id, email, name, tier, daily_limit, created_at
FROM users
WHERE email = ?;

-- name: CreateUser :execresult
INSERT INTO users (email, name, tier, daily_limit)
VALUES (?, ?, ?, ?);

-- name: UpdateUserTier :exec
UPDATE users SET tier = ?, daily_limit = ?
WHERE id = ?;

-- name: ListPaidTierUsers :many
SELECT id, email, name, tier, daily_limit, created_at
FROM users
WHERE tier != 'free';
