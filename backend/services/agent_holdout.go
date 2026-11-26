package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"sort"
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/pkg/errs"
	"kuznica/api/repositories"
)

// Честная оценка агента (ревью B1): агент учится на прошлом, эффект меряется на будущем.
//   - Временной разрез: агент видит журнал только до начала отложенной недели (AGENT_HOLDOUT_FROM;
//     по умолчанию — последние 7 дней журнала). Эффект правок меряется реплеем этой недели — набор
//     holdout_week: все поиски недели прогоняются через B3 и B4 (make eval SET=holdout_week CONFIGS=B3,B4).
//   - Тексты набора test исключаются из входа агента (неудачные запросы, кластеры, переформулировки,
//     похожие успешные) — AGENT_EXCLUDE_TEST=on по умолчанию. Тогда B4 на test — проверка обобщения.

// holdoutDefaultDays — длина отложенного окна.
const holdoutDefaultDays = 7

// HoldoutOptions — разрез журнала для агента. Нулевое значение — разреза нет (тесты на своих журналах).
type HoldoutOptions struct {
	// Enabled — агент не видит журнал с начала отложенного окна.
	Enabled bool
	// From — начало отложенного окна; нулевое — последние Days дней журнала (по историческим событиям).
	From time.Time
	// Days — длина окна, дней (0 — 7).
	Days int
	// ExcludeTest — тексты набора test не попадают во вход агента.
	ExcludeTest bool
}

// HoldoutFromEnv — разрез из окружения: AGENT_HOLDOUT_FROM = "" (последние 7 дней журнала) | off |
// ГГГГ-ММ-ДД (по Москве); AGENT_HOLDOUT_DAYS (7); AGENT_EXCLUDE_TEST = on (по умолчанию) | off.
// Неверная дата — разрез по умолчанию (не выключение): честность оценки не должна зависеть от опечатки.
func HoldoutFromEnv() HoldoutOptions {
	h := HoldoutOptions{Enabled: true, ExcludeTest: true}
	switch v := strings.ToLower(strings.TrimSpace(os.Getenv("AGENT_HOLDOUT_FROM"))); v {
	case "", "auto":
	case "off", "false", "0":
		h.Enabled = false
	default:
		if t, err := time.ParseInLocation("2006-01-02", v, mskZone); err == nil {
			h.From = t
		}
	}
	if d := strings.TrimSpace(os.Getenv("AGENT_HOLDOUT_DAYS")); d != "" {
		if n, ok := parseSmallInt(d); ok && n > 0 && n <= 60 {
			h.Days = n
		}
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AGENT_EXCLUDE_TEST"))) {
	case "off", "false", "0":
		h.ExcludeTest = false
	}
	return h
}

func parseSmallInt(s string) (int, bool) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' || n > 10000 {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	return n, s != ""
}

func (h HoldoutOptions) days() int {
	if h.Days > 0 {
		return h.Days
	}
	return holdoutDefaultDays
}

// holdoutRange — отложенное окно [from, to). Явная дата — от неё; иначе по последнему историческому
// событию журнала (не live: живые поиски сегодняшнего дня окно не сдвигают), без таких — по последнему
// любому, без событий — по текущему времени. Границы — полночь по Москве.
func (s *Services) holdoutRange(h HoldoutOptions, searches []domain.SearchEvent) (time.Time, time.Time) {
	var last, lastAny time.Time
	for i := range searches {
		at := searches[i].CreatedAt
		if at.After(lastAny) {
			lastAny = at
		}
		if searches[i].Source != domain.EventSourceLive && at.After(last) {
			last = at
		}
	}
	return s.holdoutRangeAt(h, last, lastAny)
}

// holdoutRangeAt — отложенное окно по последнему историческому и последнему любому поиску (нулевые — нет).
func (s *Services) holdoutRangeAt(h HoldoutOptions, last, lastAny time.Time) (time.Time, time.Time) {
	days := time.Duration(h.days()) * 24 * time.Hour
	if !h.From.IsZero() {
		return h.From, h.From.Add(days)
	}
	if last.IsZero() {
		last = lastAny
	}
	if last.IsZero() {
		last = s.now()
	}
	m := last.In(mskZone)
	to := time.Date(m.Year(), m.Month(), m.Day(), 0, 0, 0, 0, mskZone).Add(24 * time.Hour)
	return to.Add(-days), to
}

// holdoutInfo — что агент видел и что отложено (в эпизод запуска и шаг get_failed_queries).
type holdoutInfo struct {
	Enabled      bool      `json:"enabled"`
	SeenFrom     time.Time `json:"seen_from"`
	SeenTo       time.Time `json:"seen_to"`
	HoldoutFrom  *string   `json:"holdout_from"`
	HoldoutTo    *string   `json:"holdout_to"`
	HiddenEvents int       `json:"hidden_searches"`
	ExcludeTest  bool      `json:"exclude_test"`
	TestTexts    int       `json:"test_texts_excluded"`
}

