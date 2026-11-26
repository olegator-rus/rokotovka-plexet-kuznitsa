package services

import (
	"context"
	"sort"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
)

// Страница результатов контура выдачи (флаг FEATURE_RESULTS_V3): лицензионные
// фильтры и фасеты, сортировки, группировка, уточнения, счётчики. Работает над семействами, которые уже
// построило ранжирование B3 (search_b3.go), — в памяти, без LLM и векторов. Флаг выключен, вложенный поиск слота
// решения или лестница контура коммерции — плана нет, выдача и ответ как в контуре коммерции. Сортировка «Рекомендуемые» и
// группировка по семейству порядок не меняют: test контура поиска и лестница p2 считаются на том же порядке.

// Сортировки и группировки выдачи.
const (
	SortRecommended = "recommended"
	SortRelevance   = "relevance"
	SortPriceAsc    = "price_asc"
	SortPriceDesc   = "price_desc"
	SortDelivery    = "delivery"

	GroupFamily = "family"
	GroupNone   = "none"
)

// sortNotes — пояснение у сортировки: название не обещает метода, который не применяется.
var sortNotes = map[string]string{
	SortRecommended: "Рекомендуемые: по соответствию, затем популярность и выгода. Точные совпадения — первыми",
	SortRelevance:   "По соответствию запросу: без популярности, маржи и других коммерческих факторов",
	SortPriceAsc:    "Сначала дешевле — по цене «от» продукта. Позиции с ценой по запросу — в конце",
	SortPriceDesc:   "Сначала дороже — по цене «от» продукта. Позиции с ценой по запросу — в конце",
	SortDelivery:    "По способу поставки: электронная, затем OEM и физическая. Без данных о поставке — в конце",
}

// rejectFacet — позиция снята фильтром выдачи (trace.rejected).
const rejectFacet = "facet_filter"

// resultsPlan — параметры страницы результатов одного запроса.
type resultsPlan struct {
	sort, group string
	filters     []licFilter
	exact       bool
	// facets, refinements, counts — посчитаны в apply; all — семейства после фильтров и маржи (до limit).
	facets      []out.Facet
	refinements []out.Refinement
	counts      out.ResultCounts
	all         []*familyHit
	// filteredOut — фильтры сняли всё, хотя без них выдача была.
	filteredOut bool
}

// newResultsPlan — план страницы результатов; nil — флаг выключен или это вложенный поиск.
func (s *Services) newResultsPlan(ctx context.Context, req in.SearchRequest) *resultsPlan {
	if !s.featuresFor(ctx).ResultsV3 || isSlotSearch(ctx) {
		return nil
	}
	p := &resultsPlan{sort: SortRecommended, group: GroupFamily, filters: parseLicFilters(req.Filters)}
	if _, ok := sortNotes[req.Sort]; ok {
		p.sort = req.Sort
	}
	if req.Group == GroupNone {
		p.group = GroupNone
	}
	return p
}

// hwRequest — запрос для шага оборудования: фильтры страницы результатов он не видит (их считает план в
// памяти), кроме «не этот вендор» — его оборудование применяет ещё при отборе кандидатов.
func (p *resultsPlan) hwRequest(req in.SearchRequest) in.SearchRequest {
	if p == nil || len(req.Filters) == 0 {
		return req
	}
	var keep []in.Filter
	for _, f := range req.Filters {
		if !licenseFacetKeys[f.Key] || f.Key == facetVendor && f.Op == "not" {
			keep = append(keep, f)
		}
	}
	req.Filters = keep
	return req
}

// marginAllowed — сдвиг маржи только у «Рекомендуемых»: у остальных сортировок порядок задан явно.
func (p *resultsPlan) marginAllowed() bool { return p == nil || p.sort == SortRecommended }

// setExact — точный артикул или парт-номер: свёртка выключена, показан сам товар, уточнений нет.
func (p *resultsPlan) setExact(route string, signals []string, hw *hwPlan) {
	if p == nil {
		return
	}
	p.exact = hw.exactQuery() || route == domain.RouteExact && contains(signals, "sku")
}

