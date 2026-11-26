package services

import (
	"context"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"kuznica/api/domain"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"

	"go.uber.org/zap"
)

// Подсказки (ARCHITECTURE §2.7) и связанные запросы. Индекс — успешные запросы из логов (без тех, что
// заканчивались пустой выдачей), названия семейств, вендоры и категории каталога. Совпадение — префикс
// строки или слова; если префиксов нет — n-граммы (опечатка в начале ввода).

const (
	suggestMaxRows  = 8
	suggestMaxQuery = 100
	suggestMinNgram = 0.34
	// suggestMinFreq — запрос из логов попадает в подсказки, только если его искали не меньше двух раз:
	// разовый запрос одного покупателя («1с для ООО …») другим не показывается.
	suggestMinFreq   = 2
	relatedMax       = 5
	relatedMin       = 3
	suggestLabelQ    = "Запросы"
	suggestLabelProd = "Товары"
	suggestLabelVend = "Вендоры"
	suggestLabelCat  = "Категории"
)

// Типы групп подсказок.
const (
	suggestQuery    = "query"
	suggestProduct  = "product"
	suggestVendor   = "vendor"
	suggestCategory = "category"
)

// suggestCaps — сколько строк группы показывать максимум (итого не больше 8).
var suggestCaps = map[string]int{suggestQuery: 4, suggestProduct: 3, suggestVendor: 2, suggestCategory: 2}

var suggestTypes = []string{suggestQuery, suggestProduct, suggestVendor, suggestCategory}

var suggestLabels = map[string]string{suggestQuery: suggestLabelQ, suggestProduct: suggestLabelProd,
	suggestVendor: suggestLabelVend, suggestCategory: suggestLabelCat}

// suggestEntry — строка индекса.
type suggestEntry struct {
	kind, text, folded, id string
	weight                 float64
	// category — категория запроса (по названному семейству): для связанных запросов.
	category string
	family   string
}

// suggestIndex — индекс подсказок одного снимка каталога.
type suggestIndex struct {
	lex     *searchLexicon
	entries []suggestEntry
}

// eventLoader — источник исторических логов (mock: events.jsonl).
type eventLoader interface {
	LoadEvents(ctx context.Context) ([]domain.LogEvent, error)
}

// suggestions — индекс для текущего каталога; перестраивается при смене снимка.
func (s *Services) suggestions(ctx context.Context, lx *searchLexicon) *suggestIndex {
	s.core.suggestMu.Lock()
	defer s.core.suggestMu.Unlock()
	if s.core.suggest != nil && s.core.suggest.lex == lx {
		return s.core.suggest
	}
	idx := &suggestIndex{lex: lx}
	rules := newSearchRules(1, lx.baseAliases)
	counts, empty := map[string]int{}, map[string]bool{}
	var order []string
	if el, ok := s.source.(eventLoader); ok {
		evs, err := el.LoadEvents(ctx)
		if err != nil {
			zap.L().Warn("подсказки: логи не прочитаны — только каталог", zap.Error(err))
		}
		for _, e := range evs {
			if e.Type != "search" || e.Query == "" {
				continue
			}
			q := foldText(e.Query)
			if e.ResultsCount == nil || *e.ResultsCount == 0 {
				empty[q] = true
				continue
			}
			if counts[q] == 0 {
				order = append(order, q)
			}
			counts[q]++
		}
	}
	// Запрос из логов показывается так, как его понял поиск: набранный в неверной раскладке («1с ,e[ufknthbz»)
	// — после исправления раскладки; то, что и после нормализации осталось мусором, отбрасывается. Одинаковые
	// после исправления запросы складываются.
	byText := map[string]int{}
	for _, q := range order {
		if empty[q] || utf8.RuneCountInString(q) > suggestMaxQuery {
			continue
		}
		nr := normalizeFull(q, lx, rules, false)
		text := q
		for _, c := range nr.changes {
			if c.Kind == changeLayout {
				text = foldText(nr.normalized())
				break
			}
		}
		if !readableQuery(text) {
			continue
		}
		if i, ok := byText[text]; ok {
			idx.entries[i].weight += float64(counts[q])
			continue
		}
		e := suggestEntry{kind: suggestQuery, text: text, folded: text, weight: float64(counts[q])}
		if text != q {
			nr = normalizeFull(text, lx, rules, false)
		}
		if fs := lx.matchFamilies(nr.tokens); len(fs) == 1 {
			e.family, e.category = fs[0], lx.familyCat[fs[0]]
		} else if len(fs) > 1 && lx.familyCat[fs[0]] == lx.familyCat[fs[len(fs)-1]] {
			e.category = lx.familyCat[fs[0]]
		}
		byText[text] = len(idx.entries)
		idx.entries = append(idx.entries, e)
	}
	idx.entries = slices.DeleteFunc(idx.entries, func(e suggestEntry) bool { return e.weight < suggestMinFreq })
	for _, f := range lx.familyOrder {
		pop := 0.0
		for _, pid := range lx.families[f] {
			pop = max(pop, lx.docs[pid].Popularity)
		}
		idx.entries = append(idx.entries, suggestEntry{kind: suggestProduct, text: lx.familyName[f], folded: foldText(lx.familyName[f]),
			id: f, weight: pop, family: f, category: lx.familyCat[f]})
	}
	for id, name := range lx.vendorName {
		idx.entries = append(idx.entries, suggestEntry{kind: suggestVendor, text: name, folded: foldText(name), id: id})
	}
	for id, name := range lx.categoryName {
		idx.entries = append(idx.entries, suggestEntry{kind: suggestCategory, text: name, folded: foldText(name), id: id, category: id})
	}
	sort.SliceStable(idx.entries, func(i, j int) bool {
		a, b := idx.entries[i], idx.entries[j]
		if a.weight != b.weight {
			return a.weight > b.weight
		}
		return a.text < b.text
	})
	s.core.suggest = idx
	return idx
}

