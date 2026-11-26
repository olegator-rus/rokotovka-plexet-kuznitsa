// Package opensearchtest — поддельный OpenSearch в памяти для тестов: ровно те вызовы, что делает
// клиент externalApi/opensearch, без JVM и Docker. Поиск упрощён: wildcard — подстрока в title.exact,
// multi_match — число совпавших слов, knn — косинус. Порядок и формат ответов — как у настоящего.
package opensearchtest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"unicode"
)

// Server — поддельный кластер.
type Server struct {
	*httptest.Server
	mu      sync.Mutex
	indices map[string]*index
	aliases map[string]string // алиас → индекс
	// Requests — пути запросов по порядку (для проверок теста).
	Requests []string
	// SearchBodies — тела запросов _search по порядку.
	SearchBodies []map[string]any
}

type index struct {
	body map[string]any
	docs map[string]map[string]any
	ids  []string
}

// New — запущенный поддельный кластер; закрывается через t.Cleanup вызывающего (srv.Close).
func New() *Server {
	s := &Server{indices: map[string]*index{}, aliases: map[string]string{}}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	return s
}

// Docs — документы индекса или алиаса по id.
func (s *Server) Docs(name string) map[string]map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ix := s.resolve(name); ix != nil {
		return ix.docs
	}
	return nil
}

// Alias — индекс, на который указывает алиас.
func (s *Server) Alias(alias string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.aliases[alias]
}

