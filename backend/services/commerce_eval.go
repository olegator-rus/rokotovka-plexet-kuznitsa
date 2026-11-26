package services

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"

	"go.uber.org/zap"
)

// Коммерческая лестница C0–C3: набор p2 прогоняется через тот же
// сервис поиска с флагами контура коммерции на запрос (WithFeatures): C0 = B4, C1 = C0 + оборудование, C2 = C1 + блок
// допродажи, C3 = C2 + политика маржи. Рядом — прогоны с одним изменённым компонентом (лестница не разделяет вклад
// взаимосвязанных функций). Метрики блока — на companion_pairs.json (EvaluateCompanionPairs трека допродажи);
// разбор единиц — по шагу трассы hardware против expect.filters набора; индекс маржи — по синтетической марже.
// Непосчитанное — null с пометкой в notes, а не ноль.

// commerceCfg — конфигурация лестницы.
type commerceCfg struct {
	name     string
	features Features
	base     string // для прогона с одним изменением — от какой конфигурации
	changed  string // какой компонент изменён (+ включён / − выключен)
	// agent — C4: принятые правки SearchOps Commerce (действующая версия связей и веса маржи по категориям).
	// Остальные конфигурации — связи каталога (импорт) и только общий вес маржи.
	agent bool
}

var commerceLadder = []commerceCfg{
	{name: "C0"},
	{name: "C1", features: Features{Hardware: true}},
	{name: "C2", features: Features{Hardware: true, Companions: true}},
	{name: "C3", features: Features{Hardware: true, Companions: true, MarginPolicy: true}},
	{name: "C4", features: Features{Hardware: true, Companions: true, MarginPolicy: true}, agent: true},
}

var commerceSingles = []commerceCfg{
	{name: "C0+companions", features: Features{Companions: true}, base: "C0", changed: "+companions"},
	{name: "C0+margin", features: Features{MarginPolicy: true}, base: "C0", changed: "+margin"},
	{name: "C3-hardware", features: Features{Companions: true, MarginPolicy: true}, base: "C3", changed: "-hardware"},
	{name: "C3-companions", features: Features{Hardware: true, MarginPolicy: true}, base: "C3", changed: "-companions"},
}

// p2Expect — ожидания запроса набора p2 (queries.json → expect).
type p2Expect struct {
	AnchorFamily string `json:"anchor_family"`
	Filters      []struct {
		Key   string `json:"key"`
		Op    string `json:"op"`
		Value any    `json:"value"` // число, строка перечисления («SSD», «без ОС») или bool
	} `json:"filters"`
}

// CommerceEvaluation — GET /api/evaluations/commerce?set=p2: последний прогон лестницы.
func (s *Services) CommerceEvaluation(ctx context.Context, set string) (*out.CommerceEval, error) {
	if set == "" {
		set = domain.EvalSetP2
	}
	if set != domain.EvalSetP2 {
		return nil, errs.NewValidationError(errs.MsgInvalidQuery).WithDetails(errs.FieldError{Field: "set",
			Code: errs.CodeInvalidFormat, Message: "Коммерческая лестница считается на наборе p2"})
	}
	empty := &out.CommerceEval{Set: set, Ladder: []out.CommerceConfig{}, SingleChanges: []out.CommerceConfig{},
		Notes: []string{"прогона ещё не было: make eval SET=p2"}}
	if !s.repos.Enabled() {
		return empty, nil
	}
	row, err := s.repos.LatestCommerceEvaluation(ctx, set)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return empty, nil
	}
	var res out.CommerceEval
	if err := json.Unmarshal(row.Metrics, &res); err != nil {
		return nil, fmt.Errorf("коммерческая лестница %s: %w", row.RunID, err)
	}
	return &res, nil
}

