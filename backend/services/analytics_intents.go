package services

import (
	"context"
	"encoding/json"
	"slices"
	"sort"
	"time"

	"kuznica/api/domain"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
	"kuznica/api/repositories"
)

// Учёт поисковых намерений контура выдачи (§11.6, RESEARCH контура выдачи «Ж. Метрики»). Единица учёта —
// намерение: ввод от фокуса в строке до исхода. Клик по товару в панели — исход намерения, а не пропавший поиск.
// Агрегаты считает база (repositories.IntentsSince); aggregateIntents — эталон в Go для тестов.

const maxIntentIDLen = 64

var intentOutcomeLabels = map[string]string{
	domain.IntentCommit: "отправлен запрос", domain.IntentSuggestion: "выбрана подсказка",
	domain.IntentProduct: "товар из панели", domain.IntentCategory: "категория", domain.IntentVendor: "производитель",
	domain.IntentAbandon: "отказ",
}

// actionEvents — действия с товаром: успех намерения.
var actionEvents = []string{domain.EventClick, domain.EventAddToCart, domain.EventOrder, domain.EventPriceRequest}

// checkIntentID — intent_id: у событий контура выдачи обязателен; не длиннее 64 символов из [A-Za-z0-9_-].
func checkIntentID(typ, id string) []errs.FieldError {
	if id == "" {
		if slices.Contains(domain.IntentEventTypes, typ) {
			return []errs.FieldError{{Field: "intent_id", Code: errs.CodeRequired, Message: "Обязательное поле события " + typ}}
		}
		return nil
	}
	if len(id) > maxIntentIDLen {
		return []errs.FieldError{{Field: "intent_id", Code: errs.CodeOutOfRange, Message: "Слишком длинный идентификатор"}}
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return []errs.FieldError{{Field: "intent_id", Code: errs.CodeInvalidFormat, Message: "intent_id — латиница, цифры, - и _"}}
		}
	}
	return nil
}

// intentForLog — intent_id поиска для журнала: неверный (длинный, чужие символы) не пишется, поиск не отклоняется.
func intentForLog(id string) *string {
	if id == "" || checkIntentID("", id) != nil {
		return nil
	}
	return &id
}

// intentAnalytics — блок намерений за период; без базы — nil.
func (s *Services) intentAnalytics(ctx context.Context, since time.Time) (*out.IntentAnalytics, error) {
	if !s.repos.Enabled() {
		return nil, nil
	}
	sum, err := s.repos.IntentsSince(ctx, since)
	if err != nil {
		return nil, err
	}
	return intentsFrom(sum), nil
}

// intentsFrom — доли и исходы из агрегатов.
func intentsFrom(sum *repositories.IntentSummary) *out.IntentAnalytics {
	res := &out.IntentAnalytics{Outcomes: []out.IntentOutcome{}, Sources: map[string]int{}}
	by := map[string]*out.IntentOutcome{}
	for _, o := range domain.IntentOutcomes {
		by[o] = &out.IntentOutcome{Outcome: o, Label: intentOutcomeLabels[o]}
	}
	var legacy, unified repositories.IntentRateRow
	for _, g := range sum.Groups {
		res.Sources[g.Source] += g.N
		outcome := domain.IntentOutcomeEvents[g.OutcomeEvent]
		if outcome == "" {
			res.Open += g.N
			continue
		}
		res.Intents += g.N
		o := by[outcome]
		o.Count += g.N
		converted := g.Acted || g.PanelProduct
		if converted {
			o.Converted += g.N
		}
		unified.Den += g.N
		if converted && outcome != domain.IntentAbandon {
			unified.Num += g.N
		}
		if outcome == domain.IntentCommit || outcome == domain.IntentSuggestion {
			legacy.Den += g.N
			if g.Acted {
				legacy.Num += g.N
			}
		}
	}
	for _, o := range domain.IntentOutcomes {
		v := by[o]
		v.Share = ratio(v.Count, res.Intents)
		res.Outcomes = append(res.Outcomes, *v)
	}
	res.ConversionLegacy, res.ConversionUnified = rate(legacy), rate(unified)
	res.EmptyResults, res.EmptySuggestions = rate(sum.EmptyResults), rate(sum.EmptySuggestions)
	return res
}

