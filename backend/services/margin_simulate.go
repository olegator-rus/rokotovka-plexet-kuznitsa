package services

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"

	"go.uber.org/zap"
)

// Симуляция рычага маржи (POST /api/margin/simulate): размеченный набор (tune по умолчанию;
// test заморожен и для подбора не используется) один раз прогоняется через поиск витрины на весе контура поиска, выдача
// до политики запоминается; положения рычага применяются к ней тем же кодом, что в поиске (marginReorder), — так
// каждое следующее движение рычага считается за миллисекунды. Базовая выдача кэшируется до смены каталога или
// версии правил.

const (
	// marginSimWorkers — параллельных поисков при сборе базовой выдачи.
	marginSimWorkers = 4
	// marginSimTimeout — сбор базовой выдачи целиком (идёт в фоне, запрос ждёт не дольше своего срока).
	marginSimTimeout = 5 * time.Minute
	// marginAffectedWorst — сколько худших по потере запросов показывать сверх запросов со сменой топ-1/топ-3.
	marginAffectedWorst = 5
	// marginAffectedMax — потолок списка затронутых запросов на положение.
	marginAffectedMax = 40
	// marginShowTop — позиций в «было / стало».
	marginShowTop = 10
)

// MsgMarginSimRunning — базовая выдача ещё собирается.
const MsgMarginSimRunning = "Базовая выдача набора ещё считается — повторите через несколько секунд"

// marginSimQuery — запрос набора и его выдача до политики маржи.
type marginSimQuery struct {
	q     domain.EvalQuery
	in    marginInput
	ideal []int
}

// marginCapture — базовая выдача набора.
type marginCapture struct {
	key     string
	set     string
	setHash string
	queries []marginSimQuery
	errors  int
	titles  map[string]string
	catalog string
	rules   int
}

// marginSimState — фоновый сбор базовой выдачи и кэш.
type marginSimState struct {
	mu       sync.Mutex
	running  bool
	done     int
	total    int
	captures map[string]*marginCapture
	pending  map[string]chan struct{}
	errs     map[string]error
	last     map[string]*out.MarginSimulation // ключ → результат по всем положениям
}

func (st *marginSimState) lastResult(key string) *out.MarginSimulation {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.last[key]
}

// marginSimKey — ключ кэша набора tune: каталог и версия правил.
func (s *Services) marginSimKey() string { return s.marginSimKeyFor(domain.EvalSetTune) }

func (s *Services) marginSimKeyFor(set string) string {
	st := s.catalog.Load()
	if st == nil {
		return set
	}
	return fmt.Sprintf("%s|%s|%d", set, st.version, st.rulesVersion)
}

// MarginSimulationStatus — GET /api/margin/simulate/status.
func (s *Services) MarginSimulationStatus() out.MarginSimStatus {
	st := &s.margin().sim
	st.mu.Lock()
	defer st.mu.Unlock()
	return out.MarginSimStatus{Running: st.running, Done: st.done, Total: st.total}
}

// SimulateMargin — POST /api/margin/simulate.
func (s *Services) SimulateMargin(ctx context.Context, req in.MarginSimulateRequest) (*out.MarginSimulation, error) {
	started := time.Now()
	set := strings.ToLower(strings.TrimSpace(req.Set))
	if set == "" {
		set = domain.EvalSetTune
	}
	if !slices.Contains(domain.EvalSets, set) {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(errs.FieldError{
			Field: "set", Code: errs.CodeInvalidFormat, Message: "Набор — tune, test, regression или demo"})
	}
	positions := req.Positions
	all := len(positions) == 0
	if all {
		positions = MarginPositions()
	}
	for _, w := range positions {
		if !validMarginWeight(w) {
			return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(errs.FieldError{
				Field: "positions", Code: errs.CodeInvalidFormat, Message: MsgMarginWeight})
		}
	}
	key := s.marginSimKeyFor(set)
	sim := &s.margin().sim
	if all {
		if r := sim.lastResult(key); r != nil {
			c := *r
			c.Cached = true
			c.DurationMs = time.Since(started).Milliseconds()
			return &c, nil
		}
	}
	capture, cached, err := s.marginCaptureFor(ctx, set, key)
	if err != nil {
		return nil, err
	}
	pol, _ := s.activeMarginPolicy(ctx)
	res := marginSimulate(capture, MarginPositions(), marginEpsilon)
	res.CurrentWeight, res.PolicyVersion, res.Synthetic = pol.Weight, pol.Version, s.marginSynthetic()
	res.Cached = cached
	sim.mu.Lock()
	if sim.last == nil {
		sim.last = map[string]*out.MarginSimulation{}
	}
	c := *res
	sim.last[key] = &c
	sim.mu.Unlock()
	if !all {
		items := res.Items[:0:0]
		for _, it := range res.Items {
			if slices.ContainsFunc(positions, func(w float64) bool { return sameWeight(w, it.Weight) }) {
				items = append(items, it)
			}
		}
		res.Items = items
	}
	res.DurationMs = time.Since(started).Milliseconds()
	return res, nil
}

