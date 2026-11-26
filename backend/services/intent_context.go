package services

import (
	"sort"
	"strings"
	"unicode/utf8"

	"kuznica/api/domain"
)

// Срез справочника для промпта разбора (маршрут D). Целиком справочник фида — 1 232 вендора и 551 категория,
// около 17 тыс. входных токенов на вызов. В промпт идут только кандидаты по словам запроса: совпадения со словами
// названий вендоров, категорий и семейств (целиком, по основе, транслитом, по звучанию), алиасы словаря и то,
// что уже нашли правила. Сверка ответа (intent_validate.go) остаётся по всему справочнику: id вне среза, но из
// каталога принимается, выдуманный — отбрасывается.

// Размер среза.
const (
	intentMaxVendors    = 20
	intentMaxCategories = 15
	// intentMinCategories — если кандидатов-категорий меньше, в срез добавляются корневые разделы: запрос о
	// задаче («чем чертить планы») словами названий не совпадает, а раздел модель выберет по смыслу.
	intentMinCategories = 5
	// intentMaxFamilies — сколько лучших совпавших семейств дают своих вендоров и категории.
	intentMaxFamilies = 40
)

// Веса совпадений.
const (
	intentScoreRules  = 100 // нашли правила
	intentScoreAlias  = 30  // алиас словаря с этой целью
	intentScoreName   = 10  // слово названия вендора или категории
	intentScoreFamily = 3   // слово названия семейства этого вендора или категории
)

// intentToken — слово названия с заранее посчитанными формами для сравнения.
type intentToken struct{ w, stem, phon string }

func newIntentToken(w string) intentToken {
	t := intentToken{w: w, stem: stemWord(w)}
	if utf8.RuneCountInString(w) >= minPhoneticRunes {
		if k := phoneticKey(w); utf8.RuneCountInString(k) >= minPhoneticKey {
			t.phon = k
		}
	}
	return t
}

func intentTokens(s string) []intentToken {
	var out []intentToken
	for _, w := range strings.Fields(intentNorm(s)) {
		if lexStopWords[w] || utf8.RuneCountInString(w) < 2 {
			continue
		}
		out = append(out, newIntentToken(w))
	}
	return out
}

// matches — слово запроса q совпадает со словом названия n: целиком; по основе; одно — начало другого (от 4 букв);
// кириллица транслитом равна латинице; одинаковый фонетический ключ («фотошоп» — photoshop).
func (q intentToken) matches(n intentToken) bool {
	switch {
	case q.w == n.w:
		return true
	case hasDigit(q.w) || hasDigit(n.w):
		return false // числа и версии — только целиком
	case q.stem == n.stem && utf8.RuneCountInString(q.stem) >= minStemRunes:
		return true
	case q.phon != "" && q.phon == n.phon:
		return true
	}
	a, b := q.w, n.w
	if utf8.RuneCountInString(a) > utf8.RuneCountInString(b) {
		a, b = b, a
	}
	if utf8.RuneCountInString(a) >= 4 && strings.HasPrefix(b, a) {
		return true
	}
	if isCyrillicWord(q.w) && isLatinWord(n.w) {
		// «касперского» — kaspersky: основа транслитом — начало латинского слова.
		lat := translit(q.stem)
		return translit(q.w) == n.w || utf8.RuneCountInString(lat) >= 5 && strings.HasPrefix(n.w, lat)
	}
	return false
}

// intentIndex — слова названий справочника для подбора среза.
type intentIndex struct {
	vendors    map[string][]intentToken
	categories map[string][]intentToken
	families   map[string][]intentToken
	roots      []string
	parent     map[string]string
}

func newIntentIndex(snap *domain.CatalogSnapshot, families map[string]intentFamily) *intentIndex {
	ix := &intentIndex{vendors: map[string][]intentToken{}, categories: map[string][]intentToken{},
		families: map[string][]intentToken{}, parent: map[string]string{}}
	for _, v := range snap.Vendors {
		ix.vendors[v.ID] = intentTokens(v.Name + " " + strings.ReplaceAll(v.ID, "-", " "))
	}
	for _, c := range snap.Categories {
		ix.categories[c.ID] = intentTokens(c.Name)
		if c.ParentID == nil || *c.ParentID == "" {
			ix.roots = append(ix.roots, c.ID)
		} else {
			ix.parent[c.ID] = *c.ParentID
		}
	}
	sort.Strings(ix.roots)
	for id, f := range families {
		ix.families[id] = intentTokens(f.Name)
	}
	return ix
}

