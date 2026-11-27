package services

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"kuznica/api/domain"
	"kuznica/api/out"

	"go.uber.org/zap"
)

// Песочница агента (ARCHITECTURE §3.1 шаг 7, §6 «Условия допуска»): правка накладывается на действующие
// правила снимком и прогоняется тем же сервисом поиска (SearchWith с SearchOptions.Rules) на целевом
// кластере и на регрессионном наборе. Правка допускается, только если целевой кластер улучшился, точные
// запросы не ухудшились, новых нарушений условий лицензии и ложных «нашлось» нет и на регрессионном
// наборе ухудшилось не больше порога. Метрики считает код; модель видит только итог.
//
// Набор test замороженный: его разметка в решениях агента не участвует. Запросы кластера из test
// оцениваются без разметки — по тому, стоит ли первым ожидаемое правкой семейство.

// evalCount — находилось из.
type evalCount struct {
	Found int `json:"found"`
	Total int `json:"total"`
}

// targetQueryEval — один запрос целевого кластера до и после правки.
type targetQueryEval struct {
	Text        string   `json:"text"`
	Freq        int      `json:"freq"`
	Judged      bool     `json:"judged"`
	BeforeFound bool     `json:"before_found"`
	AfterFound  bool     `json:"after_found"`
	BeforeTop   []string `json:"before_top"`
	AfterTop    []string `json:"after_top"`
	AfterEmpty  bool     `json:"after_empty"`
}

// patchEval — результат прогона правки (оценка прогона с подробностями).
type patchEval struct {
	Target struct {
		Before  evalCount         `json:"before"`
		After   evalCount         `json:"after"`
		Worse   int               `json:"worse"`
		Queries []targetQueryEval `json:"queries"`
	} `json:"target"`
	Regression struct {
		Worse        int              `json:"worse"`
		Better       int              `json:"better"`
		Total        int              `json:"total"`
		Sets         []string         `json:"sets"`
		WorseQueries []out.EvalChange `json:"worse_queries"`
		// BetterQueries — первые улучшившиеся (контур автономии: прогон «без правки» при проверке эффекта — это запросы,
		// которым правка навредила).
		BetterQueries []out.EvalChange `json:"better_queries,omitempty"`
	} `json:"regression"`
	ExactWorse              int      `json:"exact_worse"`
	ConstraintViolationsNew int      `json:"constraint_violations_new"`
	FalseRescueNew          int      `json:"false_rescue_new"`
	Passed                  bool     `json:"passed"`
	Reasons                 []string `json:"reasons"`
	RulesVersion            int      `json:"rules_version"`
	Ms                      int64    `json:"ms"`
}

// sandbox — общие данные прогонов одного запуска: разметка, справочник позиций, базовая линия регрессии.
type sandbox struct {
	s       *Services
	runID   string
	active  *ruleSet
	cat     *evalCatalog
	lex     *searchLexicon
	sets    []string
	maxWors int
	// byText — размеченные запросы не из test по нормализованному тексту.
	byText map[string]domain.EvalQuery
	// regression — запросы регрессионного набора и их выдача на действующих правилах.
	regression []domain.EvalQuery
	once       sync.Once
	baseline   map[string]out.EvalQueryResult
	baseErr    error
	// regStatus — наборы регрессии на текущем каталоге (контур фида): на фиде без своих наборов правка не допускается.
	regStatus out.AgentRegression
	// Контур фида, фид заказчика: регрессия — наборы «по цели» оценки (feed_regression, feed_tune), вердикт — цель
	// ответа (Success@1, первые 4, жёсткие условия), сравнение до/после — TargetChanges. feed — песочница на фиде.
	feed       bool
	targetQs   []domain.TargetQuery
	targetOnce sync.Once
	targetBase map[string]out.EvalQueryResult
	targetErr  error
}