// marginCaptureFor — базовая выдача набора из кэша или фоновым сбором; запрос ждёт не дольше своего срока.
func (s *Services) marginCaptureFor(ctx context.Context, set, key string) (*marginCapture, bool, error) {
	sim := &s.margin().sim
	sim.mu.Lock()
	if c := sim.captures[key]; c != nil {
		sim.mu.Unlock()
		return c, true, nil
	}
	ch, running := sim.pending[key]
	if !running {
		if sim.pending == nil {
			sim.pending, sim.captures, sim.errs = map[string]chan struct{}{}, map[string]*marginCapture{}, map[string]error{}
		}
		ch = make(chan struct{})
		sim.pending[key] = ch
		delete(sim.errs, key)
		go s.runMarginCapture(set, key, ch)
	}
	sim.mu.Unlock()
	select {
	case <-ch:
	case <-ctx.Done():
		return nil, false, errs.NewUnavailableError(MsgMarginSimRunning)
	}
	sim.mu.Lock()
	defer sim.mu.Unlock()
	if err := sim.errs[key]; err != nil {
		return nil, false, err
	}
	return sim.captures[key], false, nil
}

func (s *Services) runMarginCapture(set, key string, ch chan struct{}) {
	sim := &s.margin().sim
	ctx, cancel := context.WithTimeout(context.Background(), marginSimTimeout)
	defer cancel()
	c, err := s.captureMarginBaseline(ctx, set, key)
	sim.mu.Lock()
	delete(sim.pending, key)
	sim.running = false
	if err != nil {
		sim.errs[key] = err
		zap.L().Warn("маржа: базовая выдача не собрана", zap.String("set", set), zap.Error(err))
	} else {
		sim.captures = map[string]*marginCapture{key: c} // держим только последний снимок
		for k := range sim.last {
			if k != key {
				delete(sim.last, k)
			}
		}
	}
	sim.mu.Unlock()
	close(ch)
}

// captureMarginBaseline — прогон набора через поиск витрины: выдача до политики маржи по каждому запросу.
func (s *Services) captureMarginBaseline(ctx context.Context, set, key string) (*marginCapture, error) {
	queries, err := s.evalQueries(ctx, set, nil)
	if err != nil {
		return nil, err
	}
	cat, err := s.evalCatalog(ctx)
	if err != nil {
		return nil, err
	}
	st := s.catalog.Load()
	if st == nil || st.lex == nil {
		return nil, errs.NewUnavailableError(MsgNoIndex)
	}
	sim := &s.margin().sim
	sim.mu.Lock()
	sim.running, sim.done, sim.total = true, 0, len(queries)
	sim.mu.Unlock()

	c := &marginCapture{key: key, set: set, setHash: setHash(queries), catalog: st.version, rules: st.rulesVersion,
		titles: map[string]string{}}
	results := make([]*marginSimQuery, len(queries))
	jobs := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for range marginSimWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				q := queries[i]
				probe := &MarginProbe{}
				sr := in.SearchRequest{Query: q.Text, Config: domain.ConfigLive, Limit: evalSearchLimit}
				if q.ClientID != nil {
					sr.ClientID = *q.ClientID
				}
				_, err := s.SearchWith(ctx, "margin-sim-"+q.ID, sr, SearchOptions{NoLog: true, MarginProbe: probe})
				mu.Lock()
				if err != nil {
					c.errors++
				} else {
					fam := cat.familyGrades(q)
					ideal := make([]int, 0, len(fam))
					for _, g := range fam {
						ideal = append(ideal, g)
					}
					results[i] = &marginSimQuery{q: q, in: probe.input, ideal: ideal}
				}
				mu.Unlock()
				sim.mu.Lock()
				sim.done++
				sim.mu.Unlock()
			}
		}()
	}
	for i := range queries {
		if ctx.Err() != nil {
			break
		}
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, r := range results {
		if r == nil {
			continue
		}
		c.queries = append(c.queries, *r)
		for _, cd := range r.in.cands {
			if d, ok := st.lex.docs[cd.productID]; ok {
				c.titles[cd.productID] = d.Title
			}
		}
	}
	return c, nil
}

