package services

import (
	"context"
	"errors"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"kuznica/api/domain"
	"kuznica/api/externalApi/catalogsource"
	"kuznica/api/in"
	"kuznica/api/out"

	"go.uber.org/zap"
)

// Понимание запросов на данных заказчика (контур фида):
//   - словарь с типом связи и основанием — файл данных, читается при старте и накладывается на словарь версии
//     правил (applyLexicon): жаргон, написание и ребрендинг подставляются, преемник — показывается с
//     пояснением, drop снимает запись импорта, которая на этом каталоге ведёт не туда;
//   - намерение по категории (categoryIntent): главное слово запроса — название корневой категории или
//     категории второго уровня → основная выдача из неё, остальные смыслы — уточнения;
//   - числа и короткие коды как названия: «365», «сканер-вс», «max patrol» (joinSplitNames, nameNumbers,
//     compoundNames в search_lexicon.go).

// lexiconLoader — источник каталога со словарём понимания запросов (generic: mapping.lexicon).
type lexiconLoader interface {
	LoadLexicon(ctx context.Context) ([]domain.LexiconEntry, error)
}

// LoadLexicon — словарь из источника каталога. Нет файла — словаря нет; ошибка разбора — предупреждение и
// работа на словаре импорта (битый файл не должен ронять поиск).
func (s *Services) LoadLexicon(ctx context.Context) {
	l, ok := s.source.(lexiconLoader)
	if !ok {
		return
	}
	es, err := l.LoadLexicon(ctx)
	switch {
	case errors.Is(err, catalogsource.ErrNoFile):
		return
	case err != nil:
		zap.L().Warn("словарь понимания запросов не прочитан — словарь импорта", zap.Error(err))
		return
	}
	s.SetLexicon(es)
	zap.L().Info("словарь понимания запросов загружен", zap.Int("entries", len(es)))
}

// SetLexicon — словарь понимания запросов (тесты и старт). nil — без словаря.
func (s *Services) SetLexicon(es []domain.LexiconEntry) {
	cp := append([]domain.LexiconEntry(nil), es...)
	s.core.lexicon.Store(&cp)
	s.core.latest.Store(nil)
}

func (s *Services) lexicon() []domain.LexiconEntry {
	if p := s.core.lexicon.Load(); p != nil {
		return *p
	}
	return nil
}

// lexiconKey — ключ записи словаря: слова через пробел после свёртки.
func lexiconKey(alias string) string { return strings.Join(strings.Fields(foldText(alias)), " ") }

// applyLexicon — правила r со словарём понимания запросов поверх. Запись словаря заменяет запись импорта с тем же
// ключом; запись, которую изменил агент (её нет в словаре импорта в том же виде), остаётся за агентом. Вендорские
// записи импорта, чья фраза в названиях семейств чаще означает чужие продукты («kit», «universe»), снимаются:
// иначе слово запроса становится строгим фильтром по вендору.
func applyLexicon(r *searchRules, lx *searchLexicon, entries []domain.LexiconEntry) *searchRules {
	if r == nil || lx == nil {
		return r
	}
	base := map[string]domain.Alias{}
	for _, a := range lx.baseAliases {
		if k := lexiconKey(a.Alias); k != "" {
			if _, dup := base[k]; !dup {
				base[k] = a
			}
		}
	}
	fromImport := func(k string) bool {
		a, ok := r.aliases[k]
		if !ok {
			return true
		}
		b, okb := base[k]
		return okb && b.Canonical == a.Canonical && b.Target == a.Target && b.TargetID == a.TargetID
	}
	aliases := make(map[string]domain.Alias, len(r.aliases)+len(entries))
	for k, a := range r.aliases {
		if a.Target == domain.AliasTargetVendor && fromImport(k) && lx.vendorAmbiguous[strings.Join(wordsOf(k), " ")] {
			continue
		}
		aliases[k] = a
	}
	categories, successors := map[string][]string{}, map[string]domain.LexiconEntry{}
	for _, e := range entries {
		k := lexiconKey(e.Alias)
		if k == "" || !fromImport(k) {
			continue
		}
		if e.Kind == domain.LexiconDrop {
			delete(aliases, k)
			continue
		}
		a := domain.Alias{Alias: k, Canonical: e.Canonical, Kind: e.Kind, Target: e.Target}
		switch e.Target {
		case domain.AliasTargetVendor:
			if lx.vendorName[e.TargetIDs[0]] == "" {
				continue // вендора нет в этом каталоге
			}
			a.TargetID = e.TargetIDs[0]
		case domain.AliasTargetCategory:
			var ids []string
			for _, id := range e.TargetIDs {
				if lx.categoryName[id] != "" {
					ids = append(ids, id)
				}
			}
			if len(ids) == 0 {
				continue
			}
			a.TargetID = ids[0]
			categories[k] = ids
		}
		if e.Kind == domain.LexiconSuccessor {
			successors[k] = e
		}
		aliases[k] = a
	}
	list := make([]domain.Alias, 0, len(aliases))
	keys := make([]string, 0, len(aliases))
	for k := range aliases {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		list = append(list, aliases[k])
	}
	out := newSearchRules(r.version, list)
	out.categories, out.successors = categories, successors
	return out
}

