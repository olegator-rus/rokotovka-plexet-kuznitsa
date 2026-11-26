package services

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Товарная секция панели (RESEARCH Б): текстовый путь без векторов и реранкера, свёртка по семейству
// в Go (не collapse: свёрнутые кандидаты нужны ещё и для долей категорий и бейджей; бизнес-слой после свёртки — в
// коде, collapse с rescore в OpenSearch 3.9 несовместим). Конфигурация instant:
//  1. верхняя корзина точных совпадений: артикул, парт-номер, точное название — не сдвигается ничем;
//  2. порог релевантности (слабые кандидаты слот не занимают);
//  3. свёртка по семейству, представитель — самый релевантный вариант;
//  4. маржа — только разрешение равенства среди кандидатов в пределах instantTieEps от соседа и instantNearBest от
//     лучшего; сдвиг ≤ 1; выключена на префиксе короче instantMarginMinRunes и на точном артикуле;
//  5. ничьи — детерминированно: популярность, затем id семейства.

const (
	instantSlots          = 6
	instantMinRel         = 0.3
	instantTieEps         = 0.05
	instantNearBest       = 0.85
	instantMarginMinRunes = 4
	instantCodePrefixMax  = 6
	// productsTimeout — бюджет товарной секции; не успели — ответ без товаров, кэш дописывается в фоне.
	productsTimeout   = 150 * time.Millisecond
	productsBackgound = 3 * time.Second
	productCacheSize  = 5000
	productCacheTTL   = 10 * time.Minute
)

// instantQuery — тело текстового запроса панели: слова как префиксы (bool_prefix — последнее слово начало слова) и
// начала слов названий (title.ngram), все слова обязательны. Категория и описание — с малым весом: «антивирус»
// находит продукты, в названии которых этого слова нет.
func instantQuery(text string, size int) map[string]any {
	text = engineText(text)
	return map[string]any{
		"size":    size,
		"_source": sourceExcludes,
		"query": map[string]any{"bool": map[string]any{"should": []any{
			map[string]any{"multi_match": map[string]any{"query": text, "type": "bool_prefix", "operator": "and",
				"fields": []string{"title^2", "family_name^3", "vendor^2", "aliases^2", "sku^5", "category_path^0.5", "description^0.3"}}},
			map[string]any{"multi_match": map[string]any{"query": text, "operator": "and", "fields": []string{"title.ngram"}}},
		}, "minimum_should_match": 1}},
		"sort": []any{"_score", map[string]any{"popularity": "desc"}, map[string]any{"product_id": "asc"}},
	}
}

// instantFamily — семейство-кандидат.
type instantFamily struct {
	id      string
	head    indexDoc
	rel     float64
	exact   bool
	skuHit  bool
	pop     float64
	margin  float64
	members int // вариантов семейства среди кандидатов
}

// instantResult — товарная секция до обрезки по слотам: всё, что нужно категориям, бейджам и трассе.
type instantResult struct {
	text     string
	families []instantFamily // после свёртки, порога и маржи, по порядку показа
	total    int             // семейств выше порога
	docs     []indexDoc      // кандидаты до свёртки (для бейджей)
	before   []panelCandidate
	shifts   []panelShift
	margin   string // почему маржа выключена; "" — применялась
	skuExact bool
	ms       float64
}

// panelCandidate — кандидат до свёртки (трасса панели).
type panelCandidate struct {
	ProductID string  `json:"product_id"`
	FamilyID  string  `json:"family_id"`
	Score     float64 `json:"score"`
}

// panelShift — сдвиг маржой (трасса панели, только бэк-офис).
type panelShift struct {
	FamilyID string `json:"family_id"`
	From     int    `json:"from"`
	To       int    `json:"to"`
}