// apply — фасеты и уточнения по выдаче до фильтров, затем фильтры по вариантам и сортировка.
func (p *resultsPlan) apply(rc rankCtx, fams []*familyHit, chips []out.Chip, query string, rejected *[]domain.RejectedHit) []*familyHit {
	if p == nil {
		return fams
	}
	if p.exact {
		p.group = GroupNone
	}
	named := map[string]bool{}
	for _, c := range chips {
		if k := chipFacet[c.Field]; k != "" {
			named[k] = true
		}
	}
	fams = p.representatives(rc, fams)
	fams = p.fill(rc, fams)
	p.facets = licenseFacets(rc.lx, fams, p.filters, named, isHardwareResult(fams), p.group == GroupNone)
	res := fams
	if len(p.filters) > 0 {
		res = make([]*familyHit, 0, len(fams))
		changed := false
		for _, fh := range fams {
			var heads []indexDoc
			for _, d := range fh.heads {
				if passAll(p.filters, d, "") {
					heads = append(heads, d)
				} else {
					addRejected(rejected, d.ProductID, rejectFacet, "снята фильтром выдачи")
				}
			}
			if len(heads) == 0 {
				continue
			}
			cp := *fh
			cp.heads = heads
			if heads[0].ProductID != fh.head.ProductID {
				cp.head, cp.headOK = heads[0], fh.fits[heads[0].ProductID]
				rc.score(&cp)
				changed = true
			}
			res = append(res, &cp)
		}
		if changed {
			sortFamilies(res)
			// Дописанные по фильтру категории или производителя — всегда после найденных поиском.
			sort.SliceStable(res, func(i, j int) bool { return !isFill(res[i]) && isFill(res[j]) })
		}
		p.filteredOut = len(res) == 0 && len(fams) > 0
	}

	taken := map[string]bool{}
	for k := range named {
		taken[k] = true
	}
	for _, f := range p.filters {
		taken[f.key] = true
	}
	if !p.exact {
		p.refinements = refinementsFromFamilies(rc.lx, query, res, taken, maxRefinements)
	}
	p.counts = out.ResultCounts{Families: len(res)}
	for _, fh := range res {
		p.counts.Variants += len(fh.heads)
	}
	p.sortFamilies(res)
	return res
}

// maxFillFamilies — сколько семейств категории или вендора дописывается к выдаче по фильтру из панели.
const maxFillFamilies = 40

// fill — переход из панели по категории или производителю (§11.7): фильтр category/vendor eq — это просьба
// показать товары этой категории (вендора), даже если текст запроса их не нашёл («антивирус» → «Антивирусы для
// бизнеса», где в названиях нет слова). Найденные поиском семейства остаются первыми, за ними — остальные
// семейства каталога под фильтром и условиями запроса, по популярности. Скор у дописанных — без релевантности.
func (p *resultsPlan) fill(rc rankCtx, fams []*familyHit) []*familyHit {
	var scope []licFilter
	for _, f := range p.filters {
		if (f.key == facetCategory || f.key == facetVendor) && f.op == "eq" {
			scope = append(scope, f)
		}
	}
	if len(scope) == 0 || rc.lx == nil {
		return fams
	}
	have := map[string]bool{}
	for _, fh := range fams {
		have[fh.id] = true
	}
	var add []*familyHit
	var discard []domain.RejectedHit
	for _, fid := range rc.lx.familyOrder {
		if have[fid] || len(rc.lx.families[fid]) == 0 {
			continue
		}
		if !passAll(scope, rc.lx.docs[rc.lx.families[fid][0]], "") {
			continue
		}
		fh := rc.buildFamily(fid, 0, &discard)
		if fh == nil {
			continue
		}
		fh.group = domain.GroupMatch
		fh.signals = append(fh.signals, signalFilterFill)
		rc.score(fh)
		add = append(add, fh)
	}
	sort.SliceStable(add, func(i, j int) bool {
		if add[i].head.Popularity != add[j].head.Popularity {
			return add[i].head.Popularity > add[j].head.Popularity
		}
		return add[i].id < add[j].id
	})
	if len(add) > maxFillFamilies {
		add = add[:maxFillFamilies]
	}
	if len(add) == 0 {
		return fams
	}
	return append(append([]*familyHit{}, fams...), add...)
}

func isFill(fh *familyHit) bool { return contains(fh.signals, signalFilterFill) }

// signalFilterFill — семейство дописано по фильтру категории или производителя (трасса items[].signals).
const signalFilterFill = "filter_fill"