// successorHit — сработавшая запись «преемник»: исходные слова и пояснение.
type successorHit struct {
	from string
	note string
}

// --- намерение по категории ---

// categoryIntent — основная выдача из категорий намерения и остальные смыслы (уточнения).
type categoryIntent struct {
	cats []string
	// word — главное слово (основа существительного): у оборудования в основной выдаче остаются позиции, чьё
	// название начинается с него («Рабочая станция DEPO Race» в «Rack-серверах» — не сервер).
	word string
	// other — категории остальных смыслов главного слова: уточнения под выдачей.
	other []meaning
	// source — lexicon | category_name.
	source string
	// textOptional — слова запроса сказаны самой категорией («антивирус», «сервер»): в тексте позиции их может не
	// быть, они только поднимают её.
	textOptional bool
}

// signalCategoryIntent — сигнал маршрута: основная выдача из категории намерения.
const signalCategoryIntent = "category_intent"

// Окончания для сравнения слова запроса со словом названия категории: «сервер» = «серверы», но не «серверное».
var (
	adjEndings  = []string{"ыми", "ими", "ого", "его", "ому", "ему", "ый", "ий", "ой", "ая", "яя", "ое", "ее", "ые", "ие", "ым", "им", "ых", "их", "ую", "юю"}
	nounEndings = []string{"ами", "ями", "ов", "ев", "ей", "ам", "ям", "ах", "ях", "ом", "ем", "ы", "и", "а", "я", "у", "ю", "е", "ь"}
)

// isAdjective — кириллическое слово с окончанием прилагательного («серверное», «офисные»).
func isAdjective(w string) bool {
	if utf8.RuneCountInString(w) < 5 || !isCyrillicWord(w) {
		return false
	}
	for _, e := range adjEndings {
		if strings.HasSuffix(w, e) {
			return true
		}
	}
	return false
}

// nounStem — основа существительного: без падежного окончания, не короче четырёх букв. Латиница — как есть.
func nounStem(w string) string {
	if !isCyrillicWord(w) {
		return w
	}
	for _, e := range nounEndings {
		if strings.HasSuffix(w, e) && utf8.RuneCountInString(w)-utf8.RuneCountInString(e) >= 4 {
			return strings.TrimSuffix(w, e)
		}
	}
	return w
}