// RunCommerceEvaluation — `make eval SET=p2`: лестница C0–C3 и прогоны с одним изменением; persist — записать.
func (s *Services) RunCommerceEvaluation(ctx context.Context, persist bool) (*out.CommerceEval, error) {
	started := time.Now()
	queries, err := s.evalQueries(ctx, domain.EvalSetP2, nil)
	if err != nil {
		return nil, err
	}
	cat, err := s.evalCatalog(ctx)
	if err != nil {
		return nil, err
	}
	pairs := s.companionPairs(ctx)
	res := &out.CommerceEval{RunID: newEvalRunID(started), Set: domain.EvalSetP2, SetHash: setHash(queries), Queries: len(queries),
		Pairs: len(pairs), Commit: buildCommit(), StartedAt: started.UTC(), Synthetic: s.marginSynthetic(),
		Ladder: []out.CommerceConfig{}, SingleChanges: []out.CommerceConfig{}, Notes: []string{}}
	marginW, marginOK := 0.0, false
	var marginCats map[string]float64
	if pol, err := s.activeMarginPolicy(ctx); err == nil {
		marginW, marginOK, marginCats = pol.Weight, true, pol.Categories
	}
	res.MarginWeight = marginW
	// C4 = C3 + принятые правки агента: C0–C3 идут на связях каталога (песочница справочников),
	// C4 — на действующей версии связей; веса маржи по категориям (MARGIN_OPPORTUNITY) — только в C4.
	baseRel, relBase, relActive := s.commerceBaseline(ctx, &res.Notes)
	pct := map[string]float64{}
	if cs := s.catalog.Load(); cs != nil && cs.lex != nil {
		pct = marginPercentiles(cs.lex) // та же нормировка, что у политики маржи (margin.go)
	}
	ready := map[string]bool{}
	byName := map[string]*out.CommerceConfig{}
	var prev []out.EvalQueryResult
	run := func(c commerceCfg) (*out.CommerceConfig, []out.EvalQueryResult, error) {
		cfgStart := time.Now()
		cctx := WithFeatures(ctx, c.features)
		opts := SearchOptions{NoLog: true}
		if c.features.MarginPolicy {
			opts = MarginC3(opts, marginW)
		}
		relVersion := relActive
		if c.agent {
			if c.features.MarginPolicy && len(marginCats) > 0 {
				opts.Margin.Categories = marginCats
			}
		} else if baseRel != nil {
			cctx, relVersion = withCommerceOverride(cctx, baseRel), relBase
		}
		cc := &out.CommerceConfig{Config: c.name, Base: c.base, Changed: c.changed,
			Features: out.Features(s.featuresFor(cctx)), Regressions: []out.EvalChange{}, RelationsVersion: relVersion}
		if c.agent && opts.Margin != nil {
			cc.MarginCategories = opts.Margin.Categories
		}
		per := make([]out.EvalQueryResult, 0, len(queries))
		var unitsOK, unitsN, mpnOK, mpnN, exOK, exN int
		var mIdx, mDen float64
		hwSteps := 0
		for _, q := range queries {
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}
			sr := in.SearchRequest{Query: q.Text, Config: domain.ConfigB4, Limit: evalSearchLimit, SessionID: evalRequestPrefix + res.RunID}
			if q.ClientID != nil {
				sr.ClientID = *q.ClientID
			}
			t := time.Now()
			resp, err := s.SearchWith(cctx, evalRequestID(res.RunID, strings.ReplaceAll(c.name, "-", "m"), q.ID), sr, opts)
			o := evalOutcome{Ms: float64(time.Since(t).Microseconds()) / 1000, Err: err}
			var steps map[string]map[string]any
			if err == nil {
				o.Total, o.EmptyMarker = resp.Total, resp.Empty != nil
				if resp.Trace != nil {
					o.Route = resp.Trace.Route
					cc.RulesVersion, cc.CatalogVersion = resp.Trace.RulesVersion, resp.Trace.CatalogVersion
					steps = traceSteps(resp.Trace)
				}
				for _, it := range resp.Items {
					o.Items = append(o.Items, evalItem{ProductID: it.ProductID, FamilyID: it.FamilyID, Group: it.Group})
				}
			}
			r := evalScoreQuery(q, o, cat)
			per = append(per, r)
			if steps["hardware"] != nil {
				hwSteps++
			}
			// Точность первого места: запросы, где в разметке есть точное совпадение (оценка 3).
			if hasGrade(q, 3) && r.Error == "" {
				top1 := len(r.Grades) > 0 && r.Grades[0] == 3
				exN++
				if top1 {
					exOK++
				}
				if q.Type == "p2_mpn" {
					mpnN++
					if top1 {
						mpnOK++
					}
				}
			}
			var ex p2Expect
			// p2_license: «16 ядер» у ПО — количество лицензий, шаг оборудования его намеренно не разбирает.
			if q.Type != "p2_license" && len(q.Expect) > 0 && json.Unmarshal(q.Expect, &ex) == nil &&
				len(ex.Filters) > 0 && c.features.Hardware {
				unitsN++
				if unitsMatch(steps["hardware"], ex) {
					unitsOK++
				}
			}
			if v, ok := shownMarginIndex(r.Top, pct); ok {
				mIdx += v
				mDen++
			}
		}
		cc.Metrics.NDCG10 = evalAggregate(per).NDCG10
		cc.Metrics.ExactTop1 = ratio(exOK, exN)
		cc.Metrics.MPNTop1 = ratio(mpnOK, mpnN)
		if c.features.Hardware && hwSteps > 0 {
			cc.Metrics.UnitsParseAcc = ratio(unitsOK, unitsN)
			ready["hardware"] = true
		}
		if mDen > 0 {
			v := evalRound4(mIdx / mDen)
			cc.Metrics.MarginIndex = &v
		}
		// Шаг margin вырезается из ответа (числа маржи — только в трассе бэк-офиса), поэтому готовность политики —
		// по флагу и прочитанной действующей политике.
		if c.features.MarginPolicy && s.featuresFor(cctx).MarginPolicy && marginOK {
			ready["margin"] = true
		}
		if s.featuresFor(cctx).Companions && len(pairs) > 0 {
			sc := s.EvaluateCompanionPairs(cctx, pairs)
			cc.Metrics.CompatViolations = &sc.CompatViolations
			cc.Metrics.RequiredRecall, cc.Metrics.Coverage = sc.RequiredRecall, sc.Coverage
			cc.Block = &sc
			ready["companions"] = true
		}
		cc.Metrics.Errors = evalAggregate(per).Errors
		cc.DurationMs = time.Since(cfgStart).Milliseconds()
		return cc, per, nil
	}
	for i, c := range commerceLadder {
		cc, per, err := run(c)
		if err != nil {
			return nil, err
		}
		if i > 0 {
			cc.Against = commerceLadder[i-1].name
			regs, _ := evalCompare(prev, per, cc.Against)
			cc.Regressions = evalHead(regs, 10)
		}
		prev = per
		res.Ladder = append(res.Ladder, *cc)
		byName[c.name] = &res.Ladder[len(res.Ladder)-1]
	}
	for _, c := range commerceSingles {
		cc, _, err := run(c)
		if err != nil {
			return nil, err
		}
		res.SingleChanges = append(res.SingleChanges, *cc)
	}
	res.Ready = out.CommerceReady{Hardware: ready["hardware"], Companions: ready["companions"], Margin: ready["margin"]}
	if !ready["hardware"] {
		res.Notes = append(res.Notes, "оборудование: не готово — шага hardware в трассе нет, точность разбора единиц не считается")
	}
	if !ready["companions"] {
		res.Notes = append(res.Notes, "блок допродажи: не готово или нет companion_pairs.json — метрики блока не считаются")
	}
	if !ready["margin"] {
		res.Notes = append(res.Notes, "политика маржи: не готово — флаг выключен или политика не прочитана")
	} else if sameWeight(marginW, marginBaseWeight) {
		res.Notes = append(res.Notes, fmt.Sprintf("политика маржи: действующий вес %.2f равен весу контура поиска — C3 совпадает с C2 по построению", marginW))
	}
	if baseRel == nil && len(marginCats) == 0 {
		res.Notes = append(res.Notes, "C4: принятых правок SearchOps Commerce нет (связи — как в каталоге, весов маржи по категориям нет) — C4 совпадает с C3 по построению")
	}
	res.Notes = append(res.Notes, "точность блока по LLM-судье: не готово (судья другого семейства на парах блока не прогонялся)")
	if res.Synthetic {
		res.Notes = append(res.Notes, "маржа в данных синтетическая: индекс показанной маржи — модель, не факт")
	}
	res.DurationMs = time.Since(started).Milliseconds()
	res.Repro = fmt.Sprintf("set=%s hash=%s n=%d pairs=%d commit=%s", res.Set, res.SetHash, res.Queries, res.Pairs, res.Commit)
	zap.L().Info("коммерческая лестница: прогон", zap.String("run_id", res.RunID), zap.Int("queries", len(queries)),
		zap.Int64("ms", res.DurationMs))
	if persist && s.repos.Enabled() {
		raw, _ := json.Marshal(res)
		row := &domain.Evaluation{RunID: res.RunID, SetHash: res.SetHash, Queries: res.Queries, Config: domain.EvalSetP2,
			Commit: res.Commit, Metrics: raw, DurationMs: int(res.DurationMs)}
		if len(res.Ladder) > 0 {
			row.RulesVersion, row.CatalogVersion = res.Ladder[0].RulesVersion, res.Ladder[0].CatalogVersion
		}
		if err := s.repos.SaveCommerceEvaluation(ctx, row); err != nil {
			return res, fmt.Errorf("коммерческая лестница: результат не записан: %w", err)
		}
	}
	return res, nil
}

