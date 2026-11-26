-- name: GetCurrentSubscription :one
-- The subscription that gives the user access now: active, or cancelled but
-- still inside its paid period. At most one exists (subscriptions_one_current).
SELECT * FROM subscriptions
WHERE user_id = ? AND status IN ('active', 'cancelled')
ORDER BY id DESC
LIMIT 1;

-- name: GetSubscriptionByReference :one
SELECT * FROM subscriptions
WHERE provider_reference = ?;

-- name: ListUserSubscriptions :many
SELECT * FROM subscriptions
WHERE user_id = ?
ORDER BY created_at DESC, id DESC;

-- name: CreatePendingSubscription :exec
INSERT OR IGNORE INTO subscriptions (user_id, plan, provider_reference, status, amount)
VALUES (?, ?, ?, 'pending', ?);

-- name: ActivatePendingSubscription :execresult
UPDATE subscriptions
SET status = 'active', current_period_start = ?, current_period_end = ?
WHERE provider_reference = ? AND status = 'pending';

-- name: InsertActiveSubscription :execresult
INSERT OR IGNORE INTO subscriptions
    (user_id, plan, provider_reference, status, amount, current_period_start, current_period_end)
VALUES (?, ?, ?, 'active', ?, ?, ?);

-- name: SupersedeSubscription :exec
UPDATE subscriptions SET status = 'superseded'
WHERE id = ? AND status IN ('active', 'cancelled');

-- name: CancelCurrentSubscription :execresult
UPDATE subscriptions SET status = 'cancelled'
WHERE user_id = ? AND status = 'active';

-- name: ExpireSubscription :execresult
UPDATE subscriptions SET status = 'expired'
WHERE id = ? AND status IN ('active', 'cancelled');

-- name: MarkSubscriptionRefunded :exec
UPDATE subscriptions SET status = 'refunded'
WHERE id = ?;

-- name: ListExpiredSubscriptions :many
-- Current subscriptions whose period has ended (period ends are UTC
-- "YYYY-MM-DD HH:MM:SS", so text comparison is time order).
SELECT * FROM subscriptions
WHERE status IN ('active', 'cancelled') AND current_period_end <= ?
ORDER BY current_period_end
LIMIT ?;

-- name: SetSubscriptionPeriodEnd :exec
UPDATE subscriptions SET current_period_end = ?
WHERE id = ?;

-- name: RestoreSubscription :execresult
-- Makes a superseded row current again (after the payment that replaced it was refunded).
UPDATE subscriptions SET status = 'active', current_period_end = ?
WHERE id = ? AND status = 'superseded';
