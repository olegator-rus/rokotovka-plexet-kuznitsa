package services

import (
	"strings"
	"unicode"
)

// Запросы к OpenSearch. Строит их только код: ни запрос покупателя, ни ответ модели в тело запроса не
// попадают иначе как значением поля.

// candidatesTop — сколько кандидатов берёт каждый источник (BM25 и kNN), ARCHITECTURE §2.4.
const candidatesTop = 50

// sourceExcludes — вектор в выдаче не нужен: он большой, а позиция ответа собирается из остальных полей.
var sourceExcludes = map[string]any{"excludes": []string{"embedding"}}

// bm25Fields — поля multi_match B1 с весами. B1 — точка отсчёта «BM25 без улучшений»:
// поля не меняются.
var bm25Fields = []string{
	"title.exact^5", "title.ru^3", "title.ngram", "family_name^3", "aliases^2", "vendor^2", "sku^5",
	"description", "attributes",
}

// b3Fields — поля текстовой ветки B3 (контур фида): у фида заказчика длинные названия позиций с
// условиями лицензии и бедные описания — название семейства весит больше названия позиции, точное совпадение
// с ним — как с артикулом; полный путь категории ищется с весом 1.
var b3Fields = []string{
	"title.exact^5", "title.ru^3", "title.ngram", "family_name^4", "family_name.exact^5", "aliases^2", "vendor^2",
	"sku^5", "category_path", "description", "attributes",
}

// substringQuery — B0: нормализованный запрос как подстрока названия (title.exact уже приведён тем же
// нормализатором). Реконструкция исходного поиска магазина: совпало целиком — нашлось, иначе пусто.
// Порядок при равном скоре — по популярности.
func substringQuery(normalized string, size int) map[string]any {
	return map[string]any{
		"size":    size,
		"_source": sourceExcludes,
		"query": map[string]any{"wildcard": map[string]any{"title.exact": map[string]any{
			"value": "*" + escapeWildcard(normalized) + "*", "case_insensitive": true,
		}}},
		"sort": []any{"_score", map[string]any{"popularity": "desc"}, map[string]any{"product_id": "asc"}},
	}
}

// bm25Query — B1: multi_match по мультиполям. best_fields с tie_breaker: лучшее поле решает, остальные
// добавляют треть веса.
func bm25Query(normalized string, size int) map[string]any {
	return map[string]any{
		"size":    size,
		"_source": sourceExcludes,
		"query": map[string]any{"multi_match": map[string]any{
			"query": engineText(normalized), "fields": bm25Fields, "type": "best_fields", "tie_breaker": 0.3,
		}},
		"sort": []any{"_score", map[string]any{"popularity": "desc"}, map[string]any{"product_id": "asc"}},
	}
}

// knnQuery — dense: k ближайших по вектору запроса.
// filters — обязательные условия B3 (продление, отечественное, вендор): фильтр внутри kNN, иначе k ближайших
// заполнят позиции, которые всё равно будут отсеяны.
func knnQuery(vector []float32, k int, filters ...any) map[string]any {
	knn := map[string]any{"vector": vector, "k": k}
	if len(filters) > 0 {
		knn["filter"] = map[string]any{"bool": map[string]any{"filter": filters}}
	}
	return map[string]any{
		"size":    k,
		"_source": sourceExcludes,
		"query":   map[string]any{"knn": map[string]any{"embedding": knn}},
	}
}

// escapeWildcard — экранирование спецсимволов wildcard: * ? и обратная косая.
func escapeWildcard(s string) string {
	return strings.NewReplacer(`\`, `\\`, `*`, `\*`, `?`, `\?`).Replace(s)
}

// b3MinMatch — B3: из 3+ слов запроса может не совпасть треть, 1–2 слова обязательны все. Иначе «pro» в
// «windows 11 pro» притягивает Acrobat Pro и Zoom Pro.
const b3MinMatch = "2<-34%"

// bm25QueryB3 — B3: multi_match с minimum_should_match по тексту запроса; исходные слова алиасов — отдельной
// веткой should (найти «Р7-Офис» по «офис», когда алиас заменил слово на office).
func bm25QueryB3(text, expansions, minMatch string, size int, filters []any) map[string]any {
	text, expansions = engineText(text), engineText(expansions)
	should := []any{map[string]any{"multi_match": map[string]any{
		"query": text, "fields": b3Fields, "type": "best_fields", "tie_breaker": 0.3,
		"minimum_should_match": minMatch,
	}}}
	if expansions != "" {
		should = append(should, map[string]any{"multi_match": map[string]any{
			"query": expansions, "fields": b3Fields, "type": "best_fields", "tie_breaker": 0.3,
			"boost": 0.5,
		}})
	}
	return map[string]any{
		"size":    size,
		"_source": sourceExcludes,
		"query": map[string]any{"bool": map[string]any{"should": should, "minimum_should_match": 1,
			"filter": append([]any{}, filters...)}},
		"sort": []any{"_score", map[string]any{"popularity": "desc"}, map[string]any{"product_id": "asc"}},
	}
}

// engineMaxUnits — сколько слов запроса уходит в текстовый запрос движка. Каждое слово разворачивается в предложение
// по каждому из полей b3Fields (и в n-граммы title.ngram); у OpenSearch потолок — 1024 предложения
// (too_many_nested_clauses → 502). 32 слова — с запасом больше любого осмысленного запроса покупателя.
const engineMaxUnits = 32

// engineText — текст запроса для движка: эмодзи, символы и невидимые знаки форматирования (So, Sk, Co, Cs, Cf,
// селекторы вариантов) — пробелом: смысла для каталога в них нет, а стандартный токенизатор делает из каждого
// отдельное слово («😀» ×150 — 150 слов). Затем не больше engineMaxUnits слов; иероглифы, кана, хангыль и тайское
// письмо токенизатор режет по знаку — там слово считается по числу таких знаков.
func engineText(s string) string {
	if s == "" {
		return s
	}
	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 0xFE00 && r <= 0xFE0F, r == 0x20E3, unicode.IsControl(r),
			unicode.In(r, unicode.So, unicode.Sk, unicode.Co, unicode.Cs, unicode.Cf):
			return ' '
		}
		return r
	}, s)
	words := strings.Fields(clean)
	budget := engineMaxUnits
	out := make([]string, 0, len(words))
	for _, w := range words {
		if budget <= 0 {
			break
		}
		cost, cut := 1, len(w)
		n := 0
		for i, r := range w {
			if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul, unicode.Thai, unicode.Lao,
				unicode.Khmer, unicode.Myanmar) {
				n++
				if n > budget {
					cut = i
					break
				}
			}
		}
		if n > 0 {
			cost = min(n, budget)
		}
		out = append(out, w[:cut])
		budget -= cost
	}
	return strings.Join(out, " ")
}
