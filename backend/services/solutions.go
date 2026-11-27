package services

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
)

// Закупка под задачу: запрос «рабочее место для 20 сотрудников» — просьба о
// комплекте. Намерение распознаётся правилами по триггерам утверждённых шаблонов (solution_templates.json), LLM не
// вызывается. Каждый слот заполняется тем же поиском (запрос слота + условия слота фильтрами), отбор — по категории
// слота, его условиям и условиям запроса (отечественное, исключённый вендор). Количество — из запроса по стратегии
// слота; внутри семейства и редакции берётся фасовка, которой дешевле всего закрыть N мест. Основная выдача не
// меняется: решение — отдельное поле ответа и шаг трассы solution.

// StepSolution — шаг трассы «Решение по шаблону».
const StepSolution = "solution"

// FieldSolution — поле чипа «Комплексная закупка»: снятый чип выключает шаблон для запроса.
const FieldSolution = "solution"

// maxSlotAlternatives — сколько вариантов «заменить» у слота.
const maxSlotAlternatives = 3

// slotSearchLimit — сколько позиций просит поиск слота.
const slotSearchLimit = 20

// Причины в шаге трассы.
const (
	solutionNoTemplate = "no_template"
	solutionRemoved    = "removed_by_user"
	solutionNested     = "slot_search"
	solutionError      = "error"
	slotNoCandidates   = "no_candidates"
	slotCondition      = "condition_unmet"
	slotDependsMissing = "depends_on_empty_slot"
)

// Откуда позиция слота: из поиска по запросу слота или (поиск ничего не дал под условия) из каталога категории.
const (
	slotFromSearch  = "search"
	slotFromCatalog = "catalog"
)

// SolutionStep — шаг трассы solution.
type SolutionStep struct {
	domain.StepBase
	Error          string              `json:"error,omitempty"`
	TemplateID     string              `json:"template_id,omitempty"`
	Trigger        string              `json:"trigger,omitempty"`
	Seats          int                 `json:"seats"`
	QuantitySource string              `json:"quantity_source,omitempty"` // query | default
	Conditions     []string            `json:"conditions"`
	Slots          []SolutionSlotTrace `json:"slots"`
}

// SolutionSlotTrace — как заполнен слот: запрос, фильтры, источник, выбранная позиция и количество.
type SolutionSlotTrace struct {
	ID           string      `json:"id"`
	Query        string      `json:"query"`
	CategoryID   string      `json:"category_id"`
	Filters      []in.Filter `json:"filters"`
	Source       string      `json:"source,omitempty"` // search | catalog
	ProductID    string      `json:"product_id,omitempty"`
	Quantity     int         `json:"quantity"`
	QuantityRule string      `json:"quantity_rule,omitempty"`
	Alternatives []string    `json:"alternatives"`
	Rejected     int         `json:"rejected"`
	Skipped      bool        `json:"skipped,omitempty"`
	Reason       string      `json:"reason,omitempty"`
	Ms           float64     `json:"ms"`
}

// slotSearchKey — поиск идёт за слотом решения: без шаблона (нет рекурсии) и без разбора моделью.
type slotSearchKey struct{}

func isSlotSearch(ctx context.Context) bool {
	v, _ := ctx.Value(slotSearchKey{}).(bool)
	return v
}

// templateMatch — найденный шаблон и сработавший триггер.
type templateMatch struct {
	tmpl    *domain.SolutionTemplate
	trigger string
}

// matchTemplate — утверждённый шаблон по триггерам: триггер — подстрока запроса с начала слова (не после числа).
// Несколько — раньше в запросе, при равенстве длиннее.
func matchTemplate(templates []domain.SolutionTemplate, query string) *templateMatch {
	q := " " + foldText(query) + " "
	var best *templateMatch
	bestPos := math.MaxInt
	for i := range templates {
		t := &templates[i]
		if !t.Approved || len(t.Slots) == 0 {
			continue
		}
		for _, trig := range t.Triggers {
			tr := foldText(trig)
			if tr == "" {
				continue
			}
			pos := triggerPos(q, tr)
			if pos < 0 {
				continue
			}
			if pos < bestPos || pos == bestPos && len(tr) > len(best.trigger) {
				best, bestPos = &templateMatch{tmpl: t, trigger: tr}, pos
			}
		}
	}
	return best
}

