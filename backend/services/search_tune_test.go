//go:build tune

package services

import (
	"fmt"
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

// Подбор порога и весов на tune-наборе (не на test): живой индекс products, без записи в журнал.
//
//	cd backend && OPENSEARCH_URL=http://127.0.0.1:9201 EMBEDDER_URL=http://127.0.0.1:8089 \
//	  go test -tags tune ./services/ -run Tune -count=1 -v
//
// SET — набор (по умолчанию tune), SHOW=1 — построчно запросы B3 без релевантного в топ-10.
func TestTuneSet(t *testing.T) {
	url := os.Getenv("OPENSEARCH_URL")
	if url == "" {
		t.Skip("OPENSEARCH_URL не задан")
	}
	set := os.Getenv("SET")
	if set == "" {
		set = domain.EvalSetTune
	}
	src := catalogsource.MockJSON{Dir: "../../data/mock"}
	svc := NewServices(nil, Options{Search: opensearch.New(url, 30*time.Second),
		Embedder: tei.New(os.Getenv("EMBEDDER_URL"), 30*time.Second), Source: src})
	svc.loadCatalogState(t.Context())
	qs, err := src.LoadQueries(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	type stat struct{ n, empty, judged, hit, hit1 int }
	for _, cfg := range []string{"B0", "B1", "B2", "B3"} {
		all := stat{}
		byType := map[string]*stat{}
		var ms []float64
		var misses []string
		for _, q := range qs {
			if q.Set != set {
				continue
			}
			req := in.SearchRequest{Query: q.Text, Config: cfg}
			if q.ClientID != nil {
				req.ClientID = *q.ClientID
			}
			started := time.Now()
			resp, err := svc.SearchWith(t.Context(), "tune", req, SearchOptions{NoLog: true})
			if err != nil {
				t.Fatalf("%s %q: %v", cfg, q.Text, err)
			}
			ms = append(ms, float64(time.Since(started).Microseconds())/1000)
			st := byType[q.Type]
			if st == nil {
				st = &stat{}
				byType[q.Type] = st
			}
			for _, x := range []*stat{&all, st} {
				x.n++
				if len(resp.Items) == 0 {
					x.empty++
				}
			}
			if len(q.Judgments) == 0 {
				continue
			}
			hit, hit1 := false, false
			for i, it := range resp.Items {
				if i >= 10 {
					break
				}
				if q.Judgments[it.ProductID] >= 2 {
					hit = true
					hit1 = hit1 || i == 0
				}
				for _, v := range it.Variants {
					hit = hit || q.Judgments[v.ProductID] >= 2
				}
			}
			for _, x := range []*stat{&all, st} {
				x.judged++
				if hit {
					x.hit++
				}
				if hit1 {
					x.hit1++
				}
			}
			if !hit && cfg == "B3" {
				top := []string{}
				for i, it := range resp.Items {
					if i < 3 {
						top = append(top, it.ProductID)
					}
				}
				misses = append(misses, fmt.Sprintf("  %-10s %-45q norm=%q top=%v", q.Type, q.Text, resp.Normalized, top))
			}
		}
		sort.Float64s(ms)
		t.Logf("%s %s: запросов %d, пустых %d (%.1f%%), с разметкой %d, релевантный в топ-10 %d (%.1f%%), на 1-м месте %d; p50 %.1f мс, p95 %.1f мс",
			set, cfg, all.n, all.empty, pct(all.empty, all.n), all.judged, all.hit, pct(all.hit, all.judged), all.hit1,
			ms[len(ms)/2], ms[len(ms)*95/100])
		types := make([]string, 0, len(byType))
		for k := range byType {
			types = append(types, k)
		}
		sort.Strings(types)
		var b strings.Builder
		for _, k := range types {
			s := byType[k]
			fmt.Fprintf(&b, " %s %d/%d пуст, %d/%d хит;", k, s.empty, s.n, s.hit, s.judged)
		}
		t.Log(b.String())
		if os.Getenv("SHOW") != "" && cfg == "B3" {
			t.Log("B3 без релевантного в топ-10:\n" + strings.Join(misses, "\n"))
		}
	}
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return 100 * float64(a) / float64(b)
}
