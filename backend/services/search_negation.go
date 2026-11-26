package services

import (
	"cmp"
	"slices"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// fieldFamilyNot — чип исключённого продукта («1с без итс», «ос кроме windows»).
const fieldFamilyNot = "family_not"

// negComparatives — «не дороже», «не менее»: это оператор числа (hardware_parse.go), а не исключение.
var negComparatives = setOf("дороже", "дешевле", "более", "менее", "больше", "меньше", "старше", "младше",
	"выше", "ниже", "раньше", "позже", "для", "на", "только", "просто", "нужен", "нужна", "нужно", "надо",
	"работает", "работают", "находит", "ищется")

// negationSpans — индексы слов-исключений и того, что исключено: «не касперский», «кроме dr.web»,
// «без продления», «без poe». Исключённое — одно слово или два, если вместе они называют вендора или продукт
// («кроме dr web», «без операционной системы»). Числа и операторы («не дороже 100 000») не исключение.
func negationSpans(tokens []string, lx *searchLexicon, rules *searchRules) map[int]bool {
	out := map[int]bool{}
	n := len(tokens)
	for i := 0; i < n-1; i++ {
		if !negWords[tokens[i]] && tokens[i] != "исключая" {
			continue
		}
		next := tokens[i+1]
		if negComparatives[next] || negWords[next] || startsWithDigit(next) || len(wordsOf(next)) == 0 {
			continue
		}
		span := 1
		if i+2 < n && !negWords[tokens[i+2]] && !startsWithDigit(tokens[i+2]) {
			two := []string{next, tokens[i+2]}
			switch {
			case osWords[next] && osWords[tokens[i+2]]:
				span = 2
			case len(lx.matchVendors(two, rules)) > 0 && len(lx.matchVendors(two[:1], rules)) == 0:
				span = 2
			}
		}
		for k := i; k <= i+span; k++ {
			out[k] = true
		}
		i += span
	}
	return out
}

func startsWithDigit(t string) bool { return t != "" && t[0] >= '0' && t[0] <= '9' }

// maskTokens — копия токенов, в которой исключённые слова пустые: индексы не сдвигаются, а «без продления»
// не становится условием продления.
func maskTokens(tokens []string, masked map[int]bool) []string {
	if len(masked) == 0 {
		return tokens
	}
	cp := slices.Clone(tokens)
	for i := range masked {
		if i < len(cp) {
			cp[i] = ""
		}
	}
	return cp
}

// negatedEntities — что исключено: вендоры и продукты из слов после «не/без/кроме». Слова-исключения уже
// помечены как атрибуты (attrs.consumed), в текстовый поиск и в «товара нет» они не идут.
func negatedEntities(tokens []string, spans map[int]bool, lx *searchLexicon, rules *searchRules) (vendors, families []string, label map[string]string) {
	label = map[string]string{}
	var cur []string
	flush := func() {
		if len(cur) == 0 {
			return
		}
		allOS := true
		for _, t := range cur {
			allOS = allOS && osWords[t] && !strings.HasPrefix(t, "win") && !strings.HasPrefix(t, "винд")
		}
		if a := extractAttrs(cur); allOS || len(a.consumed) > 0 || a.renewal || a.licType != "" {
			cur = nil // «без операционной системы», «без продления» — условие, а не продукт
			return
		}
		if vs := lx.matchVendors(cur, rules); len(vs) > 0 {
			for _, v := range vs {
				if !slices.Contains(vendors, v) {
					vendors = append(vendors, v)
				}
			}
		} else {
			for _, f := range lx.matchFamilies(cur) {
				if !slices.Contains(families, f) {
					families = append(families, f)
					label[f] = strings.Join(cur, " ")
				}
			}
		}
		cur = nil
	}
	for i, t := range tokens {
		switch {
		case !spans[i]:
			flush()
		case negWords[t] || t == "исключая":
			flush()
		default:
			cur = append(cur, t)
		}
	}
	flush()
	return vendors, families, label
}

// applyNegation — исключённое не может остаться положительным: убирается из вендоров, семейств и эталона
// аналога (в том числе после разбора моделью), становится фильтром «кроме» и чипом.
func (e *queryEntities) applyNegation(vendors, families []string, label map[string]string, removed map[string]map[string]bool) {
	e.familyNotLabel = label
	for _, v := range vendors {
		if !removed[fieldVendorNot][v] {
			e.vendorNot = append(e.vendorNot, v)
		}
	}
	for _, f := range families {
		if !removed[fieldFamilyNot][f] && !removed[fieldFamilyNot][label[f]] {
			e.familyNot = append(e.familyNot, f)
		}
	}
	e.dropNegated()
}

func (e *queryEntities) dropNegated() {
	if len(e.vendorNot) == 0 && len(e.familyNot) == 0 {
		return
	}
	keep := func(xs, not []string) []string {
		var out []string
		for _, x := range xs {
			if !slices.Contains(not, x) {
				out = append(out, x)
			}
		}
		return out
	}
	e.vendors = keep(e.vendors, e.vendorNot)
	e.families = keep(e.families, e.familyNot)
	e.named = keep(e.named, e.familyNot)
	e.reference = keep(e.reference, e.familyNot)
	if slices.Contains(e.familyNot, e.parsedFamily) {
		e.parsedFamily = ""
	}
}

// negationFilters — фильтр «кроме» для BM25 и векторов.
func negationFilters(ent queryEntities) []any {
	var not []any
	if len(ent.vendorNot) > 0 {
		not = append(not, map[string]any{"terms": map[string]any{"vendor_id": ent.vendorNot}})
	}
	if len(ent.familyNot) > 0 {
		not = append(not, map[string]any{"terms": map[string]any{"family_id": ent.familyNot}})
	}
	if len(not) == 0 {
		return nil
	}
	return []any{map[string]any{"bool": map[string]any{"must_not": not}}}
}

// negationChips — «Не Kaspersky», «Не 1С:ИТС»; вендоры, уже исключённые разбором оборудования, не дублируются.
func negationChips(chips []out.Chip, ent queryEntities, lx *searchLexicon) []out.Chip {
	for _, v := range ent.vendorNot {
		if slices.ContainsFunc(chips, func(c out.Chip) bool { return c.Field == fieldVendorNot && c.Value == v }) {
			continue
		}
		name := lx.vendorName[v]
		if name == "" {
			name = v
		}
		chips = append(chips, out.Chip{Field: fieldVendorNot, Value: v, Op: "not", Label: "Не " + name, Strict: true})
	}
	// Одно исключение — один чип: «ос кроме windows» — «Не windows», а не семь чипов по семействам Windows.
	byLabel := map[string][]string{}
	var order []string
	for _, f := range ent.familyNot {
		l := ent.familyNotLabel[f]
		if _, ok := byLabel[l]; !ok {
			order = append(order, l)
		}
		byLabel[l] = append(byLabel[l], f)
	}
	for _, l := range order {
		fs := byLabel[l]
		if len(fs) == 1 || l == "" {
			for _, f := range fs {
				chips = append(chips, out.Chip{Field: fieldFamilyNot, Value: f, Op: "not", Label: "Не " + strings.TrimSpace(lx.familyName[f]), Strict: true})
			}
			continue
		}
		r := []rune(l)
		chips = append(chips, out.Chip{Field: fieldFamilyNot, Value: l, Op: "not", Label: "Не " + strings.ToUpper(string(r[:1])) + string(r[1:]), Strict: true})
	}
	return chips
}

// withoutNegatedChanges — правки нормализации только в исключённых словах («не касперский» → kaspersky) не
// делают запрос точным: маршрут считается по тому, что ищут.
func (nr normResult) withoutNegatedChanges(spans map[int]bool) normResult {
	if len(spans) == 0 {
		return nr
	}
	neg := map[string]bool{}
	for i := range spans {
		if i < len(nr.tokens) {
			for _, w := range wordsOf(nr.tokens[i]) {
				neg[w] = true
			}
		}
	}
	var keep []domain.Change
	for _, c := range nr.changes {
		all := len(wordsOf(c.To)) > 0
		for _, w := range wordsOf(c.To) {
			all = all && neg[w]
		}
		if !all {
			keep = append(keep, c)
		}
	}
	nr.changes = keep
	return nr
}

// namedFillMax — сколько семейств добирается за названное, которого поиск не принёс.
const namedFillMax = 6

// namedFill — названное в запросе обязано быть в выдаче. Категория без продукта («антивирус», «антивирус не
// касперский») — семейства категории, которых нет в найденном: в названиях KES и Dr.Web нет слова «антивирус»,
// по тексту и векторам их не достать. Вендор, от которого ничего не нашлось («kaspersky или dr.web», «дрвеб»), —
// его семейства (в названной категории, если она есть). Исключённое («не касперский») не добирается.
// Добранные идут с порогом релевантности — ниже найденного; допустимость (метрика, срок, реестр) — buildFamily.
func namedFill(lx *searchLexicon, ent queryEntities, fams []*familyHit) []string {
	if ent.nonProduct || ent.noise {
		return nil
	}
	have := map[string]bool{}
	haveVendor := map[string]bool{}
	for _, fh := range fams {
		have[fh.id] = true
		haveVendor[lx.familyVendor[fh.id]] = true
	}
	// Раздел назван общим словом («антивирус» → «Антивирусы и информационная безопасность»): добираются только
	// подкатегории, в названии которых есть слово запроса, — антивирусы, а не Secret Net.
	stems := map[string]bool{}
	for _, t := range ent.entityTokens {
		for _, w := range wordsOf(t) {
			if len([]rune(w)) >= 4 {
				stems[stemOf(w)] = true
			}
		}
	}
	sub := map[string]bool{}
	for c := range lx.categoryName {
		if c == ent.category || lx.categoryRoot[c] != ent.category {
			continue
		}
		for _, w := range wordsOf(lx.categoryName[c]) {
			if len([]rune(w)) >= 4 && stems[stemOf(w)] {
				sub[c] = true
			}
		}
	}
	inCat := func(f string) bool {
		c := lx.familyCat[f]
		if len(sub) > 0 {
			return sub[c]
		}
		return ent.category == "" || c == ent.category || lx.categoryRoot[c] == ent.category
	}
	allowed := func(f string) bool {
		return !have[f] && inCat(f) && !slices.Contains(ent.familyNot, f) && !slices.Contains(ent.vendorNot, lx.familyVendor[f])
	}
	order := slices.Clone(lx.familyOrder)
	pop := func(f string) float64 {
		best := 0.0
		for _, pid := range lx.families[f] {
			best = max(best, float64(lx.docs[pid].Popularity))
		}
		return best
	}
	slices.SortStableFunc(order, func(a, b string) int { return cmp.Compare(pop(b), pop(a)) })
	var out []string
	switch {
	case len(ent.vendors) > 0:
		for _, v := range ent.vendors {
			if haveVendor[v] {
				continue
			}
			n := 0
			for _, f := range order {
				if n < namedFillMax/2 && lx.familyVendor[f] == v && allowed(f) {
					out = append(out, f)
					n++
				}
			}
		}
	case ent.category != "" && len(ent.families) == 0:
		for _, f := range order {
			if len(out) < namedFillMax && allowed(f) {
				out = append(out, f)
			}
		}
	}
	return out
}

// orWords — «A или B»: покупатель выбирает между продуктами.
var orWords = setOf("или", "либо", "or", "/")

// applyAlternatives — «windows или astra», «office или р7»: каждая сторона ищется отдельно, вендоры и
// семейства объединяются. Иначе вендор одной стороны становится строгим фильтром и вторая пропадает.
func (e *queryEntities) applyAlternatives(lx *searchLexicon, rules *searchRules) {
	if !slices.ContainsFunc(e.textTokens, func(t string) bool { return orWords[t] }) {
		return
	}
	var sides [][]string
	cur := []string{}
	for _, t := range e.textTokens {
		switch {
		case orWords[t]:
			sides = append(sides, cur)
			cur = []string{}
		case !lexStopWords[t]:
			cur = append(cur, t)
		}
	}
	sides = append(sides, cur)
	if len(sides) < 2 {
		return
	}
	add := func(xs []string, x string) []string {
		if x == "" || slices.Contains(xs, x) {
			return xs
		}
		return append(xs, x)
	}
	for _, side := range sides {
		if len(side) == 0 {
			continue
		}
		vs := lx.matchVendors(side, rules)
		for _, v := range vs {
			e.vendors = add(e.vendors, v)
		}
		if len(vs) > 0 {
			continue
		}
		// Из совпавших — те, что названы полнее всего: «office» — Microsoft Office, а не Small Office Security.
		fs := lx.matchFamilies(side)
		top := 0.0
		for _, f := range fs {
			top = max(top, lx.familyCoverage(f, side))
		}
		for _, f := range fs {
			if c := lx.familyCoverage(f, side); c >= top-1e-9 {
				e.families = add(e.families, f)
				if c >= namedCoverageMin {
					e.named = add(e.named, f)
				}
			}
		}
	}
	if len(e.vendors) == 0 {
		return
	}
	// Сторона названа продуктом («windows»): его вендор — в фильтр вендоров, иначе фильтр другой стороны его отсечёт.
	for _, f := range e.families {
		e.vendors = add(e.vendors, lx.familyVendor[f])
	}
	e.dropNegated()
}

// withoutHandledNegations — исключение, которое разбор оборудования уже сделал условием («ноутбук без windows» —
// «Без ОС»), не дублируется исключением семейств.
func (p *hwPlan) withoutHandledNegations(families []string, label map[string]string) []string {
	if p == nil || len(p.negations) == 0 {
		return families
	}
	var out []string
	for _, f := range families {
		handled := false
		for _, n := range p.negations {
			handled = handled || strings.HasSuffix(n, " "+label[f])
		}
		if !handled {
			out = append(out, f)
		}
	}
	return out
}
