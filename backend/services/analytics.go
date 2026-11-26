package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/externalApi/catalogsource"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
	"kuznica/api/repositories"

	"go.uber.org/zap"
)

// Аналитика поиска: плитки, воронка и проблемные запросы по журналам search_events и
// behavior_events — живым и импортированным из исторических логов (source = mock_log).
//
// Правила подсчёта:
//   - поиски прогона оценки (request_id eval-…) не считаются;
//   - буквы, набранные до Enter, — не поиск: запрос, который в той же сессии в течение typingWindow
//     продолжен более длинным запросом с тем же началом, отбрасывается;
//   - действие покупателя относится к поиску по request_id, а без него — к последнему поиску той же
//     сессии не раньше чем за attributionWindow;
//   - переформулировка — следующий поиск той же сессии с другим текстом в течение reformulateWindow,
//     а по этому поиску клика не было.

const (
	analyticsDefaultDays = 30
	analyticsMaxDays     = 365
	typingWindow         = 10 * time.Second
	reformulateWindow    = 60 * time.Second
	attributionWindow    = 30 * time.Minute
	problemsLimit        = 50
	diagnosisUnparsed    = "не разобрано"
)

// Роли аналитики (переключатель в шапке).
var analyticsRoles = []string{"ecom", "product", "content", "head"}

// Исходы проблемного запроса.
const (
	outcomeEmpty        = "empty"
	outcomeReformulated = "reformulated"
	outcomeNoClick      = "no_click"
)

var outcomeLabels = map[string]string{
	outcomeEmpty:        "пустая выдача",
	outcomeReformulated: "переформулировка",
	outcomeNoClick:      "нет клика",
}

// Analytics — GET /api/analytics. Без базы или без данных — нули и пустые списки.
func (s *Services) Analytics(ctx context.Context, days int, role string) (*out.AnalyticsResponse, error) {
	if days == 0 {
		days = analyticsDefaultDays
	}
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		role = analyticsRoles[0]
	}
	var bad []errs.FieldError
	if days < 1 || days > analyticsMaxDays {
		bad = append(bad, errs.FieldError{Field: "days", Code: errs.CodeOutOfRange, Message: fmt.Sprintf("Период — от 1 до %d дней", analyticsMaxDays)})
	}
	if !slices.Contains(analyticsRoles, role) {
		bad = append(bad, errs.FieldError{Field: "role", Code: errs.CodeInvalidFormat, Message: "Роль — ecom, product, content или head"})
	}
	if len(bad) > 0 {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(bad...)
	}
	now := s.now().UTC()
	since := now.Add(-time.Duration(days) * 24 * time.Hour)
	// Исходы поисков считает база (repositories/analytics_sql.go): события периода в память не читаются.
	sum, problems := &repositories.SearchSummary{}, []repositories.QueryOutcome(nil)
	if s.repos.Enabled() {
		var err error
		if sum, problems, err = s.repos.SearchOutcomesSince(ctx, since, problemsLimit); err != nil {
			return nil, err
		}
	}
	resp := analyticsFrom(sum, problems)
	resp.Role, resp.Days, resp.From, resp.To = role, days, since, now
	if st := s.store(); st != nil {
		ps, err := st.ListProposals(ctx, "", "")
		if err != nil {
			return nil, err
		}
		linkProposals(resp, ps)
	}
	if err := s.agentVerdicts(ctx, resp); err != nil {
		return nil, err
	}
	if role == "head" {
		eff, err := s.effect(ctx, EffectRequest{Days: days}, sum)
		if err != nil {
			return nil, err
		}
		resp.Effect = eff
	}
	resp.Commerce = s.analyticsCommerce(ctx, role, days, since, sum.Total.Searches)
	return resp, nil
}

// searchOutcome — что случилось после одного поиска.
type searchOutcome struct {
	ev                                     *domain.SearchEvent
	clicked, carted, ordered, reformulated bool
}

// computeAnalytics — плитки, воронка и проблемные запросы по событиям в памяти (эталон SQL-агрегации и путь
// тестов). searches и behavior — по времени.
func computeAnalytics(searches []domain.SearchEvent, behavior []domain.BehaviorEvent) *out.AnalyticsResponse {
	sum, problems := aggregateOutcomes(searches, behavior, problemsLimit)
	return analyticsFrom(sum, problems)
}

