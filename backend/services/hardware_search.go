package services

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Шаг hardware в конвейере B3 (search_b3.go): сущности, чипы, маршрут, фильтры OpenSearch, точные поля
// (парт-номер, модель) как отдельный список RRF, отказы по условиям, совместимость позиций, фасеты, трасса.

// Поля чипов оборудования.
const (
	fieldMPN       = "mpn"
	fieldCompat    = "compatible_with"
	fieldModel     = "model_id"
	fieldPrice     = "price_rub"
	fieldQuantity  = "quantity"
	fieldVendorNot = "vendor_not"
	stepHardware   = "hardware"
)

// Причины отказа оборудования (trace.rejected[].reason).
const (
	rejectSpec           = "spec_mismatch"
	rejectPrice          = "price_mismatch"
	rejectVendorNot      = "vendor_excluded"
	rejectHWIncompatible = "compat_incompatible"
	rejectCompatUnknown  = "compat_unknown"
)

// Точные поля в RRF: список парт-номера или модели весит exactListWeight обычных списков — точный товар
// не уступает место близким по смыслу (сдвиг весов к точным полям).
const (
	exactListWeight = 3.0
	exactListTop    = 20
	// structListWeight — вес списка «по условиям» (structuredHits): как у BM25 и векторов.
	structListWeight = 1.0
	compatListWeight = 2.0
)

// applyEntities — сущности запроса под разбор оборудования: категория, исключённые вендоры, цель «для» не
// делает вендора цели строгим фильтром (он забран из текста), неясные слова — без парт-номеров.
func (p *hwPlan) applyEntities(ent *queryEntities) {
	if p == nil {
		return
	}
	if ent.categoryByAlias && p.category != "" && p.category != ent.category {
		if p.hx.hwCats[ent.category] {
			p.category = ent.category // «ssd» → SSD, а не «Жесткие диски» по синониму
		} else {
			p.category = "" // «удаленный доступ» — ПО, а не «Точки доступа»
		}
	}
	if !p.active() && !p.zoneCategory {
		return
	}
	switch {
	case p.category != "":
		ent.category, ent.categoryByWords = p.category, true
	case (p.zoneCategory || p.multiCat) && p.hx.hwCats[ent.category]:
		ent.category = "" // «сумка для ноутбука»: ноутбук — не то, что ищут
	}
	if p.mpn != nil {
		ent.noise = false // «P0092»: одна буква, но это начало парт-номера, а не шум
	}
	if len(p.vendorNot) > 0 {
		var vs []string
		for _, v := range ent.vendors {
			if !contains(p.vendorNot, v) {
				vs = append(vs, v)
			}
		}
		ent.vendors = vs
	}
	if len(ent.unknown) > 0 && p.active() {
		// Парт-номера, модели и слова категорий оборудования в любой форме («мониторов») — не «товара нет».
		var unk []string
		for _, u := range ent.unknown {
			if !contains(p.protected, u) && p.hx.categoryOfWord(u) == "" {
				unk = append(unk, u)
			}
		}
		ent.unknown = unk
	}
	// Названный вендор или семейство без единого совместимого товара (servermax для gen11): показываются
	// позиции с неизвестной совместимостью — с пометкой, несовместимые — никогда.
	if p.compatMode == "filter" && (len(ent.vendors) > 0 || len(ent.named) > 0) {
		any := false
		for _, ids := range p.hx.products {
			for _, id := range ids {
				d := p.lx.docs[id]
				if (contains(ent.vendors, d.VendorID) || contains(ent.named, d.FamilyID)) &&
					(p.category == "" || d.CategoryID == p.category) && p.compatOK(id) {
					any = true
				}
			}
		}
		p.allowUnknown = !any
	}
}

