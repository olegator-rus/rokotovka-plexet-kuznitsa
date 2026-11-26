package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
)

// Отчёт SearchOps для маркетолога (контур фида, кадр 1) — первый экран SearchOps. Вход тот же,
// что у агента (agentQueryStatsSince): неудачные запросы журнала текущего каталога — на фиде это тестовый поток
// (запросы заказчика, прогоны сайта, синтетика из каталога) и живые поиски; старый mock-журнал не попадает
// (agentLogScope). Каждый неудачный запрос попадает в раздел по доказательству, а не по догадке:
//   - вывод агента (диагноз кластера или предложение) — главнее всего;
//   - иначе действующий поиск (без записи в журнал): сервисный вопрос — «Сервисные запросы»; первая точная карточка
//     «по запросу» — «Спрос без предложения»;
//   - иначе журнал: покупатели переформулировали запрос и выбрали товар — «Товар есть, но не найден»;
//   - иначе причина не доказана — «Ещё не разобраны» (агент разберёт по расписанию или по кнопке).
// «Слова клиента, которых нет в каталоге» — непонятные поиску слова всех неудачных запросов (словарь и названия
// каталога, как у кластеров агента). Потерянной выручки в рублях в отчёте нет: частоты потока модельные.

// Разделы отчёта.
const (
	ReportNoOffer      = "no_offer"
	ReportNotFound     = "not_found"
	ReportUnknownWords = "unknown_words"
	ReportService      = "service"
)

// Подпричины строки (ReportRow.Kind).
const (
	reportKindOnRequest  = "on_request"
	reportKindAbsent     = "not_in_catalog"
	reportKindSubstitute = "substitute"
	reportKindNotUnder   = "not_understood"
	reportKindRanked     = "ranked_low"
	reportKindContent    = "content_gap"
	reportKindReform     = "found_after_reform"
	reportKindService    = "service"
	reportKindWord       = "unknown_word"
	reportKindPending    = "pending"
)

var reportKindLabels = map[string]string{
	reportKindOnRequest:  "найдено, но только по запросу",
	reportKindAbsent:     "нет в каталоге",
	reportKindSubstitute: "нет в каталоге, есть аналог",
	reportKindNotUnder:   "поиск не понял слово",
	reportKindRanked:     "стоит низко",
	reportKindContent:    "в карточке нет слов",
	reportKindReform:     "нашли только после переформулировки",
	reportKindService:    "вопрос не о товаре",
	reportKindWord:       "слова нет в каталоге",
	reportKindPending:    "агент ещё не разбирал",
}

// reportSectionDefs — разделы в порядке показа.
var reportSectionDefs = []struct{ code, title, hint string }{
	{ReportNoOffer, "Спрос без предложения",
		"Ищут то, что нельзя купить: в каталоге только «по запросу» или товара нет. Это задача каталогу и категорийному менеджеру, а не правка поиска."},
	{ReportNotFound, "Товар есть, но не найден",
		"Товар продаётся, а поиск его не показал: опечатка, жаргон, другое написание. Это агент чинит правкой поиска."},
	{ReportUnknownWords, "Слова клиента, которых нет в каталоге",
		"Слова из неудачных запросов, которых нет ни в названиях товаров, ни в словаре поиска."},
	{ReportService, "Сервисные запросы",
		"Вопросы о заказе, оплате, доставке и документах: отвечает справка, а не товары."},
}

// ReportStreamLabel — подпись потока на экране ().
const ReportStreamLabel = "тестовый поток: запросы заказчика, прогоны сайта, синтетика из каталога"

const (
	reportDefaultDays = 7
	reportMaxDays     = 90
	// ReportProbeLimit — сколько неудачных запросов (по потерянному спросу) проверяется действующим поиском.
	ReportProbeLimit = 150
	reportParallel   = 6
	reportCacheTTL   = 10 * time.Minute
	reportWordRows   = 40
)

// reportProbe — ответ действующего поиска на запрос (без журнала).
type reportProbe struct {
	err       bool
	service   bool
	onRequest bool
	empty     bool
	found     *out.ReportFound
}

var reportCache = struct {
	sync.Mutex
	m map[string]reportCacheEntry
}{m: map[string]reportCacheEntry{}}

type reportCacheEntry struct {
	at time.Time
	p  reportProbe
}

