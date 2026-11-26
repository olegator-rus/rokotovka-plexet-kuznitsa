package opensearch

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"kuznica/api/pkg/errs"
)

func server(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New(srv.URL, time.Second)
}

// Сбои превращаются в статусы errs: недоступен — 503, таймаут — 504, 5xx — 502, 404 — IsNotFound.
func TestErrorMapping(t *testing.T) {
	down := New("http://127.0.0.1:1", time.Second)
	if e := errs.FromExternal(down.Ping(t.Context())); e == nil || e.HTTPStatus() != http.StatusServiceUnavailable {
		t.Fatalf("недоступен: %v", e)
	}
	slow := server(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})
	slow.http.Timeout = 50 * time.Millisecond
	if e := errs.FromExternal(slow.Ping(t.Context())); e == nil || e.HTTPStatus() != http.StatusGatewayTimeout {
		t.Fatalf("таймаут: %v", e)
	}
	broken := server(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
	if e := errs.FromExternal(broken.Ping(t.Context())); e == nil || e.HTTPStatus() != http.StatusBadGateway {
		t.Fatalf("500: %v", e)
	}
	missing := server(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	if err := missing.Refresh(t.Context(), "x"); !IsNotFound(err) {
		t.Fatalf("404: %v", err)
	}
	if names, err := missing.AliasIndices(t.Context(), "products"); err != nil || names != nil {
		t.Fatalf("алиаса нет — пусто без ошибки: %v %v", names, err)
	}
	var nilClient *Client
	if nilClient.Enabled() || !errors.Is(nilClient.Ping(t.Context()), ErrDisabled) {
		t.Fatal("выключенный клиент")
	}
}

// Отказ поиска цитирует запрос — в ошибку попадает только тип.
func TestSearchErrorHidesQuery(t *testing.T) {
	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":{"type":"query_shard_exception","reason":"failed to create query: касперский"}}`))
	})
	_, err := c.Search(t.Context(), "products", map[string]any{})
	if err == nil || strings.Contains(err.Error(), "касперский") || !strings.Contains(err.Error(), "query_shard_exception") {
		t.Fatalf("ошибка: %v", err)
	}
}

// Отклонённый документ bulk — ошибка с причиной.
func TestBulkItemErrors(t *testing.T) {
	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/x-ndjson" {
			t.Errorf("Content-Type %q", r.Header.Get("Content-Type"))
		}
		_, _ = w.Write([]byte(`{"errors":true,"items":[{"index":{"_id":"a","status":201}},{"index":{"_id":"b","status":400,"error":{"type":"mapper_parsing_exception","reason":"bad field"}}}]}`))
	})
	err := c.Bulk(t.Context(), []byte("{}\n{}\n"))
	if err == nil || !strings.Contains(err.Error(), "b: mapper_parsing_exception: bad field") {
		t.Fatalf("bulk: %v", err)
	}
}