// representatives — представитель семейства в карточке: если ни один вариант не закрывает количество из
// запроса целиком, вперёд ставится ближайший по количеству (наибольший не больше запрошенного, иначе наименьший
// больший) новый вариант той же метрики. Только показ: скор и порядок семейств не пересчитываются.
func (p *resultsPlan) representatives(rc rankCtx, fams []*familyHit) []*familyHit {
	n := rc.attrs.quantity
	if n <= 1 {
		return fams
	}
	res := make([]*familyHit, len(fams))
	for i, fh := range fams {
		res[i] = fh
		if fh.headOK || len(fh.heads) < 2 {
			continue
		}
		best := -1
		score := func(d indexDoc) (int, bool) {
			if d.LicenseQuantity == nil || d.LicenseType == domain.LicenseRenewal || d.LicenseType == domain.LicenseUpgrade {
				return 0, false
			}
			if m := rc.attrs.metric; m != "" && d.LicenseMetric != nil && *d.LicenseMetric != m {
				return 0, false
			}
			q := *d.LicenseQuantity
			if q <= n {
				return n - q, true
			}
			return n + q, true // больше нужного — после всех меньших
		}
		bestDist := 0
		for j, d := range fh.heads {
			if dist, ok := score(d); ok && (best < 0 || dist < bestDist) {
				best, bestDist = j, dist
			}
		}
		if best <= 0 {
			continue
		}
		cp := *fh
		cp.heads = append([]indexDoc{fh.heads[best]}, append(append([]indexDoc{}, fh.heads[:best]...), fh.heads[best+1:]...)...)
		cp.head = cp.heads[0]
		res[i] = &cp
	}
	return res
}

// isHardwareResult — первая позиция выдачи — оборудование.
func isHardwareResult(fams []*familyHit) bool {
	return len(fams) > 0 && docKind(fams[0].head) == domain.KindHardware
}

// keep — семейства в окончательном порядке (после маржи), до обрезки по limit.
func (p *resultsPlan) keep(fams []*familyHit) {
	if p != nil {
		p.all = fams
	}
}

// familyPrice — цена «от» семейства: минимум по допустимым вариантам с ценой; nil — у всех цена по запросу.
func familyPrice(fh *familyHit) *float64 {
	var best *float64
	for _, d := range fh.heads {
		if docPriced(d) && (best == nil || *d.PriceRub < *best) {
			v := *d.PriceRub
			best = &v
		}
	}
	return best
}

var deliveryRank = map[string]int{deliveryElectronic: 0, deliveryOEM: 1, deliveryPhysical: 2, "": 3}

// familyDelivery — лучший способ поставки среди вариантов семейства.
func familyDelivery(fh *familyHit) int {
	best := deliveryRank[""]
	for _, d := range fh.heads {
		best = min(best, deliveryRank[docDelivery(d)])
	}
	return best
}

// sortFamilies — явная сортировка. relevance — группа, ступень названного семейства, точное количество, затем
// только релевантность (без популярности, маржи, контекста клиента и бизнес-масштаба); цена — «по запросу» в конце
// в рекомендуемом порядке; поставка — устойчиво поверх рекомендуемого порядка.
func (p *resultsPlan) sortFamilies(fs []*familyHit) {
	switch p.sort {
	case SortRelevance:
		sort.SliceStable(fs, func(i, j int) bool {
			a, b := fs[i], fs[j]
			if ga, gb := groupOrder[a.group], groupOrder[b.group]; ga != gb {
				return ga < gb
			}
			if a.tier != b.tier {
				return a.tier > b.tier
			}
			if a.exactQty != b.exactQty {
				return a.exactQty > b.exactQty
			}
			if a.parts.Relevance != b.parts.Relevance {
				return a.parts.Relevance > b.parts.Relevance
			}
			return a.id < b.id
		})
	case SortPriceAsc, SortPriceDesc:
		desc := p.sort == SortPriceDesc
		sort.SliceStable(fs, func(i, j int) bool {
			return priceLess(familyPrice(fs[i]), familyPrice(fs[j]), desc)
		})
	case SortDelivery:
		sort.SliceStable(fs, func(i, j int) bool { return familyDelivery(fs[i]) < familyDelivery(fs[j]) })
	}
}

// priceLess — по цене; без цены — всегда в конце (нулём не считается).
func priceLess(a, b *float64, desc bool) bool {
	switch {
	case a == nil || b == nil:
		return a != nil && b == nil
	case desc:
		return *a > *b
	}
	return *a < *b
}