func evalHead(list []out.EvalChange, n int) []out.EvalChange {
	if len(list) > n {
		return list[:n]
	}
	if list == nil {
		return []out.EvalChange{}
	}
	return list
}

func hasGrade(q domain.EvalQuery, g int) bool {
	for _, v := range q.Judgments {
		if v == g {
			return true
		}
	}
	return false
}

// traceSteps — шаги трассы по имени (как JSON-объекты).
func traceSteps(tr *domain.Trace) map[string]map[string]any {
	res := map[string]map[string]any{}
	for _, st := range tr.Steps {
		b, err := json.Marshal(st)
		if err != nil {
			continue
		}
		var m map[string]any
		if json.Unmarshal(b, &m) != nil {
			continue
		}
		if name, _ := m["name"].(string); name != "" {
			res[name] = m
		}
	}
	return res
}

// unitsMatch — шаг hardware разобрал ровно те числовые условия, что ожидает набор (ключ, операция, значение).
func unitsMatch(step map[string]any, ex p2Expect) bool {
	if step == nil {
		return false
	}
	units, _ := step["units"].([]any)
	got := map[string]bool{}
	for _, u := range units {
		m, _ := u.(map[string]any)
		got[fmt.Sprintf("%v|%v|%s", m["key"], m["op"], unitValueKey(m["value"]))] = true
	}
	if len(got) != len(ex.Filters) {
		return false
	}
	for _, f := range ex.Filters {
		if !got[fmt.Sprintf("%s|%s|%s", f.Key, f.Op, unitValueKey(f.Value))] {
			return false
		}
	}
	return true
}