// marginSimulate — показатели положений рычага на снятой базовой выдаче (чистая функция).
func marginSimulate(c *marginCapture, positions []float64, eps float64) *out.MarginSimulation {
	res := &out.MarginSimulation{Set: c.set, SetHash: c.setHash, Queries: len(c.queries), Epsilon: eps,
		BaseWeight: marginBaseWeight, CatalogVersion: c.catalog, RulesVersion: c.rules, Items: []out.MarginPosition{}}
	var base evalMean
	for _, q := range c.queries {
		if v := marginNDCG(q, identityOrder(len(q.in.cands))); v != nil {
			base.add(*v)
			res.Judged++
		}
	}
	if v := base.value(); v != nil {
		res.BaseNDCG10 = *v
	}
	for _, w := range positions {
		res.Items = append(res.Items, marginSimulatePosition(c, w, eps))
	}
	// Упор рычага: от 0.05 вверх и вниз — до первого положения, где запросы выходят за ε.
	sorted := slices.Clone(res.Items)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Weight < sorted[j].Weight })
	bi := slices.IndexFunc(sorted, func(p out.MarginPosition) bool { return sameWeight(p.Weight, marginBaseWeight) })
	res.StopWeight, res.MinWeight = marginBaseWeight, marginBaseWeight
	avail := map[float64]bool{marginBaseWeight: true}
	if bi >= 0 {
		for i := bi; i < len(sorted) && sorted[i].ShareOverEps == 0; i++ {
			res.StopWeight = sorted[i].Weight
			avail[sorted[i].Weight] = true
		}
		for i := bi; i >= 0 && sorted[i].ShareOverEps == 0; i-- {
			res.MinWeight = sorted[i].Weight
			avail[sorted[i].Weight] = true
		}
	}
	for i := range res.Items {
		res.Items[i].Available = avail[res.Items[i].Weight]
	}
	res.Note = fmt.Sprintf("Оценка на %d размеченных запросах набора %s. Маржа по показам в первой десятке "+
		"(перцентиль в категории). Не прогноз продаж.", res.Judged, c.set)
	return res
}

// marginNDCG — NDCG@10 запроса при порядке order; nil — разметки нет.
func marginNDCG(q marginSimQuery, order []int) *float64 {
	grades := make([]int, len(order))
	for i, idx := range order {
		grades[i] = q.q.Judgments[q.in.cands[idx].productID]
	}
	return evalNDCG(grades, q.ideal, evalTopK)
}

// marginIndexOf — индекс показанной маржи: нормированная маржа первых 10 с весом 1/log2(i+2); relevantOnly —
// только позиции с оценкой ≥ 2. ok=false — считать не по чему.
func marginIndexOf(q marginSimQuery, order []int, relevantOnly bool) (float64, bool) {
	num, den := 0.0, 0.0
	for i, idx := range order {
		if i >= evalTopK {
			break
		}
		c := q.in.cands[idx]
		if relevantOnly && q.q.Judgments[c.productID] < relevantGrade {
			continue
		}
		d := 1 / math.Log2(float64(i)+2)
		num += d * c.pct
		den += d
	}
	if den == 0 {
		return 0, false
	}
	return num / den, true
}

