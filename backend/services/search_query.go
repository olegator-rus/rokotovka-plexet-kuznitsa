package services

import "strings"

// Запросы к OpenSearch. Строит их только код: ни запрос покупателя, ни ответ модели в тело запроса не
// попадают иначе как значением поля.

// candidatesTop — сколько кандидатов берёт каждый источник (BM25 и kNN), ARCHITECTURE §2.4.
const candidatesTop = 50

// sourceExcludes — вектор в выдаче не нужен: он большой, а позиция ответа собирается из остальных полей.
var sourceExcludes = map[string]any{"excludes": []string{"embedding"}}

// bm25Fields — поля multi_match B1 с весами.
var bm25Fields = []string{
	"title.exact^5", "title.ru^3", "title.ngram", "family_name^3", "aliases^2", "vendor^2", "sku^5",
	"description", "attributes",
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
			"query": normalized, "fields": bm25Fields, "type": "best_fields", "tie_breaker": 0.3,
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
	should := []any{map[string]any{"multi_match": map[string]any{
		"query": text, "fields": bm25Fields, "type": "best_fields", "tie_breaker": 0.3,
		"minimum_should_match": minMatch,
	}}}
	if expansions != "" {
		should = append(should, map[string]any{"multi_match": map[string]any{
			"query": expansions, "fields": bm25Fields, "type": "best_fields", "tie_breaker": 0.3,
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