// Suggest — GET /api/suggest.
func (s *Services) Suggest(ctx context.Context, q, clientID string) (*out.SuggestResponse, error) {
	q = foldText(q)
	if utf8.RuneCountInString(q) > maxQueryRunes {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(errs.FieldError{
			Field: "q", Code: errs.CodeOutOfRange, Message: "Слишком длинный запрос"})
	}
	resp := &out.SuggestResponse{Groups: []out.SuggestGroup{}}
	st := s.catalog.Load()
	if st == nil || st.lex == nil || utf8.RuneCountInString(q) < 2 {
		return resp, nil
	}
	idx := s.suggestions(ctx, st.lex)
	purchased := st.purchased[clientID]
	hits := idx.match(q, purchased)
	if total(hits) == 0 {
		if alt, ok := switchLayout(q); ok {
			hits = idx.match(alt, purchased)
		}
	}
	if total(hits) == 0 {
		hits = idx.matchNgrams(q)
	}
	for _, kind := range suggestTypes {
		if len(hits[kind]) > 0 {
			resp.Groups = append(resp.Groups, out.SuggestGroup{Type: kind, Label: suggestLabels[kind], Items: hits[kind]})
		}
	}
	return resp, nil
}

func total(h map[string][]out.SuggestItem) int {
	n := 0
	for _, v := range h {
		n += len(v)
	}
	return n
}

// match — префикс строки или любого слова строки. Купленные клиентом семейства — выше.
func (idx *suggestIndex) match(q string, purchased map[string]bool) map[string][]out.SuggestItem {
	type cand struct {
		e     suggestEntry
		start int
		rank  int
	}
	byKind := map[string][]cand{}
	for _, e := range idx.entries {
		pos := prefixAt(e.folded, q)
		if pos < 0 {
			continue
		}
		rank := 1
		if pos == 0 {
			rank = 0
		}
		if e.family != "" && purchased["f:"+e.family] {
			rank--
		}
		byKind[e.kind] = append(byKind[e.kind], cand{e, pos, rank})
	}
	ql := utf8.RuneCountInString(q)
	picked := map[string][]out.SuggestItem{}
	for kind, cs := range byKind {
		sort.SliceStable(cs, func(i, j int) bool { return cs[i].rank < cs[j].rank })
		for _, c := range cs {
			item := out.SuggestItem{Text: c.e.text, Highlight: []int{c.start, c.start + ql}}
			if c.e.id != "" {
				id := c.e.id
				item.ID = &id
			}
			picked[kind] = append(picked[kind], item)
		}
	}
	return capGroups(picked)
}