// newSandbox — песочница запуска; без наборов запросов регрессия пуста (и правка не проходит).
func (s *Services) newSandbox(ctx context.Context, runID string, active *ruleSet, opts AgentOptions) (*sandbox, error) {
	cat, err := s.evalCatalog(ctx)
	if err != nil {
		return nil, err
	}
	sb := &sandbox{s: s, runID: runID, active: active, cat: cat, sets: opts.RegressionSets, maxWors: opts.MaxRegressionWorse,
		byText: map[string]domain.EvalQuery{}}
	if cs := s.catalog.Load(); cs != nil {
		sb.lex = cs.lex
	}
	if loader, ok := s.source.(queryLoader); ok {
		all, err := loader.LoadQueries(ctx)
		if err == nil {
			for _, q := range all {
				if q.Set == domain.EvalSetTest {
					continue
				}
				sb.byText[foldQuery(q.Text)] = q
				if slices.Contains(sb.sets, q.Set) {
					sb.regression = append(sb.regression, q)
				}
			}
		}
	}
	sort.Slice(sb.regression, func(i, j int) bool { return sb.regression[i].ID < sb.regression[j].ID })
	scope := s.agentLogScope(ctx)
	bySet := map[string]int{}
	for _, q := range sb.regression {
		bySet[q.Set]++
	}
	if d, ok := s.source.(targetSetsDirer); ok && d.TargetSetsDir() != "" || !scope.All && !scope.Historical {
		// Фид заказчика: наборы mock здесь не читаются, регрессия — наборы «по цели» оценки.
		sb.feed, sb.sets, sb.regression, bySet = true, opts.FeedRegressionSets, nil, map[string]int{}
		seen := map[string]bool{}
		for _, name := range sb.sets {
			qs, err := s.TargetSetQueries(ctx, name)
			if err != nil {
				continue // набора нет — это и будет «наборы регрессии на фиде не заданы»
			}
			for _, q := range qs {
				if !seen[q.ID] {
					seen[q.ID] = true
					sb.targetQs = append(sb.targetQs, q)
					bySet[name]++
				}
			}
		}
	}
	sb.regStatus = regressionStatus(sb.sets, bySet, sb.feed, scope.Catalog, sb.maxWors)
	return sb, nil
}

// Наборы проверки правок агента (договорённость с оценкой, контур фида). На mock — наборы AGENT_REGRESSION_SETS
// (regression, tune; test — никогда) из LoadQueries источника (data/mock/queries.json), вердикт — разметка позиций.
// На фиде заказчика (у источника есть target_sets или нет исторического журнала) наборы mock не читаются: регрессия —
// наборы «по цели» AGENT_FEED_REGRESSION_SETS (feed_regression = эталоны и ТЗ, feed_tune = соседи и набор понимания;
// оценка — services/eval_target.go: TargetSetQueries, EvalTargetQuery, TargetChanges), закрытый набор агенту
// недоступен. Нет файлов наборов — агент честно пишет «наборы регрессии на фиде не заданы» и правку не допускает;
// появились — берутся со следующего запуска без перезапуска API.

// MsgFeedNoRegression — на фиде нет наборов, на которых агент проверяет правку.
const MsgFeedNoRegression = "Наборы регрессии на фиде не заданы"

// regressionStatus — наборы регрессии песочницы: сколько запросов каждого набора, можно ли допускать правку.
func regressionStatus(sets []string, bySet map[string]int, feed bool, catalog string, maxWorse int) out.AgentRegression {
	st := out.AgentRegression{Sets: append([]string{}, sets...), BySet: map[string]int{}, MaxWorse: maxWorse,
		Feed: feed, CatalogVersion: catalog}
	var parts []string
	for _, name := range sets {
		st.BySet[name] = bySet[name]
		st.Queries += bySet[name]
		parts = append(parts, fmt.Sprintf("%s — %d", name, bySet[name]))
	}
	switch {
	case st.Queries > 0:
		st.Ready = true
		st.Message = fmt.Sprintf("Правка допускается, если на наборах %s ухудшилось не больше %d запросов",
			strings.Join(parts, ", "), maxWorse)
	case st.Feed:
		st.Message = fmt.Sprintf("%s (%s): правки агента не допускаются", MsgFeedNoRegression, strings.Join(sets, ", "))
	default:
		st.Message = "Регрессионный набор пуст — правку не на чем проверить"
	}
	return st
}