func marginSimulatePosition(c *marginCapture, w, eps float64) out.MarginPosition {
	p := out.MarginPosition{Weight: w, GuardsByKind: map[string]int{}, Affected: []out.MarginAffected{}}
	var ndcg, delta, idx, idxRel evalMean
	var deltas []float64
	over, judged := 0, 0
	var affected []out.MarginAffected
	for _, q := range c.queries {
		n := len(q.in.cands)
		before := identityOrder(n)
		r := marginReorder(q.in, w, marginMaxShift)
		order := r.order
		guards := slices.Clone(r.guards)
		nb := marginNDCG(q, before)
		if nb != nil {
			judged++
			loss := func(o []int) float64 { return *nb - *marginNDCG(q, o) }
			if loss(order) > eps+1e-9 {
				over++
				var rolled int
				order, rolled = marginApplyEpsilon(q.in, order, eps, loss)
				if rolled > 0 {
					p.EpsilonRollbacks++
					guards = append(guards, domain.MarginGuardEpsilon)
				}
			}
		}
		for _, g := range guards {
			p.GuardsByKind[g]++
			p.GuardsFired++
		}
		var na *float64
		if nb != nil {
			na = marginNDCG(q, order)
			ndcg.add(*na)
			d := *na - *nb
			delta.add(d)
			deltas = append(deltas, d)
		}
		if v, ok := marginIndexOf(q, order, false); ok {
			idx.add(v)
		}
		if v, ok := marginIndexOf(q, order, true); ok {
			idxRel.add(v)
		}
		changed := !sameIntOrder(order, before)
		if changed {
			p.QueriesWithShifts++
		}
		top1 := n > 0 && order[0] != 0
		top3 := false
		for i := 0; i < min(3, n); i++ {
			top3 = top3 || order[i] != i
		}
		if top1 {
			p.Top1Changed++
			if slices.Contains(exactTypes, q.q.Type) {
				p.ExactTop1Changed++
			}
		}
		if top3 {
			p.Top3Changed++
		}
		if changed {
			a := out.MarginAffected{QueryID: q.q.ID, Query: q.q.Text, Type: q.q.Type, NDCGBefore: nb, NDCGAfter: na,
				Top1Changed: top1, Top3Changed: top3, Guards: guards,
				Before: marginRefs(c, q, before, before), After: marginRefs(c, q, order, before)}
			if na != nil {
				a.NDCGDelta = evalRound4(*na - *nb)
			}
			if a.Guards == nil {
				a.Guards = []string{}
			}
			affected = append(affected, a)
		}
	}
	if v := ndcg.value(); v != nil {
		p.NDCG10 = *v
	}
	if v := delta.value(); v != nil {
		p.NDCGDeltaMean = *v
	}
	if len(deltas) > 0 {
		slices.Sort(deltas)
		rank := min(max(int(math.Ceil(0.1*float64(len(deltas)))), 1), len(deltas))
		p.NDCGDeltaP10 = evalRound4(deltas[rank-1])
	}
	if judged > 0 {
		p.ShareOverEps = evalRound4(float64(over) / float64(judged))
	}
	if v := idx.value(); v != nil {
		p.MarginIndex = *v
	}
	if v := idxRel.value(); v != nil {
		p.MarginIndexRelevant = *v
	}
	p.Affected = marginPickAffected(affected)
	return p
}