// SearchOpsReport — GET /api/agent/report?days=: отчёт для маркетолога по журналу текущего каталога.
func (s *Services) SearchOpsReport(ctx context.Context, days int) (*out.SearchOpsReport, error) {
	if days == 0 {
		days = reportDefaultDays
	}
	if days < 1 || days > reportMaxDays {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(errs.FieldError{Field: "days",
			Code: errs.CodeOutOfRange, Message: fmt.Sprintf("Период — от 1 до %d дней", reportMaxDays)})
	}
	st := s.store()
	if st == nil || !s.repos.Enabled() {
		return nil, errs.NewUnavailableError("Сервис запущен без базы данных: журнала поиска нет")
	}
	now := s.now().UTC()
	stats, _, err := s.agentQueryStatsSince(ctx, now.Add(-time.Duration(days)*24*time.Hour), s.agent.opts.Holdout)
	if err != nil {
		return nil, err
	}
	res := &out.SearchOpsReport{Days: days, CheckedAt: now, RulesVersion: 1, Pending: []out.ReportRow{}}
	var lx *searchLexicon
	if cs := s.catalog.Load(); cs != nil {
		res.CatalogVersion, lx = cs.version, cs.lex
	}
	snap, err := s.repos.LatestRules(ctx)
	if err != nil {
		return nil, err
	}
	if snap != nil {
		res.RulesVersion = snap.Version
	}
	var rules *searchRules
	if lx != nil {
		rules = s.rulesFor(ctx, domain.ConfigB4, lx, nil)
	}
	flow := s.reportFlow()
	verdicts, err := s.reportVerdicts(ctx, res.CatalogVersion)
	if err != nil {
		return nil, err
	}

	// Итоги и слои потока.
	layerQueries, layerSearches := map[string]int{}, map[string]int{}
	for text, q := range stats.byText {
		res.Totals.Searches += q.Freq
		res.Totals.Queries++
		for _, l := range flowLayers(flow[text]) {
			layerQueries[l]++
			layerSearches[l] += q.Freq
		}
	}
	failed := stats.failed()
	res.Totals.FailedQueries = len(failed)
	for _, q := range failed {
		res.Totals.FailedSearches += q.Failures
	}
	res.Stream = out.ReportStream{Label: ReportStreamLabel, FreqLabel: "частоты модельные", Layers: []out.ReportLayer{}}
	for _, l := range []string{FlowLayerCustomer, FlowLayerSite, FlowLayerSynthetic, "live"} {
		if layerQueries[l] == 0 {
			continue
		}
		label := FlowLayerLabels[l]
		if l == "live" {
			label = "живой поиск"
		}
		res.Stream.Layers = append(res.Stream.Layers, out.ReportLayer{Code: l, Label: label, Queries: layerQueries[l],
			Searches: layerSearches[l]})
	}

	probeQs := failed
	if len(probeQs) > ReportProbeLimit {
		probeQs, res.Truncated = probeQs[:ReportProbeLimit], true
	}
	probes := s.reportProbes(ctx, probeQs, res.RulesVersion, res.CatalogVersion)

	sections := map[string]*out.ReportSection{}
	for _, d := range reportSectionDefs {
		sections[d.code] = &out.ReportSection{Code: d.code, Title: d.title, Hint: d.hint, Rows: []out.ReportRow{}}
	}
	type wordAgg struct {
		row   out.ReportRow
		sess  int
		layer map[string]bool
	}
	words := map[string]*wordAgg{}
	for i, q := range probeQs {
		row := reportRow(q, flow[q.Text])
		row.Found = probes[i].found
		if v, ok := verdicts[q.Text]; ok {
			row.Agent, row.Proposal = v.verdict, v.proposal
		}
		var unknown []string
		if lx != nil {
			_, unknown = queryTerms(q.Text, lx, rules)
		}
		row.Words = unknown
		section := classifyReportRow(&row, q, probes[i])
		if section == "" {
			res.Pending = append(res.Pending, row)
		} else {
			sec := sections[section]
			sec.Rows = append(sec.Rows, row)
			sec.Searches += q.Freq
			sec.Failures += q.Failures
		}
		if section == ReportService {
			continue // слова сервисных вопросов («доставка») — не пробел каталога
		}
		for _, w := range unknown {
			a := words[w]
			if a == nil {
				a = &wordAgg{row: out.ReportRow{Query: w, Kind: reportKindWord, KindLabel: reportKindLabels[reportKindWord],
					Layers: []string{}}, layer: map[string]bool{}}
				words[w] = a
			}
			a.row.Freq += q.Freq
			a.row.Failures += q.Failures
			a.row.Empty += q.Empty
			a.row.NoClick += q.NoClick
			a.row.Reformulated += q.Reformulated
			a.sess += q.FailSessions
			if len(a.row.Examples) < 3 {
				a.row.Examples = append(a.row.Examples, q.Text)
			}
			for _, l := range row.Layers {
				a.layer[l] = true
			}
			if a.row.Agent == nil && row.Agent != nil {
				a.row.Agent, a.row.Proposal = row.Agent, row.Proposal
			}
		}
	}
	wl := make([]*wordAgg, 0, len(words))
	for _, a := range words {
		wl = append(wl, a)
	}
	sort.Slice(wl, func(i, j int) bool {
		if wl[i].row.Failures != wl[j].row.Failures {
			return wl[i].row.Failures > wl[j].row.Failures
		}
		return wl[i].row.Query < wl[j].row.Query
	})
	ws := sections[ReportUnknownWords]
	for i, a := range wl {
		if i >= reportWordRows {
			break
		}
		for _, l := range []string{FlowLayerCustomer, FlowLayerSite, FlowLayerSynthetic, "live"} {
			if a.layer[l] {
				a.row.Layers = append(a.row.Layers, l)
			}
		}
		a.row.Evidence = fmt.Sprintf("в %d %s: %s", len(a.row.Examples), pluralRu(len(a.row.Examples), "неудачном запросе",
			"неудачных запросах", "неудачных запросах"), "«"+strings.Join(a.row.Examples, "», «")+"»")
		ws.Rows = append(ws.Rows, a.row)
		ws.Searches += a.row.Freq
		ws.Failures += a.row.Failures
	}
	for _, d := range reportSectionDefs {
		res.Sections = append(res.Sections, *sections[d.code])
	}
	return res, nil
}