// decorate — поля ответа страницы результатов: счётчики, сортировка, группировка, фасеты, уточнения, атрибуты
// карточек; без группировки — позиции вариантов отдельными строками. Вызывается после hw.decorate (фасеты
// характеристик уже в resp.Facets).
func (p *resultsPlan) decorate(rc rankCtx, resp *out.SearchResponse, limit int, exactDoc func(indexDoc) bool) {
	if p == nil {
		return
	}
	resp.Counts = &out.ResultCounts{Families: p.counts.Families, Variants: p.counts.Variants}
	resp.Sort = &out.ResultSort{Value: p.sort, Note: sortNotes[p.sort]}
	resp.Group = p.group
	resp.Refinements = p.refinements
	if resp.Refinements == nil {
		resp.Refinements = []out.Refinement{}
	}
	have := map[string]bool{}
	for _, f := range resp.Facets {
		have[f.Key] = true
	}
	for _, f := range p.facets {
		if !have[f.Key] {
			resp.Facets = append(resp.Facets, f)
		}
	}
	if p.filteredOut && resp.Empty == nil {
		resp.Empty = &out.Empty{Message: "По выбранным фильтрам позиций нет — снимите один из фильтров"}
	}
	if p.group == GroupNone {
		resp.Items = p.flatten(rc, limit, exactDoc)
	}
	for i := range resp.Items {
		it := &resp.Items[i]
		p.attrs(rc.lx, it)
		if len(it.Variants) > 0 {
			best := it.PriceRub
			if it.PriceOnRequest {
				best = nil
			}
			for j := range it.Variants {
				p.attrs(rc.lx, &it.Variants[j])
				v := it.Variants[j]
				if v.PriceRub != nil && !v.PriceOnRequest && (best == nil || *v.PriceRub < *best) {
					best = v.PriceRub
				}
			}
			if best != nil {
				x := *best
				it.PriceFromRub = &x
			}
		}
	}
}

// flatten — без группировки: каждая допустимая позиция — отдельная строка; на точном артикуле сам товар первым.
func (p *resultsPlan) flatten(rc rankCtx, limit int, exactDoc func(indexDoc) bool) []out.SearchItem {
	var flat []out.SearchItem
	for _, fh := range p.all {
		it := rc.toItem(fh)
		vs := it.Variants
		it.Variants = nil
		flat = append(flat, it)
		for _, v := range vs {
			v.Variants = nil
			flat = append(flat, v)
		}
	}
	switch p.sort {
	case SortPriceAsc, SortPriceDesc:
		desc := p.sort == SortPriceDesc
		price := func(it out.SearchItem) *float64 {
			if it.PriceOnRequest {
				return nil
			}
			return it.PriceRub
		}
		sort.SliceStable(flat, func(i, j int) bool { return priceLess(price(flat[i]), price(flat[j]), desc) })
	case SortDelivery:
		rank := func(it out.SearchItem) int { return deliveryRank[docDelivery(rc.lx.docs[it.ProductID])] }
		sort.SliceStable(flat, func(i, j int) bool { return rank(flat[i]) < rank(flat[j]) })
	}
	if p.exact && exactDoc != nil {
		sort.SliceStable(flat, func(i, j int) bool {
			return exactDoc(rc.lx.docs[flat[i].ProductID]) && !exactDoc(rc.lx.docs[flat[j].ProductID])
		})
	}
	if len(flat) > limit {
		flat = flat[:limit]
	}
	if flat == nil {
		flat = []out.SearchItem{}
	}
	return flat
}

// attrs — атрибуты карточки из документа индекса.
func (p *resultsPlan) attrs(lx *searchLexicon, it *out.SearchItem) {
	d, ok := lx.docs[it.ProductID]
	if !ok {
		return
	}
	a := &out.ItemAttrs{Edition: d.LicenseEdition, LicenseType: d.LicenseType, Quantity: d.LicenseQuantity,
		DurationMonths: d.LicenseDuration, Delivery: docDelivery(d), InReestr: d.InReestr}
	if d.LicenseMetric != nil {
		a.LicenseMetric = *d.LicenseMetric
	}
	it.Attrs = a
}

// exactDocMatcher — позиция названа в запросе артикулом или парт-номером.
func exactDocMatcher(tokens []string, hw *hwPlan) func(indexDoc) bool {
	set := map[string]bool{}
	for _, t := range tokens {
		set[foldText(t)] = true
	}
	return func(d indexDoc) bool {
		if d.SKU != "" && set[foldText(d.SKU)] {
			return true
		}
		return hw != nil && hw.mpn != nil && !hw.mpn.Prefix && d.MPNNormalized != "" && d.MPNNormalized == hw.mpn.Norm
	}
}

// joinQuery — текст уточнения для строки поиска (панель дописывает его к запросу).
func joinQuery(query, label string) string {
	return strings.TrimSpace(query + " " + strings.ToLower(label))
}