// headScore — насколько слово w (существительное запроса) — главное в названии категории: 3 — название из
// одного этого слова, 2 — первое слово, 1 — другое существительное; слова после «для» уточняют («Антивирусы
// для дома») и не считаются; 0 — нет.
func headScore(name, w string) int {
	ws := wordsOf(name)
	stem := nounStem(w)
	best := 0
	afterFor := false
	pos := 0
	for _, cw := range ws {
		if cw == "для" {
			afterFor = true
			continue
		}
		if isLatinWord(cw) && pos == 0 && !isLatinWord(w) {
			continue // «Rack-серверы», «3D Сканеры»: латинская приставка перед словом
		}
		if afterFor || lexStopWords[cw] {
			pos++
			continue
		}
		if !isAdjective(cw) && nounStem(cw) == stem {
			sc := 1
			if pos == 0 {
				sc = 2
			}
			if len(ws) == 1 {
				sc = 3
			}
			best = max(best, sc)
		}
		pos++
	}
	return best
}

// mentions — слово w встречается в названии в любой форме (основа из шести букв или основа существительного).
func mentions(name, w string) bool {
	st, ns := stemOf(w), nounStem(w)
	for _, cw := range wordsOf(name) {
		if stemOf(cw) == st || nounStem(cw) == ns {
			return true
		}
	}
	return false
}

// findCategoryIntent — намерение по категории. Из словаря (запись с целью category) — сразу; иначе по словам:
// одно значимое слово (или все слова запроса из названия одной категории), не называющее продукт или вендора,
// совпадает с главным словом названия категории. Явный фильтр категории в запросе — намерения нет.
func findCategoryIntent(nr normResult, ent queryEntities, lx *searchLexicon, rules *searchRules, req in.SearchRequest) *categoryIntent {
	for _, f := range req.Filters {
		if f.Key == facetCategory {
			return nil
		}
	}
	if rules != nil {
		for _, a := range nr.aliasHits {
			ids := rules.categories[lexiconKey(a.Alias)]
			if len(ids) > 0 && len(ent.vendors) > 0 && !vendorsIn(lx, ent.vendors, ids) {
				continue // «касперский … офис»: названный вендор не из этих категорий — слово значит другое
			}
			if len(ids) > 0 {
				ci := &categoryIntent{cats: ids, word: nounStem(foldText(a.Canonical)), source: "lexicon"}
				for _, id := range ids {
					ci.textOptional = ci.textOptional || headScore(lx.categoryName[id], foldText(a.Canonical)) > 0
				}
				return ci
			}
		}
	}
	if len(ent.vendors) > 0 || len(ent.named) > 0 || ent.nonProduct || ent.noise {
		return nil
	}
	var sig []string
	tokens := ent.textTokens
	if at := indexOf(tokens, "для"); at > 0 {
		tokens = tokens[:at] // «антивирус для сервера»: после «для» — платформа или назначение, главное слово до него
	}
	for _, t := range tokens {
		if lexStopWords[t] || isProtectedToken(t) || ent.vendorTokens[t] || lx.compoundNames[t] || len(wordsOf(t)) != 1 {
			continue
		}
		sig = append(sig, t)
	}
	if len(sig) == 0 || len(sig) > 3 || len(lx.matchFamilies(sig)) > 0 {
		return nil
	}
	type cand struct {
		id    string
		score int
		word  string
	}
	var cands []cand
	best := 0
	for id, name := range lx.categoryName {
		if lx.categoryFamilies[id] == 0 {
			continue
		}
		for _, w := range sig {
			if isAdjective(w) {
				continue
			}
			sc := headScore(name, w)
			if sc == 0 {
				continue
			}
			all := true
			for _, o := range sig {
				all = all && (o == w || mentions(name, o))
			}
			if !all {
				continue
			}
			cands = append(cands, cand{id, sc, w})
			best = max(best, sc)
		}
	}
	if best == 0 {
		return nil
	}
	chosen := map[string]bool{}
	roots := map[string]bool{}
	word := ""
	for _, c := range cands {
		if c.score == best {
			chosen[c.id] = true
			roots[lx.categoryRoot[c.id]] = true
			word = c.word
		}
	}
	if best < 2 && len(roots) > 1 {
		return nil // слово — второстепенное в названиях разных разделов: смысл неясен
	}
	// Корень и его подкатегории с тем же главным словом — подкатегории точнее («Антивирусы и безопасность» →
	// «Антивирусы для организаций», «Антивирусы для дома»).
	for id := range chosen {
		if lx.categoryRoot[id] != id {
			delete(chosen, lx.categoryRoot[id])
		}
	}
	ci := &categoryIntent{word: nounStem(word), source: "category_name", textOptional: true}
	for id := range chosen {
		ci.cats = append(ci.cats, id)
	}
	sort.Slice(ci.cats, func(i, j int) bool {
		a, b := lx.categoryFamilies[ci.cats[i]], lx.categoryFamilies[ci.cats[j]]
		if a != b {
			return a > b
		}
		return ci.cats[i] < ci.cats[j]
	})
	ci.other = otherMeanings(lx, ci, word)
	return ci
}