// aggregateOutcomes — то же, что считают repositories.SearchSummarySince и ProblemQueriesSince, но по событиям
// в памяти: по ним тест сверяет SQL с прежним расчётом.
func aggregateOutcomes(searches []domain.SearchEvent, behavior []domain.BehaviorEvent, limit int) (*repositories.SearchSummary, []repositories.QueryOutcome) {
	all := buildOutcomes(searches, behavior)
	bySource := map[string]*repositories.SearchTotals{}
	total := &repositories.SearchTotals{}
	type sessSets struct{ problem, click, order map[string]bool }
	sets := map[*repositories.SearchTotals]*sessSets{}
	problems := map[string]*repositories.QueryOutcome{}
	for _, o := range all {
		src := sourceOf(o.ev.Source)
		st := bySource[src]
		if st == nil {
			st = &repositories.SearchTotals{Source: src}
			bySource[src] = st
		}
		sess := sessionKey(o.ev.SessionID, o.ev.RequestID)
		empty := o.ev.ResultsCount == 0
		for _, t := range []*repositories.SearchTotals{st, total} {
			ss := sets[t]
			if ss == nil {
				ss = &sessSets{map[string]bool{}, map[string]bool{}, map[string]bool{}}
				sets[t] = ss
			}
			t.Searches++
			if empty {
				t.Empty++
			} else {
				t.NonEmpty++
				if !o.clicked {
					t.NoClick++
				}
			}
			if empty || !o.clicked {
				t.Problems++
				ss.problem[sess] = true
			}
			if o.clicked {
				t.Clicks++
				ss.click[sess] = true
			}
			if o.carted {
				t.Carts++
			}
			if o.ordered {
				t.Orders++
				ss.order[sess] = true
			}
		}
		key := foldQuery(o.ev.Query)
		if key == "" {
			continue
		}
		p := problems[key]
		if p == nil {
			p = &repositories.QueryOutcome{Query: key}
			problems[key] = p
		}
		p.Freq++
		switch {
		case empty:
			p.Empty++
		case o.reformulated:
			p.Reformulated++
		case !o.clicked:
			p.NoClick++
		}
		if o.clicked {
			p.Clicks++
		}
		if o.ordered {
			p.Orders++
		}
	}
	for t, ss := range sets {
		t.ProblemSessions, t.ClickSessions, t.OrderSessions = len(ss.problem), len(ss.click), len(ss.order)
	}
	sum := &repositories.SearchSummary{BySource: []repositories.SearchTotals{}, Total: *total}
	for _, st := range bySource {
		sum.BySource = append(sum.BySource, *st)
	}
	sort.Slice(sum.BySource, func(i, j int) bool { return sum.BySource[i].Source < sum.BySource[j].Source })
	var list []repositories.QueryOutcome
	for _, p := range problems {
		if p.Empty+p.Reformulated+p.NoClick > 0 {
			list = append(list, *p)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		a, b := list[i], list[j]
		fa, fb := a.Empty+a.Reformulated+a.NoClick, b.Empty+b.Reformulated+b.NoClick
		if fa != fb {
			return fa > fb
		}
		if a.Freq != b.Freq {
			return a.Freq > b.Freq
		}
		return a.Query < b.Query
	})
	if len(list) > limit {
		list = list[:limit]
	}
	return sum, list
}

// analyticsFrom — ответ аналитики из сумм исходов и проблемных запросов (они уже отсортированы и обрезаны).
func analyticsFrom(sum *repositories.SearchSummary, problems []repositories.QueryOutcome) *out.AnalyticsResponse {
	resp := &out.AnalyticsResponse{Funnel: []out.FunnelStep{}, Problems: []out.ProblemQuery{}, Sources: map[string]int{}}
	for _, st := range sum.BySource {
		resp.Sources[st.Source] = st.Searches
	}
	tot := sum.Total
	t := &resp.Tiles
	t.Searches, t.Empty, t.NoClick, t.Orders = tot.Searches, tot.Empty, tot.NoClick, tot.Orders
	t.EmptyRate = share(t.Empty, t.Searches)
	t.NoClickRate = share(t.NoClick, tot.NonEmpty)
	t.OrderRate = share(t.Orders, t.Searches)
	resp.Funnel = []out.FunnelStep{
		{Step: "searches", Label: "Поиски", Count: t.Searches, Rate: share(t.Searches, t.Searches)},
		{Step: "clicks", Label: "Клики", Count: tot.Clicks, Rate: share(tot.Clicks, t.Searches)},
		{Step: "carts", Label: "Корзина", Count: tot.Carts, Rate: share(tot.Carts, t.Searches)},
		{Step: "orders", Label: "Заказы", Count: t.Orders, Rate: share(t.Orders, t.Searches)},
	}
	for _, q := range problems {
		p := out.ProblemQuery{Query: q.Query, Diagnosis: diagnosisUnparsed, Freq: q.Freq, Empty: q.Empty,
			Reformulated: q.Reformulated, NoClick: q.NoClick, Clicks: q.Clicks, Orders: q.Orders}
		p.Failures = p.Empty + p.Reformulated + p.NoClick
		p.Outcome = outcomeNoClick
		if p.Reformulated >= p.NoClick && p.Reformulated > 0 {
			p.Outcome = outcomeReformulated
		}
		if p.Empty >= max(p.Reformulated, p.NoClick) && p.Empty > 0 {
			p.Outcome = outcomeEmpty
		}
		p.OutcomeLabel = outcomeLabels[p.Outcome]
		resp.Problems = append(resp.Problems, p)
	}
	return resp
}

