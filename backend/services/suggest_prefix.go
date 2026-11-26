package services

import (
	"sort"
	"strings"
	"sync/atomic"
	"unicode/utf8"
)

// Режим префикса нормализатора (RESEARCH А «Нормализатор получает режим префикса»). На неполном
// вводе исправления не переписывают запрос, а добавляют кандидатов: раскладка — дополнительный кандидат; опечатки —
// с 3-го символа и не в последнем (недописанном) слове, только если по набранному точных кандидатов нет; алиасы —
// только для законченных слов; парт-номера и артикулы — бережная канонизация (без дефисов, пробелов и регистра) и
// префикс. LLM здесь нет.

// Виды кандидатов префикса.
const (
	prefixTyped  = "typed"
	prefixLayout = "layout"
	prefixTypo   = "typo"
	prefixAlias  = "alias"
)

// prefixCand — вариант текста для словаря подсказок и товарной секции.
type prefixCand struct {
	text string
	kind string
}

// prefixQuery — разобранный неполный ввод.
type prefixQuery struct {
	folded string
	tokens []string
	// lastComplete — последнее слово закончено (после него пробел).
	lastComplete bool
	// cands — кандидаты: первым — как набрано; затем алиасы законченных слов и раскладка. Опечатки — typo().
	cands []prefixCand
	// code — канонический вид артикула или парт-номера (есть цифра, ≥ 4 знаков); "" — не похоже на код.
	code string
}

// minTypoRunes — опечатки исправляются начиная с третьего символа слова (fixTypo сам не трогает слова короче 4).
const minTypoRunes = 3

// normalizePrefix — разбор ввода в режиме префикса.
func normalizePrefix(q string, lx *searchLexicon, rules *searchRules) prefixQuery {
	pq := prefixQuery{folded: foldText(q)}
	pq.lastComplete = strings.HasSuffix(q, " ")
	for _, t := range strings.Fields(pq.folded) {
		if t = strings.TrimRight(t, "?!"); t != "" {
			pq.tokens = append(pq.tokens, t)
		}
	}
	typed := strings.Join(pq.tokens, " ")
	if typed == "" {
		return pq
	}
	pq.cands = append(pq.cands, prefixCand{text: typed, kind: prefixTyped})
	if c := strings.Join(pq.tokens, ""); hasDigit(c) && utf8.RuneCountInString(c) >= 4 {
		pq.code = normalizeMPN(c)
	}
	if lx == nil {
		return pq
	}
	complete, last := pq.split()
	if rules != nil && len(complete) > 0 {
		var nr normResult
		aliased := applyAliases(append([]string(nil), complete...), rules, &nr, nil)
		if t := strings.Join(append(aliased, last...), " "); t != typed {
			pq.cands = append(pq.cands, prefixCand{text: t, kind: prefixAlias})
		}
	}
	if t, ok := layoutCandidate(pq.tokens, pq.lastComplete, lx, rules); ok && t != typed {
		pq.cands = append(pq.cands, prefixCand{text: t, kind: prefixLayout})
	}
	return pq
}

// split — законченные слова и недописанное последнее (пусто, если ввод кончается пробелом).
func (pq prefixQuery) split() ([]string, []string) {
	if pq.lastComplete || len(pq.tokens) == 0 {
		return pq.tokens, nil
	}
	return pq.tokens[:len(pq.tokens)-1], pq.tokens[len(pq.tokens)-1:]
}

// typo — кандидат с исправленными опечатками в законченных словах (не в последнем недописанном); ok=false — нечего
// исправлять. Зовётся, только если по набранному точных кандидатов нет.
func (pq prefixQuery) typo(lx *searchLexicon, rules *searchRules) (prefixCand, bool) {
	if lx == nil {
		return prefixCand{}, false
	}
	complete, last := pq.split()
	fixed := append([]string(nil), complete...)
	changed := false
	for i, t := range fixed {
		if utf8.RuneCountInString(t) < minTypoRunes || isProtectedToken(t) {
			continue
		}
		if to, c := fixTypo(t, lx, rules); c != "" && to != t {
			fixed[i], changed = to, true
		}
	}
	if !changed {
		return prefixCand{}, false
	}
	return prefixCand{text: strings.Join(append(fixed, last...), " "), kind: prefixTypo}, true
}

// layoutCandidate — ввод в другой раскладке: каждое законченное слово после перевода известно каталогу, последнее —
// начало слова каталога; набранное при этом каталогу не известно. Цифры и коды не переводятся.
func layoutCandidate(tokens []string, lastComplete bool, lx *searchLexicon, rules *searchRules) (string, bool) {
	out := make([]string, len(tokens))
	switched := false
	for i, t := range tokens {
		last := i == len(tokens)-1 && !lastComplete
		to, ok := switchLayout(t)
		if !ok || hasDigit(t) {
			out[i] = t
			continue
		}
		typedOK := knownWord(t, lx, rules) || last && lx.hasPrefix(t)
		convOK := knownWord(to, lx, rules) || last && lx.hasPrefix(to)
		switch {
		case typedOK:
			out[i] = t
		case convOK:
			out[i], switched = to, true
		default:
			return "", false
		}
	}
	return strings.Join(out, " "), switched
}

