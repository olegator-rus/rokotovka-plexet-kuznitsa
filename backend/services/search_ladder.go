package services

import (
	"math"
	"sort"
	"strconv"
	"sync"
	"time"

	"kuznica/api/domain"
	"kuznica/api/out"

	"go.uber.org/zap"
)

// Лестница «минимум 4 товара» и контракт карточки. После выдачи, вне
// BM25 и векторов: цель ответа (search_target.go), match_type у каждой карточки, «Можно купить сейчас»,
// «Популярное в наличии», состояние цены, цена «от», скидка, строка «почему здесь» и счётчики.
//
// Жёсткие условия (продление, тип организации, платформа, вендор, число мест) не ослабляются ни на одном шаге:
// добавленные карточки собираются тем же buildFamily (rejectReason) и только из семейств, чья голова подходит
// полностью. «Популярное в наличии» и «Можно купить сейчас» — только на витрине (не в прогонах оценки: в метрики
// релевантности они не входят) и в Total не считаются.

// StepAnswer — шаг трассы: цель и лестница.
const StepAnswer = "answer"

// ladderMin — сколько разных семейств должно быть в ответе, если они есть (ТЗ: «не меньше 4 товаров»).
const ladderMin = 4

// maxAlternatives — сколько карточек «Можно купить сейчас» добавляется к продукту только по запросу.
const maxAlternatives = 3

// matchRank — порядок ступеней в выдаче: полные совпадения выше частичных.
var matchRank = map[string]int{out.MatchExact: 0, out.MatchVariant: 1, out.MatchRelated: 2, out.MatchAlternative: 3, out.MatchFallback: 4}

// Строки «почему здесь» (§3, подпись альтернативы —).
const (
	explainSKU         = "точный артикул"
	explainFamily      = "совпадение с названием семейства"
	explainVendor      = "назван производитель"
	explainVariant     = "другой вариант лицензии того же семейства"
	explainRelated     = "похожее по названию и назначению"
	explainAnalog      = "аналог: тот же класс решения"
	explainAlternative = "Тот же класс решения, можно купить сейчас. Совместимость с вашими приложениями нужно проверить"
	explainFallback    = "популярное в наличии"
	explainSide        = "сопутствующее: курс, техподдержка или модуль к продукту"
)

// AnswerStep — шаг answer трассы: какая цель выбрана и почему, как добрана лестница.
type AnswerStep struct {
	domain.StepBase
	Target       string         `json:"target"`
	TargetReason string         `json:"target_reason"`
	BrandOnly    bool           `json:"brand_only"`
	Covering     []string       `json:"covering"`
	CoverageMiss string         `json:"coverage_miss,omitempty"` // семейство без собранного URL: цель — SKU
	Words        []string       `json:"words"`
	Families     int            `json:"families"`
	SKUs         int            `json:"skus"`
	ByMatchType  map[string]int `json:"by_match_type"`
	Alternatives int            `json:"alternatives"`
	Fallback     int            `json:"fallback"`
	FallbackOff  bool           `json:"fallback_off,omitempty"`
}

// answerInput — что нужно шагу answer.
type answerInput struct {
	rc       rankCtx
	words    []string
	fams     []*familyHit // семейства выдачи до обрезки по limit
	exactDoc func(indexDoc) bool
	category string
	limit    int
	// storefront — поиск витрины (не прогон оценки): добираются «Можно купить сейчас» и «Популярное в наличии».
	storefront bool
	// fallbackOff — SEARCH_FALLBACK_BLOCK=off: под сервисным запросом и бессмыслицей блока нет.
	fallbackOff bool
	serviceLike bool // сервисный запрос или бессмыслица
}