// chips — чипы и найденные атрибуты оборудования вдобавок к чипам правил.
func (p *hwPlan) chips(chips []out.Chip, found []domain.FoundAttribute) ([]out.Chip, []domain.FoundAttribute) {
	if !p.active() {
		return chips, found
	}
	add := func(c out.Chip) {
		chips = append(chips, c)
		found = append(found, domain.FoundAttribute{Field: c.Field, Value: c.Value, Strict: c.Strict, Source: domain.ParseRules})
	}
	if p.mpn != nil {
		v := strings.ToUpper(p.mpn.Text)
		if !p.mpn.Prefix && len(p.mpn.Products) > 0 {
			if d := p.lx.docs[p.mpn.Products[0]]; d.MPNRaw != "" {
				v = d.MPNRaw
			}
		}
		label := "Парт-номер " + v
		if p.mpn.Prefix {
			label = "Парт-номер начинается с " + v
		}
		add(out.Chip{Field: fieldMPN, Value: v, Label: label, Strict: true})
	}
	if p.model != "" {
		add(out.Chip{Field: fieldModel, Value: p.model, Label: "Модель " + p.hx.modelByID[p.model].name})
	}
	for _, c := range p.conds {
		if c.Source != "query" {
			continue // фасеты показываются фасетами, а не чипами
		}
		add(p.condChip(c))
	}
	if p.priceMin != nil {
		add(out.Chip{Field: fieldPrice, Value: fmtValue(*p.priceMin), Op: "gte", Unit: "₽", Strict: true,
			Label: "Цена от " + hwNum(*p.priceMin) + " ₽"})
	}
	if p.priceMax != nil {
		add(out.Chip{Field: fieldPrice, Value: fmtValue(*p.priceMax), Op: "lte", Unit: "₽", Strict: true,
			Label: "Цена до " + hwNum(*p.priceMax) + " ₽"})
	}
	for _, v := range p.vendorNot {
		name := p.lx.vendorName[v]
		if name == "" {
			name = v
		}
		add(out.Chip{Field: fieldVendorNot, Value: v, Op: "not", Label: "Не " + name, Strict: true})
	}
	if p.quantity > 0 {
		add(out.Chip{Field: fieldQuantity, Value: itoa(p.quantity), Label: itoa(p.quantity) + " шт"})
	}
	if p.target != "" {
		name := p.hx.modelByID[p.target].name
		switch {
		case p.compatMode == "filter" && !p.allowUnknown:
			add(out.Chip{Field: fieldCompat, Value: p.target, Label: "Совместимо с " + name, Strict: true})
		default:
			add(out.Chip{Field: fieldCompat, Value: p.target, Label: "Совместимость с " + name + " не подтверждена"})
		}
	}
	return chips, found
}

// condChip — чип условия: «Память ≥ 32 ГБ», «Диагональ 24–27″», «Без ОС», «Тип памяти: DDR4».
func (p *hwPlan) condChip(c hwCond) out.Chip {
	a := p.hx.attrs[c.Key]
	label := a.label
	if label == "" {
		label = c.Key
	}
	if c.typ != "number" {
		switch {
		case strings.HasPrefix(foldText(c.Str), "без"):
			return out.Chip{Field: c.Key, Value: c.Str, Op: c.Op, Label: strings.ToUpper(c.Str[:2]) + c.Str[2:], Strict: true}
		case c.Str == "true" && c.Op == "not":
			return out.Chip{Field: c.Key, Value: c.Str, Op: c.Op, Label: "Без " + label, Strict: true}
		case c.Str == "true":
			return out.Chip{Field: c.Key, Value: c.Str, Op: c.Op, Label: label, Strict: true}
		}
		return out.Chip{Field: c.Key, Value: c.Str, Op: c.Op, Label: label + ": " + opSymbol(c.Op) + c.Str, Strict: true}
	}
	unit := a.unit
	sep := " "
	if unit == "″" || unit == "" {
		sep = ""
	}
	val := hwNum(c.Value)
	value := fmtValue(c.Value)
	if c.Op == "range" {
		val = hwNum(c.Value) + "–" + hwNum(c.Value2)
		value = fmtValue(c.Value) + "-" + fmtValue(c.Value2)
	}
	return out.Chip{Field: c.Key, Value: value, Op: c.Op, Unit: unit, Strict: true,
		Label: strings.TrimSpace(label + " " + opSymbol(c.Op) + val + sep + unit)}
}

// route — запрос по оборудованию разбирается правилами: парт-номер — маршрут A (точный), остальное не уходит на
// маршрут D (LLM в момент запроса для оборудования не вызывается).
func (p *hwPlan) route(route string, signals []string, ent queryEntities) (string, []string) {
	if !p.active() || ent.noise || ent.nonProduct {
		return route, signals
	}
	if p.mpn != nil {
		return domain.RouteExact, []string{"mpn"}
	}
	if route == domain.RouteIntent {
		return domain.RouteNormalized, append(signals, stepHardware)
	}
	return route, signals
}

