package services

import (
	"maps"
	"strings"
)

// Аналоги по классу: справочник связей (substitute) не знает замен названного продукта («аналог фотошопа» — у Adobe
// Photoshop связей нет) — аналогами становятся семейства того же класса: категория названного продукта и категории
// его же вендора, где названо то же слово («Adobe Photoshop Elements» — обработка фото). Другие вендоры; семейства, в
// названии которых есть слово продукта из запроса («Digital Film Tools Adobe Photoshop Plug-ins»), — дополнения к
// нему, а не замена. Порядок решают релевантность и реранкер, допустимость — условия покупки (buildFamily).

// confidentReference — референс назван продуктом, а не угадан: семейство найдено по словам запроса и запрос покрывает
// его название (все слова названия, кроме имени вендора). «замена windows» → «Retrospect for Windows», «аналог касперского» (только вендор) — класс
// по такому референсу не строится: честное «аналогов нет» лучше чужой категории.
func confidentReference(lx *searchLexicon, ent queryEntities) bool {
	if len(ent.reference) == 0 {
		return false
	}
	if ent.referenceByVendor {
		return true // «аналог anydesk», «замена microsoft office»: линейка названного вендора в его основном классе
	}
	in := map[string]bool{}
	for _, t := range ent.entityTokens {
		for _, w := range wordsOf(t) {
			in[w] = true
		}
	}
	sig := 0
	for _, t := range ent.entityTokens {
		for _, w := range wordsOf(t) {
			if !lexStopWords[w] && !analogWords[w] {
				sig++
			}
		}
	}
	for _, r := range ent.reference {
		fromText := contains(ent.families, r)
		if c := ent.category; c != "" && !fromText && lx.categoryRoot[c] != "" && lx.categoryRoot[lx.familyCat[r]] != lx.categoryRoot[c] {
			// «замена windows»: референс угадан моделью из резервного копирования, а запрос — об ОС.
			return false
		}
		vendor := map[string]bool{}
		for _, w := range wordsOf(lx.vendorName[lx.familyVendor[r]] + " " + lx.familyVendor[r]) {
			vendor[w] = true
		}
		n, missing, vendorNamed := 0, false, false
		for _, w := range wordsOf(lx.familyName[r]) {
			if vendor[w] {
				vendorNamed = vendorNamed || in[w]
				continue
			}
			if lexStopWords[w] {
				continue
			}
			missing = missing || !in[w]
			n++
		}
		switch {
		case n == 0 && vendorNamed: // «AnyDesk», «TeamViewer»: название продукта — имя вендора
		case n > 0 && !missing:
		case lx.familyCoverage(r, ent.entityTokens) >= namedCoverageMin: // «kaspersky endpoint security» → «… для бизнеса Расширенный»
		case fromText && sig > 0 && lx.queryHits(r, ent.entityTokens) >= sig: // «аналог maxpatrol»: все слова запроса — в названии
		default:
			return false // «замена windows» → «Retrospect for Windows»: продукт угадан по одному общему слову
		}
	}
	return true
}

// classAnalogs — кандидаты-аналоги по классу для референсов refs; named — все семейства, совпавшие со словами
// запроса; tokens — слова запроса.
func classAnalogs(lx *searchLexicon, refs, named, tokens []string) []string {
	vendors, cats, words := map[string]bool{}, map[string]bool{}, map[string]bool{}
	in := map[string]bool{}
	for _, t := range tokens {
		for _, w := range wordsOf(t) {
			in[w] = true
		}
	}
	cats = mainClass(lx, refs)
	for _, r := range refs {
		vendors[lx.familyVendor[r]] = true
		for w := range lx.familyWords[r] {
			if in[w] {
				words[w] = true
			}
		}
	}
	roots := map[string]bool{}
	for c := range cats {
		roots[lx.categoryRoot[c]] = true
	}
	if len(refs) <= 3 {
		for _, f := range named {
			if vendors[lx.familyVendor[f]] && lx.familyCat[f] != "" && roots[lx.categoryRoot[lx.familyCat[f]]] {
				cats[lx.familyCat[f]] = true
			}
		}
	}
	var out []string
	for _, f := range lx.familyOrder {
		if !cats[lx.familyCat[f]] || vendors[lx.familyVendor[f]] {
			continue
		}
		addon := false
		for _, w := range wordsOf(lx.familyName[f]) {
			addon = addon || addonWords[w] // «FilmConvert Adobe Plugin»: дополнение к чужому продукту, а не замена
		}
		for w := range words {
			addon = addon || lx.familyWords[f][w]
		}
		for _, pid := range lx.families[f] {
			if addon {
				break
			}
			for _, w := range wordsOf(lx.docs[pid].Title) {
				addon = addon || words[w] // «PROCAD … Add-on to AUTOCAD» — дополнение к названному продукту
			}
		}
		if !addon {
			out = append(out, f)
		}
	}
	return out
}

// addonWords — слова названия, по которым семейство — дополнение к другому продукту.
var addonWords = setOf("plugin", "plugins", "plug", "addon", "add", "плагин", "плагины", "надстройка")

