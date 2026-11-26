package services

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"

	"go.uber.org/zap"
)

// Оценка «по цели» на фиде заказчика (контур эталонов): наборы эталонов, примеров ТЗ, соседей и
// закрытый набор прогоняются через тот же сервис поиска, что отвечает покупателю, в конфигурациях сравнения BASIS:
// BM25 → BM25 + нормализатор (B1) → гибрид (B2) → полный продукт (live). Метрики — Success@1 по цели, попадание в
// первые 4, MRR@10, доли ответов с ≥4 семействами и SKU, нарушения жёстких условий, p50/p95 и ₽ за запрос по трассам
// и llm_usage. Прогон пишется в evaluations, как у прочих наборов.
//
// Холодный прогон (Cold): кэш разборов модели, векторов и оценок реранкера не читается — каждый запрос маршрута D
// идёт в модель; кэш витрины прогон не меняет. Тёплый — второй проход по набору: кэши заполнены первым (повторный
// запрос покупателя).

// targetSearchLimit — карточек в ответе прогона: как у витрины по умолчанию.
const targetSearchLimit = 20

// targetWarmupQuery — запрос прогрева процесса перед каждой конфигурацией (в метрики не входит).
const targetWarmupQuery = "windows" // короткий: маршрут без модели, в llm_usage не попадает

// Режимы прогона (TargetCost.Mode).
const (
	TargetModeCold = "cold"
	TargetModeWarm = "warm"
)

// Тексты отказов.
const (
	MsgTargetNoSets     = "Наборы «по цели» не найдены: в mapping.yaml нет target_sets"
	MsgTargetUnknownSet = "Набор «по цели» — etalon, tz, neighbors, understanding, feed_tune или feed_regression"
	MsgTargetClosed     = "Закрытый набор прогоняется только из CLI с явным путём (--file); агенту и API он недоступен"
	MsgTargetNoCatalog  = "Каталог не загружен: оценка «по цели» ждёт словарь каталога"
	MsgTargetConfig     = "Конфигурация — BM25, B0–B4 или live"
)

// targetSetsDirer — источник данных с каталогом наборов «по цели» (catalogsource.Generic).
type targetSetsDirer interface{ TargetSetsDir() string }

// TargetSetQueries — запросы набора «по цели» по имени: etalon, tz, neighbors, understanding, feed_tune,
// feed_regression. Наборы агента SearchOps — feed_tune (настройка) и feed_regression (регрессия). Закрытый набор
// здесь не отдаётся никогда.
func (s *Services) TargetSetQueries(_ context.Context, set string) ([]domain.TargetQuery, error) {
	set = strings.ToLower(strings.TrimSpace(set))
	if set == domain.TargetSetClosed {
		return nil, errs.NewForbiddenError(MsgTargetClosed)
	}
	files, ok := domain.TargetSetFiles[set]
	if !ok {
		return nil, errs.NewValidationError(MsgTargetUnknownSet)
	}
	d, ok := s.source.(targetSetsDirer)
	if !ok || d.TargetSetsDir() == "" {
		return nil, errs.NewUnavailableError(MsgTargetNoSets)
	}
	var qs []domain.TargetQuery
	seen := map[string]bool{}
	for _, f := range files {
		rows, err := ReadTargetQueries(filepath.Join(d.TargetSetsDir(), f))
		if err != nil {
			return nil, err
		}
		for _, q := range rows {
			if seen[q.ID] {
				return nil, fmt.Errorf("набор %s: id %s повторяется в %s", set, q.ID, f)
			}
			seen[q.ID] = true
			qs = append(qs, q)
		}
	}
	return qs, nil
}

// ReadTargetQueries — строки JSONL набора «по цели» с проверкой: id и текст есть, id не повторяются, тип цели
// известен.
func ReadTargetQueries(path string) ([]domain.TargetQuery, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("набор «по цели»: %w", err)
	}
	defer f.Close()
	var qs []domain.TargetQuery
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var q domain.TargetQuery
		if err := json.Unmarshal([]byte(line), &q); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", filepath.Base(path), n, err)
		}
		if err := validTargetQuery(q); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", filepath.Base(path), n, err)
		}
		if seen[q.ID] {
			return nil, fmt.Errorf("%s:%d: id %s повторяется", filepath.Base(path), n, q.ID)
		}
		seen[q.ID] = true
		qs = append(qs, q)
	}
	return qs, sc.Err()
}