// triggerPos — позиция триггера с начала слова; вхождение сразу после числа — единица количества лицензии
// («1с на 10 рабочих мест», «р7 офис на 10 рабочих мест»), а не просьба о комплекте, и не считается. -1 — нет.
func triggerPos(q, tr string) int {
	from := 0
	for {
		i := strings.Index(q[from:], " "+tr)
		if i < 0 {
			return -1
		}
		pos := from + i
		before := strings.TrimRight(q[:pos], " ")
		if before == "" || before[len(before)-1] < '0' || before[len(before)-1] > '9' {
			return pos
		}
		from = pos + 1
	}
}

// seatsRe — «20 сотрудников», «на 10 человек», «15 рабочих мест», «50 пользователей».
var seatsRe = regexp.MustCompile(`(\d{1,5})\s*(сотрудник|человек|чел\b|пользовател|рабоч\S* мест|мест|компьютер|пк\b|ноутбук)`)

// solutionSeats — число мест из запроса: явное «N сотрудников|человек|мест», затем количество лицензий и закупки
// из разбора; нет — 1 (одно рабочее место, «для нового сотрудника»).
func solutionSeats(query string, licQty, hwQty int) (int, string) {
	if m := seatsRe.FindStringSubmatch(foldText(query)); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
			return n, "query"
		}
	}
	switch {
	case licQty > 0:
		return licQty, "query"
	case hwQty > 0:
		return hwQty, "query"
	}
	return 1, "default"
}

// solutionInput — что из основного запроса нужно решению.
type solutionInput struct {
	req     in.SearchRequest
	cfg     string
	seats   int
	qtySrc  string
	origin  string   // ru — только отечественное
	vendors []string // исключённые вендоры
}

// slotResult — заполненный слот.
type slotResult struct {
	slot  out.SolutionSlot
	trace SolutionSlotTrace
	doc   *indexDoc // выбранная позиция (для условий зависимых слотов)
}

// solutionStep — шаг решения по шаблону. Флаг выключен, поиск слота или шаблона нет — nil (шага нет) или
// skipped. Сбой не ломает поиск: шаг с error, поле ответа пустое.
func (s *Services) solutionStep(ctx context.Context, st *catalogState, si solutionInput, resp *out.SearchResponse) (step *SolutionStep) {
	if !s.featuresFor(ctx).Solutions || isSlotSearch(ctx) {
		return nil
	}
	t := time.Now()
	step = &SolutionStep{StepBase: domain.StepBase{Name: StepSolution}, Conditions: []string{}, Slots: []SolutionSlotTrace{}}
	defer func() {
		if r := recover(); r != nil {
			step.Error, step.Reason = fmt.Sprint(r), solutionError
			resp.Solution = nil
		}
		step.Ms = msSince(t)
	}()
	cc := s.commerceFor(ctx)
	if cc == nil || st == nil || st.lex == nil {
		step.Skipped, step.Reason = true, solutionNoTemplate
		return step
	}
	m := matchTemplate(cc.solutions, si.req.Query)
	if m == nil {
		step.Skipped, step.Reason = true, solutionNoTemplate
		return step
	}
	step.TemplateID, step.Trigger = m.tmpl.ID, m.trigger
	if removedSet(si.req.RemovedChips)[FieldSolution][m.tmpl.ID] {
		step.Skipped, step.Reason = true, solutionRemoved
		return step
	}
	step.Seats, step.QuantitySource = si.seats, si.qtySrc
	if si.origin == "ru" {
		step.Conditions = append(step.Conditions, "origin=ru")
	}
	for _, v := range si.vendors {
		step.Conditions = append(step.Conditions, "vendor_not="+v)
	}
	resp.Chips = append(resp.Chips, out.Chip{Field: FieldSolution, Value: m.tmpl.ID, Label: "Комплексная закупка"})

	slots := m.tmpl.Slots
	results := make([]*slotResult, len(slots))
	// Независимые слоты — параллельно, зависимые (условие или совместимость с другим слотом) — после них.
	var wg sync.WaitGroup
	for i := range slots {
		if slotDependency(slots[i]) != "" {
			continue
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					results[i] = emptySlot(slots[i], si, solutionError+": "+fmt.Sprint(r))
				}
			}()
			results[i] = s.fillSlot(ctx, st, cc, slots[i], si, nil, conditionalOn(slots, slots[i].ID))
		}(i)
	}
	wg.Wait()
	byID := map[string]*slotResult{}
	for i, r := range results {
		if r != nil {
			byID[slots[i].ID] = r
		}
	}
	for i := range slots {
		dep := slotDependency(slots[i])
		if dep == "" {
			continue
		}
		results[i] = s.fillSlot(ctx, st, cc, slots[i], si, byID[dep], conditionalOn(slots, slots[i].ID))
		byID[slots[i].ID] = results[i]
	}

	sol := &out.Solution{TemplateID: m.tmpl.ID, Title: m.tmpl.Title, Seats: si.seats, Slots: []out.SolutionSlot{},
		Source: "Собрано по шаблону Softline"}
	total, known := 0.0, true
	for _, r := range results {
		step.Slots = append(step.Slots, r.trace)
		sol.Slots = append(sol.Slots, r.slot)
		if r.slot.Item == nil || !r.slot.Active {
			continue // условный слот («ОС, если ноутбук без ОС») при выбранной позиции не нужен — в сумму не входит
		}
		if r.slot.Item.PriceRub == nil || r.slot.Item.PriceMode != domain.PriceFixed {
			known = false
			sol.OnRequest++
			continue
		}
		total += *r.slot.Item.PriceRub * float64(r.slot.Quantity)
	}
	filled := 0
	for _, sl := range sol.Slots {
		if sl.Item != nil && sl.Active {
			filled++
		}
	}
	if filled == 0 {
		step.Reason = slotNoCandidates
		return step
	}
	if known {
		v := math.Round(total*100) / 100
		sol.TotalRub = &v
	}
	resp.Solution = sol
	return step
}

