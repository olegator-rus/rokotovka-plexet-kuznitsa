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
	for _, p := range snap.Products {
		c.products[p.ID] = p
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
	if expand {
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
