package services

import (
	"encoding/json"
	"strings"
	"testing"
)

// Тело B1 собирается без живого OpenSearch: поля с весами, нормализованный запрос, 50 кандидатов, без вектора.
func TestBM25Query(t *testing.T) {
	raw, err := json.Marshal(bm25Query("kaspersky на 10 пк", 50))
	if err != nil {
		t.Fatal(err)
	}
	var q struct {
		Size   int `json:"size"`
		Source struct {
			Excludes []string `json:"excludes"`
		} `json:"_source"`
		Query struct {
			MultiMatch struct {
				Query      string   `json:"query"`
				Fields     []string `json:"fields"`
				Type       string   `json:"type"`
				TieBreaker float64  `json:"tie_breaker"`
			} `json:"multi_match"`
		} `json:"query"`
	}
	if err := json.Unmarshal(raw, &q); err != nil {
		t.Fatal(err)
	}
	mm := q.Query.MultiMatch
	want := "title.exact^5 title.ru^3 title.ngram family_name^3 aliases^2 vendor^2 sku^5 description attributes"
	if q.Size != 50 || mm.Query != "kaspersky на 10 пк" || strings.Join(mm.Fields, " ") != want || mm.Type != "best_fields" {
		t.Fatalf("запрос B1: %s", raw)
	}
	if len(q.Source.Excludes) != 1 || q.Source.Excludes[0] != "embedding" {
		t.Fatalf("вектор не должен возвращаться: %s", raw)
	}
}

// B0: подстрока в title.exact; спецсимволы wildcard экранируются — запрос «*» не находит всё подряд.
func TestSubstringQuery(t *testing.T) {
	raw, _ := json.Marshal(substringQuery(`win*10? \x`, 50))
	if !strings.Contains(string(raw), `"title.exact":{"case_insensitive":true,"value":"*win\\*10\\? \\\\x*"}`) {
		t.Fatalf("wildcard: %s", raw)
	}
}

func TestKNNQuery(t *testing.T) {
	raw, _ := json.Marshal(knnQuery([]float32{0.5, 0.25}, 50))
	if !strings.Contains(string(raw), `"knn":{"embedding":{"k":50,"vector":[0.5,0.25]}}`) || !strings.Contains(string(raw), `"size":50`) {
		t.Fatalf("knn: %s", raw)
	}
}