// maxIntentChips — сколько остальных смыслов показывать уточнениями.
const maxIntentChips = 4

// otherMeanings — категории, где главное слово тоже встречается, но не главным: «сервер» → «Серверное ПО»,
// «Серверные опции»; затем категории, где не меньше minMeaningFamilies семейств называют это слово (в том числе
// латиницей: «Альт Сервер 11», «Windows Server» → раздел «Операционные системы»). Внутри — по числу таких семейств.
func otherMeanings(lx *searchLexicon, ci *categoryIntent, word string) []meaning {
	in := map[string]bool{}
	for _, c := range ci.cats {
		in[c] = true
	}
	byName := map[string]bool{}
	for id, name := range lx.categoryName {
		if in[id] || lx.categoryRoot[id] == id || lx.categoryFamilies[id] == 0 || in[lx.categoryRoot[id]] && headScore(name, word) > 0 {
			continue
		}
		if mentions(name, word) {
			byName[id] = true
		}
	}
	stem, lat := nounStem(word), translit(word)
	count := map[string]int{}
	for _, f := range lx.familyOrder {
		c := lx.familyCat[f]
		if in[c] || in[lx.categoryRoot[c]] || lx.consumer[f] || byName[c] || !lx.familyBuyable(f) {
			continue // смысл — то, что можно купить: семейства только по запросу его не задают
		}
		for _, w := range wordsOf(lx.familyName[f]) {
			if nounStem(w) == stem || stemOf(w) == stemOf(word) || lat != "" && w == lat {
				count[lx.categoryRoot[c]]++ // смысл по семействам — раздел целиком: «Операционные системы»
				break
			}
		}
	}
	// Категория, названная словом, — уточнение «весь раздел»: hits — её семейства, которые можно купить (столько
	// карточек покажет фильтр по ней), а не совпавшие по названию (count — по разделам, у подкатегории всегда 0).
	inCat := map[string]int{}
	for _, f := range lx.familyOrder {
		if c := lx.familyCat[f]; byName[c] && lx.familyBuyable(f) {
			inCat[c]++
		}
	}
	var ms []meaning
	for id := range byName {
		ms = append(ms, meaning{id: id, families: max(inCat[id], 1), byName: true})
	}
	for id, n := range count {
		if !byName[id] && n >= minMeaningFamilies {
			ms = append(ms, meaning{id: id, families: n})
		}
	}
	sort.Slice(ms, func(i, j int) bool {
		if ms[i].byName != ms[j].byName {
			return ms[i].byName
		}
		if ms[i].families != ms[j].families {
			return ms[i].families > ms[j].families
		}
		return ms[i].id < ms[j].id
	})
	if len(ms) > maxIntentChips {
		ms = ms[:maxIntentChips]
	}
	return ms
}

// minMeaningFamilies — со скольких семейств, называющих слово, категория становится отдельным смыслом.
const minMeaningFamilies = 3

