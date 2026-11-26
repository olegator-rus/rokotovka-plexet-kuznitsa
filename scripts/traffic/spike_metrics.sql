-- Метрики триггера «всплеск» по журналу (раздел 2): скользящее окно 60 мин, отметки
-- каждые 5 мин. Только чтение. Запуск:
--   psql "$DATABASE_URL" -v start=2026-05-27T12:13:42Z -v src=traffic -f scripts/traffic/spike_metrics.sql
-- src — traffic (только генератор) или live,traffic (весь живой журнал).
WITH s AS (
  SELECT request_id, created_at, results_count,
         COALESCE(NULLIF(session_id, ''), 'req:' || request_id) AS sess,
         btrim(regexp_replace(replace(lower(query), 'ё', 'е'), '\s+', ' ', 'g')) AS q
  FROM search_events
  WHERE source = ANY (string_to_array(:'src', ',')) AND created_at >= :'start'::timestamptz
    AND request_id NOT LIKE 'eval-%'
),
c AS (SELECT DISTINCT request_id FROM behavior_events
      WHERE type = 'click' AND request_id IS NOT NULL AND created_at >= :'start'::timestamptz),
n AS (
  SELECT s.*, (c.request_id IS NOT NULL) AS clicked, LEAD(q) OVER w AS nq, LEAD(created_at) OVER w AS nat
  FROM s LEFT JOIN c USING (request_id) WINDOW w AS (PARTITION BY sess ORDER BY created_at, request_id)
),
r AS (SELECT *, (NOT clicked AND nat IS NOT NULL AND nat - created_at <= interval '60 seconds'
                 AND nq <> q AND nq <> '') AS reform FROM n),
marks AS (SELECT generate_series(date_trunc('minute', :'start'::timestamptz) + interval '5 min',
                                 (SELECT max(created_at) FROM s) + interval '5 min', interval '5 min') AS m)
SELECT to_char(m AT TIME ZONE 'Europe/Moscow', 'DD.MM HH24:MI') AS t,
  count(r.*) AS searches,
  round(avg((results_count = 0)::int)::numeric, 3) AS empty_share,
  round(avg(reform::int)::numeric, 3) AS reform_share,
  (SELECT max(k) FROM (SELECT count(*) AS k FROM r r2 WHERE r2.created_at > m - interval '60 min' AND r2.created_at <= m
      AND (r2.results_count = 0 OR r2.reform) GROUP BY r2.q) z) AS max_fail_freq,
  (SELECT q FROM r r2 WHERE r2.created_at > m - interval '60 min' AND r2.created_at <= m
      AND (r2.results_count = 0 OR r2.reform) GROUP BY r2.q ORDER BY count(*) DESC, q LIMIT 1) AS top_fail_q
FROM marks LEFT JOIN r ON r.created_at > m - interval '60 min' AND r.created_at <= m
GROUP BY m ORDER BY m;
