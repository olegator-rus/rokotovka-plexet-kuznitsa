package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Агрегаты аналитики поиска в SQL (ревью M6): события периода не читаются в память API — база считает исход
// каждого поиска оконными функциями и отдаёт суммы. Правила те же, что у services.buildOutcomes (эталон в Go,
// равенство проверяет analytics_sql_test.go на живой базе):
//   - поиски прогона оценки (request_id eval-…) и вкладки «Один запрос» (source = compare) не считаются;
//   - набор до Enter: поиск, который в той же сессии в течение 10 с продолжен более длинным запросом с тем же
//     началом, отбрасывается;
//   - действие покупателя (click, add_to_cart, order) относится к поиску по request_id, а без него — к
//     последнему поиску той же сессии не позже действия и не раньше чем за 30 мин;
//   - переформулировка — следующий поиск сессии в течение 60 с с другим текстом, а клика по этому не было.
//
// Нормализация запроса — как services.foldQuery: нижний регистр, ё → е, одиночные пробелы.

// outcomesCTE — исход каждого поиска периода [@since, @until): o(request_id, created_at, results_count, source,
// sess, q, clicked, carted, ordered, product, reformulated). product — товар первого (по времени) действия с
// товаром, отнесённого к поиску.
const outcomesCTE = `
WITH s AS (
	SELECT request_id, created_at, results_count,
		COALESCE(NULLIF(source, ''), 'live') AS source,
		COALESCE(NULLIF(session_id, ''), 'req:' || request_id) AS sess,
		btrim(regexp_replace(replace(lower(query), 'ё', 'е'), '\s+', ' ', 'g')) AS q
	FROM search_events
	WHERE created_at >= @since AND created_at < @until AND request_id NOT LIKE 'eval-%' AND source <> 'compare'
),
typed AS (
	SELECT s.*, LEAD(q) OVER w AS next_q, LEAD(created_at) OVER w AS next_at
	FROM s WINDOW w AS (PARTITION BY sess ORDER BY created_at, request_id)
),
kept AS (
	SELECT request_id, created_at, results_count, source, sess, q FROM typed
	WHERE NOT (next_at IS NOT NULL AND next_at - created_at <= interval '10 seconds'
		AND next_q <> q AND starts_with(next_q, q))
),
b AS (
	SELECT id, type, request_id, session_id, created_at, product_id FROM behavior_events
	WHERE created_at >= @since AND created_at < @until AND type IN ('click', 'add_to_cart', 'order')
),
by_req AS (
	SELECT b.id, b.type, b.created_at, b.product_id, k.request_id AS rid FROM b JOIN kept k ON k.request_id = b.request_id
),
unmatched AS (
	SELECT b.id, b.type, b.session_id AS sess, b.created_at, b.product_id FROM b
	LEFT JOIN kept k ON k.request_id = b.request_id
	WHERE k.request_id IS NULL AND b.session_id IS NOT NULL
),
stream AS (
	SELECT sess, created_at, 0 AS kind, request_id, NULL::text AS type, NULL::uuid AS id, NULL::text AS product_id
	FROM kept WHERE sess IN (SELECT sess FROM unmatched)
	UNION ALL
	SELECT sess, created_at, 1, NULL, type, id, product_id FROM unmatched
),
grp AS (
	SELECT stream.*, COUNT(request_id) OVER (PARTITION BY sess ORDER BY created_at, kind, request_id
		ROWS UNBOUNDED PRECEDING) AS g
	FROM stream
),
by_sess AS (
	SELECT a.id, a.type, a.created_at, a.product_id, s.request_id AS rid
	FROM grp a JOIN grp s ON s.sess = a.sess AND s.g = a.g AND s.kind = 0
	WHERE a.kind = 1 AND a.g > 0 AND a.created_at - s.created_at <= interval '30 minutes'
),
flags AS (
	SELECT rid, bool_or(type IN ('add_to_cart', 'order')) AS carted, bool_or(type = 'order') AS ordered,
		(array_agg(product_id ORDER BY created_at, id) FILTER (WHERE product_id <> ''))[1] AS product
	FROM (SELECT id, type, created_at, product_id, rid FROM by_req
		UNION ALL SELECT id, type, created_at, product_id, rid FROM by_sess) x GROUP BY rid
),
outc AS (
	SELECT k.*, f.rid IS NOT NULL AS clicked, COALESCE(f.carted, false) AS carted, COALESCE(f.ordered, false) AS ordered,
		f.product
	FROM kept k LEFT JOIN flags f ON f.rid = k.request_id
),
o AS (
	SELECT outc.*, (NOT clicked AND LEAD(created_at) OVER w IS NOT NULL
		AND LEAD(created_at) OVER w - created_at <= interval '60 seconds' AND LEAD(q) OVER w <> q) AS reformulated
	FROM outc WINDOW w AS (PARTITION BY sess ORDER BY created_at, request_id)
)`

// openEnd — верхняя граница периода «до сейчас» (@until без отсечки).
var openEnd = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)