func validTargetQuery(q domain.TargetQuery) error {
	if q.ID == "" || strings.TrimSpace(q.Text) == "" {
		return fmt.Errorf("нет id или текста")
	}
	return validTargetExpect(q.Expect)
}

func validTargetExpect(e domain.TargetExpect) error {
	switch e.Type {
	case domain.ExpectFamily:
		if len(e.GroupIDs) == 0 {
			return fmt.Errorf("family без group_ids")
		}
	case domain.ExpectVendor, domain.ExpectVendorCategory:
		if e.VendorSlug == "" {
			return fmt.Errorf("%s без vendor_slug", e.Type)
		}
	case domain.ExpectCategory:
		if len(e.CategoryIDs) == 0 {
			return fmt.Errorf("category без category_ids")
		}
	case domain.ExpectSKU:
		if len(e.OfferIDs) == 0 {
			return fmt.Errorf("sku без offer_ids")
		}
	case domain.ExpectNone:
	default:
		return fmt.Errorf("тип цели %q неизвестен", e.Type)
	}
	for _, a := range e.Also {
		if err := validTargetExpect(a); err != nil {
			return err
		}
	}
	return nil
}

// lexTargetLookup — справочник позиций вердикта из словаря каталога в памяти.
type lexTargetLookup struct {
	lx       *searchLexicon
	famURL   map[string]string
	catNames map[string][]string
}

var targetLookups sync.Map // *searchLexicon → *lexTargetLookup

// targetLookupOf — справочник вердикта для словаря каталога (один на версию каталога).
func targetLookupOf(lx *searchLexicon) *lexTargetLookup {
	if v, ok := targetLookups.Load(lx); ok {
		return v.(*lexTargetLookup)
	}
	l := &lexTargetLookup{lx: lx, famURL: map[string]string{}, catNames: map[string][]string{}}
	for _, d := range lx.docs {
		if d.FamilyURL != "" && l.famURL[d.FamilyID] == "" {
			l.famURL[d.FamilyID] = d.FamilyURL
		}
	}
	for id, name := range lx.categoryName {
		k := strings.ToLower(strings.TrimSpace(name))
		l.catNames[k] = append(l.catNames[k], id)
	}
	v, _ := targetLookups.LoadOrStore(lx, l)
	return v.(*lexTargetLookup)
}

func (l *lexTargetLookup) doc(productID string) (targetDoc, bool) {
	d, ok := l.lx.docs[productID]
	if !ok {
		return targetDoc{}, false
	}
	vendor := d.VendorSlug
	if vendor == "" {
		vendor = d.VendorID
	}
	return targetDoc{Family: d.FamilyID, Vendor: vendor, Category: d.CategoryID, Kind: d.Kind,
		LicenseKind: d.LicenseKind, OrgType: d.LicenseOrgType, Duration: d.LicenseDuration,
		TierMin: d.LicenseTierMin, TierMax: d.LicenseTierMax, OfferURL: d.OfferURL}, true
}

func (l *lexTargetLookup) familyURL(familyID string) string { return l.famURL[familyID] }

func (l *lexTargetLookup) categoriesNamed(name string) []string {
	return l.catNames[strings.ToLower(strings.TrimSpace(name))]
}

// TargetEvalRequest — что прогонять. File — явный путь набора (закрытый); Set тогда — подпись прогона.
type TargetEvalRequest struct {
	Set     string
	File    string
	Configs []string
	// Cold — холодный прогон: кэш разборов модели, векторов и реранкера не читается и не пишется.
	Cold bool
	// Progress — строка хода прогона (CLI); nil — молча.
	Progress func(string)
}