// meaning — другой смысл главного слова: категория и сколько её семейств его называют.
type meaning struct {
	id       string
	families int
	byName   bool
}

// filter — условие отбора кандидатов: категория или её корень среди категорий намерения.
func (ci *categoryIntent) filter() any {
	return map[string]any{"terms": map[string]any{"category_ids": ci.cats}}
}

// keeps — позиция остаётся в основной выдаче намерения: ПО — всегда; оборудование — если название начинается с
// главного слова («Сервер INFERIT», «Rack-сервер DEPO»), а не «Рабочая станция DEPO Race» или «Радиатор для
// сервера» из той же категории.
func (ci *categoryIntent) keeps(d indexDoc) bool {
	if ci == nil || ci.source != "category_name" || docKind(d) == domain.KindSoftware {
		return true
	}
	first := strings.Fields(foldText(d.Title))
	if len(first) == 0 {
		return false
	}
	for _, part := range strings.Split(first[0], "-") {
		if nounStem(strings.Trim(part, "\"(),.:")) == ci.word {
			return true
		}
	}
	return false
}

// keepHits — кандидаты источника без позиций, которых намерение не держит.
func (ci *categoryIntent) keepHits(lx *searchLexicon, hits []domain.RankedHit) []domain.RankedHit {
	if ci == nil || ci.source != "category_name" {
		return hits
	}
	out := make([]domain.RankedHit, 0, len(hits))
	for _, h := range hits {
		if d, ok := lx.docs[h.ProductID]; ok && !ci.keeps(d) {
			continue
		}
		out = append(out, h)
	}
	for i := range out {
		out[i].Rank = i + 1
	}
	return out
}

// intentRefinements — остальные смыслы главного слова: уточнение «тот же запрос в категории …».
func intentRefinements(lx *searchLexicon, ci *categoryIntent, query string) []out.Refinement {
	if ci == nil {
		return nil
	}
	var res []out.Refinement
	field, op := facetCategory, refOpEq
	for _, m := range ci.other {
		label := strings.TrimSpace(strings.TrimSuffix(lx.categoryName[m.id], "(Импортозамещение)"))
		value := m.id
		res = append(res, out.Refinement{Label: label, Query: query, Field: &field, Value: &value, Op: &op,
			Hits: m.families})
	}
	return res
}

// withIntentRefinements — уточнения остальных смыслов первыми, без повторов категорий.
func withIntentRefinements(have []out.Refinement, add []out.Refinement) []out.Refinement {
	if len(add) == 0 {
		return have
	}
	seen := map[string]bool{}
	res := make([]out.Refinement, 0, len(have)+len(add))
	for _, r := range add {
		seen[*r.Value] = true
		res = append(res, r)
	}
	for _, r := range have {
		if r.Field != nil && *r.Field == facetCategory && r.Value != nil && seen[*r.Value] {
			continue
		}
		res = append(res, r)
	}
	return res
}

// --- числа, коды и названия, набранные раздельно ---

// changeJoin — слова, набранные раздельно, а в каталоге слитно («max patrol» → maxpatrol).
const changeJoin = "join"

// joinSplitNames — два соседних слова одного письма, которые слитно — слово названий каталога, а раздельно
// подряд в названиях семейств не встречаются: «max patrol» → maxpatrol. «astra linux» не склеивается: так
// пишут в названиях.
func joinSplitNames(tokens []string, lx *searchLexicon, res *normResult) []string {
	if lx == nil || len(tokens) < 2 {
		return tokens
	}
	out := make([]string, 0, len(tokens))
	for i := 0; i < len(tokens); i++ {
		if i+1 < len(tokens) {
			a, b := tokens[i], tokens[i+1]
			same := isLatinWord(a) && isLatinWord(b) || isCyrillicWord(a) && isCyrillicWord(b)
			if same && utf8.RuneCountInString(a) >= 2 && utf8.RuneCountInString(b) >= 2 && !lexStopWords[a] && !lexStopWords[b] &&
				lx.core[a+b] > 0 && !lx.familyBigrams[a+" "+b] {
				res.changes = append(res.changes, domain.Change{Kind: changeJoin, From: a + " " + b, To: a + b})
				out = append(out, a+b)
				i++
				continue
			}
		}
		out = append(out, tokens[i])
	}
	return out
}