// prefixAt — позиция (в символах) начала слова folded, с которого начинается q; -1 — нет.
func prefixAt(folded, q string) int {
	if strings.HasPrefix(folded, q) {
		return 0
	}
	runes := 0
	prev := ' '
	for i, r := range folded {
		if (prev == ' ' || prev == '-' || prev == ':' || prev == '(' || prev == '"') && i > 0 && strings.HasPrefix(folded[i:], q) {
			return runes
		}
		prev = r
		runes++
	}
	return -1
}

// matchNgrams — триграммы: доля общих триграмм запроса и слова строки.
func (idx *suggestIndex) matchNgrams(q string) map[string][]out.SuggestItem {
	qg := trigrams(q)
	if len(qg) == 0 {
		return map[string][]out.SuggestItem{}
	}
	type cand struct {
		e   suggestEntry
		sim float64
	}
	var cs []cand
	for _, e := range idx.entries {
		best := 0.0
		for _, w := range strings.Fields(e.folded) {
			best = max(best, jaccard(qg, trigrams(w)))
		}
		if best >= suggestMinNgram {
			cs = append(cs, cand{e, best})
		}
	}
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].sim > cs[j].sim })
	picked := map[string][]out.SuggestItem{}
	for _, c := range cs {
		item := out.SuggestItem{Text: c.e.text}
		if c.e.id != "" {
			id := c.e.id
			item.ID = &id
		}
		picked[c.e.kind] = append(picked[c.e.kind], item)
	}
	return capGroups(picked)
}

// capGroups — не больше suggestCaps на группу и 8 строк суммарно (обрезаются группы в обратном порядке).
func capGroups(picked map[string][]out.SuggestItem) map[string][]out.SuggestItem {
	for k, v := range picked {
		if len(v) > suggestCaps[k] {
			picked[k] = v[:suggestCaps[k]]
		}
	}
	for total(picked) > suggestMaxRows {
		for i := len(suggestTypes) - 1; i >= 0 && total(picked) > suggestMaxRows; i-- {
			k := suggestTypes[i]
			if len(picked[k]) > 1 {
				picked[k] = picked[k][:len(picked[k])-1]
			}
		}
	}
	return picked
}

func trigrams(s string) map[string]bool {
	rs := []rune(" " + s + " ")
	g := map[string]bool{}
	for i := 0; i+3 <= len(rs); i++ {
		g[string(rs[i:i+3])] = true
	}
	return g
}

func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for k := range a {
		if b[k] {
			inter++
		}
	}
	return float64(inter) / float64(len(a)+len(b)-inter)
}

// readableQuery — в запросе только буквы, цифры, пробелы и знаки, которые бывают в названиях ПО (. - + / #):
// остатки неверной раскладки («,e[»), которые нормализация не распознала, в подсказки не идут.
func readableQuery(q string) bool {
	letters := 0
	for _, r := range q {
		switch {
		case unicode.IsLetter(r):
			letters++
		case unicode.IsDigit(r), unicode.IsSpace(r), strings.ContainsRune(".-+/#", r):
		default:
			return false
		}
	}
	return letters > 0
}

// relatedQueries — 3–5 успешных запросов из логов того же семейства или категории, что и первая позиция
// выдачи. Меньше трёх — пусто: случайные строки хуже, чем ничего.
func (s *Services) relatedQueries(ctx context.Context, lx *searchLexicon, resp *out.SearchResponse) []string {
	res := []string{}
	if len(resp.Items) == 0 {
		return res
	}
	top := resp.Items[0]
	cat := lx.familyCat[top.FamilyID]
	idx := s.suggestions(ctx, lx)
	self := map[string]bool{foldText(resp.Query): true, resp.Normalized: true}
	var same, near []string
	for _, e := range idx.entries {
		if e.kind != suggestQuery || self[e.folded] {
			continue
		}
		switch {
		case e.family == top.FamilyID:
			same = append(same, e.text)
		case e.category != "" && e.category == cat:
			near = append(near, e.text)
		}
	}
	for _, q := range append(same, near...) {
		if len(res) >= relatedMax {
			break
		}
		res = append(res, q)
	}
	if len(res) < relatedMin {
		return []string{}
	}
	return res
}