// SearchTotals — суммы исходов поисков одного источника (source = "" — по всем источникам).
type SearchTotals struct {
	Source   string `json:"source"`
	Searches int    `json:"searches"`
	Empty    int    `json:"empty"` // выдача пустая
	NonEmpty int    `json:"non_empty"`
	NoClick  int    `json:"no_click"` // выдача не пустая, клика нет
	Clicks   int    `json:"clicks"`   // поиски с кликом (корзина и заказ — тоже клик)
	Carts    int    `json:"carts"`
	Orders   int    `json:"orders"`
	// Problems — поиски с пустой выдачей или без клика. Счёт сессий (problem/click/order_sessions) нужен был только
	// модели эффекта и удалён с ней в контуре доводки.
	Problems int `json:"problems"`
}

// SearchSummary — суммы исходов за период: по источникам (по алфавиту) и итог.
type SearchSummary struct {
	BySource []SearchTotals
	Total    SearchTotals
}

// QueryOutcome — исходы одного нормализованного запроса.
type QueryOutcome struct {
	Query        string `json:"query"`
	Freq         int    `json:"freq"`
	Empty        int    `json:"empty"`
	Reformulated int    `json:"reformulated"` // не пустая выдача, переформулировка
	NoClick      int    `json:"no_click"`     // не пустая выдача, без переформулировки и без клика
	Clicks       int    `json:"clicks"`
	Orders       int    `json:"orders"`
}

// totalsSQL — суммы исходов из o (выражение для json_build_object).
const totalsSQL = `'searches', COUNT(*),
	'empty', COUNT(*) FILTER (WHERE results_count = 0),
	'non_empty', COUNT(*) FILTER (WHERE results_count <> 0),
	'no_click', COUNT(*) FILTER (WHERE results_count <> 0 AND NOT clicked),
	'clicks', COUNT(*) FILTER (WHERE clicked),
	'carts', COUNT(*) FILTER (WHERE carted),
	'orders', COUNT(*) FILTER (WHERE ordered),
	'problems', COUNT(*) FILTER (WHERE results_count = 0 OR NOT clicked)`

// SearchOutcomesSince — суммы исходов поисков с момента since и до limit проблемных запросов (пустая выдача,
// переформулировка, нет клика) по убыванию неудач, затем частоты, затем по тексту. Один проход по событиям:
// CTE o считается один раз, три выборки из него собираются в JSON. limit = 0 — только суммы.
func (r *Repos) SearchOutcomesSince(ctx context.Context, since time.Time, limit int) (*SearchSummary, []QueryOutcome, error) {
	if !r.Enabled() {
		return nil, nil, ErrNoDatabase
	}
	var row struct {
		Total    []byte
		BySource []byte
		Problems []byte
	}
	err := r.db.WithContext(ctx).Raw(outcomesCTE+`
SELECT
	(SELECT json_build_object('source', '', `+totalsSQL+`) FROM o) AS total,
	(SELECT COALESCE(json_agg(t ORDER BY t->>'source'), '[]') FROM (
		SELECT json_build_object('source', source, `+totalsSQL+`) AS t FROM o GROUP BY source) x) AS by_source,
	(SELECT COALESCE(json_agg(p ORDER BY n), '[]') FROM (
		SELECT json_build_object('query', q, 'freq', COUNT(*),
			'empty', COUNT(*) FILTER (WHERE results_count = 0),
			'reformulated', COUNT(*) FILTER (WHERE results_count <> 0 AND reformulated),
			'no_click', COUNT(*) FILTER (WHERE results_count <> 0 AND NOT reformulated AND NOT clicked),
			'clicks', COUNT(*) FILTER (WHERE clicked),
			'orders', COUNT(*) FILTER (WHERE ordered)) AS p,
			ROW_NUMBER() OVER (ORDER BY COUNT(*) FILTER (WHERE results_count = 0 OR NOT clicked) DESC, COUNT(*) DESC,
				q COLLATE "C") AS n
		FROM o WHERE q <> '' AND @limit > 0
		GROUP BY q
		HAVING COUNT(*) FILTER (WHERE results_count = 0 OR NOT clicked) > 0
		ORDER BY n LIMIT @limit) y) AS problems`,
		map[string]any{"since": since, "until": openEnd, "limit": limit}).Scan(&row).Error
	if err != nil {
		return nil, nil, err
	}
	sum := &SearchSummary{BySource: []SearchTotals{}}
	var problems []QueryOutcome
	for _, f := range []struct {
		raw []byte
		dst any
	}{{row.Total, &sum.Total}, {row.BySource, &sum.BySource}, {row.Problems, &problems}} {
		if err := json.Unmarshal(f.raw, f.dst); err != nil {
			return nil, nil, fmt.Errorf("агрегаты аналитики: %w", err)
		}
	}
	return sum, problems, nil
}

// ProductCount — товар и число событий с ним.
type ProductCount struct {
	ProductID string
	N         int
}

// OrderedProductsSince — заказанные товары с момента since: товар → число заказов (для среднего чека по ценам
// каталога).
func (r *Repos) OrderedProductsSince(ctx context.Context, since time.Time) ([]ProductCount, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var rows []ProductCount
	err := r.db.WithContext(ctx).Raw(`SELECT product_id, COUNT(*) AS n FROM behavior_events
WHERE created_at >= ? AND type = 'order' AND product_id IS NOT NULL GROUP BY product_id ORDER BY product_id`, since).
		Scan(&rows).Error
	return rows, err
}