// unitValueKey — значение условия для сравнения: числа через %g (32 и 32.0 равны), строки без регистра и пробелов по краям.
func unitValueKey(v any) string {
	switch x := v.(type) {
	case float64:
		return fmt.Sprintf("%g", x)
	case int:
		return fmt.Sprintf("%g", float64(x))
	case string:
		return strings.ToLower(strings.TrimSpace(x))
	default:
		return fmt.Sprintf("%v", x)
	}
}

// shownMarginIndex — индекс показанной маржи первых 10: нормированная маржа с весом 1/log2(i+2), как в симуляции
// маржи (marginIndexOf); позиции с неизвестной маржой не входят.
func shownMarginIndex(top []string, pct map[string]float64) (float64, bool) {
	num, den := 0.0, 0.0
	for i, id := range top {
		if i >= evalTopK {
			break
		}
		v, ok := pct[id]
		if !ok {
			continue
		}
		d := 1 / math.Log2(float64(i)+2)
		num += d * v
		den += d
	}
	if den == 0 {
		return 0, false
	}
	return num / den, true
}

// FormatCommerceTable — лестница таблицей для `make eval SET=p2`.
func FormatCommerceTable(r *out.CommerceEval) string {
	var b strings.Builder
	f := func(v *float64, pct bool) string {
		if v == nil {
			return "—"
		}
		if pct {
			return fmt.Sprintf("%.1f%%", *v*100)
		}
		return fmt.Sprintf("%.4f", *v)
	}
	iv := func(v *int) string {
		if v == nil {
			return "—"
		}
		return fmt.Sprint(*v)
	}
	fmt.Fprintf(&b, "Коммерческая лестница · набор %s · %d запросов · %d пар · %s\n\n", r.Set, r.Queries, r.Pairs, r.RunID)
	b.WriteString("| Конфигурация | NDCG@10 | Точные топ-1 | Парт-номер топ-1 | Разбор единиц | Нарушения совм. | Полнота обяз. | Покрытие блока | Индекс маржи |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|\n")
	for _, rows := range [][]out.CommerceConfig{r.Ladder, r.SingleChanges} {
		for _, c := range rows {
			m := c.Metrics
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s | %s |\n", c.Config, f(m.NDCG10, false), f(m.ExactTop1, true),
				f(m.MPNTop1, true), f(m.UnitsParseAcc, true), iv(m.CompatViolations), f(m.RequiredRecall, true), f(m.Coverage, true),
				f(m.MarginIndex, false))
		}
	}
	for _, n := range r.Notes {
		b.WriteString("\n- " + n)
	}
	b.WriteString("\n\n" + r.Repro + "\n")
	return b.String()
}

