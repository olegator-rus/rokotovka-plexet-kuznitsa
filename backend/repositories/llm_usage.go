package repositories

import (
	"context"
	"time"

	"kuznica/api/domain"
)

// SaveLLMUsage — учёт одного вызова модели.
func (r *Repos) SaveLLMUsage(ctx context.Context, u *domain.LLMUsage) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	return r.db.WithContext(ctx).Create(u).Error
}

// RouteCostRow — итоги поисков одного маршрута: число, время, стоимость модели, источники разбора.
type RouteCostRow struct {
	Route   string
	Count   int64
	AvgMs   float64
	CostRub float64
	LLM     int64 // разобрано моделью
	Cache   int64 // разбор взят из кэша
}

// EvalRequestPrefix — префикс request_id вызовов модели из прогонов оценки (services.evalRequestPrefix).
const EvalRequestPrefix = "eval-"

// WarmupRequestPrefix — префикс request_id вызовов модели при прогреве кэша разборов (go run . warmup).
const WarmupRequestPrefix = "warmup-"

// RouteCosts — живые поиски (source = live) по маршрутам и стоимость вызовов модели, привязанных к ним по
// request_id. Импортированные исторические логи (mock_log) и прогоны оценки (eval-…) не считаются.
func (r *Repos) RouteCosts(ctx context.Context) ([]RouteCostRow, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var rows []RouteCostRow
	err := r.db.WithContext(ctx).Raw(`
SELECT e.route AS route, count(*) AS count, coalesce(avg(e.total_ms), 0) AS avg_ms,
       coalesce(sum(u.cost), 0) AS cost_rub,
       count(*) FILTER (WHERE e.parse_source = 'llm') AS llm,
       count(*) FILTER (WHERE e.parse_source = 'cache') AS cache
FROM search_events e
LEFT JOIN (SELECT request_id, sum(calculated_cost) AS cost FROM llm_usage
           WHERE request_id IS NOT NULL GROUP BY request_id) u ON u.request_id = e.request_id
WHERE e.source = 'live' AND e.route <> ''
GROUP BY e.route ORDER BY e.route`).Scan(&rows).Error
	return rows, err
}

// LLMTotals — итоги журнала вызовов: всего, по поиску, по агенту.
type LLMTotals struct {
	Calls         int64
	CostRub       float64
	AgentRuns     int64
	AgentCostRub  float64
	EvalCalls     int64
	EvalCostRub   float64
	JudgeCalls    int64
	JudgeCostRub  float64
	WarmupCalls   int64
	WarmupCostRub float64
	InputTokens   int64
	OutputTokens  int64
	AvgLatencyMs  float64
}

// LLMUsageTotals — итоги llm_usage.
func (r *Repos) LLMUsageTotals(ctx context.Context) (LLMTotals, error) {
	var t LLMTotals
	if !r.Enabled() {
		return t, ErrNoDatabase
	}
	err := r.db.WithContext(ctx).Raw(`
SELECT count(*) AS calls, coalesce(sum(calculated_cost), 0) AS cost_rub,
       count(DISTINCT agent_run_id) AS agent_runs,
       coalesce(sum(calculated_cost) FILTER (WHERE agent_run_id IS NOT NULL), 0) AS agent_cost_rub,
       count(*) FILTER (WHERE request_id LIKE ?) AS eval_calls,
       coalesce(sum(calculated_cost) FILTER (WHERE request_id LIKE ?), 0) AS eval_cost_rub,
       count(*) FILTER (WHERE request_id LIKE ?) AS judge_calls,
       coalesce(sum(calculated_cost) FILTER (WHERE request_id LIKE ?), 0) AS judge_cost_rub,
       count(*) FILTER (WHERE request_id LIKE ?) AS warmup_calls,
       coalesce(sum(calculated_cost) FILTER (WHERE request_id LIKE ?), 0) AS warmup_cost_rub,
       coalesce(sum(input_tokens), 0) AS input_tokens, coalesce(sum(output_tokens), 0) AS output_tokens,
       coalesce(avg(latency_ms), 0) AS avg_latency_ms
FROM llm_usage`, EvalRequestPrefix+"%", EvalRequestPrefix+"%", JudgeRequestPrefix+"%", JudgeRequestPrefix+"%",
		WarmupRequestPrefix+"%", WarmupRequestPrefix+"%").Scan(&t).Error
	return t, err
}

// ListLLMUsage — журнал вызовов, новые сверху, и общее число строк.
func (r *Repos) ListLLMUsage(ctx context.Context, limit, offset int) ([]domain.LLMUsage, int64, error) {
	if !r.Enabled() {
		return nil, 0, ErrNoDatabase
	}
	var total int64
	db := r.db.WithContext(ctx).Model(&domain.LLMUsage{})
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []domain.LLMUsage
	err := r.db.WithContext(ctx).Order("created_at DESC").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
}

// LLMCostSince — расход на модель (все вызовы: поиск, агент, оценка, судья) с момента since.
func (r *Repos) LLMCostSince(ctx context.Context, since time.Time) (float64, error) {
	if !r.Enabled() {
		return 0, ErrNoDatabase
	}
	var v float64
	err := r.db.WithContext(ctx).Raw(`SELECT coalesce(sum(calculated_cost), 0) FROM llm_usage WHERE created_at >= ?`, since).Scan(&v).Error
	return v, err
}