// slotDependency — слот, от которого зависит этот: условие «if_slot_attr» или совместимость «slot:server».
func slotDependency(sl domain.SolutionSlot) string {
	if c, ok := sl.Constraints["if_slot_attr"].(map[string]any); ok {
		if v, _ := c["slot"].(string); v != "" {
			return v
		}
	}
	if v, _ := sl.Constraints["compatible_with"].(string); strings.HasPrefix(v, "slot:") {
		return strings.TrimPrefix(v, "slot:")
	}
	return ""
}

func emptySlot(sl domain.SolutionSlot, si solutionInput, reason string) *slotResult {
	return &slotResult{
		slot: out.SolutionSlot{ID: sl.ID, Label: sl.Label, CategoryID: sl.CategoryID, Query: sl.Query,
			Quantity: slotQuantity(sl.QuantityStrategy, nil, si.seats), Strategy: sl.QuantityStrategy,
			Alternatives: []out.SearchItem{}, Quantities: map[string]int{}, Active: true},
		trace: SolutionSlotTrace{ID: sl.ID, Query: sl.Query, CategoryID: sl.CategoryID, Filters: []in.Filter{},
			Alternatives: []string{}, Skipped: true, Reason: reason},
	}
}

// slotConstraints — условия слота: числовые характеристики → фильтры поиска; версия, совместимость со слотом и
// условие по атрибуту другого слота проверяются в коде.
type slotConstraints struct {
	filters []in.Filter
	numeric map[string]in.Filter
	version *in.Filter
	compat  string // модель системы зависимого слота
	cond    *out.SlotCondition
}

// altIDs — альтернативы слота.
func (r *slotResult) altIDs() []string {
	ids := []string{}
	for _, a := range r.slot.Alternatives {
		ids = append(ids, a.ProductID)
	}
	return ids
}

// slotAvoid — условие, при котором позиции слота понадобится ещё один слот (ноутбук без ОС → слот ОС). Такие
// позиции ставятся после остальных: по умолчанию решение собирается из меньшего числа позиций.
type slotAvoid struct{ key, value string }