// holdsRoute — пустой текстовый поиск не переводит запрос оборудования на маршрут D.
func (p *hwPlan) holdsRoute() bool { return p.active() }

// filters — условия для OpenSearch (и BM25, и kNN): категория, числовые характеристики, цена, исключённые
// вендоры, совместимость. Перечислимые атрибуты проверяются в коде (reject): в индексе они — текст.
func (p *hwPlan) filters() []any {
	if !p.active() {
		return nil
	}
	var f, not []any
	if p.category != "" {
		f = append(f, map[string]any{"term": map[string]any{"category_ids": p.category}})
	}
	for _, c := range p.conds {
		if c.typ != "number" {
			continue
		}
		field := "specs." + c.Key
		switch c.Op {
		case "gte":
			f = append(f, rangeQ(field, map[string]any{"gte": c.Value}))
		case "lte":
			f = append(f, rangeQ(field, map[string]any{"lte": c.Value}))
		case "range":
			f = append(f, rangeQ(field, map[string]any{"gte": c.Value, "lte": c.Value2}))
		case "not":
			not = append(not, rangeQ(field, map[string]any{"gte": c.Value, "lte": c.Value}))
		default:
			tol := tolerance(c)
			f = append(f, rangeQ(field, map[string]any{"gte": c.Value - tol, "lte": c.Value + tol}))
		}
	}
	price := map[string]any{}
	if p.priceMin != nil {
		price["gte"] = *p.priceMin
	}
	if p.priceMax != nil {
		price["lte"] = *p.priceMax
	}
	if len(price) > 0 {
		f = append(f, rangeQ("price_rub", price))
	}
	if len(p.vendorNot) > 0 {
		not = append(not, map[string]any{"terms": map[string]any{"vendor_id": p.vendorNot}})
	}
	if p.compatMode == "filter" {
		var ok, bad []string
		for id, r := range p.allowed {
			switch {
			case r.State == domain.CompatVerified || r.State == domain.CompatConditional:
				ok = append(ok, id)
			case r.State == domain.CompatIncompatible:
				bad = append(bad, id)
			}
		}
		sort.Strings(ok)
		sort.Strings(bad)
		switch {
		case p.allowUnknown && len(bad) > 0:
			not = append(not, map[string]any{"terms": map[string]any{"product_id": bad}})
		case !p.allowUnknown:
			if ok == nil {
				ok = []string{}
			}
			f = append(f, map[string]any{"terms": map[string]any{"product_id": ok}})
		}
	}
	if len(not) > 0 {
		f = append(f, map[string]any{"bool": map[string]any{"must_not": not}})
	}
	return f
}

func rangeQ(field string, r map[string]any) map[string]any {
	return map[string]any{"range": map[string]any{field: r}}
}

// exactQueryBody — точные поля: парт-номер (keyword целиком, префиксы, триграммы) или модель (model_id).
func (p *hwPlan) exactQueryBody(filters []any) map[string]any {
	var should []any
	if p.mpn != nil {
		should = append(should,
			map[string]any{"term": map[string]any{"mpn_normalized": map[string]any{"value": p.mpn.Norm, "boost": 10}}},
			map[string]any{"match": map[string]any{"mpn_normalized.prefix": map[string]any{"query": p.mpn.Norm, "boost": 4}}},
			map[string]any{"match": map[string]any{"mpn_normalized.tri": map[string]any{"query": p.mpn.Norm, "minimum_should_match": "80%"}}},
		)
	}
	if p.model != "" {
		should = append(should, map[string]any{"term": map[string]any{"model_id": map[string]any{"value": p.model, "boost": 5}}})
	}
	return map[string]any{
		"size":    exactListTop,
		"_source": sourceExcludes,
		"query": map[string]any{"bool": map[string]any{"should": should, "minimum_should_match": 1,
			"filter": append([]any{}, filters...)}},
		"sort": []any{"_score", map[string]any{"popularity": "desc"}, map[string]any{"product_id": "asc"}},
	}
}