func rate(r repositories.IntentRateRow) out.IntentRate {
	return out.IntentRate{Num: r.Num, Den: r.Den, Value: ratio(r.Num, r.Den)}
}

// commitOutcomeEvent — исход search_commit по payload.outcome как событие-исход.
func commitOutcomeEvent(v any) string {
	o, _ := v.(string)
	for ev, out := range domain.IntentOutcomeEvents {
		if out == o && out != domain.IntentAbandon {
			return ev
		}
	}
	return domain.EventSearchCommit
}

// lookupResults — число результатов отправленного запроса: поиск по request_id, иначе первый поиск намерения.
func lookupResults(e domain.BehaviorEvent, results, byIntent map[string]int) (int, bool) {
	if e.RequestID != nil {
		if n, ok := results[*e.RequestID]; ok {
			return n, true
		}
	}
	if e.IntentID != nil {
		n, ok := byIntent[*e.IntentID]
		return n, ok
	}
	return 0, false
}

// aggregateIntents — то же, что repositories.IntentsSince, по событиям в памяти (эталон для тестов).
// results — число результатов поиска по request_id.
// byIntent — число результатов первого поиска намерения.
func aggregateIntents(behavior []domain.BehaviorEvent, results map[string]int, byIntent map[string]int) *repositories.IntentSummary {
	evs := slices.Clone(behavior)
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].CreatedAt.Before(evs[j].CreatedAt) })
	type intent struct {
		outcome, source     string
		acted, panelProduct bool
	}
	per := map[string]*intent{}
	sum := &repositories.IntentSummary{}
	for _, e := range evs {
		payload := map[string]any{}
		if len(e.Payload) > 0 {
			_ = json.Unmarshal(e.Payload, &payload)
		}
		switch e.Type {
		case domain.EventSearchCommit:
			if n, ok := payload["results_count"].(float64); ok {
				sum.EmptyResults.Den++
				if int(n) == 0 {
					sum.EmptyResults.Num++
				}
			} else if n, ok := lookupResults(e, results, byIntent); ok {
				sum.EmptyResults.Den++
				if n == 0 {
					sum.EmptyResults.Num++
				}
			}
		case domain.EventSuggestImpression:
			items, ok := payload["items"].(float64)
			plen, hasLen := payload["prefix_len"].(float64)
			if ok && (!hasLen || plen >= 2) {
				sum.EmptySuggestions.Den++
				if items == 0 {
					sum.EmptySuggestions.Num++
				}
			}
		}
		if e.IntentID == nil || *e.IntentID == "" {
			continue
		}
		src := sourceOf(e.Source)
		it := per[*e.IntentID]
		if it == nil {
			it = &intent{source: src}
			per[*e.IntentID] = it
		}
		if src < it.source {
			it.source = src
		}
		if _, ok := domain.IntentOutcomeEvents[e.Type]; ok && it.outcome == "" {
			it.outcome = e.Type
			if e.Type == domain.EventSearchCommit {
				it.outcome = commitOutcomeEvent(payload["outcome"])
			}
		}
		if slices.Contains(actionEvents, e.Type) {
			it.acted = true
		}
		if e.Type == domain.EventInstantProductClick {
			it.panelProduct = true
		}
	}
	groups := map[repositories.IntentGroup]int{}
	for _, it := range per {
		groups[repositories.IntentGroup{OutcomeEvent: it.outcome, Source: it.source, Acted: it.acted, PanelProduct: it.panelProduct}]++
	}
	for g, n := range groups {
		g.N = n
		sum.Groups = append(sum.Groups, g)
	}
	sort.Slice(sum.Groups, func(i, j int) bool {
		a, b := sum.Groups[i], sum.Groups[j]
		if a.OutcomeEvent != b.OutcomeEvent {
			return a.OutcomeEvent < b.OutcomeEvent
		}
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if a.Acted != b.Acted {
			return !a.Acted
		}
		return !a.PanelProduct && b.PanelProduct
	})
	return sum
}