// conditionalOn — условия других слотов шаблона на позицию слота id.
func conditionalOn(slots []domain.SolutionSlot, id string) []slotAvoid {
	var res []slotAvoid
	for _, sl := range slots {
		m, ok := sl.Constraints["if_slot_attr"].(map[string]any)
		if !ok {
			continue
		}
		if s, _ := m["slot"].(string); s == id {
			k, _ := m["key"].(string)
			v, _ := m["value"].(string)
			res = append(res, slotAvoid{k, v})
		}
	}
	return res
}

func parseSlotConstraints(sl domain.SolutionSlot, dep *slotResult, cc *commerceCatalog) (slotConstraints, string) {
	c := slotConstraints{filters: []in.Filter{}, numeric: map[string]in.Filter{}}
	keys := make([]string, 0, len(sl.Constraints))
	for k := range sl.Constraints {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := sl.Constraints[k]
		switch k {
		case "if_slot_attr":
			m, _ := v.(map[string]any)
			key, _ := m["key"].(string)
			want, _ := m["value"].(string)
			slot, _ := m["slot"].(string)
			if dep == nil || dep.doc == nil {
				return c, slotDependsMissing
			}
			// Слот подбирается всегда; входит ли он в решение — по выбранной позиции слота-условия.
			c.cond = &out.SlotCondition{Slot: slot, Key: key, Value: want, ProductIDs: []string{}}
			for _, pid := range append([]string{dep.doc.ProductID}, dep.altIDs()...) {
				if strings.EqualFold(cc.attr(pid, key), want) {
					c.cond.ProductIDs = append(c.cond.ProductIDs, pid)
				}
			}
		case "compatible_with":
			if dep == nil || dep.doc == nil || dep.doc.ModelID == "" {
				return c, slotDependsMissing
			}
			c.compat = dep.doc.ModelID
		case "version":
			m, _ := v.(map[string]any)
			op, _ := m["op"].(string)
			c.version = &in.Filter{Key: "version", Op: op, Value: fmt.Sprint(m["value"])}
		default:
			m, ok := v.(map[string]any)
			if !ok {
				continue
			}
			op, _ := m["op"].(string)
			f := in.Filter{Key: k, Op: op, Value: m["value"], Value2: m["value2"]}
			c.filters = append(c.filters, f)
			c.numeric[k] = f
		}
	}
	return c, ""
}