// IndexNames — существующие индексы.
func (s *Server) IndexNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for n := range s.indices {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func (s *Server) resolve(name string) *index {
	if real, ok := s.aliases[name]; ok {
		name = real
	}
	return s.indices[name]
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func notFound(w http.ResponseWriter) {
	writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]any{"type": "index_not_found_exception"}, "status": 404})
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Requests = append(s.Requests, r.Method+" "+r.URL.Path)
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case r.URL.Path == "/_cluster/health":
		writeJSON(w, 200, map[string]any{"status": "green"})
	case parts[0] == "_cat" && len(parts) == 3:
		prefix := strings.TrimSuffix(parts[2], "*")
		out := []map[string]string{}
		for n := range s.indices {
			if strings.HasPrefix(n, prefix) {
				out = append(out, map[string]string{"index": n})
			}
		}
		writeJSON(w, 200, out)
	case parts[0] == "_alias" && len(parts) == 2:
		real, ok := s.aliases[parts[1]]
		if !ok {
			notFound(w)
			return
		}
		writeJSON(w, 200, map[string]any{real: map[string]any{"aliases": map[string]any{parts[1]: map[string]any{}}}})
	case r.URL.Path == "/_aliases":
		var body struct {
			Actions []map[string]map[string]string `json:"actions"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		for _, a := range body.Actions {
			if add, ok := a["add"]; ok {
				s.aliases[add["alias"]] = add["index"]
			}
		}
		writeJSON(w, 200, map[string]any{"acknowledged": true})
	case r.URL.Path == "/_bulk":
		s.bulk(w, r)
	case len(parts) == 1 && r.Method == http.MethodPut:
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.indices[parts[0]] = &index{body: body, docs: map[string]map[string]any{}}
		writeJSON(w, 200, map[string]any{"acknowledged": true, "index": parts[0]})
	case len(parts) == 1 && r.Method == http.MethodDelete:
		if s.indices[parts[0]] == nil {
			notFound(w)
			return
		}
		delete(s.indices, parts[0])
		writeJSON(w, 200, map[string]any{"acknowledged": true})
	case len(parts) == 2:
		ix := s.resolve(parts[0])
		if ix == nil {
			notFound(w)
			return
		}
		switch parts[1] {
		case "_refresh":
			writeJSON(w, 200, map[string]any{})
		case "_count":
			writeJSON(w, 200, map[string]any{"count": len(ix.docs)})
		case "_mapping":
			name := parts[0]
			if real, ok := s.aliases[name]; ok {
				name = real
			}
			writeJSON(w, 200, map[string]any{name: map[string]any{"mappings": ix.body["mappings"]}})
		case "_search":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			s.SearchBodies = append(s.SearchBodies, body)
			writeJSON(w, 200, ix.search(body))
		default:
			notFound(w)
		}
	default:
		notFound(w)
	}
}

func (s *Server) bulk(w http.ResponseWriter, r *http.Request) {
	sc := bufio.NewScanner(r.Body)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	var items []map[string]any
	for sc.Scan() {
		var action map[string]map[string]string
		if json.Unmarshal(sc.Bytes(), &action) != nil || !sc.Scan() {
			break
		}
		var doc map[string]any
		_ = json.Unmarshal(sc.Bytes(), &doc)
		meta := action["index"]
		ix := s.indices[meta["_index"]]
		if ix == nil {
			items = append(items, map[string]any{"index": map[string]any{"_id": meta["_id"], "status": 404,
				"error": map[string]any{"type": "index_not_found_exception", "reason": "нет индекса"}}})
			continue
		}
		if _, ok := ix.docs[meta["_id"]]; !ok {
			ix.ids = append(ix.ids, meta["_id"])
		}
		ix.docs[meta["_id"]] = doc
		items = append(items, map[string]any{"index": map[string]any{"_id": meta["_id"], "status": 201}})
	}
	errorsFound := false
	for _, it := range items {
		if it["index"].(map[string]any)["status"].(int) >= 300 {
			errorsFound = true
		}
	}
	writeJSON(w, 200, map[string]any{"errors": errorsFound, "items": items})
}

type scored struct {
	id    string
	score float64
}

// search — упрощённый поиск по трём видам запросов, которые строит services.
func (ix *index) search(body map[string]any) map[string]any {
	size := 10
	if v, ok := body["size"].(float64); ok {
		size = int(v)
	}
	q, _ := body["query"].(map[string]any)
	var hits []scored
	switch {
	case q["wildcard"] != nil:
		spec := q["wildcard"].(map[string]any)["title.exact"].(map[string]any)
		needle := strings.Trim(strings.ReplaceAll(spec["value"].(string), `\`, ""), "*")
		for _, id := range ix.ids {
			if strings.Contains(fold(str(ix.docs[id]["title"])), fold(needle)) {
				hits = append(hits, scored{id, 1})
			}
		}
	case q["multi_match"] != nil:
		words := strings.Fields(fold(q["multi_match"].(map[string]any)["query"].(string)))
		for _, id := range ix.ids {
			d := ix.docs[id]
			text := " " + fold(strings.Join([]string{str(d["title"]), str(d["family_name"]), str(d["vendor"]),
				str(d["sku"]), str(d["description"]), str(d["attributes"]), strings.Join(strs(d["aliases"]), " ")}, " ")) + " "
			n := 0.0
			for _, w := range words {
				if strings.Contains(text, w) {
					n++
				}
			}
			if n > 0 {
				hits = append(hits, scored{id, n})
			}
		}
	case q["knn"] != nil:
		spec := q["knn"].(map[string]any)["embedding"].(map[string]any)
		qv := floats(spec["vector"])
		for _, id := range ix.ids {
			if dv := floats(ix.docs[id]["embedding"]); len(dv) == len(qv) && len(qv) > 0 {
				hits = append(hits, scored{id, (1 + cosine(qv, dv)) / 2})
			}
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		pi, _ := ix.docs[hits[i].id]["popularity"].(float64)
		pj, _ := ix.docs[hits[j].id]["popularity"].(float64)
		return pi > pj
	})
	if len(hits) > size {
		hits = hits[:size]
	}
	out := make([]map[string]any, 0, len(hits))
	for _, h := range hits {
		src := map[string]any{}
		for k, v := range ix.docs[h.id] {
			if k != "embedding" {
				src[k] = v
			}
		}
		out = append(out, map[string]any{"_id": h.id, "_score": h.score, "_source": src})
	}
	return map[string]any{"took": 1, "hits": map[string]any{"hits": out}}
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func strs(v any) []string {
	arr, _ := v.([]any)
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		out = append(out, str(x))
	}
	return out
}

func floats(v any) []float64 {
	arr, _ := v.([]any)
	out := make([]float64, 0, len(arr))
	for _, x := range arr {
		f, _ := x.(float64)
		out = append(out, f)
	}
	return out
}

func cosine(a, b []float64) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / math.Sqrt(na*nb)
}

// fold — нижний регистр и ё → е, как нормализатор индекса.
func fold(s string) string {
	var b bytes.Buffer
	for _, r := range s {
		r = unicode.ToLower(r)
		if r == 'ё' {
			r = 'е'
		}
		b.WriteRune(r)
	}
	return b.String()
}