// hasPrefix — с t начинается слово каталога (или t — само слово).
func (lx *searchLexicon) hasPrefix(t string) bool {
	if utf8.RuneCountInString(t) < 2 {
		return false
	}
	ws := lx.sortedWords()
	i := sort.SearchStrings(ws, t)
	return i < len(ws) && strings.HasPrefix(ws[i], t)
}

// lexWords — отсортированные слова словаря каталога (core) на снимок: префиксная проверка двоичным поиском.
type lexWords struct {
	lx    *searchLexicon
	words []string
}

var lexWordsCache atomic.Pointer[lexWords]

func (lx *searchLexicon) sortedWords() []string {
	if c := lexWordsCache.Load(); c != nil && c.lx == lx {
		return c.words
	}
	ws := make([]string, 0, len(lx.core))
	for w := range lx.core {
		ws = append(ws, w)
	}
	sort.Strings(ws)
	lexWordsCache.Store(&lexWords{lx: lx, words: ws})
	return ws
}

// codeIndex — артикулы и парт-номера позиций в каноническом виде, отсортированные: точное совпадение и префикс.
type codeIndex struct {
	lx    *searchLexicon
	keys  []string
	owner []string // product_id той же позиции, что keys[i]
}

var codeIndexCache atomic.Pointer[codeIndex]

func (lx *searchLexicon) codes() *codeIndex {
	if c := codeIndexCache.Load(); c != nil && c.lx == lx {
		return c
	}
	type kv struct{ k, pid string }
	var all []kv
	for pid, d := range lx.docs {
		for _, raw := range []string{d.SKU, d.MPNRaw} {
			if k := normalizeMPN(raw); utf8.RuneCountInString(k) >= 4 {
				all = append(all, kv{k, pid})
			}
		}
		if d.MPNNormalized != "" {
			all = append(all, kv{d.MPNNormalized, pid})
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].k != all[j].k {
			return all[i].k < all[j].k
		}
		return all[i].pid < all[j].pid
	})
	ci := &codeIndex{lx: lx}
	for i, x := range all {
		if i > 0 && all[i-1] == x {
			continue
		}
		ci.keys, ci.owner = append(ci.keys, x.k), append(ci.owner, x.pid)
	}
	codeIndexCache.Store(ci)
	return ci
}

// exact — позиции с таким артикулом или парт-номером.
func (ci *codeIndex) exact(code string) []string {
	var res []string
	for i := sort.SearchStrings(ci.keys, code); i < len(ci.keys) && ci.keys[i] == code; i++ {
		if !contains(res, ci.owner[i]) {
			res = append(res, ci.owner[i])
		}
	}
	return res
}

// prefix — позиции, артикул или парт-номер которых начинается с code (не больше limit).
func (ci *codeIndex) prefix(code string, limit int) []string {
	var res []string
	for i := sort.SearchStrings(ci.keys, code); i < len(ci.keys) && strings.HasPrefix(ci.keys[i], code) && len(res) < limit; i++ {
		if !contains(res, ci.owner[i]) {
			res = append(res, ci.owner[i])
		}
	}
	return res
}

// prefixCompletion — вид кандидата: набранное — начало ключа алиаса словаря.
const prefixCompletion = "completion"

// minCompletionRunes — дополнение по словарю — с третьего символа.
const minCompletionRunes = 3

// aliasCompletion — дополнение набранного по словарю алиасов (импорт + словарь понимания): ввод — начало ключа
// («эни» → «энидеск» → anydesk, «касп» → «каспер» → kaspersky, «макс» → «макспатрол» → maxpatrol). Из нескольких
// ключей — самый короткий, при равенстве — по алфавиту: «аст» → «астра» (вендор), а не «астра линукс се». vendor —
// вендор цели алиаса ("" — цель не вендор). Набранное с цифрами и коды не дополняются.
func aliasCompletion(typed string, rules *searchRules) (text, vendor string, ok bool) {
	c, ok := aliasCompletionOf(typed, rules)
	return c.text, c.vendor, ok
}

// completion — дополнение словаря: текст, вендор цели и категории цели (алиас с целью category: «офис» → офисные
// пакеты).
type completion struct {
	text, vendor string
	cats         []string
}

func aliasCompletionOf(typed string, rules *searchRules) (completion, bool) {
	text, vendor, cats, ok := aliasCompletionFull(typed, rules)
	return completion{text: text, vendor: vendor, cats: cats}, ok
}

func aliasCompletionFull(typed string, rules *searchRules) (text, vendor string, cats []string, ok bool) {
	if rules == nil || utf8.RuneCountInString(typed) < minCompletionRunes || hasDigit(typed) {
		return "", "", nil, false
	}
	best := ""
	for key := range rules.aliases {
		if !strings.HasPrefix(key, typed) {
			continue
		}
		if best == "" || len(key) < len(best) || len(key) == len(best) && key < best {
			best = key
		}
	}
	if best == "" {
		return "", "", nil, false
	}
	a := rules.aliases[best]
	text = strings.Join(strings.Fields(foldText(a.Canonical)), " ")
	if text == "" || text == typed {
		return "", "", nil, false
	}
	if a.Target == "vendor" {
		vendor = a.TargetID
	}
	return text, vendor, rules.categories[best], true
}
