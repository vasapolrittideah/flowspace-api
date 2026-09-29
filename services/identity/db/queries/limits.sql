-- name: IncrementLimitCounter :one
INSERT INTO identity_limit_counters (scope, counter_key, action, window_start, count)
VALUES (sqlc.arg(scope), sqlc.arg(counter_key), sqlc.arg(action), sqlc.arg(window_start), 1)
ON CONFLICT (scope, counter_key, action, window_start)
DO UPDATE SET count = identity_limit_counters.count + 1
RETURNING count;

-- name: GetLimitUsage :one
SELECT COALESCE(SUM(count) FILTER (WHERE window_start > sqlc.arg(hour_cutoff)::timestamptz), 0)::bigint AS hourly_count,
       COALESCE(SUM(count) FILTER (WHERE window_start > sqlc.arg(day_cutoff)::timestamptz), 0)::bigint AS daily_count,
       (SELECT window_start FROM identity_limit_counters AS recent
        WHERE recent.scope = sqlc.arg(scope)
          AND recent.counter_key = sqlc.arg(counter_key)
          AND recent.action = sqlc.arg(action)
        ORDER BY window_start DESC LIMIT 1) AS latest_at
FROM identity_limit_counters
WHERE scope = sqlc.arg(scope)
  AND counter_key = sqlc.arg(counter_key)
  AND action = sqlc.arg(action)
  AND window_start > sqlc.arg(day_cutoff)::timestamptz;
