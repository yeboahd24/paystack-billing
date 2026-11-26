-- +goose Up

-- What was charged for this subscription (smallest currency unit), recorded at
-- checkout so a later price change can't reject a payment already in flight.
ALTER TABLE subscriptions ADD COLUMN amount INTEGER;
ALTER TABLE subscriptions ADD COLUMN current_period_start TEXT;

-- Statuses:
--   pending     checkout started, not paid yet
--   active      paid, renews or expires at current_period_end
--   cancelled   paid, won't renew; access continues until current_period_end
--   superseded  replaced by a renewal or a plan change
--   expired     period ended, user moved back to free
--   refunded    payment refunded, access removed
--
-- Older versions let a user collect several 'active' rows (one per payment).
-- Keep only the newest current row per user; the rest become 'superseded'.
UPDATE subscriptions SET status = 'superseded'
WHERE status IN ('active', 'cancelled')
  AND id NOT IN (
    SELECT MAX(id) FROM subscriptions
    WHERE status IN ('active', 'cancelled')
    GROUP BY user_id
  );

-- At most one current subscription per user.
CREATE UNIQUE INDEX subscriptions_one_current
    ON subscriptions (user_id) WHERE status IN ('active', 'cancelled');

CREATE INDEX subscriptions_current_period_end_idx
    ON subscriptions (current_period_end) WHERE status IN ('active', 'cancelled');

-- +goose Down
DROP INDEX IF EXISTS subscriptions_current_period_end_idx;
DROP INDEX IF EXISTS subscriptions_one_current;
ALTER TABLE subscriptions DROP COLUMN current_period_start;
ALTER TABLE subscriptions DROP COLUMN amount;