// exactRetrieve — список точных полей. Сбой — ошибка шага в трассе, поиск продолжается без списка.
func (s *Services) exactRetrieve(ctx context.Context, p *hwPlan, filters []any) {
	if p == nil || p.mpn == nil && p.model == "" {
		return
	}
	t := time.Now()
	res, err := s.search.Search(ctx, s.alias, p.exactQueryBody(filters))
	if err == nil {
		p.exactHits, _, err = parseHits(res)
	}
	if err != nil {
		p.err = "точные поля: " + err.Error()
		p.exactHits = nil
	}
	p.ms += msSince(t)
}

// structuredHits — список «по условиям» из словаря в памяти: позиции категории, которые проходят все условия
// оборудования (характеристики, цена, совместимость); порядок — подтверждённая совместимость раньше условной,
// затем популярность. Текст карточки может не содержать слов запроса («память» при «Модуль RDIMM …»), а
// структурное совпадение — это и есть ответ на «память 16 гб для R740».
func (p *hwPlan) structuredHits() []domain.RankedHit {
	if !p.active() || p.compatMode != "filter" && (p.category == "" || len(p.conds) == 0) {
		return nil
	}
	type cand struct {
		id    string
		state int
		pop   float64
	}
	var cs []cand
	for cat, ids := range p.hx.products {
		if p.category != "" && cat != p.category {
			continue
		}
		for _, id := range ids {
			d := p.lx.docs[id]
			if r, _ := p.reject(d); r != "" {
				continue
			}
			st := 0
			if p.compatMode == "filter" {
				switch p.cc.ModelCompat(id, p.target).State {
				case domain.CompatVerified:
					st = 2
				case domain.CompatConditional:
					st = 1
				}
			}
			cs = append(cs, cand{id, st, d.Popularity})
		}
	}
	sort.Slice(cs, func(i, j int) bool {
		if cs[i].state != cs[j].state {
			return cs[i].state > cs[j].state
		}
		if cs[i].pop != cs[j].pop {
			return cs[i].pop > cs[j].pop
		}
		return cs[i].id < cs[j].id
	})
	var hits []domain.RankedHit
	for i, c := range cs {
		if i == exactListTop {
			break
		}
		hits = append(hits, domain.RankedHit{ProductID: c.id, Rank: i + 1})
	}
	return hits
}

// signalCompatVerified — позиция выше в выдаче «X для Y»: совместимость подтверждена, а не условна.
const signalCompatVerified = "compat_verified"

// compatTier — ступень позиции в запросе «X для Y»: 1 — совместимость подтверждена, 0 — остальное.
func (p *hwPlan) compatTier(d indexDoc) int {
	if p == nil || p.compatMode != "filter" || !p.active() {
		return 0
	}
	if p.cc.ModelCompat(d.ProductID, p.target).State == domain.CompatVerified {
		return 1
	}
	return 0
}

// fuse — точный список в RRF с весом exactListWeight. Найденное только точным полем считается лексическим
// попаданием (bm25_rank — ранг в точном списке): порог «только векторы» к нему не относится.
func (p *hwPlan) fuse(ranked []fused, k int) []fused {
	if p == nil {
		return ranked
	}
	p.structHits = p.structuredHits()
	if len(p.exactHits) == 0 && len(p.structHits) == 0 {
		return ranked
	}
	idx := map[string]int{}
	for i, f := range ranked {
		idx[f.ProductID] = i
	}
	add := func(hits []domain.RankedHit, w float64) {
		for _, h := range hits {
			rank := h.Rank
			i, ok := idx[h.ProductID]
			if !ok {
				ranked = append(ranked, fused{ProductID: h.ProductID})
				i = len(ranked) - 1
				idx[h.ProductID] = i
			}
			ranked[i].RRF += w / float64(k+rank)
			if ranked[i].BM25Rank == nil {
				ranked[i].BM25Rank = &rank
			}
		}
	}
	add(p.exactHits, exactListWeight)
	w := structListWeight
	if p.compatMode == "filter" {
		w = compatListWeight // «X для Y»: подтверждённая совместимость выше условной и выше текстового совпадения
	}
	add(p.structHits, w)
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].RRF != ranked[j].RRF {
			return ranked[i].RRF > ranked[j].RRF
		}
		return ranked[i].ProductID < ranked[j].ProductID
	})
	return ranked
}