// linkProposals — счётчик ждущих предложений и связь проблемного запроса с предложением агента по запросам
// кластера предложения (сравнение — foldQuery). Ждущее решения важнее решённого, новое — старого.
func linkProposals(resp *out.AnalyticsResponse, ps []domain.Proposal) {
	type link struct {
		p       *domain.Proposal
		pending bool
	}
	byQuery := map[string]link{}
	for i := range ps {
		p := &ps[i]
		pending := p.Status == domain.ProposalPending
		if pending {
			resp.Tiles.PendingProposals++
		}
		var qs []struct {
			Text string `json:"text"`
		}
		_ = json.Unmarshal(p.ClusterQueries, &qs)
		for _, q := range qs {
			k := foldQuery(q.Text)
			cur, ok := byQuery[k]
			if !ok || pending && !cur.pending || pending == cur.pending && p.CreatedAt.After(cur.p.CreatedAt) {
				byQuery[k] = link{p: p, pending: pending}
			}
		}
	}
	for i := range resp.Problems {
		l, ok := byQuery[resp.Problems[i].Query]
		if !ok {
			continue
		}
		id, status := l.p.ID.String(), l.p.Status
		diag := l.p.Diagnosis
		resp.Problems[i].ProposalID, resp.Problems[i].ProposalStatus, resp.Problems[i].ProposalDiagnosis = &id, &status, &diag
	}
}

// buildOutcomes — исход каждого поиска: клик, корзина, заказ, переформулировка; набор до Enter отброшен.
// searches сортируется по времени на месте.
func buildOutcomes(searches []domain.SearchEvent, behavior []domain.BehaviorEvent) []*searchOutcome {
	// Поиски по сессиям, без набора до Enter.
	bySession := map[string][]*searchOutcome{}
	byRequest := map[string]*searchOutcome{}
	var all []*searchOutcome
	sort.SliceStable(searches, func(i, j int) bool { return searches[i].CreatedAt.Before(searches[j].CreatedAt) })
	for i := range searches {
		o := &searchOutcome{ev: &searches[i]}
		all = append(all, o)
		bySession[sessionKey(o.ev.SessionID, o.ev.RequestID)] = append(bySession[sessionKey(o.ev.SessionID, o.ev.RequestID)], o)
	}
	typing := map[*searchOutcome]bool{}
	for _, list := range bySession {
		for i := 0; i+1 < len(list); i++ {
			cur, next := list[i], list[i+1]
			a, b := foldQuery(cur.ev.Query), foldQuery(next.ev.Query)
			if next.ev.CreatedAt.Sub(cur.ev.CreatedAt) <= typingWindow && len(b) > len(a) && strings.HasPrefix(b, a) {
				typing[cur] = true
			}
		}
	}
	kept := all[:0]
	for _, o := range all {
		if !typing[o] {
			kept = append(kept, o)
		}
	}
	all = kept
	for k := range bySession {
		bySession[k] = slices.DeleteFunc(bySession[k], func(o *searchOutcome) bool { return typing[o] })
	}
	for _, o := range all {
		byRequest[o.ev.RequestID] = o
	}

	// Действия покупателя → поиск.
	for i := range behavior {
		b := &behavior[i]
		var o *searchOutcome
		if b.RequestID != nil {
			o = byRequest[*b.RequestID]
		}
		if o == nil && b.SessionID != nil {
			list := bySession[*b.SessionID]
			for j := len(list) - 1; j >= 0; j-- {
				at := list[j].ev.CreatedAt
				if !at.After(b.CreatedAt) {
					if b.CreatedAt.Sub(at) <= attributionWindow {
						o = list[j]
					}
					break
				}
			}
		}
		if o == nil {
			continue
		}
		switch b.Type {
		case domain.EventClick:
			o.clicked = true
		case domain.EventAddToCart:
			o.clicked, o.carted = true, true
		case domain.EventOrder:
			o.clicked, o.carted, o.ordered = true, true, true
		}
	}

	// Переформулировки.
	for _, list := range bySession {
		for i := 0; i+1 < len(list); i++ {
			cur, next := list[i], list[i+1]
			if !cur.clicked && next.ev.CreatedAt.Sub(cur.ev.CreatedAt) <= reformulateWindow &&
				foldQuery(cur.ev.Query) != foldQuery(next.ev.Query) {
				cur.reformulated = true
			}
		}
	}

	return all
}