// reportRow — строка запроса: исходы и происхождение.
func reportRow(q clusterQuery, rows []FlowRow) out.ReportRow {
	r := out.ReportRow{Query: q.Text, Freq: q.Freq, Failures: q.Failures, Empty: q.Empty, NoClick: q.NoClick,
		Reformulated: q.Reformulated, Layers: flowLayers(rows)}
	if len(rows) > 0 {
		r.Origin = rows[0].Origin
	}
	return r
}

// flowLayers — слои потока, в которых есть запрос; нет в потоке — живой поиск.
func flowLayers(rows []FlowRow) []string {
	if len(rows) == 0 {
		return []string{"live"}
	}
	var res []string
	for _, l := range []string{FlowLayerCustomer, FlowLayerSite, FlowLayerSynthetic} {
		if slices.ContainsFunc(rows, func(r FlowRow) bool { return r.Layer == l }) {
			res = append(res, l)
		}
	}
	return res
}

// classifyReportRow — раздел строки по доказательству (пусто — причина не доказана); заполняет Kind и Evidence.
func classifyReportRow(row *out.ReportRow, q clusterQuery, p reportProbe) string {
	set := func(kind, evidence string) {
		row.Kind, row.KindLabel, row.Evidence = kind, reportKindLabels[kind], evidence
	}
	diag := ""
	if row.Proposal != nil {
		diag = row.Proposal.Diagnosis
	} else if row.Agent != nil {
		diag = row.Agent.Diagnosis
	}
	agentNote := func(def string) string {
		if row.Agent != nil && row.Agent.Note != "" {
			return "агент: " + shortText(row.Agent.Note, 200)
		}
		return def
	}
	switch diag {
	case domain.DiagNonProduct:
		set(reportKindService, agentNote("агент: запрос не о товаре — задача справочному индексу"))
		return ReportService
	case domain.DiagOnRequestOnly:
		set(reportKindOnRequest, agentNote("агент: товар найден, купить нельзя — задача каталогу"))
		return ReportNoOffer
	case domain.DiagAssortmentGap:
		set(reportKindAbsent, agentNote("агент: расширенный поиск по каталогу не нашёл ни товара, ни аналога"))
		return ReportNoOffer
	case domain.DiagSubstitute:
		set(reportKindSubstitute, agentNote("агент: товара нет, в каталоге есть аналог"))
		return ReportNoOffer
	case domain.DiagAliasGap, domain.DiagTypo:
		set(reportKindNotUnder, agentNote("агент: товар есть, поиск не понял слово"))
		return ReportNotFound
	case domain.DiagRankingFailure:
		set(reportKindRanked, agentNote("агент: товар в выдаче, но не первым"))
		return ReportNotFound
	case domain.DiagContentGap:
		set(reportKindContent, agentNote("агент: в карточке нет слов, которыми ищут"))
		return ReportNotFound
	}
	switch {
	case p.service:
		set(reportKindService, "поиск отвечает справкой: вопрос не о товаре")
		return ReportService
	case p.onRequest && p.found != nil:
		set(reportKindOnRequest, fmt.Sprintf("первым — «%s»: купить нельзя, только по запросу", p.found.Title))
		return ReportNoOffer
	}
	for _, r := range q.ReformulatedTo {
		if r.Clicked > 0 {
			set(reportKindReform, fmt.Sprintf("покупатели переформулировали в «%s» и выбрали товар (%d из %d)", r.Text,
				r.Clicked, r.Count))
			return ReportNotFound
		}
	}
	ev := "причина не доказана: агент ещё не разбирал"
	if row.Agent != nil {
		ev = "агент: " + row.Agent.Label
		if row.Agent.Note != "" && row.Agent.Note != row.Agent.Label {
			ev += " — " + shortText(row.Agent.Note, 160)
		}
	}
	set(reportKindPending, ev)
	return ""
}