// answerStage — цель, лестница, карточки и счётчики; пишет в resp и trace.
func answerStage(in answerInput, resp *out.SearchResponse, trace *domain.Trace) AnswerStep {
	t := time.Now()
	rc := in.rc
	dec := selectTarget(targetInput{rc: rc, words: in.words, fams: in.fams, exactDoc: in.exactDoc, category: in.category, help: resp.Help})
	resp.Target = dec.target
	step := AnswerStep{StepBase: domain.StepBase{Name: StepAnswer}, BrandOnly: dec.brandOnly, Covering: dec.covering,
		CoverageMiss: dec.coverageMiss, Words: in.words}
	if step.Covering == nil {
		step.Covering = []string{}
	}
	if step.Words == nil {
		step.Words = []string{}
	}
	if dec.target != nil {
		step.Target, step.TargetReason = dec.target.Type, dec.target.Reason
	}
	if dec.coverageMiss != "" {
		zap.L().Info("поиск: страница семейства не собрана — цель ведёт на SKU", zap.String("family_id", dec.coverageMiss))
	}
	byID := map[string]*familyHit{}
	for _, fh := range in.fams {
		byID[fh.id] = fh
	}

	// 1–3: exact / variant / related у найденного.
	for i := range resp.Items {
		it := &resp.Items[i]
		switch {
		case dec.skuDoc != nil && it.ProductID == dec.skuDoc.ProductID:
			it.MatchType, it.Explain = out.MatchExact, explainSKU
		case dec.skuDoc != nil && it.FamilyID == dec.skuDoc.FamilyID:
			it.MatchType, it.Explain = out.MatchVariant, explainVariant
		case rc.sideKind(it.ProductID) && (dec.target == nil || dec.target.Type != out.TargetFamily || !dec.families[it.FamilyID]):
			// Курс, техподдержка или модуль, о которых не просили, — не точное совпадение, а похожее.
			it.MatchType, it.Explain = out.MatchRelated, explainSide
		case it.Group == domain.GroupExact || dec.families[it.FamilyID]:
			it.MatchType, it.Explain = out.MatchExact, explainFamily
			if len(rc.ent.vendors) > 0 && !dec.families[it.FamilyID] {
				it.Explain = explainVendor
			}
		case it.Group == domain.GroupAnalog:
			it.MatchType, it.Explain = out.MatchRelated, explainAnalog
		default:
			it.MatchType, it.Explain = out.MatchRelated, explainRelated
		}
	}
	if dec.skuDoc != nil && resp.Group != GroupNone {
		resp.Items = expandSKU(rc, resp.Items, byID[dec.skuDoc.FamilyID], *dec.skuDoc)
	} else if dec.skuDoc != nil {
		resp.Items = trimSKUFamily(rc.lx, resp.Items, *dec.skuDoc) // плоская выдача: SKU и три соседа (search_sku_neighbours.go)
	}
	// Семейство цели — первой карточкой: цель и первый товар не расходятся.
	lead := ""
	if dec.target != nil && dec.skuDoc == nil && len(dec.families) == 1 {
		for f := range dec.families {
			lead = f
		}
	}
	if dec.lead != "" && dec.families[dec.lead] {
		lead = dec.lead // главный продукт линейки (search_target.go mainOfLine)
	}
	// Оговорки к карточке: продукт только под тип организации; названной версии нет — ближайшая.
	for i := range resp.Items {
		it := &resp.Items[i]
		if it.MatchType != out.MatchExact {
			continue
		}
		if note := rc.orgOnlyNote(byID[it.FamilyID]); note != "" {
			it.Explain += "; " + note
		}
		if dec.leadNote != "" && it.FamilyID == lead {
			it.Explain += "; " + dec.leadNote
		}
	}
	// Ступени по порядку, семейство цели — первым в своей ступени.
	sort.SliceStable(resp.Items, func(i, j int) bool {
		a, b := resp.Items[i], resp.Items[j]
		if ra, rb := matchRank[a.MatchType], matchRank[b.MatchType]; ra != rb {
			return ra < rb
		}
		return lead != "" && resp.Group != GroupNone && a.FamilyID == lead && b.FamilyID != lead
	})
	// Внутри одного уровня релевантности (подряд идущие карточки одной ступени, группы и ступени названности) сначала
	// то, что можно купить сейчас: порядок между уровнями не меняется — доступность не поднимает менее
	// релевантное. Без группировки, на точном артикуле и при выбранной покупателем сортировке порядок прежний.
	if dec.skuDoc == nil && resp.Group != GroupNone && (resp.Sort == nil || resp.Sort.Value == SortRecommended) {
		buyableFirstWithinLevel(resp.Items, func(it out.SearchItem) [3]int {
			t := 0
			if fh := byID[it.FamilyID]; fh != nil {
				t = fh.tier
			}
			return [3]int{matchRank[it.MatchType], groupOrder[it.Group], t}
		})
	}

	shown := map[string]bool{}
	for _, it := range resp.Items {
		shown[it.FamilyID] = true
	}
	// 4: «Можно купить сейчас» — найденное точно только по запросу.
	if in.storefront && !in.serviceLike {
		alts := alternatives(rc, resp.Items, shown)
		step.Alternatives = len(alts)
		resp.Items = insertAfterFirst(resp.Items, alts) // сразу под названным продуктом
	}
	// 5: «Популярное в наличии» — шаги 1–4 дали меньше четырёх семейств.
	if in.storefront && len(shown) < ladderMin {
		if in.serviceLike && in.fallbackOff {
			step.FallbackOff = true
		} else {
			fb := fallback(rc, resp.Items, shown, ladderMin-len(shown))
			step.Fallback = len(fb)
			resp.Items = append(resp.Items, fb...)
		}
	}
	for _, it := range resp.Items {
		if it.MatchType == out.MatchAlternative || it.MatchType == out.MatchFallback {
			trace.Items = append(trace.Items, domain.TraceItem{ProductID: it.ProductID, Group: it.Group, Signals: []string{it.MatchType}})
		}
	}
	finishCards(resp)
	resp.Comparison = priceComparison(rc.lx, resp.Items) // строка сравнения цены за место (search_comparison.go)
	if dec.clarify {
		resp.Clarifications = vendorClarifications(resp)
	}
	step.Families, step.SKUs, step.ByMatchType = cardFamilies(resp.Items), resp.Counts.SKUs, resp.Counts.ByMatchType
	step.Ms = msSince(t)
	return step
}

