package services

import "strings"

// Аналоги по классу: справочник связей (substitute) не знает замен названного продукта («аналог фотошопа» — у Adobe
// Photoshop связей нет) — аналогами становятся семейства того же класса: категория названного продукта и категории
// его же вендора, где названо то же слово («Adobe Photoshop Elements» — обработка фото). Другие вендоры; семейства, в
// названии которых есть слово продукта из запроса («Digital Film Tools Adobe Photoshop Plug-ins»), — дополнения к
// нему, а не замена. Порядок решают релевантность и реранкер, допустимость — условия покупки (buildFamily).

// confidentReference — референс назван продуктом, а не угадан: семейство найдено по словам запроса и запрос покрывает
// его название (все слова названия, кроме имени вендора). «замена windows» → «Retrospect for Windows», «аналог касперского» (только вендор) — класс
// по такому референсу не строится: честное «аналогов нет» лучше чужой категории.
func confidentReference(lx *searchLexicon, ent queryEntities) bool {
	if len(ent.families) == 0 || len(ent.reference) == 0 {
		return false
	}
	in := map[string]bool{}
	for _, t := range ent.entityTokens {
		for _, w := range wordsOf(t) {
			in[w] = true
		}
	}
	for _, r := range ent.reference {
		vendor := map[string]bool{}
		for _, w := range wordsOf(lx.vendorName[lx.familyVendor[r]] + " " + lx.familyVendor[r]) {
			vendor[w] = true
		}
		n := 0
		for _, w := range wordsOf(lx.familyName[r]) {
			if vendor[w] || lexStopWords[w] {
				continue
			}
			if !in[w] {
				return false // в названии референса есть слово, которого в запросе нет
			}
			n++
		}
		if n == 0 {
			return false
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
	for _, r := range refs {
		vendors[lx.familyVendor[r]] = true
		if c := lx.familyCat[r]; c != "" {
			cats[c] = true
		}
		for w := range lx.familyWords[r] {
			if in[w] {
				words[w] = true
			}
		}
	}
	for _, f := range named {
		if vendors[lx.familyVendor[f]] && lx.familyCat[f] != "" {
			cats[lx.familyCat[f]] = true
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