// testTexts — нормализованные тексты набора test (пусто — наборов нет: источник без разметки или без
// queries.json). Разметка есть, но не читается — ошибка (ревью-2 n8): без неё тексты test не исключить, и агент
// не стартует, а не молча учится на test.
func (s *Services) testTexts(ctx context.Context) (map[string]bool, error) {
	res := map[string]bool{}
	loader, ok := s.source.(queryLoader)
	if !ok {
		return res, nil
	}
	all, err := loader.LoadQueries(ctx)
	if isNoFile(err) {
		return res, nil
	}
	if err != nil {
		return nil, errs.NewUnavailableError(MsgAgentNoTestSet).WithCause(err)
	}
	for _, q := range all {
		if q.Set == domain.EvalSetTest {
			res[foldQuery(q.Text)] = true
		}
	}
	return res, nil
}

// MsgAgentNoTestSet — разметка не читается: исключить тексты test из входа агента нельзя.
const MsgAgentNoTestSet = "Набор запросов разметки не читается — агент не запускается: без него тексты test не исключить из входа (AGENT_EXCLUDE_TEST=on)"

// agentQueryStats — исходы поисков, которые агенту разрешено видеть: за days дней до отложенного окна,
// без текстов test.
func (s *Services) agentQueryStats(ctx context.Context, days int, h HoldoutOptions) (*queryStats, holdoutInfo, error) {
	st := s.store()
	since := s.now().Add(-time.Duration(days) * 24 * time.Hour)
	info := holdoutInfo{Enabled: h.Enabled, SeenFrom: since.UTC(), SeenTo: s.now().UTC(), ExcludeTest: h.ExcludeTest}
	if sq, ok := st.(agentStatsSQL); ok {
		stats, err := s.agentQueryStatsSQL(ctx, sq, since, h, &info)
		if err != nil {
			return nil, info, err
		}
		return stats, info, nil
	}
	searches, err := st.SearchEventsSince(ctx, since)
	if err != nil {
		return nil, info, err
	}
	behavior, err := st.BehaviorEventsSince(ctx, since)
	if err != nil {
		return nil, info, err
	}
	searches = filterSince(searches, since)
	behavior = filterBehaviorSince(behavior, since)
	if h.Enabled {
		from, to := s.holdoutRange(h, searches)
		f, t := from.In(mskZone).Format("2006-01-02"), to.Add(-time.Second).In(mskZone).Format("2006-01-02")
		info.HoldoutFrom, info.HoldoutTo, info.SeenTo = &f, &t, from.UTC()
		kept := searches[:0:0]
		for _, e := range searches {
			if e.CreatedAt.Before(from) {
				kept = append(kept, e)
			} else {
				info.HiddenEvents++
			}
		}
		searches = kept
		var kb []domain.BehaviorEvent
		for _, b := range behavior {
			if b.CreatedAt.Before(from) {
				kb = append(kb, b)
			}
		}
		behavior = kb
	}
	stats := computeQueryStats(searches, behavior)
	if h.ExcludeTest {
		tt, err := s.testTexts(ctx)
		if err != nil {
			return nil, info, err
		}
		info.TestTexts = stats.exclude(tt)
	}
	return stats, info, nil
}

// agentStatsSQL — хранилище, которое считает вход агента в базе (repositories.Repos); хранилище в памяти тестов
// его не реализует — тогда события читаются и считаются в Go (computeQueryStats, эталон).
type agentStatsSQL interface {
	SearchSpanSince(ctx context.Context, since time.Time) (repositories.SearchSpan, error)
	CountSearchesSince(ctx context.Context, since time.Time) (int, error)
	AgentQueryStatsBetween(ctx context.Context, since, until time.Time) (*repositories.AgentQueryStats, error)
}

