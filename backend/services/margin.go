package services

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync/atomic"
	"time"

	"kuznica/api/domain"
)

// Политика маржи в ранжировании (контур коммерции).
//
// Рычаг — один вес w из marginPositions. Вес контура поиска — marginBaseWeight (0.05, rank.go businessWeights):
// при w = 0.05 выдача та же, что в контуре поиска (шаг ничего не переставляет). Иначе к итоговому скору контура поиска
// прибавляется поправка внутри группы релевантности:
//   - w > 0.05: (w − 0.05) · перцентиль маржи позиции в её категории (0–1; маржи нет — 0.5, нейтрально;
//     отрицательная — −0.5, мягкое понижение без скрытия). Перцентиль, а не рубли и не доли: лицензия за 2 тыс. ₽
//     и сервер за 2 млн ₽ сравниваются честно.
//   - w < 0.05: −(0.05 − w) · margin — снимается часть слагаемого маржи контура поиска в его же нормировке; при w = 0
//     маржи в скоре нет совсем.
//
// Ограничения, которые рычаг обойти не может:
//   - точный запрос (маршрут A: артикул, парт-номер, точное название) — сдвиг отключён; первая позиция группы
//     exact с названным семейством — неподвижна;
//   - переход между группами запрещён: группа — подряд идущие семейства с одинаковыми группой, ступенью,
//     масштабом, точным количеством и полнотой условий (обязательные условия и совместимость неприкосновенны);
//   - сдвиг не больше marginMaxShift позиций вверх или вниз;
//   - на размеченном запросе (симуляция) — бюджет ε потери NDCG@10 с откатом самых дорогих перестановок;
//     в живом поиске ε не проверяется (разметки нет), поэтому положения за упором рычага не применяются.

// Константы рычага.
const (
	marginBaseWeight = 0.05
	marginEpsilon    = 0.01
	marginMaxShift   = 2
	// marginUnknownPct — нормированная маржа позиции без маржи: середина категории.
	marginUnknownPct = 0.5
	// marginNegativePct — нормированная маржа при отрицательной марже: ниже любой известной.
	marginNegativePct = -0.5
)

// marginPositions — положения рычага.
var marginPositions = []float64{0, 0.03, 0.05, 0.08, 0.12, 0.15}

// MarginPositions — положения рычага (копия).
func MarginPositions() []float64 { return append([]float64(nil), marginPositions...) }

// validMarginWeight — вес — одно из положений.
func validMarginWeight(w float64) bool {
	for _, p := range marginPositions {
		if math.Abs(p-w) < 1e-9 {
			return true
		}
	}
	return false
}

// MarginOverride — политика маржи поверх действующей: конфигурация C3 оценки, симуляция. Weight — вес рычага.
type MarginOverride struct {
	Weight float64
}

// MarginC3 — опции поиска конфигурации C3 (C2 + политика маржи) для оценки: та же база opts, вес weight.
// Политика применяется при любой конфигурации поиска B3/B4/live (без override — только на live).
func MarginC3(opts SearchOptions, weight float64) SearchOptions {
	opts.Margin = &MarginOverride{Weight: weight}
	return opts
}

// marginCand — семейство выдачи, как его видит политика маржи.
type marginCand struct {
	familyID  string
	productID string
	// key — группа релевантности: переставлять можно только внутри подряд идущих семейств с одинаковым key.
	key   string
	final float64
	raw   float64 // маржа головы, как в контуре поиска (0–1)
	pct   float64 // нормированная маржа: перцентиль в категории
	known bool
}

// marginInput — выдача одного запроса до политики маржи и признаки точного запроса.
type marginInput struct {
	cands []marginCand
	// exactQuery — точный запрос: сдвиг отключён целиком.
	exactQuery bool
	// protectTop1 — первая позиция неподвижна (группа exact с названным семейством).
	protectTop1 bool
}

// marginResult — порядок после политики и что сработало.
type marginResult struct {
	order  []int // индексы cands в новом порядке
	guards []string
}

