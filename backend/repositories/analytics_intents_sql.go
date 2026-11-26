package repositories

import (
	"context"
	"time"
)

// Учёт поисковых намерений контура выдачи в SQL (§11.6): события с intent_id за период → исход каждого
// намерения и признак действия с товаром. Правила те же, что у services.aggregateIntents (эталон в Go):
//   - исход намерения — первое по времени событие-исход (search_commit, suggestion_select, instant_product_click,
//     category_select, vendor_select, search_abandon); нет такого — намерение открыто;
//   - действие с товаром — click, add_to_cart, order или price_request с тем же intent_id;
//   - исход search_commit уточняет payload.outcome (suggestion | product | category | vendor; иначе commit);
//   - число результатов отправленного запроса — payload.results_count, без него — поиска по request_id, без него —
//     первого поиска с тем же intent_id (поиск пишет intent_id из POST /api/search);
//   - показ панели пустой, если payload.items = 0; префиксы короче 2 символов не считаются (панель на них молчит).

// IntentGroup — намерения одного исхода (первого события-исхода; "" — открытые) и источника с признаками действий.
type IntentGroup struct {
	OutcomeEvent string
	Source       string
	Acted        bool // действие с товаром (клик, корзина, заказ, запрос цены)
	PanelProduct bool // клик по товару в панели
	N            int
}

// IntentRateRow — числитель и знаменатель доли.
type IntentRateRow struct {
	Num int
	Den int
}

// IntentSummary — агрегаты намерений периода.
type IntentSummary struct {
	Groups           []IntentGroup
	EmptyResults     IntentRateRow
	EmptySuggestions IntentRateRow
}

// IntentsSince — агрегаты намерений с момента since.
func (r *Repos) IntentsSince(ctx context.Context, since time.Time) (*IntentSummary, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	sum := &IntentSummary{}
	db := r.db.WithContext(ctx)
	err := db.Raw(`
WITH e AS (
	SELECT intent_id, type, created_at, id, payload, COALESCE(NULLIF(source, ''), 'live') AS source FROM behavior_events
	WHERE created_at >= @since AND intent_id IS NOT NULL AND intent_id <> ''
),
first_out AS (
	SELECT DISTINCT ON (intent_id) intent_id,
		CASE WHEN type = 'search_commit' THEN CASE COALESCE(payload->>'outcome', '')
			WHEN 'suggestion' THEN 'suggestion_select' WHEN 'product' THEN 'instant_product_click'
			WHEN 'category' THEN 'category_select' WHEN 'vendor' THEN 'vendor_select'
			ELSE 'search_commit' END ELSE type END AS type
	FROM e
	WHERE type IN ('search_commit', 'suggestion_select', 'instant_product_click', 'category_select', 'vendor_select',
		'search_abandon')
	ORDER BY intent_id, created_at, id
),
per AS (
	SELECT intent_id, MIN(source) AS source,
		bool_or(type IN ('click', 'add_to_cart', 'order', 'price_request')) AS acted,
		bool_or(type = 'instant_product_click') AS panel_product
	FROM e GROUP BY intent_id
)
SELECT COALESCE(f.type, '') AS outcome_event, p.source, p.acted, p.panel_product, COUNT(*) AS n
FROM per p LEFT JOIN first_out f USING (intent_id)
GROUP BY 1, 2, 3, 4 ORDER BY 1, 2, 3, 4`, map[string]any{"since": since}).Scan(&sum.Groups).Error
	if err != nil {
		return nil, err
	}
	err = db.Raw(`
SELECT COUNT(*) FILTER (WHERE n = 0) AS num, COUNT(n) AS den FROM (
	SELECT COALESCE(CASE WHEN jsonb_typeof(b.payload->'results_count') = 'number'
		THEN (b.payload->>'results_count')::numeric::int END, s.results_count,
		(SELECT si.results_count FROM search_events si WHERE b.intent_id IS NOT NULL AND si.intent_id = b.intent_id
			ORDER BY si.created_at, si.request_id LIMIT 1)) AS n
	FROM behavior_events b LEFT JOIN search_events s ON s.request_id = b.request_id
	WHERE b.created_at >= ? AND b.type = 'search_commit') x`, since).Scan(&sum.EmptyResults).Error
	if err != nil {
		return nil, err
	}
	err = db.Raw(`
SELECT COUNT(*) FILTER (WHERE items = 0) AS num, COUNT(items) AS den FROM (
	SELECT CASE WHEN jsonb_typeof(payload->'items') = 'number' THEN (payload->>'items')::numeric END AS items,
		CASE WHEN jsonb_typeof(payload->'prefix_len') = 'number' THEN (payload->>'prefix_len')::numeric END AS plen
	FROM behavior_events WHERE created_at >= ? AND type = 'suggest_impression') x
WHERE plen IS NULL OR plen >= 2`, since).Scan(&sum.EmptySuggestions).Error
	return sum, err
}