// expandSKU — точный артикул: сам SKU первой карточкой, под ним до трёх других вариантов того же семейства
// (match_type variant), остальные варианты остаются вложенными.
func expandSKU(rc rankCtx, items []out.SearchItem, fh *familyHit, sku indexDoc) []out.SearchItem {
	if fh == nil {
		return items
	}
	at := -1
	for i, it := range items {
		if it.FamilyID == fh.id {
			at = i
			break
		}
	}
	if at < 0 {
		return items
	}
	head := rc.cardOf(fh, sku, nil)
	head.MatchType, head.Explain = out.MatchExact, explainSKU
	cards := []out.SearchItem{head}
	var rest []indexDoc
	for _, d := range fh.heads {
		if d.ProductID == sku.ProductID {
			continue
		}
		if len(cards) <= 3 {
			c := rc.cardOf(fh, d, nil)
			c.MatchType, c.Explain = out.MatchVariant, explainVariant
			cards = append(cards, c)
			continue
		}
		rest = append(rest, d)
	}
	if len(rest) > 0 {
		cards[0] = rc.cardOf(fh, sku, rest)
		cards[0].MatchType, cards[0].Explain = out.MatchExact, explainSKU
	}
	res := append([]out.SearchItem{}, items[:at]...)
	res = append(res, cards...)
	return append(res, items[at+1:]...)
}

// insertAfterFirst — блок карточек сразу под первой.
func insertAfterFirst(items, block []out.SearchItem) []out.SearchItem {
	if len(block) == 0 || len(items) == 0 {
		return append(items, block...)
	}
	res := make([]out.SearchItem, 0, len(items)+len(block))
	res = append(res, items[0])
	res = append(res, block...)
	return append(res, items[1:]...)
}

// cardOf — карточка семейства с заданной головой и вложенными вариантами.
func (rc rankCtx) cardOf(fh *familyHit, head indexDoc, variants []indexDoc) out.SearchItem {
	c := *fh
	c.head = head
	c.heads = append([]indexDoc{head}, variants...)
	return rc.toItem(&c)
}