// instantProducts — товарная секция по тексту text. code — канонический артикул ввода ("" — не код).
// marginOn — маржа разрешена флагами и длиной префикса; pct — перцентили маржи (nil — без маржи).
func (s *Services) instantProducts(ctx context.Context, lx *searchLexicon, text, code string, pct map[string]float64) (*instantResult, error) {
	t := time.Now()
	r := &instantResult{text: text}
	if code != "" {
		ci := lx.codes()
		if pids := ci.exact(code); len(pids) > 0 {
			// Точный артикул: один товар, свёртка и маржа выключены.
			d := lx.docs[pids[0]]
			r.skuExact, r.margin, r.total = true, "sku_exact", 1
			r.families = []instantFamily{{id: d.FamilyID, head: d, rel: 1, exact: true, skuHit: true, members: 1}}
			r.docs = []indexDoc{d}
			r.before = []panelCandidate{{ProductID: d.ProductID, FamilyID: d.FamilyID, Score: 1}}
			r.ms = msSince(t)
			return r, nil
		}
	}
	res, err := s.search.Search(ctx, s.alias, instantQuery(text, candidatesTop))
	if err != nil {
		return nil, err
	}
	top, docs, err := parseHits(res)
	if err != nil {
		return nil, err
	}
	// Варианты под сделку или организацию (контур доводки, purchase_attrs.go) — только в выдаче по запросу, который их
	// называет; в панели их нет.
	visible := top[:0]
	for _, h := range top {
		if d, ok := docs[h.ProductID]; !ok || !isOffer(d) {
			visible = append(visible, h)
		}
	}
	top = visible
	scores := map[string]float64{}
	best := 0.0
	for _, h := range top {
		scores[h.ProductID] = h.Score
		best = math.Max(best, h.Score)
	}
	// Начало парт-номера («jl25»): позиции с таким префиксом кода — кандидаты с релевантностью лучшего.
	if code != "" {
		for _, pid := range lx.codes().prefix(code, instantCodePrefixMax) {
			if _, ok := scores[pid]; !ok {
				if d, ok := lx.docs[pid]; ok {
					docs[pid] = d
					top = append(top, rankedHit(pid, len(top)+1, best))
				}
			}
			scores[pid] = math.Max(scores[pid], math.Max(best, 1))
		}
		best = math.Max(best, 1)
	}
	folded := foldText(text)
	byFam := map[string]*instantFamily{}
	var fams []*instantFamily
	for _, h := range top {
		d, ok := docs[h.ProductID]
		if !ok || d.FamilyID == "" {
			continue
		}
		r.docs = append(r.docs, d)
		r.before = append(r.before, panelCandidate{ProductID: d.ProductID, FamilyID: d.FamilyID, Score: round4(h.Score)})
		rel := 0.0
		if best > 0 {
			rel = scores[d.ProductID] / best
		}
		exact := foldText(d.Title) == folded || foldText(d.FamilyName) == folded
		f := byFam[d.FamilyID]
		if f == nil {
			f = &instantFamily{id: d.FamilyID, head: d, rel: rel, exact: exact, pop: d.Popularity, margin: pct[d.ProductID]}
			byFam[d.FamilyID] = f
			fams = append(fams, f)
		}
		f.members++
		f.exact = f.exact || exact
		better := rel > f.rel || rel == f.rel && (d.Popularity > f.head.Popularity || d.Popularity == f.head.Popularity && d.ProductID < f.head.ProductID)
		if better {
			f.head, f.rel, f.margin = d, rel, pct[d.ProductID]
		}
		f.pop = math.Max(f.pop, d.Popularity)
	}
	var kept []instantFamily
	for _, f := range fams {
		if f.exact || f.rel >= instantMinRel {
			kept = append(kept, *f)
		}
	}
	// Релевантность огрубляется до сотых: вторичные ключи работают только при ничьей.
	sort.SliceStable(kept, func(i, j int) bool {
		a, b := kept[i], kept[j]
		if a.exact != b.exact {
			return a.exact
		}
		if ra, rb := math.Round(a.rel*100), math.Round(b.rel*100); ra != rb {
			return ra > rb
		}
		if a.pop != b.pop {
			return a.pop > b.pop
		}
		return a.id < b.id
	})
	r.total = len(kept)
	if pct == nil {
		r.margin = "off"
	} else {
		kept, r.shifts = marginTieBreak(kept)
	}
	r.families = kept
	r.ms = msSince(t)
	return r, nil
}