// adjusted — скор позиции при весе w (см. описание файла).
func (c marginCand) adjusted(w float64) float64 {
	switch {
	case w > marginBaseWeight:
		return c.final + (w-marginBaseWeight)*c.pct
	case w < marginBaseWeight:
		return c.final - (marginBaseWeight-w)*c.raw
	}
	return c.final
}

// identity — порядок без изменений.
func identityOrder(n int) []int {
	o := make([]int, n)
	for i := range o {
		o[i] = i
	}
	return o
}

// marginSegments — границы групп релевантности [start, end) по baseline-порядку; при protectTop1 первая
// позиция — отдельная группа.
func marginSegments(in marginInput) [][2]int {
	var segs [][2]int
	n := len(in.cands)
	start := 0
	for i := 1; i <= n; i++ {
		cut := i == n || in.cands[i].key != in.cands[i-1].key || in.protectTop1 && i == 1
		if cut {
			segs = append(segs, [2]int{start, i})
			start = i
		}
	}
	return segs
}

// marginReorder — новый порядок при весе w: внутри групп, со сдвигом не больше maxShift. Жадно: на позицию p
// встаёт лучшая по скору позиция, поднимающаяся не больше чем на maxShift; позиция, которая иначе опустится
// больше чем на maxShift, ставится принудительно. Равные скоры — в прежнем порядке.
func marginReorder(in marginInput, w float64, maxShift int) marginResult {
	n := len(in.cands)
	res := marginResult{order: identityOrder(n), guards: []string{}}
	if n == 0 || math.Abs(w-marginBaseWeight) < 1e-9 {
		return res
	}
	free := marginFreeOrder(in, w)
	changedFree := !sameIntOrder(free, res.order)
	if in.exactQuery {
		if changedFree {
			res.guards = append(res.guards, domain.MarginGuardExactQuery)
		}
		return res
	}
	if in.protectTop1 && free[0] != 0 {
		res.guards = append(res.guards, domain.MarginGuardExactTop1)
	}
	segs := marginSegments(in)
	segOf := make([]int, n)
	for si, s := range segs {
		for i := s[0]; i < s[1]; i++ {
			segOf[i] = si
		}
	}
	// Защита групп сработала: в порядке без ограничений позиция встала выше позиции из более высокой группы.
	for i := 1; i < n; i++ {
		if segOf[free[i-1]] > segOf[free[i]] {
			res.guards = append(res.guards, domain.MarginGuardGroupLock)
			break
		}
	}
	capped := false
	order := make([]int, 0, n)
	for _, s := range segs {
		// Порядок внутри группы без потолка — чтобы понять, сработал ли потолок.
		inner := identityOrder(s[1] - s[0])
		for k := range inner {
			inner[k] += s[0]
		}
		sort.SliceStable(inner, func(a, b int) bool { return in.cands[inner[a]].adjusted(w) > in.cands[inner[b]].adjusted(w) })
		for k, idx := range inner {
			if abs(idx-(s[0]+k)) > maxShift {
				capped = true
			}
		}
		remaining := identityOrder(s[1] - s[0])
		for k := range remaining {
			remaining[k] += s[0]
		}
		for p := s[0]; p < s[1]; p++ {
			pick := -1
			if remaining[0] <= p-maxShift {
				pick = 0
			} else {
				for k, idx := range remaining {
					if idx > p+maxShift {
						break
					}
					if pick < 0 || in.cands[idx].adjusted(w) > in.cands[remaining[pick]].adjusted(w) {
						pick = k
					}
				}
			}
			order = append(order, remaining[pick])
			remaining = append(remaining[:pick], remaining[pick+1:]...)
		}
	}
	if capped {
		res.guards = append(res.guards, domain.MarginGuardMaxShift)
	}
	res.order = order
	return res
}

// marginFreeOrder — порядок по скору при весе w без ограничений (чтобы считать, какие защиты сработали).
func marginFreeOrder(in marginInput, w float64) []int {
	o := identityOrder(len(in.cands))
	sort.SliceStable(o, func(a, b int) bool { return in.cands[o[a]].adjusted(w) > in.cands[o[b]].adjusted(w) })
	return o
}