// validTargetConfigs — конфигурации прогона «по цели»; пусто — сравнение BASIS.
func validTargetConfigs(cfgs []string) ([]string, error) {
	var res []string
	for _, c := range cfgs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if strings.EqualFold(c, domain.ConfigLive) {
			c = domain.ConfigLive
		} else {
			c = strings.ToUpper(c)
		}
		if c != domain.ConfigLive && c != domain.ConfigBM25 && !slices.Contains(domain.SearchConfigs, c) {
			return nil, errs.NewValidationError(MsgTargetConfig)
		}
		if !slices.Contains(res, c) {
			res = append(res, c)
		}
	}
	if len(res) == 0 {
		res = slices.Clone(domain.TargetEvalConfigs)
	}
	return res, nil
}

// RunTargetEvaluation — прогон набора «по цели» по конфигурациям, последовательно (время ответа не искажается).
// В базу не пишет: после прогона CLI ждёт фоновые записи llm_usage (Wait) и вызывает FinishTargetEvaluation.
func (s *Services) RunTargetEvaluation(ctx context.Context, req TargetEvalRequest) (*EvalResult, error) {
	started := time.Now()
	set := strings.ToLower(strings.TrimSpace(req.Set))
	var queries []domain.TargetQuery
	var err error
	switch {
	case req.File != "":
		queries, err = ReadTargetQueries(req.File)
	default:
		queries, err = s.TargetSetQueries(ctx, set)
	}
	if err != nil {
		return nil, err
	}
	if len(queries) == 0 {
		return nil, errs.NewValidationError(MsgEvalEmptySet)
	}
	configs, err := validTargetConfigs(req.Configs)
	if err != nil {
		return nil, err
	}
	st := s.catalog.Load()
	if st == nil || st.lex == nil {
		return nil, errs.NewUnavailableError(MsgTargetNoCatalog)
	}
	lk := targetLookupOf(st.lex)
	mode := TargetModeWarm
	if req.Cold {
		mode = TargetModeCold
	}
	run := &EvalResult{RunID: newEvalRunID(started), Set: set, SetHash: targetSetHash(queries), Queries: len(queries),
		Commit: buildCommit(), StartedAt: started.UTC(), Configs: []out.EvalConfigResult{}}
	var prev []out.EvalQueryResult
	for i, cfg := range configs {
		// Прогрев процесса (ленивые справочники допродажи и подсказок строятся первым поиском): запрос вне наборов,
		// в метрики не входит; в холодном прогоне — тоже мимо кэшей.
		_, _, _ = s.evalTargetSearch(ctx, run.RunID, cfg, domain.TargetQuery{ID: "warmup", Text: targetWarmupQuery,
			Expect: domain.TargetExpect{Type: domain.ExpectNone}}, nil, req.Cold, lk)
		if !req.Cold {
			// Тёплый прогон — второй проход: первый заполняет кэши разборов, векторов и реранкера этого процесса.
			// У первого прохода свой номер: его вызовы модели в расход прогона (llm_usage по request_id) не входят.
			pre := newEvalRunID(time.Now())
			for _, q := range queries {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				_, _, _ = s.evalTargetSearch(ctx, pre, cfg, q, nil, false, lk)
			}
		}
		t := time.Now()
		res := out.EvalConfigResult{Config: cfg, PerQuery: make([]out.EvalQueryResult, 0, len(queries))}
		for j, q := range queries {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			r, rules, catalog := s.evalTargetSearch(ctx, run.RunID, cfg, q, nil, req.Cold, lk)
			if rules > 0 {
				res.RulesVersion = rules
			}
			if catalog != "" {
				res.CatalogVersion = catalog
			}
			res.PerQuery = append(res.PerQuery, r)
			if req.Progress != nil && (j+1)%25 == 0 {
				req.Progress(fmt.Sprintf("%s %s: %d из %d", set, cfg, j+1, len(queries)))
			}
		}
		res.Metrics = targetAggregate(res.PerQuery, mode)
		res.ByType = targetByGroup(res.PerQuery, mode)
		res.Regressions, res.Improvements = []out.EvalChange{}, []out.EvalChange{}
		if i > 0 {
			res.Against = configs[i-1]
			res.Regressions, res.Improvements = TargetChanges(prev, res.PerQuery, res.Against)
		}
		if res.Metrics.Errors > 0 {
			res.Warnings = append(res.Warnings, fmt.Sprintf("ошибок поиска: %d — эти запросы в метрики не вошли", res.Metrics.Errors))
		}
		res.DurationMs = time.Since(t).Milliseconds()
		prev = res.PerQuery
		run.Configs = append(run.Configs, res)
		if req.Progress != nil {
			req.Progress(fmt.Sprintf("%s %s: готово за %s", set, cfg, time.Since(t).Round(time.Second)))
		}
	}
	run.DurationMs = time.Since(started).Milliseconds()
	run.Repro = reproLine(run.Set, run.SetHash, run.Queries, run.Configs, run.Commit)
	zap.L().Info("оценка по цели: прогон завершён", zap.String("run_id", run.RunID), zap.String("set", set),
		zap.Strings("configs", configs), zap.Int("queries", len(queries)), zap.String("mode", mode))
	return run, nil
}

