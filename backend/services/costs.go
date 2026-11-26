package services

import (
	"context"
	"math"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
	"kuznica/api/repositories"
)

// Экономика (ARCHITECTURE §7): итоги по журналам search_events и llm_usage. Всё считает
// сервер; чего нет в журнале — null, а не ноль.

const (
	defaultCostsLimit = 50
	maxCostsLimit     = 200
)

// routesOrder — маршруты в таблице, все четыре даже без поисков.
var routesOrder = []string{domain.RouteExact, domain.RouteNormalized, domain.RouteSemantic, domain.RouteIntent}

// Costs — GET /api/costs?limit=&offset=.
func (s *Services) Costs(ctx context.Context, limit, offset int) (*out.Costs, error) {
	var bad []errs.FieldError
	if limit == 0 {
		limit = defaultCostsLimit
	}
	if limit < 1 || limit > maxCostsLimit {
		bad = append(bad, errs.FieldError{Field: "limit", Code: errs.CodeOutOfRange, Message: "Лимит — от 1 до 200"})
	}
	if offset < 0 {
		bad = append(bad, errs.FieldError{Field: "offset", Code: errs.CodeOutOfRange, Message: "Смещение не может быть отрицательным"})
	}
	if len(bad) > 0 {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(bad...)
	}
	if !s.repos.Enabled() {
		return nil, errs.NewUnavailableError("Журнал стоимости недоступен: сервис запущен без базы данных")
	}
	routes, err := s.repos.RouteCosts(ctx)
	if err != nil {
		return nil, err
	}
	totals, err := s.repos.LLMUsageTotals(ctx)
	if err != nil {
		return nil, err
	}
	calls, total, err := s.repos.ListLLMUsage(ctx, limit, offset)
	if err != nil {
		return nil, err
	}

	res := &out.Costs{
		LLMMode: s.llmMode,
		Pricing: out.CostsPricing{Version: PricingVersion, Date: PricingDate, Currency: "RUB", Prices: []out.CostsPrice{}},
		Routes:  []out.CostsRoute{},
		Calls:   out.CostsCalls{Total: total, Limit: limit, Offset: offset, Items: []out.CostsCall{}},
	}
	for _, p := range modelPrices {
		res.Pricing.Prices = append(res.Pricing.Prices, out.CostsPrice{Model: p.Model, Input: p.Input, Output: p.Output})
	}

	byRoute := map[string]out.CostsRoute{}
	sum := &res.Summary
	for _, r := range routes {
		sum.Searches += r.Count
		sum.LLMParses += r.LLM
		sum.CacheHits += r.Cache
		sum.SearchLLMCostRub += r.CostRub
		row := out.CostsRoute{Route: r.Route, Count: r.Count, LLMParses: r.LLM, CacheHits: r.Cache}
		if r.Count > 0 {
			row.AvgMs = costsRatio(r.AvgMs, 1, 2)
			row.AvgCostRub = costsRatio(r.CostRub, float64(r.Count), 6)
		}
		byRoute[r.Route] = row
	}
	for _, route := range routesOrder {
		row, ok := byRoute[route]
		if !ok {
			row = out.CostsRoute{Route: route}
		}
		if sum.Searches > 0 {
			row.Share = costsRatio(float64(row.Count), float64(sum.Searches), 4)
		}
		res.Routes = append(res.Routes, row)
		delete(byRoute, route)
	}
	for _, row := range byRoute { // маршрут вне A–D (старые записи) — в конец, чтобы сумма сходилась
		if sum.Searches > 0 {
			row.Share = costsRatio(float64(row.Count), float64(sum.Searches), 4)
		}
		res.Routes = append(res.Routes, row)
	}
	sum.SearchLLMCostRub = math.Round(sum.SearchLLMCostRub*1e4) / 1e4
	if sum.Searches > 0 {
		sum.AvgCostRub = costsRatio(sum.SearchLLMCostRub, float64(sum.Searches), 6)
		sum.ShareWithoutLLM = costsRatio(float64(sum.Searches-sum.LLMParses), float64(sum.Searches), 4)
	}
	if d := sum.LLMParses + sum.CacheHits; d > 0 {
		sum.CacheHitShare = costsRatio(float64(sum.CacheHits), float64(d), 4)
	}
	sum.LLMCalls = totals.Calls
	sum.LLMCostRub = math.Round(totals.CostRub*1e4) / 1e4
	sum.InputTokens, sum.OutputTokens = totals.InputTokens, totals.OutputTokens
	if totals.Calls > 0 {
		sum.AvgLLMLatencyMs = costsRatio(totals.AvgLatencyMs, 1, 1)
	}
	sum.EvalCalls, sum.EvalCostRub = totals.EvalCalls, math.Round(totals.EvalCostRub*1e4)/1e4
	sum.AgentRuns = totals.AgentRuns
	if totals.AgentRuns > 0 {
		sum.AgentRunAvgCostRub = costsRatio(totals.AgentCostRub, float64(totals.AgentRuns), 4)
	}

	for _, c := range calls {
		res.Calls.Items = append(res.Calls.Items, out.CostsCall{
			ID: c.ID.String(), Kind: llmCallKind(c), CreatedAt: c.CreatedAt, RequestID: c.RequestID, AgentRunID: c.AgentRunID,
			Provider: c.Provider, Model: c.Model, InputTokens: c.InputTokens, OutputTokens: c.OutputTokens,
			CachedTokens: c.CachedTokens, LatencyMs: c.LatencyMs, CostRub: c.CalculatedCost,
			PricingVersion: c.PricingVersion, Status: c.Status,
		})
	}
	return res, nil
}

// costsRatio — a/b, округлённое до digits знаков.
func costsRatio(a, b float64, digits int) *float64 {
	p := math.Pow(10, float64(digits))
	v := math.Round(a/b*p) / p
	return &v
}

// llmCallKind — к чему относится вызов модели: поиск, прогон оценки, агент или ручной прогон.
func llmCallKind(c domain.LLMUsage) string {
	switch {
	case c.AgentRunID != nil:
		return "agent"
	case c.RequestID == nil:
		return "other"
	case strings.HasPrefix(*c.RequestID, repositories.EvalRequestPrefix):
		return "eval"
	case strings.HasPrefix(*c.RequestID, "live-"):
		return "other" // живой прогон разбора из intent_live_test
	}
	return "search"
}
