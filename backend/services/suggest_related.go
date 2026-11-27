package services

import (
	"slices"
	"sort"
	"strings"
)

// relatedSourceRank — чем ближе подсказка к настоящему запросу покупателя, тем раньше она идёт в «Похожие».
// Словарные формы сюда не входят: это жаргон, транслит и опечатки для понимания ввода («фш», «max patrl»),
// показывать их покупателю как запрос нельзя.
var relatedSourceRank = map[string]int{"successful_query": 0, "catalog_template": 1}

// relatedPerLead — не больше двух запросов с одним первым словом: «Похожие» должны вести к разным товарам, а не
// повторять одно название с разными хвостами.
const relatedPerLead = 2

// related — добор res подсказками категории cat (главной у результатов подсказки): по источнику, затем по весу.
// Пропускаются сам запрос (self), повторы и фразы, отличающиеся от уже взятых только продолжением.
func (d *suggestDict) related(cat string, self map[string]bool, res []string, limit int) []string {
	if d == nil {
		return res
	}
	vendors := map[string]bool{}
	if d.lx != nil {
		for _, name := range d.lx.vendorName {
			vendors[foldText(name)] = true
		}
	}
	var cands []dictEntry
	for _, e := range d.entries {
		_, shown := relatedSourceRank[e.source]
		// Шаблон из одного слова или из названия производителя — это не запрос.
		if shown && !(e.source == "catalog_template" && vendors[foldText(e.text)]) && e.kind == suggestQuery && len(e.categories) > 0 && e.categories[0] == cat && e.hits > 0 &&
			readableQuery(e.text) && (e.source != "catalog_template" || strings.Contains(e.text, " ")) {
			cands = append(cands, e)
		}
	}
	sort.SliceStable(cands, func(i, j int) bool {
		a, b := cands[i], cands[j]
		if ra, rb := relatedSourceRank[a.source], relatedSourceRank[b.source]; ra != rb {
			return ra < rb
		}
		if a.weight != b.weight {
			return a.weight > b.weight
		}
		if a.hits != b.hits {
			return a.hits > b.hits
		}
		return a.norm < b.norm
	})
	taken := make([]string, 0, limit)
	leads := map[string]int{}
	for _, q := range res {
		taken = append(taken, foldText(q))
		leads[leadWord(foldText(q))]++
	}
	for q := range self {
		if q != "" {
			taken = append(taken, q)
		}
	}
	for _, e := range cands {
		if len(res) >= limit {
			break
		}
		folded := foldText(e.text)
		if leads[leadWord(folded)] >= relatedPerLead ||
			slices.ContainsFunc(taken, func(t string) bool { return samePhrase(t, folded) }) {
			continue
		}
		leads[leadWord(folded)]++
		taken = append(taken, folded)
		res = append(res, e.text)
	}
	return res
}

func leadWord(s string) string {
	w, _, _ := strings.Cut(s, " ")
	return w
}

// samePhrase — одна фраза равна другой или продолжает её по словам; окончания не различаются
// («продление касперский» и «продление касперского» — одно и то же).
func samePhrase(a, b string) bool {
	a, b = phraseStem(a), phraseStem(b)
	return a == b || strings.HasPrefix(a, b+" ") || strings.HasPrefix(b, a+" ")
}

// phraseStem — слова без окончаний: длинные слова обрезаны до шести букв.
func phraseStem(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if r := []rune(w); len(r) > 6 {
			words[i] = string(r[:6])
		}
	}
	return strings.Join(words, " ")
}