// EvalTargetQuery — один запрос набора «по цели» через поиск без журнала: для песочницы агента SearchOps (rules —
// снимок правил с правкой; nil — действующие правила). Вердикт — в EvalQueryResult.Target, сравнение до/после —
// TargetChanges.
func (s *Services) EvalTargetQuery(ctx context.Context, runID, cfg string, q domain.TargetQuery, rules *RulesSnapshot) (out.EvalQueryResult, error) {
	st := s.catalog.Load()
	if st == nil || st.lex == nil {
		return out.EvalQueryResult{}, errs.NewUnavailableError(MsgTargetNoCatalog)
	}
	r, _, _ := s.evalTargetSearch(ctx, runID, cfg, q, rules, false, targetLookupOf(st.lex))
	return r, nil
}

// evalTargetSearch — запрос через сервис поиска и вердикт; версии правил и каталога — из трассы.
func (s *Services) evalTargetSearch(ctx context.Context, runID, cfg string, q domain.TargetQuery, rules *RulesSnapshot,
	cold bool, lk targetLookup) (out.EvalQueryResult, int, string) {
	sctx := ctx
	if cold {
		sctx = withSearchModes(ctx, searchModes{noParseCache: true, noModelCache: true})
	}
	sr := in.SearchRequest{Query: q.Text, Config: cfg, Limit: targetSearchLimit, SessionID: evalRequestPrefix + runID}
	t := time.Now()
	resp, err := s.SearchWith(sctx, evalRequestID(runID, cfg, q.ID), sr, SearchOptions{Rules: rules, NoLog: true, Storefront: true})
	r := out.EvalQueryResult{ID: q.ID, Text: q.Text, Type: q.Group, Freq: 1, Top: []string{}, Grades: []int{},
		Ms: float64(time.Since(t).Microseconds()) / 1000}
	if err != nil {
		r.Error = err.Error()
		return r, 0, ""
	}
	v := judgeTarget(q, resp, lk)
	r.Total, r.Empty = resp.Total, len(resp.Items) == 0
	for i, it := range resp.Items {
		if i >= targetTopCards {
			break
		}
		r.Top = append(r.Top, it.ProductID)
	}
	rulesV, catalog := 0, ""
	if tr := resp.Trace; tr != nil {
		r.Route = tr.Route
		if tr.TotalMs > 0 {
			r.Ms = tr.TotalMs
		}
		rulesV, catalog = tr.RulesVersion, tr.CatalogVersion
		v.ParseSource = tr.ParseSource
		v.CostRub = tr.CostRub
		if l, ok := tr.LLM.(*domain.TraceLLM); ok && l != nil {
			v.LLMCalls, v.InputTokens, v.OutputTokens = max(l.Calls, 1), l.InputTokens, l.OutputTokens
		}
		v.ZeroResults = routeSignal(tr.Steps, "zero_results")
	}
	r.Target = &v
	return r, rulesV, catalog
}