const (
	// classAnalogRel — одна релевантность у всех аналогов по классу: слова запроса («на 20 пользователей») порядок не
	// решают, решает реранкер по близости к названному продукту.
	classAnalogRel = 0.05
	// classRerankTop — сколько аналогов по классу видит реранкер (у «Графических редакторов» — десятки семейств).
	classRerankTop = 10
	// classWideMax — класс шире этого — аналоги только из найденного поиском.
	classWideMax = 80
)

// classAnalogQuery — запрос к реранкеру для аналогов по классу: «аналог Adobe Photoshop. Графические редакторы» —
// названный продукт и его класс, без количества и прочих условий запроса.
func classAnalogQuery(lx *searchLexicon, refs []string) string {
	var names, cats []string
	seen := map[string]bool{}
	for _, r := range refs {
		if n := lx.familyName[r]; n != "" && !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
		if c := lx.categoryName[lx.familyCat[r]]; c != "" && !seen[c] {
			seen[c] = true
			cats = append(cats, c)
		}
	}
	q := "аналог " + strings.Join(names, ", ")
	if len(cats) > 0 {
		q += ". " + strings.Join(cats, ", ")
	}
	return q
}

// vendorClassFamilies — семейства вендора v в его основном классе: в категории запроса (по корню), если она названа
// («аналог microsoft office» — офисные пакеты, а не гарнитуры), иначе в категории второго уровня, где у вендора больше
// всего семейств («аналог anydesk» — решения удалённого доступа).
func (lx *searchLexicon) vendorClassFamilies(v, category string) []string {
	var all []string
	count := map[string]int{}
	for _, f := range lx.familyOrder {
		if lx.familyVendor[f] == v {
			all = append(all, f)
			count[lx.familyCat[f]]++
		}
	}
	keep := func(f string) bool { return false }
	switch {
	case category != "" && lx.categoryRoot[category] != "":
		root := lx.categoryRoot[category]
		keep = func(f string) bool { return lx.categoryRoot[lx.familyCat[f]] == root }
	default:
		best, n := "", 0
		for c, k := range count {
			if k > n || k == n && c < best {
				best, n = c, k
			}
		}
		keep = func(f string) bool { return lx.familyCat[f] == best }
	}
	var out []string
	for _, f := range all {
		if keep(f) {
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		return all
	}
	return out
}

// importSubstituteSiblings — соседние категории «(Импортозамещение)» под тем же разделом: у AnyDesk («Другие решения
// удаленного доступа») класс замены — и «Российские решения удаленного доступа (Импортозамещение)».
func (lx *searchLexicon) importSubstituteSiblings(cat string) []string {
	parent := lx.categoryParent[cat]
	if parent == "" {
		return nil
	}
	var out []string
	for id, name := range lx.categoryName {
		if id != cat && lx.categoryParent[id] == parent && strings.Contains(strings.ToLower(name), "импортозамещ") &&
			shareClassWord(lx.categoryName[cat], name) {
			out = append(out, id)
		}
	}
	return out
}

// mainClass — категории аналогов: у одного-трёх референсов — их категории; у линейки (больше трёх) — категория, где
// референсов больше всего. Плюс соседние «(Импортозамещение)».
func mainClass(lx *searchLexicon, refs []string) map[string]bool {
	cats := map[string]bool{}
	count := map[string]int{}
	for _, r := range refs {
		if c := lx.familyCat[r]; c != "" {
			count[c]++
		}
	}
	if len(refs) <= 3 {
		for c := range count {
			cats[c] = true
		}
	} else {
		best, n := "", 0
		for c, k := range count {
			if k > n || k == n && c < best {
				best, n = c, k
			}
		}
		if best != "" {
			cats[best] = true
		}
	}
	for c := range maps.Clone(cats) {
		for _, s := range lx.importSubstituteSiblings(c) {
			cats[s] = true
		}
	}
	return cats
}

// classGenericWords — слова названий категорий, которые класс не различают.
var classGenericWords = setOf("решения", "решений", "программы", "программное", "программ", "обеспечение", "системы",
	"другие", "другое", "российские", "российское", "российская", "российский", "импортозамещение", "средства")

// shareClassWord — у двух названий категорий общее значимое слово (по первым пяти буквам): «Программы для
// виртуализации» и «Российская виртуализация (Импортозамещение)» — да; и «Системы управления базами данных» — нет.
func shareClassWord(a, b string) bool {
	stems := map[string]bool{}
	stem := func(w string) string {
		if r := []rune(w); len(r) > 5 {
			return string(r[:5])
		}
		return w
	}
	for _, w := range wordsOf(a) {
		if len([]rune(w)) >= 4 && !classGenericWords[w] && !lexStopWords[w] {
			stems[stem(w)] = true
		}
	}
	for _, w := range wordsOf(b) {
		if len([]rune(w)) >= 4 && !classGenericWords[w] && !lexStopWords[w] && stems[stem(w)] {
			return true
		}
	}
	return false
}
