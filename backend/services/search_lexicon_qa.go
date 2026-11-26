package services

import (
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"kuznica/api/domain"
)

// Дополнения словаря каталога по ручному обходу продукта: транслит слов каталога
// («лт» → lt, «antivirus» → антивирус), начало слова («win» → windows), слова категорий и слова назначения
// («видеоконференции», «созвониться», «чертежи»), масштаб лицензий семейства (домашняя линейка или бизнес).

// minPhoneticRunes, minPhoneticKey — фонетическое совпадение кириллического слова с латинским словом каталога
// («вмваре» → vmware) — только для слов от 5 букв с ключом от 4 согласных: короткие ключи совпадают случайно.
const (
	minPhoneticRunes = 5
	minPhoneticKey   = 4
)

// minPrefixRunes — с какой длины недописанное слово ищется как начало слова каталога.
const minPrefixRunes = 3

// isLatinWord / isCyrillicWord — слово целиком из букв одного письма.
func isLatinWord(w string) bool {
	if w == "" {
		return false
	}
	for _, r := range w {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

func isCyrillicWord(w string) bool {
	if w == "" {
		return false
	}
	for _, r := range w {
		if !unicode.Is(unicode.Cyrillic, r) {
			return false
		}
	}
	return true
}

// buildTranslit — ключ транслита → слово каталога. Латинские слова — из названий (core), кириллические — из
// названий и описаний (known): «антивирус» есть в названии ESET NOD32 Антивирус. Ключ с двумя разными словами
// не используется.
func (lx *searchLexicon) buildTranslit() {
	lx.translit = map[string]string{}
	bad := map[string]bool{}
	add := func(key, w string) {
		if utf8.RuneCountInString(key) < 2 || bad[key] {
			return
		}
		if prev, ok := lx.translit[key]; ok && prev != w {
			delete(lx.translit, key)
			bad[key] = true
			return
		}
		lx.translit[key] = w
	}
	for w := range lx.core {
		if isLatinWord(w) {
			add("lat:"+w, w)
			if utf8.RuneCountInString(w) >= minPhoneticRunes {
				if k := phoneticKey(w); utf8.RuneCountInString(k) >= minPhoneticKey {
					add("phon:"+k, w)
				}
			}
		}
	}
	for w := range lx.known {
		if isCyrillicWord(w) && !lexStopWords[w] && !lexServiceWords[w] {
			add("cyr:"+translit(w), w)
		}
	}
}

// translitOf — слово каталога, которое пишется так же другим письмом; "" — нет.
func (lx *searchLexicon) translitOf(t string) string {
	switch {
	case isCyrillicWord(t):
		if w := lx.translit["lat:"+translit(t)]; w != "" {
			return w
		}
		// Фонетический ключ («вмваре» → vmware): только для слов от minPhoneticRunes букв и однозначного ключа.
		if utf8.RuneCountInString(t) >= minPhoneticRunes {
			if k := phoneticKey(t); utf8.RuneCountInString(k) >= minPhoneticKey {
				return lx.translit["phon:"+k]
			}
		}
		return ""
	case isLatinWord(t) && utf8.RuneCountInString(t) >= 4:
		return lx.translit["cyr:"+t]
	}
	return ""
}

// prefixOf — слово названий каталога, которое начинается с t («win» → windows): самое частое, при равенстве —
// первое по алфавиту. "" — нет такого или t короче minPrefixRunes.
func (lx *searchLexicon) prefixOf(t string) string {
	if utf8.RuneCountInString(t) < minPrefixRunes || !(isLatinWord(t) || isCyrillicWord(t)) {
		return ""
	}
	best, n := "", 0
	for w, c := range lx.core {
		if len(w) <= len(t) || !strings.HasPrefix(w, t) {
			continue
		}
		if c > n || c == n && w < best {
			best, n = w, c
		}
	}
	return best
}

// purposeStems — слова назначения → основа слова названия категории (не id: справочник организатора будет
// другим, а слова категорий — те же). Это словарь задач покупателя, а не сленга продуктов.
var purposeCategoryStems = []struct{ stem, category string }{
	{"видеоконф", "видеоконференц"}, {"видеосвяз", "видеоконференц"}, {"видеозвон", "видеоконференц"},
	{"созвон", "видеоконференц"}, {"созван", "видеоконференц"}, {"звонк", "видеоконференц"}, {"конференц", "видеоконференц"}, {"вебинар", "видеоконференц"},
	{"совещан", "видеоконференц"}, {"удаленк", "удаленн"}, {"удаленн", "удаленн"},
	{"чертеж", "сапр"}, {"чертит", "сапр"}, {"проектирован", "сапр"},
	{"бухгалт", "учетные"}, {"бухучет", "учетные"}, {"зарплат", "учетные"}, {"кадров", "учетные"},
	{"бэкап", "резервн"}, {"резервн", "резервн"},
}

// categoryStemLen — длина основы слова категории: «видеоконференции» и «видеоконференций» совпадают.
const categoryStemLen = 6

func stemOf(w string) string {
	rs := []rune(w)
	if len(rs) > categoryStemLen {
		rs = rs[:categoryStemLen]
	}
	return string(rs)
}

// buildCategoryWords — основа значимого слова названия категории → категория. Слово, которое встречается в
// категориях разных корней («системы», «доступа»), не используется; в пределах одного корня берётся самая
// глубокая категория с этим словом (корневая — если слово только в ней).
func (lx *searchLexicon) buildCategoryWords(cats []domain.Category) {
	lx.categoryWords, lx.categoryWordText = map[string]string{}, map[string]string{}
	roots := map[string]map[string]bool{}
	byStem := map[string][]string{}
	ids := make([]string, 0, len(cats))
	for _, c := range cats {
		ids = append(ids, c.ID)
	}
	sort.Strings(ids)
	for _, id := range ids {
		for _, w := range wordsOf(lx.categoryName[id]) {
			if utf8.RuneCountInString(w) < 4 || lexStopWords[w] || lexServiceWords[w] || !isCyrillicWord(w) && !isLatinWord(w) {
				continue
			}
			st := stemOf(w)
			if lx.categoryWordText[st] == "" {
				lx.categoryWordText[st] = w
			}
			if roots[st] == nil {
				roots[st] = map[string]bool{}
			}
			roots[st][lx.categoryRoot[id]] = true
			byStem[st] = append(byStem[st], id)
		}
	}
	for st, cs := range byStem {
		if len(roots[st]) != 1 {
			continue
		}
		// Слово в названии корня или в нескольких подкатегориях («антивирус» — дома и бизнеса) — категория-корень.
		root := lx.categoryRoot[cs[0]]
		subs := map[string]bool{}
		inRoot := false
		for _, c := range cs {
			if c == root {
				inRoot = true
			} else {
				subs[c] = true
			}
		}
		best := root
		if !inRoot && len(subs) == 1 {
			best = cs[0]
		}
		lx.categoryWords[st] = best
	}
}

// categoryOf — категория по словам запроса: слово названия категории или слово назначения. Несколько разных
// корней — неоднозначно, "". words — слова категорий, подставленные за слова назначения («созваниваться» →
// видеоконференции): ими расширяется текстовый поиск длинного запроса.
func (lx *searchLexicon) categoryOf(tokens []string) (string, []string) {
	found := ""
	var words []string
	for _, t := range tokens {
		for _, w := range wordsOf(t) {
			if utf8.RuneCountInString(w) < 4 || lexStopWords[w] || lexServiceWords[w] {
				continue
			}
			c := lx.categoryWords[stemOf(w)]
			if c == "" {
				for _, p := range purposeCategoryStems {
					if strings.HasPrefix(w, p.stem) {
						c = lx.categoryWords[stemOf(p.category)]
						if w := lx.categoryWordText[stemOf(p.category)]; c != "" && !slices.Contains(words, w) {
							words = append(words, w)
						}
						break
					}
				}
			}
			switch {
			case c == "":
			case found == "":
				found = c
			case lx.categoryRoot[found] != lx.categoryRoot[c]:
				return "", nil
			case lx.categoryRoot[found] == found && c != found:
				found = c // подкатегория точнее корня
			}
		}
	}
	return found, words
}

// consumerWords — категория или семейство для дома. Масштаб по числу мест не годится: 1С и САПР продаются
// по одному рабочему месту, но это не домашние линейки.
var consumerWords = []string{"для дома", "домашн", "home", "personal", "family", "для семьи"}

// buildScale — масштаб семейства: самая крупная лицензия и признак домашней линейки.
func (lx *searchLexicon) buildScale() {
	lx.familyCap, lx.consumer, lx.wordVendors = map[string]int{}, map[string]bool{}, map[string]map[string]bool{}
	for _, f := range lx.familyOrder {
		for w := range lx.familyWords[f] {
			if lx.wordVendors[w] == nil {
				lx.wordVendors[w] = map[string]bool{}
			}
			lx.wordVendors[w][lx.familyVendor[f]] = true
		}
		maxQ := 0
		for _, pid := range lx.families[f] {
			d := lx.docs[pid]
			if isPerUnit(d) {
				maxQ = -1
				break
			}
			if d.LicenseQuantity != nil && *d.LicenseQuantity > maxQ {
				maxQ = *d.LicenseQuantity
			}
		}
		lx.familyCap[f] = maxQ
		name := foldText(lx.categoryName[lx.familyCat[f]] + " " + lx.familyName[f])
		home := false
		for _, w := range consumerWords {
			home = home || strings.Contains(name, w)
		}
		lx.consumer[f] = home
	}
}