// routeSignal — сигнал маршрута в шаге route трассы.
func routeSignal(steps []any, signal string) bool {
	for _, st := range steps {
		var rs *domain.RouteStep
		switch v := st.(type) {
		case domain.RouteStep:
			rs = &v
		case *domain.RouteStep:
			rs = v
		}
		if rs != nil && slices.Contains(rs.Signals, signal) {
			return true
		}
	}
	return false
}

// targetSetHash — хэш набора: тексты и ожидания.
func targetSetHash(qs []domain.TargetQuery) string {
	type row struct {
		ID     string              `json:"id"`
		Text   string              `json:"text"`
		Expect domain.TargetExpect `json:"expect"`
	}
	rows := make([]row, len(qs))
	for i, q := range qs {
		rows[i] = row{q.ID, q.Text, q.Expect}
	}
	raw, _ := json.Marshal(rows)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])[:12]
}

// usagePrefix — начало request_id поисков конфигурации прогона (evalRequestID).
func usagePrefix(runID, cfg string) string {
	return evalRequestPrefix + runID[len(runID)-6:] + "-" + cfg + "-"
}

// FinishTargetEvaluation — расход модели из llm_usage по request_id прогона (CLI вызывает после Wait: строки учёта
// пишутся в фоне) и запись в evaluations.
func (s *Services) FinishTargetEvaluation(ctx context.Context, run *EvalResult, persist bool) error {
	if !s.repos.Enabled() {
		return nil
	}
	for i := range run.Configs {
		c := &run.Configs[i]
		if c.Metrics.Target == nil || c.Metrics.Target.Cost == nil {
			continue
		}
		t, err := s.repos.LLMUsageByPrefix(ctx, usagePrefix(run.RunID, c.Config))
		if err != nil {
			return fmt.Errorf("оценка по цели: llm_usage: %w", err)
		}
		cost := c.Metrics.Target.Cost
		cost.LLMCalls, cost.InputTokens, cost.OutputTokens = t.Calls, t.InputTokens, t.OutputTokens
		cost.LLMRub, cost.UsageJoined = evalRound4(t.CostRub), true
	}
	if !persist {
		return nil
	}
	if err := s.repos.SaveEvaluations(ctx, evaluationRows(run)); err != nil {
		return fmt.Errorf("оценка по цели: результат не записан: %w", err)
	}
	return nil
}

// targetAggregate — сводные метрики конфигурации: задержки и маршруты как у прочих наборов, плюс «по цели».
func targetAggregate(rs []out.EvalQueryResult, mode string) out.EvalMetrics {
	m := out.EvalMetrics{}
	var latency []float64
	byRoute := map[string][]float64{}
	t := &out.TargetMetrics{Breakdown: map[string]map[string]out.TargetCell{}}
	cost := &out.TargetCost{Mode: mode}
	var mrr evalMean
	for _, r := range rs {
		if r.Error != "" || r.Target == nil {
			m.Errors++
			continue
		}
		m.Queries++
		latency = append(latency, r.Ms)
		route := r.Route
		if route == "" {
			route = "?"
		}
		byRoute[route] = append(byRoute[route], r.Ms)
		v := r.Target
		t.Answers++
		if v.Pass {
			t.PassN++
		}
		if v.Families >= 4 {
			t.Families4N++
		}
		if v.SKUs >= 4 {
			t.SKUs4N++
		}
		if v.Cards >= 4 {
			t.Cards4N++
		}
		if v.HasHard {
			t.HardQueries++
			if v.Violations > 0 {
				t.HardViolated++
			}
			t.ViolationItems += v.Violations
		}
		if !v.ByCards {
			t.Judged++
			if v.Strict {
				t.Success1N++
			}
			if v.Top4 {
				t.Top4N++
			}
			if v.Rank > 0 {
				mrr.add(1 / float64(v.Rank))
			} else {
				mrr.add(0)
			}
			for dim, key := range map[string]string{"vendor": v.Vendor, "distortion": v.Distortion, "group": v.Group} {
				if key == "" {
					continue
				}
				if t.Breakdown[dim] == nil {
					t.Breakdown[dim] = map[string]out.TargetCell{}
				}
				c := t.Breakdown[dim][key]
				c.N++
				if v.Strict {
					c.Success1++
				}
				if v.Top4 {
					c.Top4++
				}
				t.Breakdown[dim][key] = c
			}
		}
		cost.Searches++
		cost.TraceRub += v.CostRub
		if r.Route == domain.RouteIntent {
			cost.RouteD++
		}
		switch {
		case v.LLMCalls > 0 || v.ParseSource == domain.ParseLLM:
			cost.ModelSearches++
		case v.ParseSource == domain.ParseCache:
			cost.CacheSearches++
		case r.Route == domain.RouteIntent:
			cost.RouteDNoModel++
		}
	}
	share := func(n, of int) *float64 {
		if of == 0 {
			return nil
		}
		return evalPtr(evalRound4(float64(n) / float64(of)))
	}
	t.Success1, t.Top4, t.MRR10 = share(t.Success1N, t.Judged), share(t.Top4N, t.Judged), mrr.value()
	t.Pass = share(t.PassN, t.Answers)
	t.Families4, t.SKUs4, t.Cards4 = share(t.Families4N, t.Answers), share(t.SKUs4N, t.Answers), share(t.Cards4N, t.Answers)
	if len(t.Breakdown) == 0 {
		t.Breakdown = nil
	}
	cost.TraceRub = evalRound4(cost.TraceRub)
	t.Cost = cost
	m.Target = t
	m.Latency = evalLatencyOf(latency)
	if len(byRoute) > 0 {
		m.ByRoute = make(map[string]out.EvalLatency, len(byRoute))
		for route, ms := range byRoute {
			m.ByRoute[route] = evalLatencyOf(ms)
		}
	}
	return m
}

