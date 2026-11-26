package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Вход агента SearchOps в SQL (бэклог после ревью M6): исходы поисков по нормализованному тексту считает база
// тем же outcomesCTE, что аналитика, — события периода в память API не читаются. Эталон — services.computeQueryStats
// (равенство проверяет services/agent_stats_sql_test.go на живой базе). Отличия от аналитики: переформулировка
// требует непустого следующего запроса, а для запроса отдаются ещё цели переформулировок и товары кликов.

// AgentQueryRow — исходы одного нормализованного запроса.
type AgentQueryRow struct {
	Query        string `json:"q"`
	Freq         int    `json:"freq"`
	Empty        int    `json:"empty"`
	Reformulated int    `json:"reformulated"`
	NoClick      int    `json:"no_click"`
	Clicks       int    `json:"clicks"`
	Orders       int    `json:"orders"`
	FailSessions int    `json:"fail_sessions"`
}

// AgentReformRow — запрос → во что его переформулировали.
type AgentReformRow struct {
	Query     string `json:"q"`
	Target    string `json:"target"`
	Count     int    `json:"count"`
	Clicked   int    `json:"clicked"`
	ClickedOn string `json:"clicked_on"`
}

// AgentClickRow — запрос → товар клика из поиска.
type AgentClickRow struct {
	Query     string `json:"q"`
	ProductID string `json:"product_id"`
	N         int    `json:"n"`
}

// AgentQueryStats — исходы поисков [since, until) и что осталось за until.
type AgentQueryStats struct {
	Queries []AgentQueryRow
	Reforms []AgentReformRow
	Clicks  []AgentClickRow
}

// SearchSpan — последний поиск периода: исторический (source не live/traffic) и любой; без поисков — nil.
type SearchSpan struct {
	LastHistorical *time.Time
	LastAny        *time.Time
}

// searchFilter — те же поиски, что SearchEventsSince: без прогонов оценки и вкладки «Один запрос».
const searchFilter = `request_id NOT LIKE 'eval-%' AND source <> 'compare'`

// LogScope — какие поиски журнала относятся к текущему каталогу (вход агента на фиде, контур фида): живые и поиски
// генератора трафика — только той версии каталога, на которой они сделаны (Catalog; пусто — любой; поиск без версии
// — неизвестно, на каком каталоге, не отсекается); исторический
// журнал (mock_log, версии каталога у него нет) — только если он есть у текущего источника (Historical). Весь
// журнал без отбора — Unscoped.
type LogScope struct {
	Catalog    string
	Historical bool
	All        bool
	// Since — с какого момента действует текущий каталог (первая версия правил с его catalog_version). У событий
	// поведения версии каталога нет: живые и трафик до этого момента — о прежнем каталоге. Нулевое — без границы.
	Since time.Time
}

// Unscoped — весь журнал: прежний вход агента и тесты.
var Unscoped = LogScope{All: true}

// scopeSQL — условие отбора поисков по LogScope (параметры @catalog, @historical, @all).
const scopeSQL = `(@all OR CASE WHEN COALESCE(NULLIF(source, ''), 'live') IN ('live', 'traffic')
	THEN (@catalog = '' OR COALESCE(catalog_version, '') IN ('', @catalog)) ELSE @historical END)`

// behaviorScopeSQL — тот же отбор для событий поведения (behavior_events): живые и трафик — с начала текущего
// каталога, исторические — только у источника с журналом (параметры @catalog_since, @historical, @all).
const behaviorScopeSQL = `(@all OR CASE WHEN COALESCE(NULLIF(source, ''), 'live') IN ('live', 'traffic')
	THEN created_at >= @catalog_since ELSE @historical END)`

func (sc LogScope) args(m map[string]any) map[string]any {
	m["catalog"], m["historical"], m["all"], m["catalog_since"] = sc.Catalog, sc.Historical, sc.All, sc.Since
	return m
}

// agentOutcomesCTE — outcomesCTE аналитики с отбором поисков по LogScope в первом шаге (s).
var agentOutcomesCTE = func() string {
	const anchor = "AND source <> 'compare'\n),"
	if !strings.Contains(outcomesCTE, anchor) {
		panic("agent_stats_sql: в outcomesCTE нет места для отбора по версии каталога")
	}
	return strings.Replace(outcomesCTE, anchor, "AND source <> 'compare' AND "+scopeSQL+"\n),", 1)
}()

