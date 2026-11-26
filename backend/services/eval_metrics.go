package services

import (
	"encoding/json"
	"math"
	"slices"
	"sort"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Метрики оценки качества — чистые функции над разметкой и выдачей.
//
// Договорённости:
//   - Выдача сгруппирована по семействам: позиция — лучшая лицензия семейства, её оценка — оценка этой
//     лицензии (что покупатель видит первой строкой). Идеальный порядок для NDCG — по семействам: лучшая
//     оценка каждого размеченного семейства, по убыванию (два SKU одного семейства на двух местах
//     выдачи стоять не могут).
//   - Релевантно — оценка ≥2. NDCG — с градуированной пользой 2^g−1.
//   - Recall@50 — относительно размеченного: доля релевантных семейств разметки, попавших в топ-50.

const (
	evalTopK        = 10
	evalRecallK     = 50
	relevantGrade   = 2
	exactGrade      = 3
	piecePurchase   = "поштучно"
	attrPurchaseKey = "purchase"
)

// Группы типов запросов.
var (
	exactTypes       = []string{domain.QueryExact, domain.QuerySKU}
	problemTypes     = []string{domain.QueryTypo, domain.QueryLayout, domain.QuerySlang, domain.QueryAliasGap, domain.QueryLicense, domain.QueryAnalog}
	falseRescueTypes = []string{domain.QueryAbsent, domain.QueryNoise}
	conditionTypes   = []string{domain.QueryLicense, domain.QueryRenewal}
)

// evalItem — позиция выдачи, как её видит оценка.
type evalItem struct {
	ProductID string
	FamilyID  string
	Group     string
}

// evalOutcome — выдача одного запроса одной конфигурации.
type evalOutcome struct {
	Items []evalItem // все позиции ответа (до evalRecallK)
	Total int
	// EmptyMarker — ответ сказал «товара нет в каталоге».
	EmptyMarker bool
	Route       string
	Ms          float64
	Err         error
}

// evalCatalog — то, что оценке нужно знать о позициях: семейство и лицензия.
type evalCatalog struct {
	family  map[string]string
	license map[string]domain.License
	// piece — позиция продаётся поштучно: закрывает любое количество.
	piece map[string]bool
}

// newEvalCatalog — справочник позиций для оценки из снимка каталога.
func newEvalCatalog(products []domain.Product) *evalCatalog {
	c := &evalCatalog{
		family:  make(map[string]string, len(products)),
		license: make(map[string]domain.License, len(products)),
		piece:   map[string]bool{},
	}
	for _, p := range products {
		c.family[p.ID] = p.FamilyID
		c.license[p.ID] = p.License
		var attrs map[string]any
		if len(p.Attributes) > 0 && json.Unmarshal(p.Attributes, &attrs) == nil {
			if v, ok := attrs[attrPurchaseKey].(string); ok && v == piecePurchase {
				c.piece[p.ID] = true
			}
		}
	}
	return c
}

// familyOf — семейство позиции; неизвестная позиция — сама себе семейство.
func (c *evalCatalog) familyOf(productID string) string {
	if f := c.family[productID]; f != "" {
		return f
	}
	return productID
}

// familyGrades — лучшая оценка каждого размеченного семейства.
func (c *evalCatalog) familyGrades(q domain.EvalQuery) map[string]int {
	res := map[string]int{}
	for pid, g := range q.Judgments {
		f := c.familyOf(pid)
		if g > res[f] {
			res[f] = g
		}
	}
	return res
}

// evalGain — польза оценки для NDCG.
func evalGain(g int) float64 { return math.Pow(2, float64(g)) - 1 }

// evalDCG — дисконтированная польза первых k оценок.
func evalDCG(grades []int, k int) float64 {
	sum := 0.0
	for i, g := range grades {
		if i >= k {
			break
		}
		sum += evalGain(g) / math.Log2(float64(i)+2)
	}
	return sum
}

// evalNDCG — NDCG@k; ideal — оценки в идеальном порядке. Идеал без пользы — nil: метрику не на чем считать.
func evalNDCG(grades, ideal []int, k int) *float64 {
	sorted := slices.Clone(ideal)
	sort.Sort(sort.Reverse(sort.IntSlice(sorted)))
	best := evalDCG(sorted, k)
	if best == 0 {
		return nil
	}
	v := evalDCG(grades, k) / best
	return &v
}

// evalMRR — обратный ранг первой релевантной позиции в первых k; нет такой — 0.
func evalMRR(grades []int, k int) float64 {
	for i, g := range grades {
		if i >= k {
			break
		}
		if g >= relevantGrade {
			return 1 / float64(i+1)
		}
	}
	return 0
}

// evalConditions — условия лицензии позиций с оценкой 3: метрики, типы и количества пакетов.
// Позиции «поштучно» количество не задают (закрывают любое); позиция без количества снимает требование.
type evalConditions struct {
	metrics, types map[string]bool
	quantities     map[int]bool
	anyQuantity    bool
}

func (c *evalCatalog) profile(q domain.EvalQuery) *evalConditions {
	p := &evalConditions{metrics: map[string]bool{}, types: map[string]bool{}, quantities: map[int]bool{}}
	n := 0
	for pid, g := range q.Judgments {
		if g < exactGrade {
			continue
		}
		lic, ok := c.license[pid]
		if !ok {
			continue
		}
		n++
		p.metrics[evalDeref(lic.Metric)] = true
		p.types[lic.Type] = true
		switch {
		case lic.Quantity == nil:
			p.anyQuantity = true
		case !c.piece[pid]:
			p.quantities[*lic.Quantity] = true
		}
	}
	if n == 0 {
		return nil
	}
	return p
}

// violates — позиция нужного продукта с не той метрикой, количеством или типом лицензии.
func (c *evalCatalog) violates(p *evalConditions, productID string) bool {
	lic, ok := c.license[productID]
	if !ok {
		return false
	}
	if !p.metrics[evalDeref(lic.Metric)] || !p.types[lic.Type] {
		return true
	}
	if p.anyQuantity || len(p.quantities) == 0 || c.piece[productID] || lic.Quantity == nil {
		return false
	}
	return !p.quantities[*lic.Quantity]
}

// evalScoreQuery — метрики одного запроса по его выдаче.
func evalScoreQuery(q domain.EvalQuery, o evalOutcome, cat *evalCatalog) out.EvalQueryResult {
	r := out.EvalQueryResult{
		ID: q.ID, Text: q.Text, Type: q.Type, Freq: q.Freq, Route: o.Route, Ms: o.Ms,
		Top: []string{}, Grades: []int{}, Total: o.Total,
	}
	if o.Err != nil {
		r.Error = o.Err.Error()
		return r
	}
	r.Empty = len(o.Items) == 0
	grades := make([]int, len(o.Items))
	for i, it := range o.Items {
		grades[i] = q.Judgments[it.ProductID]
		if i < evalTopK {
			r.Top = append(r.Top, it.ProductID)
			r.Grades = append(r.Grades, grades[i])
		}
	}
	fam := cat.familyGrades(q)
	ideal := make([]int, 0, len(fam))
	relevantFamilies := map[string]bool{}
	for f, g := range fam {
		ideal = append(ideal, g)
		if g >= relevantGrade {
			relevantFamilies[f] = true
		}
	}
	r.NDCG10 = evalNDCG(grades, ideal, evalTopK)
	r.HasRelevant = evalMRR(grades, evalTopK) > 0
	if len(relevantFamilies) > 0 {
		mrr := evalMRR(grades, evalTopK)
		r.MRR10 = &mrr
		found := 0
		seen := map[string]bool{}
		for i, it := range o.Items {
			f := it.FamilyID
			if f == "" {
				f = cat.familyOf(it.ProductID)
			}
			if i < evalRecallK && relevantFamilies[f] && !seen[f] {
				seen[f] = true
				found++
			}
		}
		rec := float64(found) / float64(len(relevantFamilies))
		r.Recall50 = &rec
	}
	if slices.Contains(exactTypes, q.Type) {
		hit := len(grades) > 0 && grades[0] >= exactGrade
		r.ExactHit = &hit
	}
	if slices.Contains(problemTypes, q.Type) && len(relevantFamilies) > 0 {
		r.Rescued = &r.HasRelevant
	}
	if slices.Contains(falseRescueTypes, q.Type) {
		labelled := o.EmptyMarker
		for _, it := range o.Items {
			labelled = labelled || it.Group == domain.GroupAnalog
		}
		fr := !r.Empty && !labelled
		r.FalseRescue = &fr
	}
	if slices.Contains(conditionTypes, q.Type) {
		if p := cat.profile(q); p != nil {
			for i, it := range o.Items {
				if i >= evalTopK {
					break
				}
				// Только позиции нужного продукта, не ставшие подходящей альтернативой (оценка ≤1):
				// чужие продукты штрафует NDCG.
				if _, judged := fam[cat.familyOf(it.ProductID)]; judged && grades[i] < relevantGrade && cat.violates(p, it.ProductID) {
					r.Violations++
				}
			}
		}
	}
	return r
}

// evalAggregate — сводные метрики по результатам запросов.
func evalAggregate(rs []out.EvalQueryResult) out.EvalMetrics {
	m := out.EvalMetrics{}
	var (
		empty, emptyW, weight                   float64
		noRel, relTop, ndcg, mrr, rec           evalMean
		exact, rescued, falseRescue, violations evalMean
		latency                                 []float64
		byRoute                                 = map[string][]float64{}
	)
	for _, r := range rs {
		if r.Error != "" {
			m.Errors++
			continue
		}
		m.Queries++
		w := float64(max(r.Freq, 1))
		weight += w
		if r.Empty {
			empty++
			emptyW += w
		}
		if r.MRR10 != nil {
			noRel.addBool(!r.Empty && !r.HasRelevant)
			relTop.addBool(r.HasRelevant)
			mrr.add(*r.MRR10)
		}
		if r.NDCG10 != nil {
			ndcg.add(*r.NDCG10)
		}
		if r.Recall50 != nil {
			rec.add(*r.Recall50)
		}
		if r.ExactHit != nil {
			exact.addBool(*r.ExactHit)
		}
		if r.Rescued != nil {
			rescued.addBool(*r.Rescued)
		}
		if r.FalseRescue != nil {
			falseRescue.addBool(*r.FalseRescue)
		}
		if slices.Contains(conditionTypes, r.Type) {
			violations.addBool(r.Violations > 0)
			m.ViolationItems += r.Violations
		}
		latency = append(latency, r.Ms)
		route := r.Route
		if route == "" {
			route = "?"
		}
		byRoute[route] = append(byRoute[route], r.Ms)
	}
	if m.Queries > 0 {
		m.EmptyRate = evalPtr(evalRound4(empty / float64(m.Queries)))
		m.EmptyRateWeighted = evalPtr(evalRound4(emptyW / weight))
	}
	m.NoRelevantTop10 = noRel.value()
	m.RelevantTop10 = relTop.value()
	m.NDCG10 = ndcg.value()
	m.MRR10 = mrr.value()
	m.Recall50 = rec.value()
	m.ExactAccuracy = exact.value()
	m.RescuedRate = rescued.value()
	m.FalseRescueRate = falseRescue.value()
	m.ViolationRate = violations.value()
	m.Latency = evalLatencyOf(latency)
	if len(byRoute) > 0 {
		m.ByRoute = make(map[string]out.EvalLatency, len(byRoute))
		for route, ms := range byRoute {
			m.ByRoute[route] = evalLatencyOf(ms)
		}
	}
	return m
}

// evalMean — среднее с учётом пустого множества.
type evalMean struct {
	sum float64
	n   int
}

func (m *evalMean) add(v float64) { m.sum += v; m.n++ }

func (m *evalMean) addBool(b bool) {
	if b {
		m.add(1)
		return
	}
	m.add(0)
}

// value — среднее, округлённое до 4 знаков; пусто — nil.
func (m *evalMean) value() *float64 {
	if m.n == 0 {
		return nil
	}
	return evalPtr(evalRound4(m.sum / float64(m.n)))
}

// evalLatencyOf — P50 и P95 по методу ближайшего ранга.
func evalLatencyOf(ms []float64) out.EvalLatency {
	if len(ms) == 0 {
		return out.EvalLatency{}
	}
	s := slices.Clone(ms)
	slices.Sort(s)
	return out.EvalLatency{Count: len(s), P50: evalPercentile(s, 50), P95: evalPercentile(s, 95)}
}

// evalPercentile — p-й процентиль отсортированного ряда (ближайший ранг).
func evalPercentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(math.Ceil(p / 100 * float64(len(sorted))))
	rank = min(max(rank, 1), len(sorted))
	return math.Round(sorted[rank-1]*100) / 100
}