// reject — нарушенное условие оборудования у позиции; "" — допустима. Проверяется и то, что уже отфильтровал
// OpenSearch: варианты семейства в выдачу попадают без поиска, и условие должно держаться и для них.
func (p *hwPlan) reject(d indexDoc) (string, string) {
	if !p.active() {
		return "", ""
	}
	for _, c := range p.conds {
		if c.typ == "number" {
			v, ok := d.Specs[c.Key]
			if !ok || !numOK(c, v) {
				have := "нет значения"
				if ok {
					have = hwNum(v)
				}
				return rejectSpec, fmt.Sprintf("%s: %s, нужно %s", c.Key, have, p.condChip(c).Label)
			}
			continue
		}
		have := p.hx.docAttrs[d.ProductID][c.Key]
		eq := strings.EqualFold(have, c.Str)
		if c.Op == "not" && eq || c.Op != "not" && !eq {
			return rejectSpec, fmt.Sprintf("%s: «%s», нужно %s", c.Key, have, p.condChip(c).Label)
		}
	}
	if p.priceMax != nil && (d.PriceRub == nil || *d.PriceRub > *p.priceMax) || p.priceMin != nil && (d.PriceRub == nil || *d.PriceRub < *p.priceMin) {
		return rejectPrice, "цена вне условия запроса"
	}
	if contains(p.vendorNot, d.VendorID) {
		return rejectVendorNot, "вендор исключён в запросе"
	}
	if p.compatMode == "filter" {
		r := p.allowed[d.ProductID]
		if _, ok := p.allowed[d.ProductID]; !ok {
			r = p.cc.ModelCompat(d.ProductID, p.target)
		}
		switch r.State {
		case domain.CompatIncompatible:
			return rejectHWIncompatible, "несовместимо с " + p.hx.modelByID[p.target].name + ": " + r.Condition
		case domain.CompatUnknown:
			if p.allowUnknown && p.category == "" && !p.hx.components[d.CategoryID] {
				return rejectCompatUnknown, "не комплектующее к " + p.hx.modelByID[p.target].name
			}
			if !p.allowUnknown {
				return rejectCompatUnknown, "совместимость с " + p.hx.modelByID[p.target].name + " не подтверждена"
			}
		}
	}
	return "", ""
}

func numOK(c hwCond, v float64) bool {
	const eps = 1e-6
	switch c.Op {
	case "gte":
		return v >= c.Value-eps
	case "lte":
		return v <= c.Value+eps
	case "range":
		return v >= c.Value-eps && v <= c.Value2+eps
	case "not":
		return v < c.Value-eps || v > c.Value+eps
	}
	tol := tolerance(c) + eps
	return v > c.Value-tol && v < c.Value+tol
}

// approxTolerance — допуск равенства у характеристик, которые называют округлённо: «монитор 24 дюйма» — это
// 23,8″, «ноутбук 16 дюймов» — и 15,6″.
var approxTolerance = map[string]float64{"screen_inch": 0.7}

// approxShare — допуск в долях значения: объём диска и мощность ИБП называют круглыми числами («ssd 1 тб» —
// это и 960 ГБ, «ибп 3 квт» — и 2 700 Вт), а в каталоге — точные.
var approxShare = map[string]float64{"storage_gb": 0.1, "power_w": 0.12}

func tolerance(c hwCond) float64 {
	return max(approxTolerance[c.Key], approxShare[c.Key]*c.Value)
}

// decorate — совместимость позиций с целью запроса, фасеты категории, честная пустая выдача по условиям.
func (p *hwPlan) decorate(resp *out.SearchResponse) {
	if !p.active() {
		return
	}
	if p.target != "" {
		set := func(it *out.SearchItem) {
			r := p.cc.ModelCompat(it.ProductID, p.target)
			it.Compatibility = &out.ItemCompatibility{TargetModel: p.target, State: r.State, Condition: r.Condition}
		}
		for i := range resp.Items {
			set(&resp.Items[i])
			for j := range resp.Items[i].Variants {
				set(&resp.Items[i].Variants[j])
			}
		}
	}
	resp.Facets = p.facets(resp)
	if resp.Empty != nil && len(resp.Items) == 0 && (len(p.conds) > 0 || p.compatMode == "filter" || p.priceMax != nil || p.priceMin != nil) {
		resp.Empty.Message = fmt.Sprintf("По условиям запроса «%s» позиций нет — снимите одно из условий", resp.Query)
	}
}

