package services

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"kuznica/api/domain"
	"kuznica/api/pkg/errs"
)

// Каталог для инструментов агента: снимок из базы (или источника) в памяти запуска. Поиск здесь — не
// боевой: без порога, бизнес-слоя и индекса OpenSearch, по словам карточек, подстрокам, транслиту и
// векторам семейств, чтобы проверить, есть ли товар вообще («нет в топ-10» ≠ «нет в каталоге»).

type agentFamily struct {
	ID         string
	Name       string
	VendorID   string
	CategoryID string
	Products   []string
	words      map[string]bool
	text       string
}

type agentCatalog struct {
	products   map[string]domain.Product
	families   map[string]*agentFamily
	order      []string
	vendors    map[string]domain.Vendor
	categories map[string]domain.Category
	relations  []domain.Relation
	known      map[string]bool
	// vendorWords — слова названия производителя и его алиасов → производитель.
	vendorWords map[string]string

	vecOnce sync.Once
	vecs    [][]float32
}

// agentCatalog — снимок каталога для запуска.
func (s *Services) agentCatalog(ctx context.Context) (*agentCatalog, error) {
	var snap *domain.CatalogSnapshot
	if s.repos.Enabled() {
		var err error
		if snap, err = s.repos.LoadCatalog(ctx); err != nil {
			return nil, err
		}
	}
	if snap == nil && s.source != nil {
		var err error
		if snap, err = s.source.Load(ctx); err != nil {
			return nil, errs.NewUnavailableError(MsgSourceBroken).WithCause(err)
		}
	}
	if snap == nil {
		return nil, errs.NewUnavailableError(MsgNoSource)
	}
	return newAgentCatalog(snap), nil
}

func newAgentCatalog(snap *domain.CatalogSnapshot) *agentCatalog {
	c := &agentCatalog{products: map[string]domain.Product{}, families: map[string]*agentFamily{}, vendors: map[string]domain.Vendor{},
		categories: map[string]domain.Category{}, relations: snap.Relations, known: map[string]bool{}, vendorWords: map[string]string{}}
	for _, v := range snap.Vendors {
		c.vendors[v.ID] = v
		c.vendorWords[strings.Join(wordsOf(v.Name), " ")] = v.ID
		c.vendorWords[strings.Join(wordsOf(v.ID), " ")] = v.ID
	}
	for _, a := range snap.Aliases {
		if a.Target == domain.AliasTargetVendor && a.TargetID != "" {
			c.vendorWords[strings.Join(wordsOf(a.Alias), " ")] = a.TargetID
		}
	}
	for _, k := range snap.Categories {
		c.categories[k.ID] = k
	}
	large := len(snap.Products) > agentLargeCatalog
	for _, p := range snap.Products {
		f := c.families[p.FamilyID]
		if f == nil {
			f = &agentFamily{ID: p.FamilyID, Name: p.FamilyName, VendorID: p.VendorID, CategoryID: p.CategoryID, words: map[string]bool{}}
			c.families[p.FamilyID] = f
			c.order = append(c.order, p.FamilyID)
		}
		f.Products = append(f.Products, p.ID)
		txt := strings.Join([]string{p.Title, p.FamilyName, p.SKU, p.Description, attributesText(p.Attributes)}, " ")
		for _, w := range wordsOf(txt) {
			f.words[w] = true
			c.known[w] = true
		}
		if f.text == "" {
			f.text = p.FamilyName + ". " + p.Description
		}
		if large {
			// Фид заказчика (136 тыс. позиций): слова карточки уже в f.words, инструментам нужны название, лицензия,
			// цена и начало описания — параметры и полное описание в памяти запуска не держим.
			p.Attributes, p.Description = nil, truncateRunes(p.Description, 400)
		}
		c.products[p.ID] = p
	}
	for _, f := range c.families {
		for _, w := range wordsOf(c.vendorName(f.VendorID) + " " + c.categories[f.CategoryID].Name) {
			f.words[w] = true
			c.known[w] = true
		}
	}
	return c
}

func (c *agentCatalog) vendorName(id string) string {
	if v, ok := c.vendors[id]; ok {
		return v.Name
	}
	return id
}

func (c *agentCatalog) familyOfProduct(pid string) string {
	if p, ok := c.products[pid]; ok {
		return p.FamilyID
	}
	return pid
}

