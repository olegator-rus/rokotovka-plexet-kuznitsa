package handlers

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"kuznica/api/externalApi/catalogsource"
	"kuznica/api/externalApi/opensearch"
	"kuznica/api/externalApi/opensearch/opensearchtest"
	"kuznica/api/out"
	"kuznica/api/services"

	"github.com/gofiber/fiber/v2"
)

// newSearchApp — таблица маршрутов на поддельном OpenSearch с импортированной фикстурой, без базы.
func newSearchApp(t *testing.T) *fiber.App {
	t.Helper()
	fake := opensearchtest.New()
	t.Cleanup(fake.Close)
	svc := services.NewServices(nil, services.Options{
		Now: func() time.Time { return fixedNow }, Search: opensearch.New(fake.URL, 0),
		Source: catalogsource.MockJSON{Dir: "../testdata/catalog"},
	})
	app := fiber.New(ServerConfig(DefaultTimeouts))
	app.Use(RequestID)
	RegisterRoutes(app, NewHandlerManager(svc, Options{PublicOrigin: testOrigin}))
	resp := post(t, app, "/api/catalog/import", "")
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("импорт: %d", resp.StatusCode)
	}
	return app
}

func post(t *testing.T, app *fiber.App, path, body string) *httpResponse {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Origin", testOrigin)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	return &httpResponse{StatusCode: resp.StatusCode, Body: raw, RequestID: resp.Header.Get("X-Request-Id")}
}

type httpResponse struct {
	StatusCode int
	Body       []byte
	RequestID  string
}

// Поиск: 200, request_id ответа и трассы = X-Request-Id, трасса из десяти шагов.
func TestSearchEndpoint(t *testing.T) {
	app := newSearchApp(t)
	resp := post(t, app, "/api/search", `{"query":"Kaspersky","config":"B1","client_id":"client-a","session_id":"s-1"}`)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status %d: %s", resp.StatusCode, resp.Body)
	}
	var body out.SearchResponse
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body.RequestID == "" || body.RequestID != resp.RequestID || body.Trace.RequestID != resp.RequestID ||
		len(body.Items) != 2 || len(body.Trace.Steps) != 10 || body.Normalized != "kaspersky" {
		t.Fatalf("ответ: %s", resp.Body)
	}
}

// Ошибки поиска — в едином конверте: не JSON и пустой запрос — 422, без OpenSearch — 503.
func TestSearchEndpointErrors(t *testing.T) {
	app := newSearchApp(t)
	for body, want := range map[string]int{
		`не json`:                     fiber.StatusUnprocessableEntity,
		`{"query":""}`:                fiber.StatusUnprocessableEntity,
		`{"query":"x","config":"Z"}`:  fiber.StatusUnprocessableEntity,
		`{"query":"x"} {"query":"y"}`: fiber.StatusUnprocessableEntity,
	} {
		resp := post(t, app, "/api/search", body)
		var env out.ErrorBody
		if resp.StatusCode != want || json.Unmarshal(resp.Body, &env) != nil || env.Error.TraceID != resp.RequestID {
			t.Fatalf("%s → %d %s", body, resp.StatusCode, resp.Body)
		}
	}
	resp := post(t, newTestApp(), "/api/search", `{"query":"x"}`)
	if resp.StatusCode != fiber.StatusServiceUnavailable || !strings.Contains(string(resp.Body), "OPENSEARCH_URL") {
		t.Fatalf("без OpenSearch: %d %s", resp.StatusCode, resp.Body)
	}
}

func TestMetaEndpoint(t *testing.T) {
	app := newSearchApp(t)
	resp, err := app.Test(httptest.NewRequest("GET", "/api/meta", nil))
	if err != nil {
		t.Fatal(err)
	}
	var m out.Meta
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || m.CatalogVersion != "fixture-1" || len(m.Clients) != 3 || m.DefaultConfig != "live" || m.LLMMode != "off" {
		t.Fatalf("meta: %d %+v", resp.StatusCode, m)
	}
}

// События: неизвестный тип — 422 с деталями; верное событие без базы — 503.
func TestEventsEndpoint(t *testing.T) {
	app := newTestApp()
	resp := post(t, app, "/api/events", `{"type":"teleport"}`)
	var env out.ErrorBody
	if resp.StatusCode != fiber.StatusUnprocessableEntity || json.Unmarshal(resp.Body, &env) != nil ||
		len(env.Error.Details) != 1 || env.Error.Details[0].Field != "type" {
		t.Fatalf("неизвестный тип: %d %s", resp.StatusCode, resp.Body)
	}
	resp = post(t, app, "/api/events", `{"type":"click","request_id":"r","product_id":"kis-2dev-1y","position":0}`)
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("без базы: %d %s", resp.StatusCode, resp.Body)
	}
}

// Импорт без OpenSearch — 503 с причиной.
func TestImportEndpointWithoutSearch(t *testing.T) {
	resp := post(t, newTestApp(), "/api/catalog/import", "")
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("импорт без OpenSearch: %d %s", resp.StatusCode, resp.Body)
	}
}