// commerceBaseline — справочники контура коммерции со связями каталога (для C0–C3), если действующая версия связей
// отличается от каталога (принятые relation_patch); nil — отличий нет или цепочка не прочитана (пометка в notes).
// Ограничение (как у песочницы агента): словарь поиска (аналоги, «С этим покупают») не подменяется — правка
// substitute влияет на основную выдачу всех ступеней одинаково.
func (s *Services) commerceBaseline(ctx context.Context, notes *[]string) (*commerceCatalog, int, int) {
	cs := s.catalog.Load()
	if cs == nil || cs.commerce == nil || s.relStore() == nil {
		return nil, 0, 0
	}
	ch, err := s.relationChain(ctx)
	if err != nil {
		*notes = append(*notes, "C4: цепочка версий связей не прочитана — все ступени на действующих связях")
		return nil, 0, 0
	}
	if !ch.Materialized || relationsFingerprint(ch.Active) == relationsFingerprint(ch.Base) {
		return nil, ch.Version, ch.Version
	}
	base := 0
	if n := len(ch.Versions); n > 0 {
		base = ch.Versions[n-1].Version
	}
	*notes = append(*notes, fmt.Sprintf("C0–C3 — связи каталога (v%d), C4 — действующая версия связей v%d; словарь поиска "+
		"(аналоги) в песочнице не подменяется", base, ch.Version))
	return cs.commerce.withRelations(relationsOfDocs(ch.Base)), base, ch.Version
}