// targetByGroup — метрики по частям набора (etalon, tz, neighbor, catalog, understanding).
func targetByGroup(rs []out.EvalQueryResult, mode string) map[string]out.EvalMetrics {
	groups := map[string][]out.EvalQueryResult{}
	for _, r := range rs {
		groups[r.Type] = append(groups[r.Type], r)
	}
	res := make(map[string]out.EvalMetrics, len(groups))
	for g, list := range groups {
		m := targetAggregate(list, mode)
		m.ByRoute = nil
		if m.Target != nil {
			m.Target.Breakdown, m.Target.Cost = nil, nil
		}
		res[g] = m
	}
	return res
}

// TargetChanges — ухудшения и улучшения cur относительно prev по id запроса: цель (Success@1), первые 4, прочие
// условия строки, жёсткие условия. Для агента SearchOps: правка допустима, если worse пуст.
func TargetChanges(prev, cur []out.EvalQueryResult, against string) (worse, better []out.EvalChange) {
	worse, better = []out.EvalChange{}, []out.EvalChange{}
	before := make(map[string]out.EvalQueryResult, len(prev))
	for _, r := range prev {
		before[r.ID] = r
	}
	for _, c := range cur {
		p, ok := before[c.ID]
		if !ok || p.Error != "" || c.Error != "" || p.Target == nil || c.Target == nil {
			continue
		}
		a, b := p.Target, c.Target
		var w, g []string
		switch {
		case a.Strict && !b.Strict:
			w = append(w, "lost_target")
		case !a.Strict && b.Strict:
			g = append(g, "gained_target")
		}
		switch {
		case a.Top4 && !b.Top4:
			w = append(w, "lost_top4")
		case !a.Top4 && b.Top4:
			g = append(g, "gained_top4")
		}
		if a.Strict == b.Strict {
			switch {
			case a.Pass && !b.Pass:
				w = append(w, "lost_pass")
			case !a.Pass && b.Pass:
				g = append(g, "gained_pass")
			}
		}
		switch {
		case b.Violations > a.Violations:
			w = append(w, "new_violation")
		case b.Violations < a.Violations:
			g = append(g, "fixed_violation")
		}
		ch := out.EvalChange{QueryID: c.ID, Text: c.Text, Type: c.Type, Freq: 1, Against: against}
		if len(w) > 0 {
			ch.Reasons = w
			worse = append(worse, ch)
		}
		if len(g) > 0 {
			ch.Reasons = g
			better = append(better, ch)
		}
	}
	return worse, better
}
