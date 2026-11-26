//go:build integration

package services

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"kuznica/api/externalApi/opensearch"
)

// Парт-номера на живом OpenSearch: как режут парт-номер прежний анализатор
// plain и новые поля mpn_normalized, и находит ли позицию ввод с дефисами, пробелами, слэшами, по началу и по куску.
//
//	OPENSEARCH_URL=http://127.0.0.1:9201 go test -tags integration ./services/ -run LiveMPN -v
func TestLiveMPNAnalyzer(t *testing.T) {
	url := os.Getenv("OPENSEARCH_URL")
	if url == "" {
		t.Skip("OPENSEARCH_URL не задан")
	}
	const alias = "itest_mpn"
	osc := opensearch.New(url, 30*time.Second)
	svc := NewServices(nil, Options{Search: osc, Source: fixtureSource(), IndexAlias: alias})
	t.Cleanup(func() {
		names, _ := osc.Indices(context.Background(), alias+"_v*")
		for _, n := range names {
			_ = osc.DeleteIndex(context.Background(), n)
		}
	})
	res, err := svc.ImportCatalog(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	index := res.Index

	analyze := func(body map[string]any) []string {
		t.Helper()
		raw, _ := json.Marshal(body)
		resp, err := http.Post(url+"/"+index+"/_analyze", "application/json", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out struct {
			Tokens []struct {
				Token string `json:"token"`
			} `json:"tokens"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("_analyze %s: %d %v", raw, resp.StatusCode, err)
		}
		toks := make([]string, 0, len(out.Tokens))
		for _, tk := range out.Tokens {
			toks = append(toks, tk.Token)
		}
		return toks
	}
	for _, mpn := range []string{"SNS-R740-32G", "MZ-7L3960HCJR-00A07", "P00924-B21", "J9775A#ABB", "0XF8K", "PER740-BASE-01"} {
		plain := analyze(map[string]any{"analyzer": "plain", "text": mpn})
		norm := analyze(map[string]any{"field": "mpn_normalized", "text": mpn})
		prefix := analyze(map[string]any{"analyzer": "mpn_prefix", "text": mpn})
		tri := analyze(map[string]any{"analyzer": "mpn_tri", "text": mpn})
		t.Logf("%-20s plain=%v | normalized=%v | prefix=%d (%s…%s) | tri=%d (%s…)", mpn, plain, norm, len(prefix),
			first(prefix), last(prefix), len(tri), strings.Join(tri[:min(3, len(tri))], ","))
		if len(norm) != 1 || norm[0] != normalizeMPN(mpn) {
			t.Errorf("%s: нормализатор %v, в Go %q", mpn, norm, normalizeMPN(mpn))
		}
	}

	hit := func(q map[string]any) string {
		t.Helper()
		r, err := osc.Search(t.Context(), alias, map[string]any{"query": q, "size": 1})
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Hits.Hits) == 0 {
			return ""
		}
		return r.Hits.Hits[0].ID
	}
	for name, q := range map[string]map[string]any{
		"term с пробелом и слэшем": {"term": map[string]any{"mpn_normalized": "sns r740/32g"}},
		"term без дефисов":         {"term": map[string]any{"mpn_normalized": "SNSR74032G"}},
		"начало":                   {"match": map[string]any{"mpn_normalized.prefix": "sns-r74"}},
		"кусок из середины":        {"match": map[string]any{"mpn_normalized.tri": map[string]any{"query": "r740-32", "minimum_should_match": "80%"}}},
	} {
		if id := hit(q); id != "ram-32g-rdimm" {
			t.Errorf("%s: первая позиция %q", name, id)
		}
	}
	if id := hit(map[string]any{"range": map[string]any{"specs.ram_gb": map[string]any{"gte": 32}}}); id == "" {
		t.Error("specs.ram_gb ≥ 32: ничего")
	}
	if id := hit(map[string]any{"term": map[string]any{"compatible_with": "dell-poweredge-r740"}}); id != "ram-32g-rdimm" {
		t.Errorf("compatible_with: %q", id)
	}
}

func first(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	return xs[0]
}

func last(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	return xs[len(xs)-1]
}