// RegressionStatus — наборы регрессии агента на текущем каталоге (GET /api/agent/settings, лента запуска).
func (s *Services) RegressionStatus(ctx context.Context) out.AgentRegression {
	o := s.agent.opts
	scope := s.agentLogScope(ctx)
	bySet := map[string]int{}
	if d, ok := s.source.(targetSetsDirer); ok && d.TargetSetsDir() != "" || !scope.All && !scope.Historical {
		for _, name := range o.FeedRegressionSets {
			if qs, err := s.TargetSetQueries(ctx, name); err == nil {
				bySet[name] = len(qs)
			}
		}
		return regressionStatus(o.FeedRegressionSets, bySet, true, scope.Catalog, o.MaxRegressionWorse)
	}
	if loader, ok := s.source.(queryLoader); ok {
		if all, err := loader.LoadQueries(ctx); err == nil {
			for _, q := range all {
				if q.Set != domain.EvalSetTest && slices.Contains(o.RegressionSets, q.Set) {
					bySet[q.Set]++
				}
			}
		}
	}
	return regressionStatus(o.RegressionSets, bySet, false, scope.Catalog, o.MaxRegressionWorse)
}

// searchOne — запрос через поиск витрины (agentSearchConfig) со снимком правил (nil — действующие правила), без журнала.
func (sb *sandbox) searchOne(ctx context.Context, q domain.EvalQuery, rules *domain.RulesSnapshot) out.EvalQueryResult {
	o := sb.s.evalSearch(ctx, sb.runID, agentSearchConfig, q, rules)
	r := evalScoreQuery(q, o.evalOutcome, sb.cat)
	return r
}

// searchOneSafe — searchOne в рабочей горутине: паника поиска — ошибка запроса, а не падение API.
func (sb *sandbox) searchOneSafe(ctx context.Context, q domain.EvalQuery, rules *domain.RulesSnapshot) (r out.EvalQueryResult) {
	defer func() {
		if p := recover(); p != nil {
			zap.L().Error("песочница: паника поиска", zap.String("query_id", q.ID), zap.Any("panic", p))
			r = out.EvalQueryResult{ID: q.ID, Text: q.Text, Type: q.Type, Freq: q.Freq, Top: []string{}, Grades: []int{},
				Error: fmt.Sprint("паника поиска: ", p)}
		}
	}()
	return sb.searchOne(ctx, q, rules)
}

// searchMany — запросы параллельно (4 потока): ответ поиска на одном запросе от других не зависит.
func (sb *sandbox) searchMany(ctx context.Context, qs []domain.EvalQuery, rules *domain.RulesSnapshot) []out.EvalQueryResult {
	res := make([]out.EvalQueryResult, len(qs))
	var wg sync.WaitGroup
	ch := make(chan int)
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range ch {
				res[i] = sb.searchOneSafe(ctx, qs[i], rules)
			}
		}()
	}
	for i := range qs {
		if ctx.Err() != nil {
			break
		}
		ch <- i
	}
	close(ch)
	wg.Wait()
	return res
}

// baselineFor — выдача регрессионного набора на действующих правилах (один раз за запуск).
func (sb *sandbox) baselineFor(ctx context.Context) (map[string]out.EvalQueryResult, error) {
	sb.once.Do(func() {
		rs := sb.searchMany(ctx, sb.regression, sb.active.snapshot())
		if err := ctx.Err(); err != nil {
			sb.baseErr = err
			return
		}
		sb.baseline = map[string]out.EvalQueryResult{}
		for _, r := range rs {
			sb.baseline[r.ID] = r
		}
	})
	return sb.baseline, sb.baseErr
}