// intentHits — сколько слов запроса совпало со словами названия.
func intentHits(query, name []intentToken) int {
	n := 0
	for _, q := range query {
		for _, t := range name {
			if q.matches(t) {
				n++
				break
			}
		}
	}
	return n
}

// intentSlice — кандидаты в вендоры и категории для одного запроса.
type intentSlice struct {
	vendors, categories []string
}

// slice — кандидаты по словам запроса, алиасам словаря и разбору правил.
func (d *intentDict) slice(normalized string, guess domain.Intent) intentSlice {
	ix := d.index
	query := intentTokens(normalized)
	vScore, cScore := map[string]int{}, map[string]int{}
	addCat := func(id string, sc int) {
		if id == "" || d.categories[id] == "" {
			return
		}
		cScore[id] += sc
	}
	// Алиасы словаря: фраза алиаса — слова запроса подряд.
	padded := " " + intentNorm(normalized) + " "
	for key, a := range d.aliases {
		if key == "" || !strings.Contains(padded, " "+key+" ") {
			continue
		}
		switch a.Target {
		case domain.AliasTargetVendor:
			if _, ok := d.vendors[a.TargetID]; ok {
				vScore[a.TargetID] += intentScoreAlias
			}
		case domain.AliasTargetCategory:
			addCat(a.TargetID, intentScoreAlias)
		default:
			query = append(query, intentTokens(a.Canonical)...) // «винда» → windows: ищем и по канонической форме
		}
	}
	if len(query) > 0 {
		for id, name := range ix.vendors {
			if n := intentHits(query, name); n > 0 {
				vScore[id] += n * intentScoreName
			}
		}
		for id, name := range ix.categories {
			if n := intentHits(query, name); n > 0 {
				addCat(id, n*intentScoreName)
			}
		}
		type famHit struct {
			id string
			n  int
		}
		var fams []famHit
		for id, name := range ix.families {
			if n := intentHits(query, name); n > 0 {
				fams = append(fams, famHit{id, n})
			}
		}
		sort.Slice(fams, func(i, j int) bool {
			if fams[i].n != fams[j].n {
				return fams[i].n > fams[j].n
			}
			return fams[i].id < fams[j].id
		})
		if len(fams) > intentMaxFamilies {
			fams = fams[:intentMaxFamilies]
		}
		for _, fh := range fams {
			f := d.families[fh.id]
			if _, ok := d.vendors[f.VendorID]; ok {
				vScore[f.VendorID] += fh.n * intentScoreFamily
			}
			addCat(f.CategoryID, fh.n*intentScoreFamily)
		}
	}
	if guess.VendorID != "" {
		if _, ok := d.vendors[guess.VendorID]; ok {
			vScore[guess.VendorID] += intentScoreRules
		}
	}
	addCat(guess.CategoryID, intentScoreRules)
	if f, ok := d.families[guess.ReferenceFamily]; ok {
		vScore[f.VendorID] += intentScoreRules
		addCat(f.CategoryID, intentScoreRules)
	}
	s := intentSlice{vendors: topScored(vScore, intentMaxVendors), categories: topScored(cScore, intentMaxCategories)}
	if len(s.categories) < intentMinCategories {
		in := map[string]bool{}
		for _, c := range s.categories {
			in[c] = true
		}
		for _, r := range ix.roots {
			if !in[r] {
				s.categories = append(s.categories, r)
			}
		}
	}
	return s
}

// topScored — до n id с наибольшим счётом (при равенстве — по id).
func topScored(score map[string]int, n int) []string {
	ids := make([]string, 0, len(score))
	for id, sc := range score {
		if sc > 0 {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		if score[ids[i]] != score[ids[j]] {
			return score[ids[i]] > score[ids[j]]
		}
		return ids[i] < ids[j]
	})
	if len(ids) > n {
		ids = ids[:n]
	}
	return ids
}

// promptFor — системный промпт разбора с срезом справочника для этого запроса.
func (d *intentDict) promptFor(normalized string, guess domain.Intent) string {
	s := d.slice(normalized, guess)
	cats := make([]string, 0, len(s.categories))
	for _, id := range s.categories {
		cats = append(cats, id+" — "+d.categories[id])
	}
	vends := make([]string, 0, len(s.vendors))
	for _, id := range s.vendors {
		vends = append(vends, id+" — "+d.vendors[id].Name)
	}
	if len(vends) == 0 {
		vends = []string{"(кандидатов нет — vendor: null)"}
	}
	return strings.NewReplacer("{{CATEGORIES}}", strings.Join(cats, "\n"), "{{VENDORS}}", strings.Join(vends, "\n")).Replace(d.promptText)
}