// agentQueryStatsSQL — то же, что ветка в памяти agentQueryStats, но агрегаты считает база: граница отложенного
// окна — по последнему поиску периода, исходы — за [since, начало окна).
func (s *Services) agentQueryStatsSQL(ctx context.Context, sq agentStatsSQL, since time.Time, h HoldoutOptions,
	info *holdoutInfo) (*queryStats, error) {
	until := time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	if h.Enabled {
		span, err := sq.SearchSpanSince(ctx, since)
		if err != nil {
			return nil, err
		}
		var lastHist, lastAny time.Time
		if span.LastHistorical != nil {
			lastHist = *span.LastHistorical
		}
		if span.LastAny != nil {
			lastAny = *span.LastAny
		}
		from, to := s.holdoutRangeAt(h, lastHist, lastAny)
		f, t := from.In(mskZone).Format("2006-01-02"), to.Add(-time.Second).In(mskZone).Format("2006-01-02")
		info.HoldoutFrom, info.HoldoutTo, info.SeenTo = &f, &t, from.UTC()
		// Окно может начинаться раньше периода (короткий days): скрыты поиски периода с начала окна.
		hidden, err := sq.CountSearchesSince(ctx, maxTime(from, since))
		if err != nil {
			return nil, err
		}
		info.HiddenEvents = hidden
		until = from
	}
	agg, err := sq.AgentQueryStatsBetween(ctx, since, until)
	if err != nil {
		return nil, err
	}
	stats := queryStatsFromSQL(agg)
	if h.ExcludeTest {
		tt, err := s.testTexts(ctx)
		if err != nil {
			return nil, err
		}
		info.TestTexts = stats.exclude(tt)
	}
	return stats, nil
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func filterSince(evs []domain.SearchEvent, since time.Time) []domain.SearchEvent {
	res := evs[:0:0]
	for _, e := range evs {
		if !e.CreatedAt.Before(since) {
			res = append(res, e)
		}
	}
	return res
}

func filterBehaviorSince(evs []domain.BehaviorEvent, since time.Time) []domain.BehaviorEvent {
	var res []domain.BehaviorEvent
	for _, e := range evs {
		if !e.CreatedAt.Before(since) {
			res = append(res, e)
		}
	}
	return res
}

// exclude — убрать тексты из статистики: сами запросы, переформулировки в них и клики по ним.
// Возвращает, сколько различных запросов убрано.
func (st *queryStats) exclude(texts map[string]bool) int {
	if len(texts) == 0 {
		return 0
	}
	n := 0
	for k := range st.byText {
		if texts[k] {
			delete(st.byText, k)
			delete(st.clickedProduct, k)
			n++
		}
	}
	for _, q := range st.byText {
		kept := q.ReformulatedTo[:0]
		for _, r := range q.ReformulatedTo {
			if !texts[r.Text] {
				kept = append(kept, r)
			}
		}
		q.ReformulatedTo = kept
	}
	return n
}

// Реплей отложенной недели (набор holdout_week).

// holdoutQueries — поиски отложенного окна как набор прогона: различные (текст, клиент), частота — число
// поисков за окно; разметка — из queries.json по тексту и клиенту (иначе по тексту), без неё запрос
// участвует только в доле пустых.
func (s *Services) holdoutQueries(ctx context.Context) ([]domain.EvalQuery, error) {
	if !s.repos.Enabled() {
		return nil, errNoHoldoutLog
	}
	h := s.agent.opts.Holdout
	since := s.now().Add(-120 * 24 * time.Hour)
	if !h.From.IsZero() && h.From.Before(since) {
		since = h.From
	}
	searches, err := s.repos.SearchEventsSince(ctx, since)
	if err != nil {
		return nil, err
	}
	from, to := s.holdoutRange(h, searches)
	var judged []domain.EvalQuery
	if loader, ok := s.source.(queryLoader); ok {
		if judged, err = loader.LoadQueries(ctx); err != nil {
			return nil, err
		}
	}
	res := buildHoldoutQueries(searches, judged, from, to)
	if len(res) == 0 {
		return nil, errNoHoldoutLog
	}
	return res, nil
}

// buildHoldoutQueries — исторические поиски окна [from, to) как запросы прогона.
func buildHoldoutQueries(searches []domain.SearchEvent, judged []domain.EvalQuery, from, to time.Time) []domain.EvalQuery {
	byTextClient, byText := map[string]domain.EvalQuery{}, map[string]domain.EvalQuery{}
	for _, q := range judged {
		k := foldQuery(q.Text)
		byTextClient[k+"\x00"+derefStr(q.ClientID)] = q
		if _, ok := byText[k]; !ok || q.ClientID == nil {
			byText[k] = q
		}
	}
	type key struct{ text, client string }
	freq := map[key]int{}
	orig := map[key]string{}
	for _, e := range searches {
		if e.CreatedAt.Before(from) || !e.CreatedAt.Before(to) || e.Source == domain.EventSourceLive {
			continue
		}
		k := key{foldQuery(e.Query), derefStr(e.ClientID)}
		if k.text == "" {
			continue
		}
		freq[k]++
		if _, ok := orig[k]; !ok {
			orig[k] = e.Query
		}
	}
	res := make([]domain.EvalQuery, 0, len(freq))
	for k, n := range freq {
		q := domain.EvalQuery{Text: orig[k], Type: "unlabeled", Set: domain.EvalSetHoldoutWeek, Freq: n, Judgments: map[string]int{}}
		if k.client != "" {
			c := k.client
			q.ClientID = &c
		}
		m, ok := byTextClient[k.text+"\x00"+k.client]
		if !ok {
			m, ok = byText[k.text]
		}
		if ok {
			q.Type, q.Judgments, q.Absent, q.Cluster = m.Type, m.Judgments, m.Absent, m.Cluster
			q.ID = "h-" + m.ID
			if k.client != "" && derefStr(m.ClientID) != k.client {
				q.ID += "-" + k.client
			}
		} else {
			sum := sha256.Sum256([]byte(k.text + "\x00" + k.client))
			q.ID = "h-x" + hex.EncodeToString(sum[:4])
		}
		res = append(res, q)
	}
	sort.Slice(res, func(i, j int) bool { return res[i].ID < res[j].ID })
	return res
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