// expectedFamilies — семейства, которые правка обещает поставить первыми (для запросов без разметки).
func (sb *sandbox) expectedFamilies(p RulePatch) map[string]bool {
	res := map[string]bool{}
	if sb.lex == nil {
		return res
	}
	for _, a := range p.Add {
		switch a.Target {
		case domain.AliasTargetFamily:
			res[a.TargetID] = true
		case domain.AliasTargetVendor:
			for f, v := range sb.lex.familyVendor {
				if v == a.TargetID {
					res[f] = true
				}
			}
		case domain.AliasTargetCategory:
			for f, c := range sb.lex.familyCat {
				if c == a.TargetID {
					res[f] = true
				}
			}
		default:
			for _, f := range sb.lex.matchFamilies(strings.Fields(foldText(a.Canonical))) {
				res[f] = true
			}
		}
	}
	return res
}

// evaluate — прогон правки: целевой кластер и регрессия. Ошибка — только отмена контекста.
func (sb *sandbox) evaluate(ctx context.Context, cluster *failedCluster, patch RulePatch) (*patchEval, error) {
	started := time.Now()
	cand := sb.active.applyPatch(patch)
	// Номер снимка = действующий: ключ кэша разборов LLM меняется только у запросов, которые правка
	// задела (нормализованный текст другой), остальные берутся из кэша.
	cand.Version = sb.active.Version
	before, after := sb.active.snapshot(), cand.snapshot()
	ev := &patchEval{RulesVersion: sb.active.Version}
	ev.Target.Queries = []targetQueryEval{}
	ev.Reasons = []string{}
	expected := sb.expectedFamilies(patch)

	// Целевой кластер: запросы из журнала; разметка — только не из test.
	targetIDs := map[string]bool{}
	targetClusters := map[string]bool{}
	var tq []domain.EvalQuery
	var judged []bool
	for _, q := range cluster.Queries {
		eq, ok := sb.byText[foldQuery(q.Text)]
		if ok {
			targetIDs[eq.ID] = true
			if eq.Cluster != "" {
				targetClusters[eq.Cluster] = true
			}
		} else {
			eq = domain.EvalQuery{ID: "target-" + fmt.Sprint(len(tq)), Text: q.Text, Type: "unknown", Freq: q.Freq, Judgments: map[string]int{}}
		}
		tq = append(tq, eq)
		judged = append(judged, ok)
	}
	bres := sb.searchMany(ctx, tq, before)
	ares := sb.searchMany(ctx, tq, after)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for i, q := range tq {
		b, a := bres[i], ares[i]
		t := targetQueryEval{Text: q.Text, Freq: q.Freq, Judged: judged[i], BeforeTop: sb.topFamilies(b), AfterTop: sb.topFamilies(a),
			AfterEmpty: a.Empty}
		if judged[i] {
			t.BeforeFound, t.AfterFound = sb.topRelevant(q, b), sb.topRelevant(q, a)
			if q.Absent && b.FalseRescue != nil && a.FalseRescue != nil && !*b.FalseRescue && *a.FalseRescue {
				ev.FalseRescueNew++
			}
		} else {
			t.BeforeFound, t.AfterFound = topExpected(t.BeforeTop, expected), topExpected(t.AfterTop, expected)
		}
		ev.Target.Before.Total++
		ev.Target.After.Total++
		if t.BeforeFound {
			ev.Target.Before.Found++
		}
		if t.AfterFound {
			ev.Target.After.Found++
		}
		if t.BeforeFound && !t.AfterFound {
			ev.Target.Worse++
		}
		ev.Target.Queries = append(ev.Target.Queries, t)
	}

	if sb.feed {
		if err := sb.regressionTargets(ctx, cluster, after, ev); err != nil {
			return nil, err
		}
		ev.Passed, ev.Reasons = sb.admit(ev)
		ev.Ms = time.Since(started).Milliseconds()
		return ev, nil
	}
	// Регрессия: набор без запросов целевого кластера.
	base, err := sb.baselineFor(ctx)
	if err != nil {
		return nil, err
	}
	var rq []domain.EvalQuery
	for _, q := range sb.regression {
		if targetIDs[q.ID] || q.Cluster != "" && targetClusters[q.Cluster] {
			continue
		}
		rq = append(rq, q)
	}
	cres := sb.searchMany(ctx, rq, after)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	prev := make([]out.EvalQueryResult, 0, len(rq))
	for _, q := range rq {
		prev = append(prev, base[q.ID])
	}
	regs, imps := evalCompare(prev, cres, agentSearchConfig)
	ev.Regression.Total, ev.Regression.Worse, ev.Regression.Better = len(rq), len(regs), len(imps)
	ev.Regression.Sets = sb.sets
	ev.Regression.WorseQueries = []out.EvalChange{}
	for i, r := range imps {
		if i < 5 {
			ev.Regression.BetterQueries = append(ev.Regression.BetterQueries, r)
		}
	}
	for i, r := range regs {
		if slices.Contains(r.Reasons, "lost_exact") || r.Type == domain.QueryExact || r.Type == domain.QuerySKU {
			ev.ExactWorse++
		}
		if slices.Contains(r.Reasons, "new_violation") {
			ev.ConstraintViolationsNew++
		}
		if slices.Contains(r.Reasons, "false_rescue") {
			ev.FalseRescueNew++
		}
		if i < 5 {
			ev.Regression.WorseQueries = append(ev.Regression.WorseQueries, r)
		}
	}
	ev.Passed, ev.Reasons = sb.admit(ev)
	ev.Ms = time.Since(started).Milliseconds()
	return ev, nil
}