// fillSlot — позиция слота: поиск по запросу слота с фильтрами, отбор по категории и условиям; пусто — лучшие
// по популярности позиции категории под теми же условиями.
func (s *Services) fillSlot(ctx context.Context, st *catalogState, cc *commerceCatalog, sl domain.SolutionSlot, si solutionInput,
	dep *slotResult, avoid []slotAvoid) *slotResult {
	t := time.Now()
	res := emptySlot(sl, si, "")
	res.trace.Skipped = false
	defer func() { res.trace.Ms = msSince(t) }()
	cons, why := parseSlotConstraints(sl, dep, cc)
	res.trace.Filters = cons.filters
	if why != "" {
		res.trace.Skipped, res.trace.Reason = true, why
		res.slot.Active = false
		return res
	}
	if cons.cond != nil {
		res.slot.Condition = cons.cond
		res.slot.Active = contains(cons.cond.ProductIDs, dep.doc.ProductID)
		if !res.slot.Active {
			res.trace.Reason = slotCondition
		}
	}
	lx := st.lex
	ok := func(d indexDoc) bool { return slotAccepts(cc, d, sl, cons, si) }

	var heads []indexDoc
	sctx := context.WithValue(WithFeatures(ctx, Features{Hardware: true}), slotSearchKey{}, true)
	sr := in.SearchRequest{Query: sl.Query, Config: si.cfg, ClientID: si.req.ClientID, Limit: slotSearchLimit, Filters: cons.filters}
	if r, err := s.SearchWith(sctx, "", sr, SearchOptions{NoLog: true}); err == nil {
		seenFam := map[string]bool{}
		for _, it := range r.Items {
			cands := append([]out.SearchItem{it}, it.Variants...)
			// Позиция с метрикой стратегии слота (per_user — на пользователя) раньше остальных вариантов семейства.
			sort.SliceStable(cands, func(i, j int) bool {
				return metricFits(lx.docs[cands[i].ProductID], sl.QuantityStrategy) && !metricFits(lx.docs[cands[j].ProductID], sl.QuantityStrategy)
			})
			picked := false
			for _, c := range cands {
				d, found := lx.docs[c.ProductID]
				if !found || !ok(d) {
					continue
				}
				if !seenFam[d.FamilyID] {
					seenFam[d.FamilyID] = true
					heads = append(heads, d)
				}
				picked = true
				break
			}
			if !picked {
				res.trace.Rejected++
			}
		}
		if len(heads) > 0 {
			res.trace.Source = slotFromSearch
		}
	}
	if len(heads) == 0 {
		heads = catalogHeads(lx, ok)
		if len(heads) > 0 {
			res.trace.Source = slotFromCatalog
		}
	}
	if len(heads) == 0 {
		res.trace.Reason = slotNoCandidates
		return res
	}
	if len(avoid) > 0 {
		needsMore := func(d indexDoc) bool {
			for _, a := range avoid {
				if strings.EqualFold(cc.attr(d.ProductID, a.key), a.value) {
					return true
				}
			}
			return false
		}
		sort.SliceStable(heads, func(i, j int) bool { return !needsMore(heads[i]) && needsMore(heads[j]) })
	}
	// Альтернативы — того же рода, что выбранная позиция: та же метрика лицензии (у серверной ОС «на ядро» CAL «на
	// пользователя» — не замена, а другой товар).
	first := heads[0]
	alts := []indexDoc{first}
	for _, h := range heads[1:] {
		if sameMetric(h, first) {
			alts = append(alts, h)
		}
	}
	heads = alts
	for i, h := range heads {
		if i > maxSlotAlternatives {
			break
		}
		d, q, rule := fitPack(lx, h, sl.QuantityStrategy, si.seats, ok)
		item := solutionItem(d)
		res.slot.Quantities[d.ProductID] = q
		if i == 0 {
			res.slot.Item, res.slot.Quantity = &item, q
			res.trace.ProductID, res.trace.Quantity, res.trace.QuantityRule = d.ProductID, q, rule
			dd := d
			res.doc = &dd
			continue
		}
		res.slot.Alternatives = append(res.slot.Alternatives, item)
		res.trace.Alternatives = append(res.trace.Alternatives, d.ProductID)
	}
	return res
}

// slotAccepts — позиция подходит слоту: категория, условия слота, условия запроса; продления и снятые с продажи —
// нет (в комплект берут новую лицензию).
func slotAccepts(cc *commerceCatalog, d indexDoc, sl domain.SolutionSlot, cons slotConstraints, si solutionInput) bool {
	if d.CategoryID != sl.CategoryID && !contains(d.CategoryIDs, sl.CategoryID) {
		return false
	}
	if d.LicenseType == domain.LicenseRenewal || docPriceMode(d) == domain.PriceUnavailable || notNewLicense[d.LicenseKind] {
		return false
	}
	if notNewLicenseTitle.MatchString(strings.ToLower(d.Title)) {
		return false // продление, обновление, носитель или лицензия для иностранных лиц без разметки вида в фиде
	}
	if feedOrg[d.LicenseOrgType] != "" {
		return false // вариант фида для образования, госучреждений или НКО: условий организации шаблон не знает
	}
	if si.origin == "ru" && d.Origin != "ru" {
		return false
	}
	if contains(si.vendors, d.VendorID) {
		return false
	}
	for k, f := range cons.numeric {
		v, has := d.Specs[k]
		if !has || !filterHolds(f, v) {
			return false
		}
	}
	if f := cons.version; f != nil {
		have, err1 := strconv.ParseFloat(cc.attr(d.ProductID, "version"), 64)
		want, err2 := strconv.ParseFloat(fmt.Sprint(f.Value), 64)
		if err1 != nil || err2 != nil {
			return false
		}
		if f.Op == "gte" && have < want || f.Op != "gte" && have != want {
			return false
		}
	}
	if cons.compat != "" {
		st := cc.ModelCompat(d.ProductID, cons.compat).State
		if st != domain.CompatVerified && st != domain.CompatConditional {
			return false
		}
	}
	return true
}