// buildUnderstanding — части словаря каталога для понимания запросов: числа и составные слова из названий
// семейств, пары слов подряд в названиях, неоднозначные фразы вендоров, число семейств в категориях.
func (lx *searchLexicon) buildUnderstanding() {
	lx.nameNumbers, lx.compoundNames, lx.familyBigrams = map[string]bool{}, map[string]bool{}, map[string]bool{}
	lx.vendorAmbiguous, lx.categoryFamilies = map[string]bool{}, map[string]int{}
	// own / others — сколько семейств вендора и чужих семейств содержат слово в названии.
	byWord := map[string]map[string]int{}
	for _, f := range lx.familyOrder {
		name := lx.familyName[f]
		ws := wordsOf(name)
		for i, w := range ws {
			if strings.IndexFunc(w, unicode.IsLetter) < 0 {
				lx.nameNumbers[w] = true
			}
			if i+1 < len(ws) {
				lx.familyBigrams[w+" "+ws[i+1]] = true
			}
		}
		seen := map[string]bool{}
		for _, w := range ws {
			if seen[w] {
				continue
			}
			seen[w] = true
			if byWord[w] == nil {
				byWord[w] = map[string]int{}
			}
			byWord[w][lx.familyVendor[f]]++
		}
		for _, raw := range strings.Fields(foldText(name)) {
			t := strings.Trim(raw, "\"(),.:;!?")
			if strings.Contains(t, "-") && len(wordsOf(t)) >= 2 && strings.IndexFunc(t, unicode.IsLetter) >= 0 {
				lx.compoundNames[t] = true
			}
		}
		c := lx.familyCat[f]
		lx.categoryFamilies[c]++
		if r := lx.categoryRoot[c]; r != "" && r != c {
			lx.categoryFamilies[r]++
		}
	}
	lx.knownStems = map[string]bool{}
	for w := range lx.core {
		if isCyrillicWord(w) && utf8.RuneCountInString(w) >= minStemRunes { // «линк» — основа «линком»
			lx.knownStems[stemWord(w)] = true
		}
	}
	lx.vendorStemPhrases = map[string]string{}
	for p, v := range lx.vendorPhrases {
		if !lx.vendorAmbiguous[p] {
			addStemKey(lx.vendorStemPhrases, stemPhrase(p), v, func(prev string) bool { return prev == v })
		}
	}
	for p, v := range lx.vendorPhrases {
		if strings.Contains(p, " ") {
			continue
		}
		own, others := 0, 0
		for vid, n := range byWord[p] {
			if vid == v {
				own += n
			} else {
				others += n
			}
		}
		if others > own {
			lx.vendorAmbiguous[p] = true
			if st := stemPhrase(p); lx.vendorStemPhrases[st] == v {
				delete(lx.vendorStemPhrases, st)
			}
		}
	}
}

// fieldSuccessor — поле пояснения «на этом рынке — …» в relaxed.
const fieldSuccessor = "successor"

// setIntent — намерение по категории для сущностей запроса. Снятый покупателем чип категории снимает и намерение.
// Категория намерения по словам становится категорией запроса (чип и сужение по разделу), если её ещё нет и
// это не оборудование со своей категорией.
func (e *queryEntities) setIntent(nr normResult, lx *searchLexicon, rules *searchRules, req in.SearchRequest,
	removed map[string]map[string]bool, hardware bool) {
	ci := findCategoryIntent(nr, *e, lx, rules, req)
	if ci == nil {
		return
	}
	for _, c := range ci.cats {
		if removed[fieldCategory][c] {
			return
		}
	}
	e.intent = ci
	if e.category == "" && !hardware {
		e.category, e.categoryByWords = ci.cats[0], true
	}
}

