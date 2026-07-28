-- name: GetActiveSubscription :one
SELECT id, user_id, plan, provider_reference, status, current_period_end, created_at
FROM subscriptions
WHERE user_id = ? AND status = 'active'
LIMIT 1;

-- name: ListUserSubscriptions :many
SELECT id, user_id, plan, provider_reference, status, current_period_end, created_at
FROM subscriptions
WHERE user_id = ?
ORDER BY created_at DESC;

-- name: InsertSubscriptionIfNew :execresult
INSERT OR IGNORE INTO subscriptions (user_id, plan, provider_reference, status, current_period_end)
VALUES (?, ?, ?, 'active', ?);

-- name: CancelActiveSubscription :exec
UPDATE subscriptions SET status = 'cancelled'
WHERE user_id = ? AND status = 'active';
