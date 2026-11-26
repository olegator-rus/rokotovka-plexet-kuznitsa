package services

import (
	"context"
	"strings"
	"unicode/utf8"
)

// Сопоставление кластера с памятью агента (решения человека и ждущие предложения). Нормализованного текста мало:
// «ноушн» и «notion», «чат гпт» и «chatgpt», «ygjhn» и «нпорт» — одно и то же. Термины совпадают, если:
//   - равны или равны после транслитерации кириллицы (translit);
//   - один набран в неверной раскладке другого (switchLayout);
//   - равны их фонетические ключи (phoneticKey: транслит, «tion» → «шн», согласные без гласных) длиной от трёх
//     знаков — короче слишком много совпадений («кис» и «ксос»);
//   - векторы BGE-M3 близки: косинус ≥ memoryCosine (замер на mock: «фигма»/«figma» 0,865; разные товары —
//     не выше 0,76: «касперский»/«kaspersky» 0,758 — совпадают по фонетическому ключу).
// Без эмбеддера векторного правила нет — остальные работают.

const (
	memoryCosine     = 0.85
	memoryMinPhonKey = 3
)

// phoneticReplacements — порядок важен: сначала длинные сочетания. Цифры-заглушки — звуки, которые в латинице
// пишутся по-разному («ch», «sh», «zh», «j»).
var phoneticReplacements = strings.NewReplacer(
	"tion", "6n", "sion", "6n", "shch", "6", "sch", "6", "sh", "6", "tch", "4", "ch", "4", "dzh", "7", "zh", "7",
	"dj", "7", "j", "7", "ph", "f", "ck", "k", "qu", "kv", "q", "k", "w", "v", "x", "ks", "kh", "h",
	"ce", "se", "ci", "si", "cy", "sy", "c", "k",
)

// phoneticKey — грубый фонетический ключ термина: транслит, звукосочетания, без гласных, без повторов и
// пробелов. Цифры сохраняются («нод32» ≠ «нод»).
func phoneticKey(t string) string {
	s := phoneticReplacements.Replace(translit(strings.ToLower(t)))
	var b strings.Builder
	var prev rune
	for _, r := range s {
		if r != prev && !strings.ContainsRune("aeiouy -_.", r) {
			b.WriteRune(r)
		}
		prev = r
	}
	return b.String()
}

// termMatch — как совпали термины; "" — не совпали (без векторов).
func termMatch(a, b string) string {
	switch {
	case a == "" || b == "":
		return ""
	case a == b:
		return "term"
	case translit(a) == translit(b):
		return "translit"
	}
	if alt, ok := switchLayout(a); ok && alt == b {
		return "layout"
	}
	if alt, ok := switchLayout(b); ok && alt == a {
		return "layout"
	}
	if ka := phoneticKey(a); utf8.RuneCountInString(ka) >= memoryMinPhonKey && ka == phoneticKey(b) {
		return "phonetic"
	}
	return ""
}

// termsMatch — совпадение двух наборов терминов: по одному термину или всех слитно («чат гпт» = «chatgpt»).
func termsMatch(as, bs []string) string {
	for _, a := range as {
		for _, b := range bs {
			if m := termMatch(a, b); m != "" {
				return m
			}
		}
	}
	if len(as) > 1 || len(bs) > 1 {
		if m := termMatch(strings.Join(as, ""), strings.Join(bs, "")); m != "" && m != "term" {
			return m
		}
	}
	return ""
}

// memoryVectors — векторы терминов для сопоставления с памятью, кэш на запуск. Без эмбеддера — nil.
func (rc *runCtx) memoryVectors(ctx context.Context, texts []string) map[string][]float32 {
	if rc.memVecs == nil {
		rc.memVecs = map[string][]float32{}
	}
	var miss []string
	seen := map[string]bool{}
	for _, t := range texts {
		if _, ok := rc.memVecs[t]; !ok && t != "" && !seen[t] {
			miss = append(miss, t)
			seen[t] = true
		}
	}
	if len(miss) > 0 && rc.a != nil && !rc.memVecOff {
		vs := rc.a.s.embedTexts(ctx, miss)
		if len(vs) != len(miss) {
			// Эмбеддер недоступен: векторное правило выключается до конца запуска, без повторных попыток.
			rc.memVecOff = true
		} else {
			for i, t := range miss {
				rc.memVecs[t] = vs[i]
			}
		}
	}
	return rc.memVecs
}

// vectorMatch — максимальный косинус между терминами двух наборов не ниже memoryCosine.
func vectorMatch(vecs map[string][]float32, as, bs []string) bool {
	for _, a := range as {
		va := vecs[a]
		if va == nil {
			continue
		}
		for _, b := range bs {
			if vb := vecs[b]; vb != nil && cosine(va, vb) >= memoryCosine {
				return true
			}
		}
	}
	return false
}