// alternatives — «Можно купить сейчас»: ни одну точную карточку нельзя купить сразу — семейства того же класса и
// назначения, голова которых подходит под условия запроса и продаётся с ценой.
func alternatives(rc rankCtx, items []out.SearchItem, shown map[string]bool) []out.SearchItem {
	var exact []out.SearchItem
	for _, it := range items {
		if it.MatchType == out.MatchExact || it.MatchType == out.MatchVariant {
			exact = append(exact, it)
		}
	}
	if len(exact) == 0 {
		return nil
	}
	for _, it := range exact {
		if itemPrice(it) != nil {
			return nil
		}
		for _, v := range it.Variants {
			if v.MatchesQuery && itemPrice(v) != nil {
				return nil
			}
		}
	}
	// та же категория второго уровня (или соседняя того же типа) и то же назначение (search_alternatives.go).
	return purposeAlternatives(rc, exact[0], shown, maxAlternatives)
}

// fallback — «Популярное в наличии»: семейства в наличии с ценой из категорий второго уровня показанных карточек,
// затем из их разделов, затем из всего каталога (по одному на категорию). Порядок — популярность, затем порядок
// каталога; число офферов семейства популярностью не считается.
func fallback(rc rankCtx, items []out.SearchItem, shown map[string]bool, need int) []out.SearchItem {
	cats, roots := map[string]bool{}, map[string]bool{}
	for _, it := range items {
		if d, ok := rc.lx.docs[it.ProductID]; ok {
			c := docCat2(d)
			cats[c] = true
			if r := rc.lx.categoryRoot[c]; r != "" {
				roots[r] = true
			}
		}
	}
	var res []out.SearchItem
	add := func(r rankCtx, pred func(string) bool, onePerCat bool) {
		if len(res) >= need {
			return
		}
		got := pickFamilies(r, shown, need-len(res), pred, onePerCat, domain.GroupAnalog, out.MatchFallback, explainFallback)
		for _, it := range got {
			shown[it.FamilyID] = true
		}
		res = append(res, got...)
	}
	// Названный вендор — сначала его семейства; дальше блок подписан «Популярное в наличии» и вендор уже не условие
	// (жёсткие условия — тип лицензии, организация, платформа, число мест — остаются).
	free := rc
	free.ent.vendors = nil
	sameCat := func(fid string) bool { return cats[famCat2(rc.lx, fid)] }
	if len(cats) > 0 {
		add(rc, sameCat, false)
	}
	if len(rc.ent.vendors) > 0 {
		add(rc, func(fid string) bool { return contains(rc.ent.vendors, rc.lx.familyVendor[fid]) }, false)
		if len(cats) > 0 {
			add(free, sameCat, false)
		}
	}
	if len(cats) > 0 {
		add(free, func(fid string) bool { return roots[rc.lx.categoryRoot[famCat2(rc.lx, fid)]] }, true)
	}
	if len(res) < need {
		got := pickOrdered(free, shown, need-len(res), popularOrder(rc.lx))
		res = append(res, got...)
	}
	return res
}

// famCat2 — категория второго уровня семейства (по первой позиции).
func famCat2(lx *searchLexicon, fid string) string {
	if pids := lx.families[fid]; len(pids) > 0 {
		return docCat2(lx.docs[pids[0]])
	}
	return ""
}

// pickFamilies — до n семейств, которых нет в выдаче: подходят по pred, голова подходит под условия запроса
// полностью (жёсткие условия не ослабляются) и её можно купить сейчас. Порядок — популярность, затем каталог.
//
// onePerCat — не больше одного семейства на категорию второго уровня (разнообразие блока «Популярное»).
func pickFamilies(rc rankCtx, shown map[string]bool, n int, pred func(string) bool, onePerCat bool, group, matchType, explain string) []out.SearchItem {
	if n <= 0 {
		return nil
	}
	type cand struct {
		fh  *familyHit
		pop float64
		pos int
	}
	var cs []cand
	var sink []domain.RejectedHit
	for pos, fid := range rc.lx.familyOrder {
		if shown[fid] || !pred(fid) {
			continue
		}
		pre := false
		for _, pid := range rc.lx.families[fid] {
			if buyable(rc.lx.docs[pid]) {
				pre = true
				break
			}
		}
		if !pre {
			continue
		}
		sink = sink[:0]
		fh := rc.buildFamily(fid, 0, &sink)
		if fh == nil || !fh.headOK || !buyable(fh.head) {
			continue
		}
		cs = append(cs, cand{fh, fh.head.Popularity, pos})
		if len(cs) >= 200 {
			break // хватает для выбора: каталог уже упорядочен
		}
	}
	sort.SliceStable(cs, func(i, j int) bool {
		if cs[i].pop != cs[j].pop {
			return cs[i].pop > cs[j].pop
		}
		return cs[i].pos < cs[j].pos
	})
	var res []out.SearchItem
	usedCat := map[string]bool{}
	for _, c := range cs {
		if len(res) >= n {
			break
		}
		if cat := docCat2(c.fh.head); onePerCat {
			if usedCat[cat] {
				continue
			}
			usedCat[cat] = true
		}
		c.fh.group = group
		it := rc.toItem(c.fh)
		it.MatchType, it.Explain = matchType, explain
		res = append(res, it)
		shown[c.fh.id] = true
	}
	return res
}