func indexOf(xs []string, x string) int {
	for i, v := range xs {
		if v == x {
			return i
		}
	}
	return -1
}

// numberOwnerLead — во сколько раз у вендора-владельца больше семейств с этим числом в названии, чем у следующего.
const numberOwnerLead = 1.5

// numberOwner — запрос — только число из названий семейств («365»): вендор, у которого таких семейств больше
// всего с отрывом (Microsoft 365 — 9 семейств, CyberLink … 365 — 5). Иначе "".
func (lx *searchLexicon) numberOwner(tokens []string) string {
	num := ""
	for _, t := range tokens {
		switch {
		case lexStopWords[t]:
		case lx.nameNumbers[t] && num == "":
			num = t
		default:
			return ""
		}
	}
	if num == "" {
		return ""
	}
	count := map[string]int{}
	for _, f := range lx.familyOrder {
		for _, w := range wordsOf(lx.familyName[f]) {
			if w == num {
				count[lx.familyVendor[f]]++
				break
			}
		}
	}
	best, second, owner := 0, 0, ""
	for v, n := range count {
		switch {
		case n > best || n == best && v < owner:
			second, best, owner = best, n, v
		case n > second:
			second = n
		}
	}
	if best < 2 || float64(best) < numberOwnerLead*float64(second) {
		return ""
	}
	return owner
}

// keepFamilies — семейства выдачи без тех, чью голову намерение не держит (keeps); причина — в отказы трассы.
func (ci *categoryIntent) keepFamilies(fams []*familyHit, rejected *[]domain.RejectedHit) []*familyHit {
	if ci == nil || ci.source != "category_name" {
		return fams
	}
	out := fams[:0]
	for _, fh := range fams {
		if fh.head.ProductID != "" && !ci.keeps(fh.head) {
			addRejected(rejected, fh.head.ProductID, rejectCategory, "название не начинается с главного слова запроса — другой товар категории")
			continue
		}
		out = append(out, fh)
	}
	return out
}

// familyBuyable — у семейства есть позиция в наличии с ценой.
func (lx *searchLexicon) familyBuyable(fid string) bool {
	for _, pid := range lx.families[fid] {
		if d := lx.docs[pid]; d.Available != nil && *d.Available && d.PriceRub != nil && *d.PriceRub > 0 {
			return true
		}
	}
	return false
}

// typoByContext — опечатка с несколькими равными исправлениями («totl» → total, tool, …): берётся то, что вместе
// с соседним словом запроса стоит подряд в названии семейства («total security»), если такое одно.
func typoByContext(tokens []string, i int, lx *searchLexicon, rules *searchRules) string {
	t := tokens[i]
	rs := []rune(t)
	if lx == nil || len(rs) < 4 || knownWord(t, lx, rules) || !(isLatinWord(t) || isCyrillicWord(t)) {
		return ""
	}
	prev, next := "", ""
	if i > 0 {
		prev = tokens[i-1]
	}
	if i+1 < len(tokens) {
		next = tokens[i+1]
	}
	if prev == "" && next == "" {
		return ""
	}
	found := ""
	for w := range lx.core {
		wr := []rune(w)
		if abs(len(wr)-len(rs)) > 1 || len(wr) < 3 || wr[0] != rs[0] {
			continue
		}
		if !lx.familyBigrams[prev+" "+w] && !lx.familyBigrams[w+" "+next] {
			continue
		}
		if damerau(rs, wr, 1) > 1 {
			continue
		}
		if found != "" && found != w {
			return ""
		}
		found = w
	}
	return found
}

// --- склонения ---