// unknownWords — слова текста, которых нет в каталоге.
func (c *agentCatalog) unknownWords(text string) []string {
	var res []string
	for _, w := range wordsOf(text) {
		if !c.known[w] && !lexStopWords[w] {
			res = append(res, w)
		}
	}
	return res
}

// homoglyphs — латинские буквы, похожие на кириллицу: «1C» и «1С» — один производитель.
var vendorHomoglyphs = strings.NewReplacer("a", "а", "c", "с", "e", "е", "o", "о", "p", "р", "x", "х", "k", "к", "m", "м", "t", "т", "b", "в", "h", "н", "y", "у")

// resolveVendor — производитель по названию из ответа модели: id, название или алиас словаря.
func (c *agentCatalog) resolveVendor(name string) (string, bool) {
	key := strings.Join(wordsOf(name), " ")
	if key == "" {
		return "", false
	}
	if id, ok := c.vendorWords[key]; ok {
		return id, true
	}
	hk := vendorHomoglyphs.Replace(key)
	for k, id := range c.vendorWords {
		if vendorHomoglyphs.Replace(k) == hk {
			return id, true
		}
	}
	// «Microsoft Corporation», «компания Kaspersky» — производитель словом внутри.
	if id := c.vendorOfText(name); id != "" {
		return id, true
	}
	// Модель назвала продукт, а не производителя («КОМПАС-3D») — производитель семейства с таким названием.
	for _, fid := range c.order {
		f := c.families[fid]
		fn := strings.Join(wordsOf(f.Name), " ")
		if fn == key || strings.HasPrefix(fn, key+" ") {
			return f.VendorID, true
		}
	}
	return "", false
}

// vendorOfText — производитель, названный словом или фразой текста; "" — не назван.
func (c *agentCatalog) vendorOfText(text string) string {
	ws := wordsOf(text)
	for n := 3; n >= 1; n-- {
		for i := 0; i+n <= len(ws); i++ {
			if id, ok := c.vendorWords[strings.Join(ws[i:i+n], " ")]; ok {
				return id
			}
		}
	}
	return ""
}

// catalogHit — семейство в результате search_catalog.
type catalogHit struct {
	FamilyID string   `json:"family_id"`
	Name     string   `json:"name"`
	Vendor   string   `json:"vendor"`
	VendorID string   `json:"vendor_id"`
	Category string   `json:"category"`
	Origin   string   `json:"origin"`
	Matched  []string `json:"matched"`
	Score    float64  `json:"score"`
}

// translit — простая транслитерация кириллицы в латиницу и обратно (для расширенного поиска).
var ruLat = map[rune]string{'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e", 'ж': "zh", 'з': "z", 'и': "i",
	'й': "y", 'к': "k", 'л': "l", 'м': "m", 'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u", 'ф': "f",
	'х': "h", 'ц': "c", 'ч': "ch", 'ш': "sh", 'щ': "sch", 'ы': "y", 'э': "e", 'ю': "yu", 'я': "ya", 'ь': "", 'ъ': ""}