// marginTieBreak — маржа как разрешение равенства: соседние неточные семейства, оба близкие к лучшему и почти
// равные по релевантности, меняются местами, если у нижнего маржа выше. Каждое семейство сдвигается не больше
// чем на одну позицию; верхняя корзина точных не участвует.
func marginTieBreak(fs []instantFamily) ([]instantFamily, []panelShift) {
	shifts := []panelShift{}
	best := 0.0
	for _, f := range fs {
		if !f.exact {
			best = math.Max(best, f.rel)
		}
	}
	for i := 0; i+1 < len(fs) && i+1 < instantSlots; i++ {
		a, b := fs[i], fs[i+1]
		if a.exact || b.exact || b.rel < instantNearBest*best || a.rel-b.rel > instantTieEps || b.margin <= a.margin {
			continue
		}
		fs[i], fs[i+1] = b, a
		shifts = append(shifts, panelShift{FamilyID: b.id, From: i + 2, To: i + 1}, panelShift{FamilyID: a.id, From: i + 1, To: i + 2})
		i++ // переставленная пара больше не двигается
	}
	return fs, shifts
}

// panelProducts — до limit карточек семейств.
func panelProducts(lx *searchLexicon, r *instantResult, limit int) []out.PanelProduct {
	res := []out.PanelProduct{}
	for _, f := range r.families {
		if len(res) == limit {
			break
		}
		d := f.head
		p := out.PanelProduct{FamilyID: d.FamilyID, FamilyName: d.FamilyName, ProductID: d.ProductID, Title: d.Title,
			Kind: docKind(d), Vendor: d.Vendor, Category: d.Category, MPN: d.MPNRaw, LicenseLine: licenseLine(d),
			PriceMode: docPriceMode(d), VariantsCount: len(lx.families[d.FamilyID]), Exact: f.exact, SKUExact: f.skuHit}
		if p.Kind == "hardware" {
			p.LicenseLine = ""
			p.SpecsLine = specsLine(d)
			if l := deliveryLabels[docDelivery(d)]; l != "" {
				p.DeliveryLine = &l
			}
		}
		switch {
		case f.skuHit:
			p.VariantsCount = 1
			p.PriceFromRub = d.PriceRub
		case !d.PriceOnRequest && d.PriceRub != nil:
			// Карточка показывает позицию-голову («16 ядер») и в спецификацию кладёт её же: цена — её, а не
			// минимум семейства (доп. лицензия на 2 ядра, CAL на устройство вместо CAL на пользователя).
			p.PriceFromRub = d.PriceRub
		default:
			p.PriceFromRub = priceFrom(lx, d.FamilyID)
		}
		if p.VariantsCount == 0 {
			p.VariantsCount = 1
		}
		res = append(res, p)
	}
	return res
}

// priceFrom — наименьшая цена семейства среди позиций с ценой; nil — все «по запросу».
func priceFrom(lx *searchLexicon, fam string) *float64 {
	var best *float64
	for _, pid := range lx.families[fam] {
		d := lx.docs[pid]
		if d.PriceOnRequest || d.PriceRub == nil || isOffer(d) {
			continue // вариант под сделку или организацию (контур доводки) — не цена «от» для всех
		}
		if best == nil || *d.PriceRub < *best {
			v := *d.PriceRub
			best = &v
		}
	}
	return best
}

// specsLine — две характеристики оборудования: части названия после модели («24× 1 Гбит/с, 4× SFP»).
func specsLine(d indexDoc) string {
	parts := strings.Split(d.Title, ",")
	if len(parts) < 2 {
		return ""
	}
	rest := parts[1:]
	if len(rest) > 2 {
		rest = rest[:2]
	}
	for i := range rest {
		rest[i] = strings.TrimSpace(rest[i])
	}
	return strings.Join(rest, " · ")
}

