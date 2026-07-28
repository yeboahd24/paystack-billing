-- name: RecordUsage :exec
INSERT INTO usage_history (user_id, date, count)
VALUES (?, date('now'), 1)
ON CONFLICT(user_id, date) DO UPDATE SET count = count + 1;

-- name: GetUsageHistory :many
SELECT id, user_id, date, count
FROM usage_history
WHERE user_id = ? AND date >= date('now', ?)
ORDER BY date DESC;