// sessionKey — сессия поиска; без сессии поиск — сам себе сессия.
func sessionKey(session *string, requestID string) string {
	if session != nil && *session != "" {
		return *session
	}
	return "req:" + requestID
}

func sourceOf(s string) string {
	if s == "" {
		return domain.EventSourceLive
	}
	return s
}

// share — доля; пустой знаменатель — nil.
func share(n, d int) *float64 {
	if d == 0 {
		return nil
	}
	return evalPtr(evalRound4(float64(n) / float64(d)))
}

// foldQuery — запрос для группировки: нижний регистр, ё → е, одиночные пробелы.
func foldQuery(q string) string {
	q = strings.ReplaceAll(strings.ToLower(q), "ё", "е")
	return strings.Join(strings.Fields(q), " ")
}

// logLoader — источник, который умеет отдать исторические логи (MockJSON).
type logLoader interface {
	LoadEvents(ctx context.Context) ([]domain.LogEvent, error)
}

// ImportSearchLogs — исторические логи (events.jsonl) в search_events и behavior_events с пометкой
// source = mock_log. Повторный импорт заменяет прежние записи mock_log; живые не трогает. Нет файла
// логов — nil без ошибки.
func (s *Services) ImportSearchLogs(ctx context.Context) (*out.LogImportResult, error) {
	if !s.repos.Enabled() {
		return nil, errs.NewUnavailableError("Сервис запущен без базы данных: логи некуда положить")
	}
	loader, ok := s.source.(logLoader)
	if !ok {
		return nil, nil
	}
	started := time.Now()
	evs, err := loader.LoadEvents(ctx)
	if err != nil {
		if isNoFile(err) {
			return nil, nil
		}
		return nil, errs.NewUnavailableError(MsgSourceBroken).WithCause(err)
	}
	searches, behavior, skipped := logRows(evs)
	if err := s.repos.ReplaceMockLog(ctx, searches, behavior); err != nil {
		return nil, err
	}
	res := &out.LogImportResult{Searches: len(searches), Behavior: len(behavior), Skipped: skipped, Ms: time.Since(started).Milliseconds()}
	if len(evs) > 0 {
		res.From, res.To = evs[0].TS.UTC(), evs[0].TS.UTC()
		for _, e := range evs {
			if e.TS.Before(res.From) {
				res.From = e.TS.UTC()
			}
			if e.TS.After(res.To) {
				res.To = e.TS.UTC()
			}
		}
	}
	zap.L().Info("логи поиска импортированы", zap.Int("searches", res.Searches), zap.Int("behavior", res.Behavior),
		zap.Int("skipped", res.Skipped), zap.Int64("ms", res.Ms))
	return res, nil
}

// logRows — строки журналов из событий лога. Действие получает request_id последнего поиска своей
// сессии (не раньше attributionWindow). Неизвестный тип события — пропуск.
func logRows(evs []domain.LogEvent) ([]domain.SearchEvent, []domain.BehaviorEvent, int) {
	sorted := slices.Clone(evs)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].TS.Before(sorted[j].TS) })
	type last struct {
		id string
		at time.Time
	}
	lastSearch := map[string]last{}
	var searches []domain.SearchEvent
	var behavior []domain.BehaviorEvent
	skipped := 0
	for i, e := range sorted {
		session := e.SessionID
		switch {
		case e.Type == "search":
			id := fmt.Sprintf("log-%07d", i+1)
			n := 0
			if e.ResultsCount != nil {
				n = *e.ResultsCount
			}
			ev := domain.SearchEvent{
				RequestID: id, Query: e.Query, Normalized: foldQuery(e.Query), ResultsCount: n,
				ClientID: e.ClientID, SessionID: &session, Source: domain.EventSourceMockLog,
			}
			ev.CreatedAt = e.TS.UTC()
			searches = append(searches, ev)
			lastSearch[session] = last{id: id, at: e.TS}
		case slices.Contains(domain.BehaviorEventTypes, e.Type):
			ev := domain.BehaviorEvent{
				Type: e.Type, SessionID: &session, ClientID: e.ClientID, Position: e.Position, Source: domain.EventSourceMockLog,
			}
			if e.ProductID != "" {
				pid := e.ProductID
				ev.ProductID = &pid
			}
			if l, ok := lastSearch[session]; ok && e.TS.Sub(l.at) <= attributionWindow {
				id := l.id
				ev.RequestID = &id
			}
			ev.CreatedAt = e.TS.UTC()
			behavior = append(behavior, ev)
		default:
			skipped++
		}
	}
	return searches, behavior, skipped
}

// isNoFile — в источнике нет файла логов или наборов.
func isNoFile(err error) bool { return errors.Is(err, catalogsource.ErrNoFile) }