// admit — условия допуска правки; причины отказа — по-русски, с цифрами.
func (sb *sandbox) admit(ev *patchEval) (bool, []string) {
	reasons := []string{}
	t := ev.Target
	if t.After.Found <= t.Before.Found {
		reasons = append(reasons, fmt.Sprintf("целевая группа не улучшилась: находилось %d из %d, стало %d из %d",
			t.Before.Found, t.Before.Total, t.After.Found, t.After.Total))
	}
	if t.Worse > 0 {
		reasons = append(reasons, fmt.Sprintf("в целевой группе ухудшилось запросов: %d", t.Worse))
	}
	if ev.Regression.Worse > sb.maxWors {
		ex := ""
		if len(ev.Regression.WorseQueries) > 0 {
			ex = fmt.Sprintf(" (например, «%s»)", ev.Regression.WorseQueries[0].Text)
		}
		reasons = append(reasons, fmt.Sprintf("слишком широко: %d запросов регрессионного набора ухудшились%s",
			ev.Regression.Worse, ex))
	}
	if ev.ExactWorse > 0 {
		reasons = append(reasons, fmt.Sprintf("ухудшились точные запросы: %d", ev.ExactWorse))
	}
	if ev.ConstraintViolationsNew > 0 {
		reasons = append(reasons, fmt.Sprintf("новые нарушения условий лицензии: %d", ev.ConstraintViolationsNew))
	}
	if ev.FalseRescueNew > 0 {
		reasons = append(reasons, fmt.Sprintf("ложное «нашлось» для отсутствующего товара: %d", ev.FalseRescueNew))
	}
	if ev.Regression.Total == 0 {
		msg := "регрессионный набор пуст — правку не на чем проверить"
		if !sb.regStatus.Ready && sb.regStatus.Feed {
			msg = "наборы регрессии на фиде не заданы — правку не на чем проверить"
		}
		reasons = append(reasons, msg)
	}
	return len(reasons) == 0, reasons
}

// topFamilies — первые три семейства выдачи.
func (sb *sandbox) topFamilies(r out.EvalQueryResult) []string {
	res := []string{}
	for _, pid := range r.Top {
		f := sb.cat.familyOf(pid)
		if !slices.Contains(res, f) {
			res = append(res, f)
		}
		if len(res) == 3 {
			break
		}
	}
	return res
}

// topRelevant — первая позиция выдачи релевантна по разметке (оценка ≥ 2).
func (sb *sandbox) topRelevant(q domain.EvalQuery, r out.EvalQueryResult) bool {
	return len(r.Grades) > 0 && r.Grades[0] >= 2
}

// topExpected — первым стоит ожидаемое правкой семейство.
func topExpected(top []string, expected map[string]bool) bool {
	return len(top) > 0 && expected[top[0]]
}

