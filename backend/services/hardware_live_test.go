//go:build integration

package services

import (
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"kuznica/api/domain"
	"kuznica/api/externalApi/catalogsource"
	"kuznica/api/externalApi/opensearch"
	"kuznica/api/externalApi/tei"
	"kuznica/api/in"
)

// Живой прогон набора p2 по типам оборудования на дев-индексе (только чтение): первое место, оценка, чипы.
//
//	OPENSEARCH_URL=http://127.0.0.1:9201 EMBEDDER_URL=http://127.0.0.1:8089 \
//	  go test -tags integration ./services/ -run LiveHardwareP2 -v
func TestLiveHardwareP2(t *testing.T) {
	url := os.Getenv("OPENSEARCH_URL")
	if url == "" {
		t.Skip("OPENSEARCH_URL не задан")
	}
	alias := os.Getenv("INDEX_ALIAS")
	if alias == "" {
		alias = "products"
	}
	src := catalogsource.MockJSON{Dir: "../../data/mock"}
	snap, err := src.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{Search: opensearch.New(url, 30*time.Second), Source: src, IndexAlias: alias,
		Features: Features{Hardware: os.Getenv("HW") != "off"}}
	if e := os.Getenv("EMBEDDER_URL"); e != "" {
		opts.Embedder = tei.New(e, 30*time.Second)
	}
	svc := NewServices(nil, opts)
	svc.catalog.Store(newCatalogState(snap, 1))
	qs, err := src.LoadQueries(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	types := map[string][2]int{}
	only := os.Getenv("TYPES")
	for _, q := range qs {
		if q.Set != domain.EvalSetP2 || only != "" && !strings.Contains(only, q.Type) {
			continue
		}
		resp, err := svc.SearchWith(t.Context(), "live-hw", in.SearchRequest{Query: q.Text, Config: domain.ConfigB3, Limit: 10},
			SearchOptions{NoLog: true})
		if err != nil {
			t.Errorf("%q: %v", q.Text, err)
			continue
		}
		top, grade := "—", 0
		if len(resp.Items) > 0 {
			top = resp.Items[0].ProductID
			grade = q.Judgments[top]
		}
		var chips []string
		for _, c := range resp.Chips {
			chips = append(chips, c.Label)
		}
		var ids []string
		for i, it := range resp.Items {
			if i == 4 {
				break
			}
			ids = append(ids, it.ProductID)
		}
		best := 0
		for _, g := range q.Judgments {
			best = max(best, g)
		}
		s := types[q.Type]
		s[1]++
		if best == 3 && grade == 3 || best < 3 && (best == 0 && len(resp.Items) == 0 || best > 0 && grade == best) {
			s[0]++
		}
		types[q.Type] = s
		t.Logf("%-11s g%d/%d %-44q route=%s top=%v chips=%v", q.Type, grade, best, q.Text, resp.Trace.Route, ids, chips)
	}
	var ks []string
	for k := range types {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	for _, k := range ks {
		t.Logf("%s: лучшая оценка на первом месте %d/%d", k, types[k][0], types[k][1])
	}
}