func translit(w string) string {
	var b strings.Builder
	for _, r := range w {
		if s, ok := ruLat[r]; ok {
			b.WriteString(s)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// search — семейства по словам запроса; expand — подстроки, транслит и смысловая близость.
func (c *agentCatalog) search(ctx context.Context, s *Services, query string, expand bool, limit int) []catalogHit {
	var qws []string
	for _, w := range wordsOf(query) {
		if !lexStopWords[w] {
			qws = append(qws, w)
		}
	}
	score := map[string]float64{}
	matched := map[string][]string{}
	add := func(f, how string, v float64) {
		score[f] += v
		matched[f] = append(matched[f], how)
	}
	for _, fid := range c.order {
		f := c.families[fid]
		for _, w := range qws {
			switch {
			case f.words[w]:
				add(fid, "слово:"+w, 1)
			case utf8.RuneCountInString(w) >= 4 && hasPrefixWord(f.words, w):
				add(fid, "начало слова:"+w, 0.8)
			case expand && utf8.RuneCountInString(w) >= 3 && hasSubstring(f.words, w):
				add(fid, "подстрока:"+w, 0.5)
			case expand && translit(w) != w && (f.words[translit(w)] || utf8.RuneCountInString(w) >= 4 && hasPrefixWord(f.words, translit(w))):
				add(fid, "транслит:"+translit(w), 0.7)
			}
		}
	}
	if expand && len(c.order) > agentVecFamilies {
		// Большой каталог (фид заказчика, 12,5 тыс. семейств): векторы семейств уже лежат в индексе поиска — kNN по
		// нему вместо эмбеддинга всех семейств в каждом запуске (минуты на общем сервисе моделей).
		for fid, cs := range c.knnFamilies(ctx, s, query) {
			if cs >= 0.5 && c.families[fid] != nil {
				add(fid, "смысл:"+jsonFloat(cs), cs)
			}
		}
	} else if expand {
		if vecs := c.familyVectors(ctx, s); vecs != nil {
			if qv := s.embedTexts(ctx, []string{query}); len(qv) == 1 {
				for i, fid := range c.order {
					if cs := cosine(qv[0], vecs[i]); cs >= 0.5 {
						add(fid, "смысл:"+jsonFloat(cs), cs)
					}
				}
			}
		}
	}
	var hits []catalogHit
	for fid, sc := range score {
		f := c.families[fid]
		origin := c.vendors[f.VendorID].Origin
		hits = append(hits, catalogHit{FamilyID: fid, Name: f.Name, Vendor: c.vendorName(f.VendorID), VendorID: f.VendorID,
			Category: c.categories[f.CategoryID].Name, Origin: origin, Matched: matched[fid], Score: evalRound4(sc)})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].FamilyID < hits[j].FamilyID
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

func jsonFloat(v float64) string {
	b, _ := json.Marshal(evalRound4(v))
	return string(b)
}

func hasPrefixWord(words map[string]bool, w string) bool {
	for k := range words {
		if strings.HasPrefix(k, w) {
			return true
		}
	}
	return false
}

func hasSubstring(words map[string]bool, w string) bool {
	for k := range words {
		if utf8.RuneCountInString(k) >= 3 && (strings.Contains(k, w) || strings.Contains(w, k)) {
			return true
		}
	}
	return false
}

// familyVectors — векторы семейств (название и описание), один раз за запуск.
func (c *agentCatalog) familyVectors(ctx context.Context, s *Services) [][]float32 {
	c.vecOnce.Do(func() {
		texts := make([]string, len(c.order))
		for i, fid := range c.order {
			texts[i] = c.families[fid].text
		}
		c.vecs = s.embedTexts(ctx, texts)
	})
	return c.vecs
}

// details — карточка семейства для get_product_details.
func (c *agentCatalog) details(f *agentFamily) map[string]any {
	var pos []map[string]any
	desc := ""
	for i, pid := range f.Products {
		p := c.products[pid]
		if desc == "" {
			desc = p.Description
		}
		if i >= 8 {
			continue
		}
		pos = append(pos, map[string]any{"product_id": p.ID, "title": p.Title, "license": agentLicenseLine(p.License),
			"price_rub": p.PriceRub, "price_on_request": p.PriceOnRequest})
	}
	v := c.vendors[f.VendorID]
	return map[string]any{"family_id": f.ID, "name": f.Name, "vendor": map[string]any{"id": v.ID, "name": v.Name, "origin": v.Origin},
		"category":    map[string]any{"id": f.CategoryID, "name": c.categories[f.CategoryID].Name},
		"description": truncateRunes(desc, 400), "positions": pos, "positions_total": len(f.Products)}
}

// relationsOf — связи семейства в обе стороны.
func (c *agentCatalog) relationsOf(fid string) []map[string]any {
	var res []map[string]any
	for _, r := range c.relations {
		other, dir := "", ""
		switch fid {
		case r.FromFamily:
			other, dir = r.ToFamily, "out"
		case r.ToFamily:
			other, dir = r.FromFamily, "in"
		default:
			continue
		}
		name := other
		vendor := ""
		if f := c.families[other]; f != nil {
			name, vendor = f.Name, c.vendorName(f.VendorID)
		}
		res = append(res, map[string]any{"type": r.Type, "family_id": other, "name": name, "vendor": vendor, "direction": dir, "note": r.Note})
	}
	return res
}

// licenseLine — лицензия позиции одной строкой (для карточки агента).
func agentLicenseLine(l domain.License) string {
	parts := []string{l.Type}
	if l.Metric != nil {
		parts = append(parts, *l.Metric)
	}
	if l.Quantity != nil {
		parts = append(parts, jsonFloat(float64(*l.Quantity)))
	}
	if l.DurationMonths != nil {
		parts = append(parts, jsonFloat(float64(*l.DurationMonths))+" мес")
	}
	if l.Edition != "" {
		parts = append(parts, l.Edition)
	}
	return strings.Join(parts, " · ")
}

// Защита от подстановки (ревью M4): цель правки и производитель термина определяет код.

// canonicalWords — значимые слова canonical для сверки с названием цели (латиница и кириллица одного
// вида — одно слово: «1C» и «1С»).
func canonicalWords(text string) []string {
	var res []string
	for _, w := range wordsOf(text) {
		if lexStopWords[w] {
			continue
		}
		res = append(res, vendorHomoglyphs.Replace(w))
	}
	return res
}

// nameSet — слова названий в той же нормализации.
func nameSet(texts ...string) map[string]bool {
	res := map[string]bool{}
	for _, t := range texts {
		for _, w := range wordsOf(t) {
			res[vendorHomoglyphs.Replace(w)] = true
		}
	}
	return res
}

// checkCanonical — цель есть в каталоге, и слова canonical — слова её названия (семейство: название
// семейства, его позиций и производителя; производитель: название, id и алиасы словаря; категория: название
// категории и родителя). Возвращает производителя цели ("" у категории).
func (c *agentCatalog) checkCanonical(target, targetID, canonical string) (string, error) {
	var names map[string]bool
	var vendor, title string
	switch target {
	case domain.AliasTargetVendor:
		v, ok := c.vendors[targetID]
		if !ok {
			return "", terr("производителя %q нет в каталоге", targetID)
		}
		vendor, title = v.ID, v.Name
		texts := []string{v.Name, v.ID}
		for k, id := range c.vendorWords {
			if id == v.ID {
				texts = append(texts, k)
			}
		}
		names = nameSet(texts...)
	case domain.AliasTargetFamily:
		f := c.families[targetID]
		if f == nil {
			return "", terr("семейства %q нет в каталоге", targetID)
		}
		vendor, title = f.VendorID, f.Name
		texts := []string{f.Name, c.vendorName(f.VendorID)}
		for _, pid := range f.Products {
			texts = append(texts, c.products[pid].Title)
		}
		names = nameSet(texts...)
	case domain.AliasTargetCategory:
		k, ok := c.categories[targetID]
		if !ok {
			return "", terr("категории %q нет в каталоге", targetID)
		}
		title = k.Name
		texts := []string{k.Name}
		if k.ParentID != nil {
			texts = append(texts, c.categories[*k.ParentID].Name)
		}
		names = nameSet(texts...)
	}
	ws := canonicalWords(canonical)
	if len(ws) == 0 {
		return "", terr("canonical «%s» без значимых слов", canonical)
	}
	var extra []string
	for _, w := range ws {
		if !names[w] {
			extra = append(extra, w)
		}
	}
	if len(extra) > 0 {
		return "", terr("canonical «%s» не называет цель %s «%s» (лишние слова: %s): поиск подставит canonical, а не target_id",
			canonical, target, title, strings.Join(extra, ", "))
	}
	return vendor, nil
}

// termVendor — производитель канонической формы алиаса на термин: названный в ней производитель, иначе
// единственный производитель семейств, в словах которых есть все слова формы; общий термин («антивирус»,
// несколько производителей) — "".
func (c *agentCatalog) termVendor(canonical string) string {
	if v := c.vendorOfText(canonical); v != "" {
		return v
	}
	ws := wordsOf(canonical)
	if len(ws) == 0 {
		return ""
	}
	vendors := map[string]bool{}
	for _, fid := range c.order {
		f := c.families[fid]
		all := true
		for _, w := range ws {
			if !lexStopWords[w] && !f.words[w] {
				all = false
				break
			}
		}
		if all {
			vendors[f.VendorID] = true
		}
	}
	if len(vendors) != 1 {
		return ""
	}
	for v := range vendors {
		return v
	}
	return ""
}

// vendorNames — названия производителей из доказательств, через запятую, по алфавиту.
func (c *agentCatalog) vendorNames(ev map[string]string) string {
	var names []string
	for v := range ev {
		names = append(names, c.vendorName(v))
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// termVendorMinScore — совпадение слова с каталогом, которое считается доказательством производителя:
// слово, начало слова, транслит или смысловая близость не ниже 0,7 (подстрока — 0,5 — нет).
const termVendorMinScore = 0.7

// termVendors — чей продукт называют словом, по данным, а не по ответу модели: производитель в самом
// слове; семейства, по которым кликали после переформулировки запросов кластера с этим словом; первое
// семейство поиска слова по каталогу. Значение — откуда доказательство.
func (cr *clusterRun) termVendors(ctx context.Context, alias string) map[string]string {
	cat := cr.rc.cat
	res := map[string]string{}
	if v := cat.vendorOfText(alias); v != "" {
		res[v] = "производитель в слове"
	}
	aw := wordsOf(alias)
	var qs []clusterQuery
	for _, q := range cr.c.Queries {
		if containsWords(wordsOf(q.Text), aw) {
			qs = append(qs, q)
		}
	}
	if len(qs) == 0 {
		qs = cr.c.Queries
	}
	for _, q := range qs {
		for _, r := range q.ReformulatedTo {
			if r.Clicked == 0 || r.ClickedOn == "" {
				continue
			}
			if f := cat.families[cat.familyOfProduct(r.ClickedOn)]; f != nil && res[f.VendorID] == "" {
				res[f.VendorID] = "клик после переформулировки «" + shortText(q.Text, 40) + "» → «" + shortText(r.Text, 40) + "»"
			}
		}
	}
	if hits := cat.search(ctx, cr.rc.a.s, alias, true, 1); len(hits) > 0 && hits[0].Score >= termVendorMinScore {
		if res[hits[0].VendorID] == "" {
			res[hits[0].VendorID] = "каталог: " + hits[0].Name
		}
	}
	return res
}

// containsWords — все слова sub есть в ws.
func containsWords(ws, sub []string) bool {
	if len(sub) == 0 {
		return false
	}
	set := map[string]bool{}
	for _, w := range ws {
		set[w] = true
	}
	for _, w := range sub {
		if !set[w] {
			return false
		}
	}
	return true
}

// purchasable — позиций семейства, которые можно купить сейчас (цена есть, не «по запросу»).
func (c *agentCatalog) purchasable(fid string) int {
	f := c.families[fid]
	if f == nil {
		return 0
	}
	n := 0
	for _, pid := range f.Products {
		if p := c.products[pid]; !p.PriceOnRequest && p.PriceRub != nil && *p.PriceRub > 0 {
			n++
		}
	}
	return n
}

// agentLargeCatalog — больше стольких позиций каталог считается большим (фид): позиции хранятся без параметров.
const agentLargeCatalog = 20000

// agentVecFamilies — до стольких семейств векторы считаются в запуске (mock); больше — kNN по индексу поиска.
const agentVecFamilies = 3000

// knnFamilies — ближайшие семейства по вектору запроса из индекса поиска: семейство → косинус (лучшая позиция
// семейства). Индекс без векторов или сервис моделей недоступен — пусто (расширенный поиск остаётся по словам).
func (c *agentCatalog) knnFamilies(ctx context.Context, s *Services, query string) map[string]float64 {
	res := map[string]float64{}
	if s.search == nil || !s.search.Enabled() || !s.embedder.Enabled() || s.vectorsOff(ctx) {
		return res
	}
	vec, err := s.queryVector(ctx, query)
	if err != nil {
		return res
	}
	hits, err := s.search.Search(ctx, s.alias, knnQuery(vec, 60))
	if err != nil {
		return res
	}
	top, docs, err := parseHits(hits)
	if err != nil {
		return res
	}
	for _, h := range top {
		d, ok := docs[h.ProductID]
		if !ok || d.FamilyID == "" {
			continue
		}
		// lucene cosinesimil: score = (1 + cos) / 2.
		cs := 2*h.Score - 1
		if cs > res[d.FamilyID] {
			res[d.FamilyID] = cs
		}
	}
	return res
}