func sameIntOrder(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// marginBlocks — независимые перестановки порядка: минимальные отрезки [a, b], которые порядок переводит сам
// в себя. Откат одного отрезка к прежнему порядку не трогает остальные и не нарушает ограничений.
func marginBlocks(order []int) [][2]int {
	var blocks [][2]int
	start, maxIdx := 0, -1
	for p, idx := range order {
		maxIdx = max(maxIdx, idx)
		if maxIdx == p {
			if p > start || order[start] != start {
				blocks = append(blocks, [2]int{start, p + 1})
			}
			start = p + 1
		}
	}
	return blocks
}

// marginBlockCost — цена перестановки по релевантности: сумма разниц скора контура поиска по парам, которые
// поменялись местами (выше оказалась позиция с меньшим скором).
func marginBlockCost(in marginInput, order []int, b [2]int) float64 {
	cost := 0.0
	for i := b[0]; i < b[1]; i++ {
		for j := i + 1; j < b[1]; j++ {
			if order[i] > order[j] {
				cost += math.Max(0, in.cands[order[j]].final-in.cands[order[i]].final)
			}
		}
	}
	return cost
}

// marginApplyEpsilon — бюджет качества на размеченном запросе: пока потеря NDCG@10 (loss) больше eps,
// самая дорогая по релевантности перестановка откатывается к прежнему порядку. rolled — сколько откачено.
func marginApplyEpsilon(in marginInput, order []int, eps float64, loss func([]int) float64) ([]int, int) {
	order = append([]int(nil), order...)
	rolled := 0
	for loss(order) > eps+1e-9 {
		blocks := marginBlocks(order)
		if len(blocks) == 0 {
			break
		}
		best, bestCost := -1, -1.0
		for i, b := range blocks {
			if c := marginBlockCost(in, order, b); c > bestCost {
				best, bestCost = i, c
			}
		}
		b := blocks[best]
		for p := b[0]; p < b[1]; p++ {
			order[p] = p
		}
		rolled++
	}
	return order, rolled
}

// Перцентили маржи по категориям: считаются один раз на словарь каталога (импорт меняет словарь целиком).
type marginPctTable struct {
	lx  *searchLexicon
	pct map[string]float64 // product_id → перцентиль маржи в категории
}

var marginPctCache atomic.Pointer[marginPctTable]

// marginPercentiles — перцентиль маржи каждой позиции внутри категории: (ниже + половина равных) / n по
// позициям с известной (положительной) маржей. Позиции без маржи в таблицу не входят.
func marginPercentiles(lx *searchLexicon) map[string]float64 {
	if t := marginPctCache.Load(); t != nil && t.lx == lx {
		return t.pct
	}
	byCat := map[string][]float64{}
	for _, d := range lx.docs {
		if d.Margin > 0 {
			byCat[d.CategoryID] = append(byCat[d.CategoryID], d.Margin)
		}
	}
	for _, ms := range byCat {
		sort.Float64s(ms)
	}
	pct := make(map[string]float64, len(lx.docs))
	for id, d := range lx.docs {
		if d.Margin <= 0 {
			continue
		}
		ms := byCat[d.CategoryID]
		lo := sort.SearchFloat64s(ms, d.Margin)
		hi := sort.Search(len(ms), func(i int) bool { return ms[i] > d.Margin })
		pct[id] = round4((float64(lo) + float64(hi-lo)/2) / float64(len(ms)))
	}
	marginPctCache.Store(&marginPctTable{lx: lx, pct: pct})
	return pct
}

// marginKey — группа релевантности семейства (не пересекается сдвигом).
func marginKey(fh *familyHit) string {
	return fmt.Sprintf("%s|%d|%d|%d|%t", fh.group, fh.tier, fh.scale, fh.exactQty, fh.headOK)
}

// marginInputOf — вход политики из семейств выдачи (после сортировки и отсечения хвоста).
func marginInputOf(rc rankCtx, route string, fams []*familyHit) marginInput {
	pct := marginPercentiles(rc.lx)
	in := marginInput{cands: make([]marginCand, len(fams)), exactQuery: route == domain.RouteExact}
	for i, fh := range fams {
		c := marginCand{familyID: fh.id, productID: fh.head.ProductID, key: marginKey(fh), final: fh.final, raw: fh.head.Margin}
		switch p, ok := pct[fh.head.ProductID]; {
		case ok:
			c.pct, c.known = p, true
		case fh.head.Margin < 0:
			c.pct, c.known = marginNegativePct, true
		default:
			c.pct = marginUnknownPct
		}
		in.cands[i] = c
	}
	if len(fams) > 0 && fams[0].group == domain.GroupExact && fams[0].tier > 0 {
		in.protectTop1 = true
	}
	return in
}

// marginNotes — пометки неизвестной и отрицательной маржи у переставляемых позиций.
func marginNotes(in marginInput) []string {
	var notes []string
	unknown, negative := false, false
	for _, c := range in.cands {
		unknown = unknown || !c.known
		negative = negative || c.raw < 0
	}
	if unknown {
		notes = append(notes, domain.MarginNoteUnknown)
	}
	if negative {
		notes = append(notes, domain.MarginNoteNegative)
	}
	return notes
}

// marginShifts — сдвиги позиций: откуда (1-based) куда.
func marginShifts(in marginInput, order []int) []domain.MarginShift {
	shifts := []domain.MarginShift{}
	for to, from := range order {
		if to != from {
			c := in.cands[from]
			shifts = append(shifts, domain.MarginShift{ProductID: c.productID, From: from + 1, To: to + 1, Margin: c.pct})
		}
	}
	return shifts
}

// MarginProbe — снимок выдачи до политики маржи (симуляция): поиск заполняет его и не переставляет.
type MarginProbe struct {
	input marginInput
	set   bool
}

// marginStage — политика маржи в поиске B3/B4/live: включена флагом; без override — только на витрине (live),
// B3/B4 оценки остаются выдачей контура поиска.
// Возвращает семейства в новом порядке и шаг трассы (nil — шага нет: флаг выключен или конфигурация не та).
// Ошибка чтения политики не ломает поиск: порядок контура поиска, шаг с error.
func (s *Services) marginStage(ctx context.Context, cfg string, rc rankCtx, route string, fams []*familyHit,
	opts SearchOptions) ([]*familyHit, *domain.MarginStep) {
	// Снимок для симуляции снимается и при выключенном флаге: симуляция ничего не меняет в боевом поиске.
	if opts.MarginProbe == nil && (!s.featuresFor(ctx).MarginPolicy || opts.Margin == nil && cfg != domain.ConfigLive) {
		return fams, nil
	}
	t := time.Now()
	in := marginInputOf(rc, route, fams)
	if opts.MarginProbe != nil {
		opts.MarginProbe.input, opts.MarginProbe.set = in, true
		return fams, nil
	}
	step := &domain.MarginStep{StepBase: domain.StepBase{Name: domain.StepMargin}, BaseWeight: marginBaseWeight,
		Shifts: []domain.MarginShift{}, Guards: []string{}}
	if opts.Margin != nil {
		step.Weight = opts.Margin.Weight
	} else {
		pol, err := s.activeMarginPolicy(ctx)
		step.PolicyVersion, step.Weight = pol.Version, pol.Weight
		if err != nil {
			step.Error = "политика маржи не прочитана — вес контура поиска"
		}
	}
	res := marginReorder(in, step.Weight, marginMaxShift)
	step.Shifts = marginShifts(in, res.order)
	step.Guards = append(res.guards, marginNotes(in)...)
	if len(step.Shifts) > 0 {
		next := make([]*familyHit, len(fams))
		for to, from := range res.order {
			next[to] = fams[from]
		}
		fams = next
	}
	step.Ms = msSince(t)
	return fams, step
}

// stripMarginStep — шаг маржи убирается из трассы ответа витрины (числа маржи — только в бэк-офисе:
// GET /api/traces/{id} читает сохранённую трассу целиком).
func stripMarginStep(tr *domain.Trace) {
	if tr == nil {
		return
	}
	out := tr.Steps[:0]
	for _, st := range tr.Steps {
		if _, ok := st.(*domain.MarginStep); ok {
			continue
		}
		out = append(out, st)
	}
	tr.Steps = out
}