// stemEndings — окончания, которые снимает stemWord: прилагательные и существительные, длинные первыми.
var stemEndings = func() []string {
	// «-ия», «-ии», «-ию», «-ием»: «копирования» и «копирование» — одна основа «копирован».
	es := append(append([]string{"ием", "иям", "иях", "ия", "ии", "ию"}, adjEndings...), nounEndings...)
	sort.SliceStable(es, func(i, j int) bool { return utf8.RuneCountInString(es[i]) > utf8.RuneCountInString(es[j]) })
	return es
}()

// minStemRunes — основа короче не бывает: «мтс», «ос» не склоняются.
const minStemRunes = 4

// stemWord — основа кириллического слова без падежного окончания: «касперского» и «касперский» → «касперск»,
// «сканера» → «сканер», «бэкапа» → «бэкап». Латиница, числа, артикулы и слова с цифрой — как есть.
func stemWord(w string) string {
	if !isCyrillicWord(w) {
		return w
	}
	n := utf8.RuneCountInString(w)
	for _, e := range stemEndings {
		if strings.HasSuffix(w, e) && n-utf8.RuneCountInString(e) >= minStemRunes {
			return strings.TrimSuffix(w, e)
		}
	}
	return w
}

// stemPhrase — фраза по основам слов.
func stemPhrase(p string) string {
	ws := strings.Fields(p)
	for i, w := range ws {
		ws[i] = stemWord(w)
	}
	return strings.Join(ws, " ")
}

// addStemKey — основа → значение; другое значение с той же основой (same == false) делает основу неоднозначной.
func addStemKey(m map[string]string, stem, val string, same func(prev string) bool) {
	if stem == "" {
		return
	}
	prev, ok := m[stem]
	switch {
	case !ok:
		m[stem] = val
	case prev != "" && prev != val && !same(prev):
		m[stem] = ""
	}
}

// vendorOfStem — вендор по фразе в другом падеже: основы слов совпали с фразой имени вендора или его алиаса.
func (lx *searchLexicon) vendorOfStem(p string, rules *searchRules) string {
	st := stemPhrase(p)
	if st == p && !strings.ContainsFunc(p, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) {
		return ""
	}
	if v := lx.vendorStemPhrases[st]; v != "" && lx.vendorName[v] != "" {
		return v
	}
	if rules != nil {
		if v := rules.vendorStemPhrases[st]; v != "" && lx.vendorName[v] != "" {
			return v
		}
	}
	return ""
}

// vendorsIn — у одного из вендоров есть семейство в одной из категорий (или её подкатегориях).
func vendorsIn(lx *searchLexicon, vendors, cats []string) bool {
	in := map[string]bool{}
	for _, c := range cats {
		in[c] = true
	}
	for _, f := range lx.familyOrder {
		c := lx.familyCat[f]
		if (in[c] || in[lx.categoryRoot[c]]) && contains(vendors, lx.familyVendor[f]) {
			return true
		}
	}
	return false
}

// softwareCategoryWord — слово — главное существительное названия категории не из оборудования (hw): «антивирус»,
// «офис» нет («Офисные программы» — прилагательное), «бэкап» нет; «антивирусы для организаций» — да.
func (lx *searchLexicon) softwareCategoryWord(w string, hw map[string]bool) bool {
	if utf8.RuneCountInString(w) < 5 || !isCyrillicWord(w) || isAdjective(w) {
		return false
	}
	hwRoot := map[string]bool{}
	for id := range hw {
		hwRoot[lx.categoryRoot[id]] = true
	}
	for id, name := range lx.categoryName {
		if lx.categoryFamilies[id] == 0 || hw[id] || hwRoot[id] || hw[lx.categoryRoot[id]] {
			continue // раздел, где есть оборудование («Серверы и опции»), — не ПО
		}
		if headScore(name, w) >= 2 {
			return true
		}
	}
	return false
}