// itemPrice — действующая цена позиции: > 0, не по запросу, в наличии; nil — «по запросу».
func itemPrice(it out.SearchItem) *float64 {
	if it.PriceRub == nil || *it.PriceRub <= 0 || it.PriceOnRequest || it.Available != nil && !*it.Available {
		return nil
	}
	return it.PriceRub
}

// finishCards — поля карточки контура фида у каждой карточки (все конфигурации) и счётчики ответа.
func finishCards(resp *out.SearchResponse) {
	if resp.Clarifications == nil {
		resp.Clarifications = []out.Refinement{}
	}
	skus := map[string]bool{}
	byMatch := map[string]int{}
	variants := 0
	for i := range resp.Items {
		it := &resp.Items[i]
		if it.MatchType == "" {
			it.MatchType = out.MatchRelated
			if it.Group == domain.GroupExact {
				it.MatchType = out.MatchExact
			}
		}
		byMatch[it.MatchType]++
		skus[it.ProductID] = true
		variants += 1 + len(it.Variants)
		it.PriceFrom = nil
		consider := func(x out.SearchItem) {
			if p := itemPrice(x); p != nil && x.MatchesQuery && (it.PriceFrom == nil || *p < *it.PriceFrom) {
				v := *p
				it.PriceFrom = &v
			}
		}
		consider(*it)
		for _, v := range it.Variants {
			skus[v.ProductID] = true
			consider(v)
		}
		it.PriceState = out.PriceOnRequest
		if it.PriceFrom != nil {
			it.PriceState = out.PricePriced
		}
		it.Badges = nil
		if p := itemPrice(*it); p != nil && it.OldPriceRub != nil && *it.OldPriceRub > *p {
			if n := int(math.Round((*it.OldPriceRub - *p) / *it.OldPriceRub * 100)); n >= 1 {
				it.Badges = append(it.Badges, "Скидка "+strconv.Itoa(n)+"%")
			}
		}
	}
	if resp.Counts == nil {
		resp.Counts = &out.ResultCounts{Families: cardFamilies(resp.Items), Variants: variants}
	}
	resp.Counts.SKUs = len(skus)
	resp.Counts.ByMatchType = byMatch
}

func cardFamilies(items []out.SearchItem) int {
	fams := map[string]bool{}
	for _, it := range items {
		fams[it.FamilyID] = true
	}
	return len(fams)
}

// vendorClarifications — уточняющие чипы по вендорам точных и похожих карточек (равенство между вендорами, общее
// слово-категория): до пяти, по числу карточек.
func vendorClarifications(resp *out.SearchResponse) []out.Refinement {
	type vc struct {
		name string
		n    int
		pos  int
	}
	seen := map[string]*vc{}
	var order []*vc
	for i, it := range resp.Items {
		if it.Vendor == "" || it.MatchType == out.MatchFallback || it.MatchType == out.MatchAlternative {
			continue
		}
		if v, ok := seen[it.Vendor]; ok {
			v.n++
			continue
		}
		v := &vc{name: it.Vendor, n: 1, pos: i}
		seen[it.Vendor] = v
		order = append(order, v)
	}
	if len(order) < 2 {
		return []out.Refinement{}
	}
	sort.SliceStable(order, func(i, j int) bool { return order[i].n > order[j].n })
	res := []out.Refinement{}
	for _, v := range order {
		if len(res) == 5 {
			break
		}
		field, value := "vendor", v.name
		res = append(res, out.Refinement{Label: v.name, Query: joinQuery(resp.Query, v.name), Field: &field, Value: &value, Hits: v.n})
	}
	return res
}

