-- name: ListPlans :many
SELECT name, display_name, amount, daily_limit, retention_days, description
FROM plans
ORDER BY amount ASC;

-- name: GetPlan :one
SELECT name, display_name, amount, daily_limit, retention_days, description
FROM plans
WHERE name = ?;
