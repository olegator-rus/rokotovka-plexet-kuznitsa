package tei

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"kuznica/api/pkg/errs"
)

func TestEmbedChecksContract(t *testing.T) {
	var got map[string]any
	reply := `[[0.6,0.8],[1,0]]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(reply))
	}))
	defer srv.Close()
	c := New(srv.URL, time.Second)
	vecs, err := c.Embed(t.Context(), []string{"a", "b"}, 2)
	if err != nil || len(vecs) != 2 || vecs[0][1] != 0.8 || got["normalize"] != true {
		t.Fatalf("embed: %v %v %v", vecs, err, got)
	}
	if _, err := c.Embed(t.Context(), []string{"a", "b"}, 1024); errs.FromExternal(err) == nil ||
		errs.FromExternal(err).HTTPStatus() != http.StatusBadGateway {
		t.Fatalf("размерность: %v", err)
	}
	reply = `[[0.6,0.8]]`
	if _, err := c.Embed(t.Context(), []string{"a", "b"}, 2); err == nil {
		t.Fatal("векторов меньше текстов — ошибка")
	}
	reply = `[{"index":5,"score":0.9}]`
	if _, err := c.Rerank(t.Context(), "q", []string{"a"}); err == nil {
		t.Fatal("индекс вне списка — ошибка")
	}
	reply = `[{"index":0,"score":0.9}]`
	if hits, err := c.Rerank(t.Context(), "q", []string{"a"}); err != nil || hits[0].Score != 0.9 {
		t.Fatalf("rerank: %v %v", hits, err)
	}
	var off *Client
	if off.Enabled() || New("", time.Second) != nil {
		t.Fatal("пустой адрес — выключенный клиент")
	}
}