// marginPickAffected — запросы со сменой топ-1 или топ-3 и пять худших по потере качества.
func marginPickAffected(all []out.MarginAffected) []out.MarginAffected {
	res := []out.MarginAffected{}
	seen := map[string]bool{}
	for _, a := range all {
		if a.Top1Changed || a.Top3Changed {
			res = append(res, a)
			seen[a.QueryID] = true
		}
	}
	worst := slices.Clone(all)
	sort.SliceStable(worst, func(i, j int) bool { return worst[i].NDCGDelta < worst[j].NDCGDelta })
	for i, n := 0, 0; i < len(worst) && n < marginAffectedWorst; i++ {
		if worst[i].NDCGDelta >= 0 {
			break
		}
		n++
		if !seen[worst[i].QueryID] {
			res = append(res, worst[i])
			seen[worst[i].QueryID] = true
		}
	}
	sort.SliceStable(res, func(i, j int) bool {
		if res[i].NDCGDelta != res[j].NDCGDelta {
			return res[i].NDCGDelta < res[j].NDCGDelta
		}
		return res[i].QueryID < res[j].QueryID
	})
	if len(res) > marginAffectedMax {
		res = res[:marginAffectedMax]
	}
	return res
}

// marginRefs — первые позиции выдачи для «было / стало»; moved — на сколько поднялась относительно before.
func marginRefs(c *marginCapture, q marginSimQuery, order, before []int) []out.MarginItemRef {
	pos := make(map[int]int, len(before))
	for i, idx := range before {
		pos[idx] = i
	}
	refs := []out.MarginItemRef{}
	for i, idx := range order {
		if i >= marginShowTop {
			break
		}
		cd := q.in.cands[idx]
		ref := out.MarginItemRef{ProductID: cd.productID, Title: c.titles[cd.productID], Margin: cd.pct, Moved: pos[idx] - i}
		if g, ok := q.q.Judgments[cd.productID]; ok {
			ref.Grade = &g
		}
		refs = append(refs, ref)
	}
	return refs
}

// EvalMarginRequest — прогон набора с политикой маржи (конфигурация C3 и её соседи для трека агента).
type EvalMarginRequest struct {
	Set    string
	Config string // конфигурация поиска; пусто — live
	// Options — база (снимок правил, флаги C1/C2 трека агента); Margin в ней задаёт политику (MarginC3).
	Options SearchOptions
}

// EvalWithMargin — метрики набора в конфигурации C3: тот же счёт, что у RunEvaluation (evalScoreQuery,
// evalAggregate), поиск — с опциями req.Options (политика маржи — MarginC3).
func (s *Services) EvalWithMargin(ctx context.Context, req EvalMarginRequest) (*out.EvalConfigResult, error) {
	set := strings.ToLower(strings.TrimSpace(req.Set))
	if set == "" {
		set = domain.EvalSetTune
	}
	cfg := req.Config
	if cfg == "" {
		cfg = domain.ConfigLive
	}
	queries, err := s.evalQueries(ctx, set, nil)
	if err != nil {
		return nil, err
	}
	cat, err := s.evalCatalog(ctx)
	if err != nil {
		return nil, err
	}
	started := time.Now()
	res := &out.EvalConfigResult{Config: "C3", PerQuery: make([]out.EvalQueryResult, 0, len(queries))}
	opts := req.Options
	opts.NoLog = true
	for _, q := range queries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		sr := in.SearchRequest{Query: q.Text, Config: cfg, Limit: evalSearchLimit}
		if q.ClientID != nil {
			sr.ClientID = *q.ClientID
		}
		t := time.Now()
		resp, err := s.SearchWith(ctx, "eval-c3-"+q.ID, sr, opts)
		o := evalOutcome{Ms: float64(time.Since(t).Microseconds()) / 1000, Err: err}
		if err == nil {
			o.Total, o.EmptyMarker = resp.Total, resp.Empty != nil
			if resp.Trace != nil {
				o.Route = resp.Trace.Route
				res.RulesVersion, res.CatalogVersion = resp.Trace.RulesVersion, resp.Trace.CatalogVersion
			}
			for _, it := range resp.Items {
				o.Items = append(o.Items, evalItem{ProductID: it.ProductID, FamilyID: it.FamilyID, Group: it.Group})
			}
		}
		res.PerQuery = append(res.PerQuery, evalScoreQuery(q, o, cat))
	}
	res.Metrics = evalAggregate(res.PerQuery)
	res.ByType = evalByType(res.PerQuery)
	res.Regressions, res.Improvements = []out.EvalChange{}, []out.EvalChange{}
	res.DurationMs = time.Since(started).Milliseconds()
	return res, nil
}