// notNewLicense — вид лицензии фида, который в комплект не берут: продление, обновление, переход, техподдержка,
// модуль, курс (нужна новая полная лицензия).
var notNewLicense = map[string]bool{domain.LicenseKindRenewal: true, domain.LicenseKindUpgrade: true,
	domain.LicenseKindMigration: true, domain.LicenseKindSupport: true, domain.LicenseKindModule: true,
	domain.LicenseKindCourse: true}

// notNewLicenseTitle — то же по названию позиции, когда вид лицензии в фиде не размечен.
var notNewLicenseTitle = regexp.MustCompile(`продлени|на обновление|иностранных лиц|дистрибутив|media ?pack|медиапак`)

// filterHolds — числовое условие слота.
func filterHolds(f in.Filter, v float64) bool {
	num := func(x any) (float64, bool) {
		switch n := x.(type) {
		case float64:
			return n, true
		case int:
			return float64(n), true
		}
		return 0, false
	}
	a, ok := num(f.Value)
	if !ok {
		return false
	}
	switch f.Op {
	case "gte":
		return v >= a
	case "lte":
		return v <= a
	case "range":
		b, ok := num(f.Value2)
		return v >= a && (!ok || v <= b)
	case "not":
		return v != a
	}
	return v == a
}

// catalogHeads — позиции категории под условиями слота, по популярности, по одной на семейство.
func catalogHeads(lx *searchLexicon, ok func(indexDoc) bool) []indexDoc {
	var all []indexDoc
	for _, d := range lx.docs {
		if ok(d) {
			all = append(all, d)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Popularity != all[j].Popularity {
			return all[i].Popularity > all[j].Popularity
		}
		return all[i].ProductID < all[j].ProductID
	})
	var heads []indexDoc
	seen := map[string]bool{}
	for _, d := range all {
		if !seen[d.FamilyID] {
			seen[d.FamilyID] = true
			heads = append(heads, d)
		}
	}
	return heads
}

// seatStrategy — количество слота считается от числа мест.
func seatStrategy(s string) bool {
	return s == "per_seat" || s == "per_user" || s == "per_device"
}

// slotQuantity — сколько штук позиции заказать: по числу мест — ⌈N / мест в позиции⌉, иначе 1.
func slotQuantity(strategy string, d *indexDoc, seats int) int {
	if !seatStrategy(strategy) {
		return 1
	}
	// Оборудование — штуками; ступень цены фида («от 10 до 14 узлов») — цена за единицу: N мест — N единиц.
	if d == nil || d.LicenseQuantity == nil || *d.LicenseQuantity <= 1 || isPerUnit(*d) || d.Kind == domain.KindHardware ||
		d.LicenseTierMin != nil {
		return seats
	}
	return (seats + *d.LicenseQuantity - 1) / *d.LicenseQuantity
}