// SearchSpanSince — последние поиски с момента since (для границы отложенной недели агента).
func (r *Repos) SearchSpanSince(ctx context.Context, since time.Time, sc LogScope) (SearchSpan, error) {
	var res SearchSpan
	if !r.Enabled() {
		return res, ErrNoDatabase
	}
	err := r.db.WithContext(ctx).Raw(`SELECT MAX(created_at) FILTER (WHERE source NOT IN ('live', 'traffic', '')) AS last_historical,
	MAX(created_at) AS last_any FROM search_events WHERE created_at >= @since AND `+searchFilter+` AND `+scopeSQL,
		sc.args(map[string]any{"since": since})).Scan(&res).Error
	return res, err
}

// CountSearchesSince — число поисков с момента since (скрытые от агента поиски отложенной недели).
func (r *Repos) CountSearchesSince(ctx context.Context, since time.Time, sc LogScope) (int, error) {
	if !r.Enabled() {
		return 0, ErrNoDatabase
	}
	var n int64
	err := r.db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM search_events WHERE created_at >= @since AND `+searchFilter+
		` AND `+scopeSQL, sc.args(map[string]any{"since": since})).Scan(&n).Error
	return int(n), err
}

// AgentQueryStatsBetween — исходы всех поисков периода (без отбора по каталогу): эталон для тестов.
func (r *Repos) AgentQueryStatsBetween(ctx context.Context, since, until time.Time) (*AgentQueryStats, error) {
	return r.AgentQueryStatsScoped(ctx, since, until, Unscoped)
}

// AgentQueryStatsScoped — исходы поисков периода [since, until) по нормализованному тексту, только поиски из sc: один
// проход по событиям, три выборки из CTE собираются в JSON. Пустой текст запроса не считается.
func (r *Repos) AgentQueryStatsScoped(ctx context.Context, since, until time.Time, sc LogScope) (*AgentQueryStats, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var row struct {
		Queries []byte
		Reforms []byte
		Clicks  []byte
	}
	err := r.db.WithContext(ctx).Raw(agentOutcomesCTE+`,
n AS (
	SELECT o.*, LEAD(q) OVER w AS nq, LEAD(created_at) OVER w AS nat, LEAD(clicked) OVER w AS nclicked,
		LEAD(product) OVER w AS nproduct
	FROM o WINDOW w AS (PARTITION BY sess ORDER BY created_at, request_id)
),
r AS (
	SELECT n.*, (NOT clicked AND nat IS NOT NULL AND nat - created_at <= interval '60 seconds'
		AND nq <> q AND nq <> '') AS reform
	FROM n WHERE q <> ''
)
SELECT
	(SELECT COALESCE(json_agg(x), '[]') FROM (
		SELECT q, COUNT(*) AS freq,
			COUNT(*) FILTER (WHERE results_count = 0) AS empty,
			COUNT(*) FILTER (WHERE results_count <> 0 AND reform) AS reformulated,
			COUNT(*) FILTER (WHERE results_count <> 0 AND NOT reform AND NOT clicked) AS no_click,
			COUNT(*) FILTER (WHERE clicked) AS clicks,
			COUNT(*) FILTER (WHERE ordered) AS orders,
			COUNT(DISTINCT sess) FILTER (WHERE results_count = 0 OR NOT clicked) AS fail_sessions
		FROM r GROUP BY q) x) AS queries,
	(SELECT COALESCE(json_agg(x), '[]') FROM (
		SELECT q, nq AS target, COUNT(*) AS count, COUNT(*) FILTER (WHERE nclicked) AS clicked,
			COALESCE((array_agg(nproduct ORDER BY created_at, request_id)
				FILTER (WHERE nclicked AND nproduct IS NOT NULL))[1], '') AS clicked_on
		FROM r WHERE reform GROUP BY q, nq) x) AS reforms,
	(SELECT COALESCE(json_agg(x), '[]') FROM (
		SELECT q, product AS product_id, COUNT(*) AS n
		FROM r WHERE clicked AND product IS NOT NULL GROUP BY q, product) x) AS clicks`,
		sc.args(map[string]any{"since": since, "until": until})).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	res := &AgentQueryStats{}
	for _, f := range []struct {
		raw []byte
		dst any
	}{{row.Queries, &res.Queries}, {row.Reforms, &res.Reforms}, {row.Clicks, &res.Clicks}} {
		if err := json.Unmarshal(f.raw, f.dst); err != nil {
			return nil, fmt.Errorf("вход агента: %w", err)
		}
	}
	return res, nil
}