// reportProbes — ответ действующего поиска на запросы (параллельно, с кэшем на версию правил и каталога).
func (s *Services) reportProbes(ctx context.Context, qs []clusterQuery, rulesVersion int, catalog string) []reportProbe {
	res := make([]reportProbe, len(qs))
	sem := make(chan struct{}, reportParallel)
	var wg sync.WaitGroup
	now := time.Now()
	for i, q := range qs {
		key := fmt.Sprintf("%d|%s|%s", rulesVersion, catalog, q.Text)
		reportCache.Lock()
		e, ok := reportCache.m[key]
		reportCache.Unlock()
		if ok && now.Sub(e.at) < reportCacheTTL {
			res[i] = e.p
			continue
		}
		wg.Add(1)
		go func(i int, text, key string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			p := s.reportProbe(ctx, text)
			res[i] = p
			if !p.err {
				reportCache.Lock()
				reportCache.m[key] = reportCacheEntry{at: time.Now(), p: p}
				reportCache.Unlock()
			}
		}(i, q.Text, key)
	}
	wg.Wait()
	return res
}

// reportProbe — один запрос через действующий поиск витрины (как у покупателя), без журнала.
func (s *Services) reportProbe(ctx context.Context, q string) (p reportProbe) {
	defer func() {
		if r := recover(); r != nil {
			p = reportProbe{err: true}
		}
	}()
	resp, err := s.SearchWith(ctx, agentRequestID(), in.SearchRequest{Query: q, Config: agentSearchConfig, Limit: 10,
		SessionID: "agent"}, agentSearchOpts(nil))
	if err != nil {
		return reportProbe{err: true}
	}
	p.empty = resp.Empty != nil
	p.service = resp.Help != nil || resp.Target != nil && resp.Target.Type == out.TargetService
	p.onRequest = onRequestOnly(q, resp)
	if len(resp.Items) > 0 {
		it := resp.Items[0]
		p.found = &out.ReportFound{FamilyID: it.FamilyID, Title: it.Title, MatchType: it.MatchType, PriceState: it.PriceState}
		if resp.Target != nil {
			p.found.Target = resp.Target.Type
		}
	}
	return p
}

// verdictPick — вывод агента и предложение по запросу.
type verdictPick struct {
	verdict  *out.AgentVerdict
	proposal *out.ReportProposal
}