// evalByType — сводные метрики по каждому типу запросов.
func evalByType(rs []out.EvalQueryResult) map[string]out.EvalMetrics {
	groups := map[string][]out.EvalQueryResult{}
	for _, r := range rs {
		groups[r.Type] = append(groups[r.Type], r)
	}
	res := make(map[string]out.EvalMetrics, len(groups))
	for t, g := range groups {
		m := evalAggregate(g)
		m.ByRoute = nil
		res[t] = m
	}
	return res
}

// ndcgChangeThreshold — изменение NDCG@10 запроса, которое считается ухудшением/улучшением.
const ndcgChangeThreshold = 0.05

// evalCompare — ухудшения и улучшения cur относительно prev (по id запроса).
func evalCompare(prev, cur []out.EvalQueryResult, against string) (regs, imps []out.EvalChange) {
	regs, imps = []out.EvalChange{}, []out.EvalChange{}
	before := make(map[string]out.EvalQueryResult, len(prev))
	for _, r := range prev {
		before[r.ID] = r
	}
	for _, c := range cur {
		p, ok := before[c.ID]
		if !ok || p.Error != "" || c.Error != "" {
			continue
		}
		delta := 0.0
		if p.NDCG10 != nil && c.NDCG10 != nil {
			delta = evalRound4(*c.NDCG10 - *p.NDCG10)
		}
		var worse, better []string
		if delta <= -ndcgChangeThreshold {
			worse = append(worse, "ndcg")
		}
		if delta >= ndcgChangeThreshold {
			better = append(better, "ndcg")
		}
		hasRel := p.MRR10 != nil
		if hasRel && p.HasRelevant && !c.HasRelevant {
			worse = append(worse, "lost_relevant")
		}
		if hasRel && !p.HasRelevant && c.HasRelevant {
			better = append(better, "gained_relevant")
		}
		// Пустая выдача — ухудшение там, где в разметке есть что показать.
		if hasRel && !p.Empty && c.Empty {
			worse = append(worse, "became_empty")
		}
		if hasRel && p.Empty && !c.Empty {
			better = append(better, "no_longer_empty")
		}
		if p.ExactHit != nil && c.ExactHit != nil && *p.ExactHit != *c.ExactHit {
			if *c.ExactHit {
				better = append(better, "gained_exact")
			} else {
				worse = append(worse, "lost_exact")
			}
		}
		if c.Violations > p.Violations {
			worse = append(worse, "new_violation")
		}
		if c.Violations < p.Violations {
			better = append(better, "fixed_violation")
		}
		if p.FalseRescue != nil && c.FalseRescue != nil && *p.FalseRescue != *c.FalseRescue {
			if *c.FalseRescue {
				worse = append(worse, "false_rescue")
			} else {
				better = append(better, "false_rescue_fixed")
			}
		}
		ch := out.EvalChange{QueryID: c.ID, Text: c.Text, Type: c.Type, Freq: c.Freq, Against: against,
			NDCGFrom: p.NDCG10, NDCGTo: c.NDCG10, NDCGDelta: delta}
		if len(worse) > 0 {
			ch.Reasons = worse
			regs = append(regs, ch)
		}
		if len(better) > 0 {
			ch.Reasons = better
			imps = append(imps, ch)
		}
	}
	// Сначала самые частые и самые сильные изменения.
	order := func(list []out.EvalChange) {
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].Freq != list[j].Freq {
				return list[i].Freq > list[j].Freq
			}
			return math.Abs(list[i].NDCGDelta) > math.Abs(list[j].NDCGDelta)
		})
	}
	order(regs)
	order(imps)
	return regs, imps
}

func evalPtr[T any](v T) *T { return &v }

func evalDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// evalRound4 — до 4 знаков.
func evalRound4(v float64) float64 { return math.Round(v*1e4) / 1e4 }