// fitPack — фасовка внутри семейства и редакции выбранной позиции: дешевле всего закрыть N мест (при равенстве —
// меньше лишних мест). Без цены у части фасовок сравниваются только с ценой; цен нет ни у одной — выбранная.
func fitPack(lx *searchLexicon, head indexDoc, strategy string, seats int, ok func(indexDoc) bool) (indexDoc, int, string) {
	if !seatStrategy(strategy) || head.Kind == domain.KindHardware {
		return head, slotQuantity(strategy, &head, seats), strategy
	}
	edition := docEdition(head)
	best, bestCost, bestWaste := head, math.Inf(1), math.MaxInt
	for _, pid := range lx.families[head.FamilyID] {
		d := lx.docs[pid]
		if !ok(d) || docEdition(d) != edition || d.LicenseType != head.LicenseType || !sameDuration(d, head) || !sameMetric(d, head) {
			continue
		}
		if isOffer(d) && !isOffer(head) {
			continue // вариант под сделку или организацию (контур доводки) слоту шаблона не подставляется
		}
		if d.PriceRub == nil || !tierHolds(d, seats) {
			continue
		}
		q := slotQuantity(strategy, &d, seats)
		per := 1
		if d.LicenseQuantity != nil && *d.LicenseQuantity > 1 && !isPerUnit(d) {
			per = *d.LicenseQuantity
		}
		cost, waste := *d.PriceRub*float64(q), q*per-seats
		if cost < bestCost || cost == bestCost && waste < bestWaste {
			best, bestCost, bestWaste = d, cost, waste
		}
	}
	if math.IsInf(bestCost, 1) && !tierHolds(head, seats) {
		// Ни одна ступень цены не берёт N мест (меньше нижней ступени): берётся нижняя ступень семейства — меньше
		// её минимума продавец не продаёт.
		var low *indexDoc
		for _, pid := range lx.families[head.FamilyID] {
			d := lx.docs[pid]
			if !ok(d) || docEdition(d) != edition || d.LicenseType != head.LicenseType || !sameDuration(d, head) ||
				!sameMetric(d, head) || d.PriceRub == nil || d.LicenseTierMin == nil || *d.LicenseTierMin <= seats {
				continue
			}
			if low == nil || *d.LicenseTierMin < *low.LicenseTierMin || *d.LicenseTierMin == *low.LicenseTierMin && *d.PriceRub < *low.PriceRub {
				dd := d
				low = &dd
			}
		}
		if low != nil {
			return *low, *low.LicenseTierMin, strategy + ":tier_min"
		}
	}
	rule := strategy
	if best.ProductID != head.ProductID {
		rule = strategy + ":cheapest_pack"
	}
	return best, slotQuantity(strategy, &best, seats), rule
}

// tierHolds — у позиции нет ступени цены или N мест в её ступени («от 10 до 14 узлов» — для 12 мест).
func tierHolds(d indexDoc, seats int) bool {
	return d.LicenseTierMin == nil || seats >= *d.LicenseTierMin && (d.LicenseTierMax == nil || seats <= *d.LicenseTierMax)
}

func docEdition(d indexDoc) string { return strings.ToLower(strings.TrimSpace(d.LicenseEdition)) }

func sameDuration(a, b indexDoc) bool {
	if a.LicenseDuration == nil || b.LicenseDuration == nil {
		return a.LicenseDuration == nil && b.LicenseDuration == nil
	}
	return *a.LicenseDuration == *b.LicenseDuration
}

// solutionItem — позиция слота для ответа (как в выдаче, без группы и скора).
func solutionItem(d indexDoc) out.SearchItem {
	return out.SearchItem{ProductID: d.ProductID, FamilyID: d.FamilyID, Title: d.Title, Vendor: d.Vendor, Category: d.Category,
		LicenseLine: licenseLine(d), PriceRub: d.PriceRub, PriceOnRequest: d.PriceOnRequest, MatchesQuery: true,
		Kind: docKind(d), MPN: d.MPNRaw, PriceMode: docPriceMode(d), Required: []out.Companion{}}
}

// withSeats — число мест из запроса (solutionSeats).
func (si solutionInput) withSeats(query string, licQty, hwQty int) solutionInput {
	si.seats, si.qtySrc = solutionSeats(query, licQty, hwQty)
	return si
}

// excludedVendors — вендоры, исключённые запросом («не Dell»); разбора оборудования нет — пусто.
func (p *hwPlan) excludedVendors() []string {
	if p == nil {
		return nil
	}
	return p.vendorNot
}

// purchaseQuantity — количество закупки оборудования («20 ноутбуков»); нет — 0.
func (p *hwPlan) purchaseQuantity() int {
	if p == nil {
		return 0
	}
	return p.quantity
}

// metricFits — метрика позиции подходит стратегии слота (per_user — user, per_device — device); иначе любая.
func metricFits(d indexDoc, strategy string) bool {
	want := map[string]string{"per_user": domain.MetricUser, "per_device": domain.MetricDevice}[strategy]
	return want == "" || d.LicenseMetric != nil && *d.LicenseMetric == want
}

// sameMetric — у позиций одна метрика лицензии (или обе без неё).
func sameMetric(a, b indexDoc) bool {
	if a.LicenseMetric == nil || b.LicenseMetric == nil {
		return a.LicenseMetric == nil && b.LicenseMetric == nil
	}
	return *a.LicenseMetric == *b.LicenseMetric
}