// reportVerdicts — вывод агента по нормализованному тексту запроса: итог кластера последних запусков (как в
// «Запросы без ответа») и предложение или задача агента (новое важнее старого; отклонённое и устаревшее не берётся).
func (s *Services) reportVerdicts(ctx context.Context, catalog string) (map[string]verdictPick, error) {
	res := map[string]verdictPick{}
	since, err := s.catalogSince(ctx, catalog)
	if err != nil {
		return nil, err
	}
	clusters, err := s.repos.RecentIssueClusters(ctx, verdictRuns)
	if err != nil {
		return nil, err
	}
	final := map[string]bool{}
	for i := range clusters {
		c := &clusters[i]
		if c.RunStartedAt.Before(since) {
			continue // разбор прежнего каталога (mock): к запросам фида он не относится
		}
		v, ok := verdictFor(c)
		for _, t := range clusterTexts(c) {
			if t == "" || final[t] {
				continue
			}
			if _, seen := res[t]; seen && !ok {
				continue
			}
			vv := v
			res[t], final[t] = verdictPick{verdict: &vv}, ok
		}
	}
	ps, err := s.store().ListProposals(ctx, "", "")
	if err != nil {
		return nil, err
	}
	sort.SliceStable(ps, func(i, j int) bool { return ps[i].CreatedAt.After(ps[j].CreatedAt) })
	taken := map[string]bool{}
	for i := range ps {
		p := &ps[i]
		if p.Status == domain.ProposalRejected || p.Status == domain.ProposalStale || p.CreatedAt.Before(since) {
			continue
		}
		var qs []struct {
			Text string `json:"text"`
		}
		_ = json.Unmarshal(p.ClusterQueries, &qs)
		rp := &out.ReportProposal{ID: p.ID.String(), Status: p.Status, Action: p.Action, Diagnosis: p.Diagnosis,
			RunID: p.RunID.String()}
		if p.ClusterID != nil {
			rp.ClusterID = *p.ClusterID
		}
		for _, q := range qs {
			t := foldQuery(q.Text)
			if t == "" || taken[t] {
				continue
			}
			taken[t] = true
			pick := res[t]
			pick.proposal = rp
			// Итог — предложение (или задача) агента; лента — кластер, из которого оно пришло.
			v := out.AgentVerdict{Code: verdictProposal, Label: verdictLabels[verdictProposal], Diagnosis: p.Diagnosis,
				RunID: rp.RunID, ClusterID: rp.ClusterID, ClusterLabel: p.ClusterLabel, Note: shortText(p.DiagnosisText, 300)}
			if pick.verdict != nil && pick.verdict.RunID == rp.RunID && v.ClusterID == "" {
				v.ClusterID = pick.verdict.ClusterID
			}
			pick.verdict = &v
			res[t] = pick
		}
	}
	return res, nil
}

// reportFlow — строки тестового потока по нормализованному тексту (происхождение строк отчёта). Файл —
// SEARCHOPS_FLOW_FILE, иначе searchops_flow.jsonl рядом с данными источника (data/softline); нет файла — все
// запросы считаются живыми.
func (s *Services) reportFlow() map[string][]FlowRow {
	path := strings.TrimSpace(os.Getenv("SEARCHOPS_FLOW_FILE"))
	if path == "" {
		if d, ok := s.source.(interface{ DataDir() string }); ok && d.DataDir() != "" {
			path = filepath.Join(d.DataDir(), "..", "searchops_flow.jsonl")
		}
	}
	res := map[string][]FlowRow{}
	if path == "" {
		return res
	}
	fi, err := os.Stat(path)
	if err != nil {
		return res
	}
	flowCache.Lock()
	defer flowCache.Unlock()
	if flowCache.path == path && flowCache.mod.Equal(fi.ModTime()) {
		return flowCache.rows
	}
	rows, err := LoadFlow(path)
	if err != nil {
		return res
	}
	for _, r := range rows {
		t := foldQuery(r.Query)
		res[t] = append(res[t], r)
	}
	flowCache.path, flowCache.mod, flowCache.rows = path, fi.ModTime(), res
	return res
}

var flowCache struct {
	sync.Mutex
	path string
	mod  time.Time
	rows map[string][]FlowRow
}

// pluralRu — форма слова по числу: 1 запрос, 2 запроса, 5 запросов.
func pluralRu(n int, one, few, many string) string {
	n %= 100
	if n >= 11 && n <= 14 {
		return many
	}
	switch n % 10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	}
	return many
}

// catalogSince — с какого момента действует текущий каталог: первая версия правил с этим catalog_version (импорт
// начинает цепочку версий). Разборы и предложения агента раньше этого момента — о прежнем каталоге. Нет версий —
// нулевое время (всё считается текущим).
func (s *Services) catalogSince(ctx context.Context, catalog string) (time.Time, error) {
	if catalog == "" {
		return time.Time{}, nil
	}
	if v, ok := catalogSinceCache.Load(catalog); ok {
		return v.(time.Time), nil
	}
	vs, err := s.repos.ListConfigVersions(ctx)
	if err != nil {
		return time.Time{}, err
	}
	var since time.Time
	for _, v := range vs {
		if v.CatalogVersion == catalog && (since.IsZero() || v.CreatedAt.Before(since)) {
			since = v.CreatedAt
		}
	}
	if !since.IsZero() {
		catalogSinceCache.Store(catalog, since)
	}
	return since, nil
}

// catalogSinceCache — начало каталога по версии (первая версия правил не меняется).
var catalogSinceCache sync.Map
