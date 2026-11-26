package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"sort"
	"strings"
	"sync"
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

// holdoutSpanLookback — насколько раньше окна запуска искать последний исторический поиск для границ отложенной недели.
const holdoutSpanLookback = 60 * 24 * time.Hour

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
		if !domain.IsLiveSource(searches[i].Source) && at.After(last) {
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
	// Контур фида: вход агента — только поиски текущего каталога (agentLogScope). CatalogVersion — версия каталога;
	// NoHistory — у источника нет исторического журнала (фид заказчика): отложенной недели нет, агент видит тестовый
	// поток и живые поиски этой версии каталога.
	CatalogVersion string `json:"catalog_version,omitempty"`
	NoHistory      bool   `json:"no_history,omitempty"`
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
	return s.agentQueryStatsSince(ctx, s.now().Add(-time.Duration(days)*24*time.Hour), h)
}

// agentQueryStatsSince — вход агента с момента since. Окно, которое начинается в отложенной неделе или после неё
// (ночной разбор за сутки, всплеск за час на живом журнале стенда), видит поиски после недели: они не входят в реплей
// holdout_week. Окно, начатое раньше недели (разбор за 30 дней), — как раньше: до начала недели.
func (s *Services) agentQueryStatsSince(ctx context.Context, since time.Time, h HoldoutOptions) (*queryStats, holdoutInfo, error) {
	st := s.store()
	scope := s.agentLogScope(ctx)
	if !scope.Historical && !scope.All {
		// Исторического журнала у источника нет: откладывать нечего, а отложенная неделя по живым поискам спрятала бы
		// весь тестовый поток.
		h.Enabled = false
	}
	info := holdoutInfo{Enabled: h.Enabled, SeenFrom: since.UTC(), SeenTo: s.now().UTC(), ExcludeTest: h.ExcludeTest,
		CatalogVersion: scope.Catalog, NoHistory: !scope.Historical && !scope.All}
	if sq, ok := st.(agentStatsSQL); ok {
		stats, err := s.agentQueryStatsSQL(ctx, sq, since, h, scope, &info)
		if err != nil {
			return nil, info, err
		}
		return stats, info, nil
	}
	// Границы отложенной недели — по журналу шире окна: у короткого окна (всплеск за час) своих исторических
	// поисков нет, и неделя иначе считалась бы от живых.
	all, err := st.SearchEventsSince(ctx, since.Add(-holdoutSpanLookback))
	if err != nil {
		return nil, info, err
	}
	behavior, err := st.BehaviorEventsSince(ctx, since)
	if err != nil {
		return nil, info, err
	}
	all = filterScope(scope, all)
	searches := filterSince(all, since)
	behavior = filterBehaviorSince(behavior, since)
	if h.Enabled {
		from, to := s.holdoutRange(h, all)
		f, t := from.In(mskZone).Format("2006-01-02"), to.Add(-time.Second).In(mskZone).Format("2006-01-02")
		info.HoldoutFrom, info.HoldoutTo, info.SeenTo = &f, &t, from.UTC()
		if !since.Before(from) {
			// Окно начинается в отложенной неделе или после неё: видно только то, что после недели.
			since = maxTime(since, to)
			from = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
			info.SeenFrom, info.SeenTo = since.UTC(), s.now().UTC()
			searches = filterSince(searches, since)
			behavior = filterBehaviorSince(behavior, since)
		}
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
	SearchSpanSince(ctx context.Context, since time.Time, sc repositories.LogScope) (repositories.SearchSpan, error)
	CountSearchesSince(ctx context.Context, since time.Time, sc repositories.LogScope) (int, error)
	AgentQueryStatsScoped(ctx context.Context, since, until time.Time, sc repositories.LogScope) (*repositories.AgentQueryStats, error)
}

// agentQueryStatsSQL — то же, что ветка в памяти agentQueryStats, но агрегаты считает база: граница отложенного
// окна — по последнему поиску периода, исходы — за [since, начало окна).
func (s *Services) agentQueryStatsSQL(ctx context.Context, sq agentStatsSQL, since time.Time, h HoldoutOptions,
	scope repositories.LogScope, info *holdoutInfo) (*queryStats, error) {
	until := time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	if h.Enabled {
		span, err := sq.SearchSpanSince(ctx, since.Add(-holdoutSpanLookback), scope)
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
		if !since.Before(from) {
			// Окно запуска начинается в отложенной неделе или после неё (ночной разбор за сутки, всплеск за час на
			// живом журнале стенда): видно только то, что после недели, — эти поиски не входят в реплей holdout_week.
			since = maxTime(since, to)
			info.SeenFrom, info.SeenTo = since.UTC(), s.now().UTC()
			agg, err := sq.AgentQueryStatsScoped(ctx, since, until, scope)
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
		// Окно может начинаться раньше периода (короткий days): скрыты поиски периода с начала окна.
		hidden, err := sq.CountSearchesSince(ctx, maxTime(from, since), scope)
		if err != nil {
			return nil, err
		}
		info.HiddenEvents = hidden
		until = from
	}
	agg, err := sq.AgentQueryStatsScoped(ctx, since, until, scope)
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
		if e.CreatedAt.Before(from) || !e.CreatedAt.Before(to) || domain.IsLiveSource(e.Source) {
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

// agentLogScope — поиски журнала, которые относятся к текущему каталогу (контур фида): живые и трафик — только версии
// каталога, на которой сделаны; исторический журнал (mock_log) — только если он есть у текущего источника (mock).
// У фида заказчика журнала нет: старый mock-журнал в базе остаётся, но во вход агента и отчёт не попадает. Без
// загруженного каталога — весь журнал.
func (s *Services) agentLogScope(ctx context.Context) repositories.LogScope {
	cs := s.catalog.Load()
	if cs == nil || cs.version == "" {
		return repositories.Unscoped
	}
	sc := repositories.LogScope{Catalog: cs.version}
	if s.repos.Enabled() {
		// Начало текущего каталога — граница событий поведения (у них нет версии каталога).
		sc.Since, _ = s.catalogSince(ctx, cs.version)
	}
	if v, ok := logScopeHist.Load(cs.version); ok {
		sc.Historical = v.(bool)
		return sc
	}
	// Журнала нет только у источника, который сам об этом говорит (ErrNoFile: фид заказчика); источник без чтения
	// журнала (фикстуры) и с журналом (mock) — исторические поиски его.
	sc.Historical = true
	if l, ok := s.source.(logLoader); ok {
		if _, err := l.LoadEvents(ctx); isNoFile(err) {
			sc.Historical = false
		}
	}
	logScopeHist.Store(cs.version, sc.Historical)
	return sc
}

// logScopeHist — есть ли у источника версии каталога исторический журнал (читается один раз на версию).
var logScopeHist sync.Map

// scopeKeep — поиск входит в отбор (ветка в памяти, тот же смысл, что repositories.scopeSQL).
func scopeKeep(sc repositories.LogScope, e *domain.SearchEvent) bool {
	if sc.All {
		return true
	}
	if domain.IsLiveSource(e.Source) {
		return sc.Catalog == "" || e.CatalogVersion == "" || e.CatalogVersion == sc.Catalog
	}
	return sc.Historical
}

// filterScope — поиски, входящие в отбор.
func filterScope(sc repositories.LogScope, evs []domain.SearchEvent) []domain.SearchEvent {
	if sc.All {
		return evs
	}
	res := evs[:0:0]
	for i := range evs {
		if scopeKeep(sc, &evs[i]) {
			res = append(res, evs[i])
		}
	}
	return res
}
