package services

import (
	"strings"
	"unicode"

	"kuznica/api/domain"
)

// Минимальная нормализация запроса (шаг 1 из ARCHITECTURE §2.1): нижний регистр, ё → е, единый вид
// дефисов и кавычек, схлопнутые пробелы. Раскладка, алиасы и опечатки — следующая волна.
//
// Та же свёртка символов стоит в индексе (char_filter fold и normalizer exact в search_index.go):
// подстрока B0 сравнивается с title.exact, и обе стороны должны быть приведены одинаково.

// foldRunes — замены символов; общие с маппингом индекса (foldMappings).
var foldRunes = map[rune]rune{
	'ё': 'е', 'Ё': 'Е',
	'‐': '-', '‑': '-', '‒': '-', '–': '-', '—': '-', '―': '-', '−': '-',
	'«': '"', '»': '"', '“': '"', '”': '"', '„': '"', '‟': '"', '″': '"',
	'\'': '"', '’': '"', '‘': '"', '`': '"',
}

// foldText — свёртка символов, нижний регистр и пробелы: то же, что normalizer exact в индексе.
func foldText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for _, r := range s {
		if f, ok := foldRunes[r]; ok {
			r = f
		}
		if unicode.IsSpace(r) {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// normalizeQuery — нормализованный запрос и изменения для трассы.
func normalizeQuery(q string) (string, []domain.Change) {
	n := foldText(q)
	changes := []domain.Change{}
	if n != q {
		changes = append(changes, domain.Change{Kind: "clean", From: q, To: n})
	}
	return n, changes
}

// Полная нормализация (ARCHITECTURE §2.1) — цепочка шагов, каждый пишет в трассу, что изменил:
// чистка → раскладка → защищённые токены → опечатки → алиасы. Раскладка и опечатки сверяются со
// словарём каталога, а не с общим словарём русского языка; «искать как введено» их отключает.

// Уверенность исправления.
const (
	confidenceHigh   = "high"
	confidenceMedium = "medium"
)

// Виды изменений (domain.Change.Kind).
const (
	changeClean  = "clean"
	changeLayout = "layout"
	changeTypo   = "typo"
	changeAlias  = "alias"
	// changeTranslit — слово каталога, набранное другим письмом («лт» → lt); changePrefix — недописанное
	// слово («win»): ищется по началу слова (title.ngram), полное слово — расширение и подсказка.
	changeTranslit = "translit"
	changePrefix   = "prefix"
)

// normResult — итог нормализации.
type normResult struct {
	// tokens — слова после всех шагов (через пробел — normalized).
	tokens []string
	// corrected — запрос после раскладки и опечаток, до алиасов: то, что показывается в плашке исправления.
	corrected string
	changes   []domain.Change
	// correction — исправление для плашки; nil — исправлять нечего или уверенность низкая.
	correction *correctionInfo
	// aliasHits — сработавшие алиасы; expansions — исходные слова алиасов (добавляются в текстовый поиск:
	// «офис» → office, но «Р7-Офис» тоже должен находиться).
	aliasHits  []domain.Alias
	expansions []string
	// prefixes — недописанное слово → слово каталога, которое с него начинается.
	prefixes map[string]string
	// protected — защищённые токены (версии, артикулы, коды): не исправляются.
	protected map[string]bool
	// raw — запрос как набран (для признака шума: разметка, SQL).
	raw string
}

// correctionInfo — исправление и его уверенность. applied=false — «Возможно, вы искали».
type correctionInfo struct {
	from, to, confidence string
	applied              bool
}

func (n normResult) normalized() string { return strings.Join(n.tokens, " ") }

// protectedWords — коды и редакции, которые не исправляются ни раскладкой, ни опечатками.
var protectedWords = setOf("pro", "x64", "x86", "lt", "se", "cal", "oem", "esd", "vm", "pdf", "ide")

// isProtectedToken — версия, артикул или код: есть цифра, дефис между буквами и цифрами или слово-код.
func isProtectedToken(t string) bool {
	if protectedWords[t] {
		return true
	}
	for _, r := range t {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// normalizeFull — нормализация запроса по словарю каталога lx и правилам rules. asTyped — без раскладки
// и опечаток.
func normalizeFull(q string, lx *searchLexicon, rules *searchRules, asTyped bool) normResult {
	res := normResult{raw: q, changes: []domain.Change{}, protected: map[string]bool{}, prefixes: map[string]string{}}
	clean := foldText(q)
	if clean != q {
		res.changes = append(res.changes, domain.Change{Kind: changeClean, From: q, To: clean})
	}
	tokens := strings.Fields(clean)
	// Хвостовые знаки вопроса и восклицания — не часть слова.
	for i, t := range tokens {
		tokens[i] = strings.TrimRight(t, "?!")
	}
	tokens = splitGlued(dropEmpty(tokens))
	original := append([]string(nil), tokens...)
	conf := ""
	if lx != nil && !asTyped {
		tokens = fixHomoglyphs(tokens, &res)
		tokens = fixLayoutWith(tokens, lx, rules, &res)
		for i, t := range tokens {
			if isProtectedToken(t) {
				res.protected[t] = true
				continue
			}
			to, c := fixTypo(t, lx, rules)
			switch c {
			case confidenceHigh:
				res.changes = append(res.changes, domain.Change{Kind: changeTypo, From: t, To: to})
				tokens[i] = to
			case confidenceMedium:
				if conf == "" {
					conf = confidenceMedium
				}
				res.suggest(original, i, to)
			}
		}
	}
	res.corrected = strings.Join(tokens, " ")
	if res.corrected != strings.Join(original, " ") {
		res.correction = &correctionInfo{from: strings.Join(original, " "), to: res.corrected,
			confidence: confidenceHigh, applied: true}
	}
	if lx != nil && !asTyped {
		tokens = completeWords(tokens, original, lx, rules, &res)
	}
	if rules != nil {
		tokens = applyAliases(tokens, rules, &res)
	}
	res.tokens = tokens
	return res
}

// completeWords — неизвестное слово, которое в каталоге пишется другим письмом, заменяется («лт» → lt,
// «antivirus» → антивирус: смешанный язык не делает запрос «товаром не из каталога»). Неизвестное слово, с
// которого начинается слово названий («win» → windows), остаётся как набрано — его находит поиск по началу
// слова, а полное слово идёт расширением и подсказкой «Возможно, вы искали». Как алиасы, это не исправление:
// плашки «Показаны результаты по…» нет.
func completeWords(tokens, original []string, lx *searchLexicon, rules *searchRules, res *normResult) []string {
	for i, t := range tokens {
		if res.protected[t] || isProtectedToken(t) || lexStopWords[t] || knownWord(t, lx, rules) {
			continue
		}
		if to := lx.translitOf(t); to != "" {
			res.changes = append(res.changes, domain.Change{Kind: changeTranslit, From: t, To: to})
			tokens[i] = to
			continue
		}
		if c := res.correction; c != nil && !c.applied {
			if ws := strings.Fields(c.to); i < len(ws) && i < len(original) && ws[i] != original[i] {
				continue // у слова уже есть исправление средней уверенности
			}
		}
		if i != len(tokens)-1 || !othersKnown(tokens, i, lx, rules) {
			continue // недописанным бывает только последнее слово, и остальные слова запроса понятны
		}
		if to := lx.prefixOf(t); to != "" {
			res.changes = append(res.changes, domain.Change{Kind: changePrefix, From: t, To: to})
			res.prefixes[t] = to
			res.expansions = append(res.expansions, to)
			if i < len(original) && original[i] == t {
				res.suggest(original, i, to)
			}
		}
	}
	return tokens
}

// othersKnown — все слова запроса, кроме i-го, известны (или служебные, или коды).
func othersKnown(tokens []string, i int, lx *searchLexicon, rules *searchRules) bool {
	for j, t := range tokens {
		if j != i && !lexStopWords[t] && !isProtectedToken(t) && !knownWord(t, lx, rules) {
			return false
		}
	}
	return true
}

// suggest — исправление средней уверенности: выдача по исходному, в плашке — «Возможно, вы искали».
// Если уже есть применённое исправление, предложение к нему не добавляется.
func (n *normResult) suggest(original []string, i int, to string) {
	if n.correction != nil && n.correction.applied {
		return
	}
	if n.correction == nil {
		n.correction = &correctionInfo{from: strings.Join(original, " "), confidence: confidenceMedium}
		n.correction.to = n.correction.from
	}
	ws := strings.Fields(n.correction.to)
	if i < len(ws) {
		ws[i] = to
	}
	n.correction.to = strings.Join(ws, " ")
}

func dropEmpty(ts []string) []string {
	out := ts[:0]
	for _, t := range ts {
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// homoglyphs — кириллические буквы, которые выглядят как латинские (и обратно).
var homoglyphs = map[rune]rune{
	'а': 'a', 'в': 'b', 'е': 'e', 'к': 'k', 'м': 'm', 'н': 'h', 'о': 'o', 'р': 'p', 'с': 'c', 'т': 't',
	'у': 'y', 'х': 'x', 'і': 'i',
}

// fixHomoglyphs — слово из латиницы с вкраплением похожих кириллических букв («prо» с русской «о»)
// приводится к латинице, и наоборот. Цифры не трогаются («1с» остаётся).
func fixHomoglyphs(tokens []string, res *normResult) []string {
	back := map[rune]rune{}
	for c, l := range homoglyphs {
		back[l] = c
	}
	for i, t := range tokens {
		lat, cyr := 0, 0
		for _, r := range t {
			switch {
			case r >= 'a' && r <= 'z':
				lat++
			case unicode.Is(unicode.Cyrillic, r):
				cyr++
			}
		}
		if lat == 0 || cyr == 0 {
			continue
		}
		to, ok := []rune(t), true
		for j, r := range to {
			switch {
			case lat >= cyr && unicode.Is(unicode.Cyrillic, r):
				to[j], ok = homoglyphs[r], ok && homoglyphs[r] != 0
			case lat < cyr && r >= 'a' && r <= 'z':
				to[j], ok = back[r], ok && back[r] != 0
			}
		}
		if ok && string(to) != t {
			res.changes = append(res.changes, domain.Change{Kind: changeTypo, From: t, To: string(to)})
			tokens[i] = string(to)
		}
	}
	return tokens
}

// Раскладка йцукен ↔ qwerty. foldText уже превратил ' и ` в ", поэтому " — это «э».
const (
	layoutEN = "qwertyuiop[]asdfghjkl;\"zxcvbnm,./{}:<>"
	layoutRU = "йцукенгшщзхъфывапролджэячсмитьбю.хъжбю"
)

var enToRu, ruToEn = func() (map[rune]rune, map[rune]rune) {
	en, ru := []rune(layoutEN), []rune(layoutRU)
	a, b := map[rune]rune{}, map[rune]rune{}
	for i := range en {
		a[en[i]] = ru[i]
		if _, ok := b[ru[i]]; !ok && ru[i] != '.' {
			b[ru[i]] = en[i]
		}
	}
	return a, b
}()

// switchLayout — слово, набранное не в той раскладке; ok=false — слово не похоже на такое (смешанные
// символы, нет букв).
func switchLayout(t string) (string, bool) {
	rs := []rune(t)
	latin, cyr := false, false
	for _, r := range rs {
		switch {
		case r >= 'a' && r <= 'z':
			latin = true
		case unicode.Is(unicode.Cyrillic, r):
			cyr = true
		}
	}
	if latin == cyr {
		return "", false
	}
	table := enToRu
	if cyr {
		table = ruToEn
	}
	out := make([]rune, len(rs))
	for i, r := range rs {
		switch m, ok := table[r]; {
		case ok:
			out[i] = m
		case unicode.IsDigit(r) || r == '-':
			out[i] = r
		default:
			return "", false
		}
	}
	return string(out), true
}

// knownWord — все слова токена известны каталогу или правилам.
func knownWord(t string, lx *searchLexicon, rules *searchRules) bool {
	ws := wordsOf(t)
	if len(ws) == 0 {
		return false
	}
	if rules != nil && rules.aliases[t].Alias != "" {
		return true
	}
	for _, w := range ws {
		if !lx.isKnown(w, rules) {
			return false
		}
	}
	return true
}

// fixLayoutWith — слово переводится по таблице клавиш, только если результат известен, а исходное — нет.
// Если так переведена большая часть слов запроса, остальные слова той же раскладки тоже переводятся:
// «rfcgthcrbq fynbdbhec» — фраза целиком.
func fixLayoutWith(tokens []string, lx *searchLexicon, rules *searchRules, res *normResult) []string {
	conv := make([]string, len(tokens))
	switched, candidates := 0, 0
	for i, t := range tokens {
		to, ok := switchLayout(t)
		if !ok || isProtectedToken(t) && !hasLetterOnly(t) {
			continue
		}
		candidates++
		if !knownWord(t, lx, rules) && knownWord(to, lx, rules) {
			conv[i] = to
			switched++
		}
	}
	if switched > 0 && switched*2 >= candidates {
		for i, t := range tokens {
			if conv[i] != "" || knownWord(t, lx, rules) {
				continue
			}
			if to, ok := switchLayout(t); ok && !hasDigit(t) {
				conv[i] = to
			}
		}
	}
	for i, to := range conv {
		if to == "" {
			continue
		}
		if res != nil {
			res.changes = append(res.changes, domain.Change{Kind: changeLayout, From: tokens[i], To: to})
		}
		tokens[i] = to
	}
	return tokens
}

func hasDigit(t string) bool {
	for _, r := range t {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func hasLetterOnly(t string) bool { return !hasDigit(t) }

// fixTypo — исправление опечатки по словарю каталога: Дамерау–Левенштейн ≤1 для слов до 5 букв, ≤2 для
// длиннее; только если ближайший вариант единственный. Уверенность: расстояние 1 в слове от 5 букв —
// high, иначе medium. Короткие (до 3 букв) и известные слова не трогаются.
func fixTypo(t string, lx *searchLexicon, rules *searchRules) (string, string) {
	rs := []rune(t)
	if len(rs) < 4 || knownWord(t, lx, rules) {
		return t, ""
	}
	for _, r := range rs {
		if !unicode.IsLetter(r) {
			return t, ""
		}
	}
	maxD := 1
	if len(rs) > 5 {
		maxD = 2
	}
	best, bestD, count := "", maxD+1, 0
	consider := func(w string) {
		wr := []rune(w)
		if abs(len(wr)-len(rs)) > maxD || len(wr) < 3 {
			return
		}
		d := damerau(rs, wr, bestD)
		if d > maxD {
			return
		}
		if (d == 2 || len(rs) <= 4) && wr[0] != rs[0] {
			return
		}
		switch {
		case d < bestD:
			best, bestD, count = w, d, 1
		case d == bestD && w != best:
			count++
		}
	}
	for w := range lx.core {
		consider(w)
	}
	if rules != nil {
		for k := range rules.aliases {
			if !strings.Contains(k, " ") {
				consider(k)
			}
		}
	}
	if count != 1 {
		return t, ""
	}
	if bestD == 1 && len(rs) >= 5 {
		return best, confidenceHigh
	}
	return best, confidenceMedium
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// damerau — расстояние Дамерау–Левенштейна (с перестановкой соседних букв); limit — дальше не считать.
func damerau(a, b []rune, limit int) int {
	n, m := len(a), len(b)
	d := make([][]int, n+1)
	for i := range d {
		d[i] = make([]int, m+1)
		d[i][0] = i
	}
	for j := 0; j <= m; j++ {
		d[0][j] = j
	}
	for i := 1; i <= n; i++ {
		rowMin := d[i][0]
		for j := 1; j <= m; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			v := min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				v = min(v, d[i-2][j-2]+1)
			}
			d[i][j] = v
			rowMin = min(rowMin, v)
		}
		if rowMin > limit {
			return limit + 1
		}
	}
	return d[n][m]
}

// applyAliases — алиасы «как пишут → как в каталоге», самый длинный ключ первым. Замена видна только в
// трассе: плашки исправления сленг не вызывает.
func applyAliases(tokens []string, rules *searchRules, res *normResult) []string {
	var out []string
	for i := 0; i < len(tokens); {
		matched := false
		for n := min(rules.maxWords, len(tokens)-i); n >= 1; n-- {
			key := strings.Join(tokens[i:i+n], " ")
			a, ok := rules.aliases[key]
			if !ok {
				continue
			}
			to := foldText(a.Canonical)
			if to != key {
				res.changes = append(res.changes, domain.Change{Kind: changeAlias, From: key, To: to})
				res.expansions = append(res.expansions, key)
			}
			res.aliasHits = append(res.aliasHits, a)
			out = append(out, strings.Fields(to)...)
			i += n
			matched = true
			break
		}
		if !matched {
			out = append(out, tokens[i])
			i++
		}
	}
	return out
}

// splitGlued — разделители и склейки внутри слова (нагрузочный тест П4): «касперский/10», «касперский,10пк» →
// «касперский 10 пк»; «10пк», «16ядер», «2года» → число и единица отдельно. Знаки «, / ; |» режутся только в
// кириллическом слове или рядом с цифрой: «,e[ufknthbz» — это «бухгалтерия» в английской раскладке, запятая там
// — буква «б». Число отделяется только от единицы количества или срока: «3d», «1с», «x64» не трогаются.
func splitGlued(tokens []string) []string {
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		for _, part := range splitSeparators(t) {
			out = append(out, splitNumberUnit(part)...)
		}
	}
	return out
}

func splitSeparators(t string) []string {
	if !strings.ContainsAny(t, ",/;|") {
		return []string{t}
	}
	rs := []rune(t)
	cyr := false
	for _, r := range rs {
		cyr = cyr || r >= 'а' && r <= 'я' || r == 'ё'
	}
	var parts []string
	start := 0
	for i, r := range rs {
		if !strings.ContainsRune(",/;|", r) {
			continue
		}
		nearDigit := i > 0 && unicode.IsDigit(rs[i-1]) || i+1 < len(rs) && unicode.IsDigit(rs[i+1])
		if !cyr && !nearDigit {
			continue
		}
		if i > start {
			parts = append(parts, string(rs[start:i]))
		}
		start = i + 1
	}
	if start < len(rs) {
		parts = append(parts, string(rs[start:]))
	}
	if len(parts) == 0 {
		return []string{t}
	}
	return parts
}

func splitNumberUnit(t string) []string {
	i := 0
	for i < len(t) && t[i] >= '0' && t[i] <= '9' {
		i++
	}
	if i == 0 || i == len(t) {
		return []string{t}
	}
	unit := t[i:]
	for _, r := range unit {
		if !unicode.IsLetter(r) {
			return []string{t}
		}
	}
	if _, ok := metricOf(unit); ok || durationUnit(unit) > 0 {
		return []string{t[:i], unit}
	}
	return []string{t}
}