// popularCache — порядок «Популярного в наличии» по всему каталогу на снимок словаря (считается один раз).
var popularCache sync.Map // *searchLexicon → []string

// popularOrder — по одному семейству из каждой категории второго уровня: сначала ПО (магазин софта), категории — по популярности семейств
// (сумма), при равенстве — по ширине ассортимента в наличии (разных семейств с ценой; на фиде популярность у всех
// одна); внутри категории — самое популярное семейство с базовой лицензией в наличии, затем порядок каталога.
func popularOrder(lx *searchLexicon) []string {
	if v, ok := popularCache.Load(lx); ok {
		return v.([]string)
	}
	type catAgg struct {
		soft  bool // ПО: магазин софта, оборудование — после
		pop   float64
		n     int
		best  string
		bestP float64
		pos   int
	}
	cats := map[string]*catAgg{}
	for pos, fid := range lx.familyOrder {
		var head *indexDoc
		for _, pid := range lx.families[fid] {
			d := lx.docs[pid]
			if buyable(d) && kindRank(lx.kindOf(d), nil) <= 1 && feedOrg[d.LicenseOrgType] == "" {
				head = &d
				break
			}
		}
		if head == nil {
			continue
		}
		c := docCat2(*head)
		a := cats[c]
		if a == nil {
			a = &catAgg{pos: pos, soft: docKind(*head) != domain.KindHardware}
			cats[c] = a
		}
		a.pop += head.Popularity
		a.n++
		if a.best == "" || head.Popularity > a.bestP {
			a.best, a.bestP = fid, head.Popularity
		}
	}
	list := make([]*catAgg, 0, len(cats))
	for _, a := range cats {
		list = append(list, a)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].soft != list[j].soft {
			return list[i].soft
		}
		if list[i].pop != list[j].pop {
			return list[i].pop > list[j].pop
		}
		if list[i].n != list[j].n {
			return list[i].n > list[j].n
		}
		return list[i].pos < list[j].pos
	})
	order := make([]string, 0, len(list))
	for _, a := range list {
		order = append(order, a.best)
	}
	popularCache.Store(lx, order)
	return order
}

// pickOrdered — до n семейств из заданного порядка, не показанных, с головой под условия запроса и в наличии.
func pickOrdered(rc rankCtx, shown map[string]bool, n int, order []string) []out.SearchItem {
	var res []out.SearchItem
	var sink []domain.RejectedHit
	for _, fid := range order {
		if len(res) >= n {
			break
		}
		if shown[fid] {
			continue
		}
		sink = sink[:0]
		fh := rc.buildFamily(fid, 0, &sink)
		if fh == nil || !fh.headOK || !buyable(fh.head) {
			continue
		}
		fh.group = domain.GroupAnalog
		it := rc.toItem(fh)
		it.MatchType, it.Explain = out.MatchFallback, explainFallback
		res = append(res, it)
		shown[fid] = true
	}
	return res
}

// sideKind — голова карточки — курс, техподдержка или модуль, о которых запрос не просит.
func (rc rankCtx) sideKind(pid string) bool {
	d, ok := rc.lx.docs[pid]
	return ok && kindRank(rc.lx.kindOf(d), rc.wantKinds) >= 3
}

// cardBuyable — у карточки есть позиция, которую можно купить сейчас и которая подходит под условия запроса.
func cardBuyable(it out.SearchItem) bool {
	if itemPrice(it) != nil {
		return true
	}
	for _, v := range it.Variants {
		if v.MatchesQuery && itemPrice(v) != nil {
			return true
		}
	}
	return false
}

// buyableFirstWithinLevel — в каждой серии подряд идущих карточек одного уровня карточки, которые можно купить сейчас,
// встают перед карточками «по запросу»; внутри обеих частей порядок прежний.
func buyableFirstWithinLevel(items []out.SearchItem, level func(out.SearchItem) [3]int) {
	for start := 0; start < len(items); {
		end := start + 1
		for end < len(items) && level(items[end]) == level(items[start]) {
			end++
		}
		run := items[start:end]
		sort.SliceStable(run, func(i, j int) bool { return cardBuyable(run[i]) && !cardBuyable(run[j]) })
		start = end
	}
}