// hardwareStep — шаг трассы hardware.
type hardwareStep struct {
	domain.StepBase
	Error        string             `json:"error,omitempty"`
	MPNDetected  *string            `json:"mpn_detected"`
	MPN          *hwMPN             `json:"mpn,omitempty"`
	Units        []hwUnitTrace      `json:"units"`
	CompatTarget *string            `json:"compat_target"`
	CompatMode   string             `json:"compat_mode,omitempty"`
	Model        string             `json:"model,omitempty"`
	Category     string             `json:"category,omitempty"`
	Quantity     int                `json:"quantity,omitempty"`
	PriceMin     *float64           `json:"price_min,omitempty"`
	PriceMax     *float64           `json:"price_max,omitempty"`
	VendorNot    []string           `json:"vendor_not"`
	Negations    []string           `json:"negations"`
	Protected    []string           `json:"protected"`
	ExactHits    []domain.RankedHit `json:"exact_hits"`
	StructHits   []domain.RankedHit `json:"structured_hits"`
	// ExactQuery — точный запрос (парт-номер): маржа на такую выдачу не влияет.
	ExactQuery bool `json:"exact_query"`
}

// step — шаг трассы; nil — флаг выключен (шага нет, трасса как в контуре поиска).
func (p *hwPlan) step() any {
	if p == nil {
		return nil
	}
	st := hardwareStep{StepBase: domain.StepBase{Name: stepHardware, Ms: p.ms}, Error: p.err, Units: p.unitTrace(),
		VendorNot: p.vendorNot, Negations: p.negations, Protected: p.protected, ExactHits: p.exactHits, StructHits: p.structHits,
		Model: p.model, Category: p.category, Quantity: p.quantity, PriceMin: p.priceMin, PriceMax: p.priceMax,
		CompatMode: p.compatMode, MPN: p.mpn, ExactQuery: p.exactQuery()}
	if !p.active() && p.err == "" {
		st.Skipped, st.Reason = true, "no_hardware_signals"
	}
	if p.mpn != nil {
		st.MPNDetected = &p.mpn.Norm
	}
	if p.target != "" {
		st.CompatTarget = &p.target
	}
	for _, s := range []*[]string{&st.VendorNot, &st.Negations, &st.Protected} {
		if *s == nil {
			*s = []string{}
		}
	}

	if st.StructHits == nil {
		st.StructHits = []domain.RankedHit{}
	}
	if st.ExactHits == nil {
		st.ExactHits = []domain.RankedHit{}
	}
	return st
}

// exactHitsOrNil — список точных полей; nil-план — пусто.
func (p *hwPlan) exactHitsOrNil() []domain.RankedHit {
	if p == nil {
		return nil
	}
	return p.exactHits
}

// hwUnitTrace — условие в трассе (units: text, key, op, value, unit): у числа value — число, у
// перечислимого — строка, у bool — true/false; цена — ключ price_rub.
type hwUnitTrace struct {
	Text   string `json:"text"`
	Key    string `json:"key"`
	Op     string `json:"op"`
	Value  any    `json:"value"`
	Value2 any    `json:"value2,omitempty"`
	Unit   string `json:"unit"`
	Source string `json:"source"`
}

func (p *hwPlan) unitTrace() []hwUnitTrace {
	res := []hwUnitTrace{}
	for _, c := range p.conds {
		u := hwUnitTrace{Text: c.Text, Key: c.Key, Op: c.Op, Unit: c.Unit, Source: c.Source}
		switch {
		case c.typ == "number":
			u.Value = c.Value
			if c.Op == "range" {
				u.Value2 = c.Value2
			}
		case c.Str == "true" || c.Str == "false":
			u.Value = c.Str == "true"
		default:
			u.Value = c.Str
		}
		res = append(res, u)
	}
	if p.priceMin != nil {
		res = append(res, hwUnitTrace{Key: fieldPrice, Op: "gte", Value: *p.priceMin, Unit: "₽", Source: "query"})
	}
	if p.priceMax != nil {
		res = append(res, hwUnitTrace{Key: fieldPrice, Op: "lte", Value: *p.priceMax, Unit: "₽", Source: "query"})
	}
	return res
}

// targetless — запрос оборудования без цели совместимости («память для r740»): там выдачу решает совместимость.
func (p *hwPlan) targetless() bool { return p == nil || p.target == "" }