// targetBaseline — выдача наборов «по цели» на действующих правилах (один раз за запуск, 4 потока).
func (sb *sandbox) targetBaseline(ctx context.Context) (map[string]out.EvalQueryResult, error) {
	sb.targetOnce.Do(func() {
		rs := sb.searchTargets(ctx, sb.targetQs, sb.active.snapshot())
		if err := ctx.Err(); err != nil {
			sb.targetErr = err
			return
		}
		sb.targetBase = map[string]out.EvalQueryResult{}
		for _, r := range rs {
			sb.targetBase[r.ID] = r
		}
	})
	return sb.targetBase, sb.targetErr
}

// searchTargets — запросы набора «по цели» через поиск витрины со снимком правил, параллельно (4 потока).
func (sb *sandbox) searchTargets(ctx context.Context, qs []domain.TargetQuery, rules *domain.RulesSnapshot) []out.EvalQueryResult {
	res := make([]out.EvalQueryResult, len(qs))
	var wg sync.WaitGroup
	ch := make(chan int)
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range ch {
				res[i] = sb.searchTargetSafe(ctx, qs[i], rules)
			}
		}()
	}
	for i := range qs {
		if ctx.Err() != nil {
			break
		}
		ch <- i
	}
	close(ch)
	wg.Wait()
	return res
}

func (sb *sandbox) searchTargetSafe(ctx context.Context, q domain.TargetQuery, rules *domain.RulesSnapshot) (r out.EvalQueryResult) {
	defer func() {
		if p := recover(); p != nil {
			zap.L().Error("песочница: паника поиска", zap.String("query_id", q.ID), zap.Any("panic", p))
			r = out.EvalQueryResult{ID: q.ID, Text: q.Text, Top: []string{}, Grades: []int{}, Error: fmt.Sprint("паника поиска: ", p)}
		}
	}()
	r, err := sb.s.EvalTargetQuery(ctx, sb.runID, agentSearchConfig, q, rules)
	if err != nil {
		r = out.EvalQueryResult{ID: q.ID, Text: q.Text, Top: []string{}, Grades: []int{}, Error: err.Error()}
	}
	return r
}

// regressionTargets — регрессия на фиде: наборы «по цели» без запросов целевого кластера, до (базовая линия
// действующих правил) и после правки; ухудшение — потеря цели, первых 4, условия строки или новое нарушение жёстких
// условий (TargetChanges).
func (sb *sandbox) regressionTargets(ctx context.Context, cluster *failedCluster, after *domain.RulesSnapshot, ev *patchEval) error {
	ev.Regression.Sets = sb.sets
	ev.Regression.WorseQueries = []out.EvalChange{}
	if len(sb.targetQs) == 0 {
		return nil // admit: «наборы регрессии на фиде не заданы»
	}
	base, err := sb.targetBaseline(ctx)
	if err != nil {
		return err
	}
	inCluster := map[string]bool{}
	for _, q := range cluster.Queries {
		inCluster[foldQuery(q.Text)] = true
	}
	var rq []domain.TargetQuery
	for _, q := range sb.targetQs {
		if !inCluster[foldQuery(q.Text)] {
			rq = append(rq, q)
		}
	}
	cur := sb.searchTargets(ctx, rq, after)
	if err := ctx.Err(); err != nil {
		return err
	}
	prev := make([]out.EvalQueryResult, 0, len(rq))
	for _, q := range rq {
		prev = append(prev, base[q.ID])
	}
	worse, better := TargetChanges(prev, cur, agentSearchConfig)
	ev.Regression.Total, ev.Regression.Worse, ev.Regression.Better = len(rq), len(worse), len(better)
	for i, r := range better {
		if i < 5 {
			ev.Regression.BetterQueries = append(ev.Regression.BetterQueries, r)
		}
	}
	for i, r := range worse {
		if slices.Contains(r.Reasons, "new_violation") {
			ev.ConstraintViolationsNew++
		}
		if i < 5 {
			ev.Regression.WorseQueries = append(ev.Regression.WorseQueries, r)
		}
	}
	return nil
}