// --- кэш товарной секции ---

// productCache — товарные кандидаты по ключу (текст, версия каталога, версия индекса, вес маржи); ничего
// персонального: ни клиента, ни договорных цен. Одинаковые одновременные запросы объединяются (singleflight).
type productCache struct {
	mu    sync.Mutex
	items map[string]productCacheItem
	max   int
	ttl   time.Duration
	calls map[string]*productCall
}

type productCacheItem struct {
	r  *instantResult
	at time.Time
}

type productCall struct {
	done chan struct{}
	r    *instantResult
	err  error
}

func newProductCache(max int, ttl time.Duration) *productCache {
	return &productCache{items: map[string]productCacheItem{}, max: max, ttl: ttl, calls: map[string]*productCall{}}
}

func (c *productCache) get(key string) (*instantResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	it, ok := c.items[key]
	if !ok || time.Since(it.at) > c.ttl {
		return nil, false
	}
	return it.r, true
}

func (c *productCache) put(key string, r *instantResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) >= c.max {
		c.items = map[string]productCacheItem{} // переполнение — сброс целиком: кэш дешёвый, прогрев быстрый
	}
	c.items[key] = productCacheItem{r: r, at: time.Now()}
}

// do — один расчёт на ключ: остальные ждут его (или свой ctx). Расчёт идёт в фоне с собственным сроком и
// дописывает кэш, даже если первый запрос уже ушёл без товаров.
func (c *productCache) do(ctx context.Context, key string, fn func(context.Context) (*instantResult, error)) (*instantResult, error) {
	c.mu.Lock()
	call, ok := c.calls[key]
	if !ok {
		call = &productCall{done: make(chan struct{})}
		c.calls[key] = call
		go func() {
			bctx, cancel := context.WithTimeout(context.Background(), productsBackgound)
			defer cancel()
			call.r, call.err = fn(bctx)
			if call.err == nil {
				c.put(key, call.r)
			}
			c.mu.Lock()
			delete(c.calls, key)
			c.mu.Unlock()
			close(call.done)
		}()
	}
	c.mu.Unlock()
	select {
	case <-call.done:
		return call.r, call.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func rankedHit(pid string, rank int, score float64) domain.RankedHit {
	return domain.RankedHit{ProductID: pid, Rank: rank, Score: score}
}

// codeRunes — длина ввода без пробелов (для порога маржи).
func codeRunes(s string) int { return utf8.RuneCountInString(strings.ReplaceAll(s, " ", "")) }

// mergeInstant — товарная секция по дополнению словаря (first) и по набранному (rest): сначала семейства дополнения,
// затем остальные без повторов.
func mergeInstant(first, rest *instantResult) *instantResult {
	if first == nil {
		return rest
	}
	if rest == nil {
		return first
	}
	m := *first
	m.families = append([]instantFamily(nil), first.families...)
	seen := map[string]bool{}
	for _, f := range m.families {
		seen[f.id] = true
	}
	for _, f := range rest.families {
		if !seen[f.id] {
			seen[f.id] = true
			m.families = append(m.families, f)
		}
	}
	m.docs = append(append([]indexDoc(nil), first.docs...), rest.docs...)
	m.before = append(append([]panelCandidate(nil), first.before...), rest.before...)
	m.shifts = append(append([]panelShift(nil), first.shifts...), rest.shifts...)
	m.total = len(m.families)
	m.ms = first.ms + rest.ms
	return &m
}

// vendorWeights — вендор → число его семейств с позициями, которые можно купить сейчас; считается один раз на снимок
// словаря.
var vendorWeights sync.Map // *searchLexicon → map[string]int

// vendorWeight — вес вендора в панели.
func vendorWeight(lx *searchLexicon, vendor string) int {
	if v, ok := vendorWeights.Load(lx); ok {
		return v.(map[string]int)[vendor]
	}
	w := map[string]int{}
	for _, fid := range lx.familyOrder {
		for _, pid := range lx.families[fid] {
			if docBuyPrice(lx.docs[pid]) != nil {
				w[lx.familyVendor[fid]]++
				break
			}
		}
	}
	v, _ := vendorWeights.LoadOrStore(lx, w)
	return v.(map[string]int)[vendor]
}

// completionFamilies — семейства цели алиаса-вендора или алиаса-категории прямо из каталога (позиции с ценой есть);
// nil — цель алиаса термин.
func completionFamilies(lx *searchLexicon, c completion) *instantResult {
	if c.vendor == "" && len(c.cats) == 0 {
		return nil
	}
	r := &instantResult{text: c.text}
	for _, fid := range lx.familyOrder {
		if c.vendor != "" && lx.familyVendor[fid] != c.vendor || c.vendor == "" && !contains(c.cats, lx.familyCat[fid]) {
			continue
		}
		var head indexDoc
		found := false
		for _, pid := range lx.families[fid] {
			d := lx.docs[pid]
			if docBuyPrice(d) == nil {
				continue
			}
			if !found || d.LicenseKind == domain.LicenseKindBase && head.LicenseKind != domain.LicenseKindBase {
				head, found = d, true
			}
		}
		if found {
			r.families = append(r.families, instantFamily{id: fid, head: head, rel: 1, pop: head.Popularity, members: 1})
			r.docs = append(r.docs, head)
		}
	}
	if len(r.families) == 0 {
		return nil
	}
	return orderCompletion(lx, r, c.cats, c.text)
}

// orderCompletion — семейства дополнения словаря: только категории цели алиаса (если она — категория), затем точные,
// те, что можно купить сейчас, название содержит текст дополнения («office» → Microsoft Office, а не Workspad),
// продукты раньше курсов и техподдержки, главная категория вендора (где больше его позиций с ценой), шире линейка (позиций
// с ценой), релевантность. «касп» → антивирусы Kaspersky, а не курс; «аст» → Astra Linux SE, а не DCImanager.
func orderCompletion(lx *searchLexicon, r *instantResult, cats []string, text string) *instantResult {
	if r == nil {
		return nil
	}
	m := *r
	fs := make([]instantFamily, 0, len(r.families))
	for _, f := range r.families {
		if len(cats) > 0 && !contains(cats, lx.familyCat[f.id]) {
			continue
		}
		fs = append(fs, f)
	}
	n, named, catCount := map[string]int{}, map[string]bool{}, map[string]int{}
	words := wordsOf(text)
	for _, f := range fs {
		for _, pid := range lx.families[f.id] {
			if docBuyPrice(lx.docs[pid]) != nil {
				n[f.id]++
			}
		}
		name := foldText(lx.familyName[f.id])
		named[f.id] = len(words) > 0
		for _, w := range words {
			named[f.id] = named[f.id] && containsWord(name, w)
		}
	}
	for _, f := range fs {
		catCount[lx.familyCat[f.id]] += n[f.id] // вес категории — позиций с ценой у семейств вендора в ней
	}
	product := func(f instantFamily) bool {
		k := f.head.LicenseKind
		return k != domain.LicenseKindCourse && k != domain.LicenseKindSupport
	}
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		switch {
		case a.exact != b.exact:
			return a.exact
		case (n[a.id] > 0) != (n[b.id] > 0):
			return n[a.id] > 0
		case named[a.id] != named[b.id]:
			return named[a.id]
		case product(a) != product(b):
			return product(a)
		case catCount[lx.familyCat[a.id]] != catCount[lx.familyCat[b.id]]:
			return catCount[lx.familyCat[a.id]] > catCount[lx.familyCat[b.id]]
		case n[a.id] != n[b.id]:
			return n[a.id] > n[b.id]
		}
		return a.rel > b.rel
	})
	m.families = fs
	m.total = len(fs)
	return &m
}
