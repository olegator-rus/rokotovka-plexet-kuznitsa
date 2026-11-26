package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"kuznica/api/out"
	"kuznica/api/services"

	"github.com/gofiber/fiber/v2"
)

// testOrigin — PUBLIC_ORIGIN тестового приложения.
const testOrigin = "http://localhost:5177"

// fixedNow — часы тестов.
var fixedNow = time.Date(2026, 5, 27, 9, 30, 0, 0, time.UTC)

// newTestApp — настоящая таблица маршрутов без базы.
func newTestApp() *fiber.App {
	svc := services.NewServices(nil, services.Options{Now: func() time.Time { return fixedNow }})
	app := fiber.New(ServerConfig(DefaultTimeouts))
	app.Use(RequestID)
	RegisterRoutes(app, NewHandlerManager(svc, Options{PublicOrigin: testOrigin}))
	return app
}

// Без базы, OpenSearch и эмбеддера health отвечает 200 и честно показывает их состояние.
func TestHealth(t *testing.T) {
	app := newTestApp()
	for _, path := range []string{"/health", "/api/health"} {
		resp, err := app.Test(httptest.NewRequest("GET", path, nil))
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("%s: status = %d", path, resp.StatusCode)
		}
		if got := resp.Header.Get("Cache-Control"); got != "no-store" {
			t.Fatalf("%s: Cache-Control = %q", path, got)
		}
		var body out.Health
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Status != "ok" || body.DB != "down" || body.Search != "down" || body.Embedder != "off" || body.Time != "2026-05-27T09:30:00Z" {
			t.Fatalf("%s: body = %+v", path, body)
		}
	}
}

// Неизвестный маршрут — 404 в едином конверте ошибок, без сырого текста роутера и с traceId.
func TestUnknownRouteIsEnvelope404(t *testing.T) {
	resp, err := newTestApp().Test(httptest.NewRequest("GET", "/api/nope", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body out.ErrorBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "NOT_FOUND" || body.Error.TraceID == "" || body.Error.TraceID != resp.Header.Get("X-Request-Id") {
		t.Fatalf("конверт = %+v, X-Request-Id = %q", body.Error, resp.Header.Get("X-Request-Id"))
	}
}

// Мутация без своего Origin отклоняется до маршрутизации; со своим — доходит до роутера.
func TestOriginGuardsMutations(t *testing.T) {
	app := newTestApp()
	for origin, want := range map[string]int{
		"":                     fiber.StatusForbidden,
		"null":                 fiber.StatusForbidden,
		"https://evil.example": fiber.StatusForbidden,
		testOrigin:             fiber.StatusNotFound,
	} {
		req := httptest.NewRequest("POST", "/api/anything", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != want {
			t.Fatalf("Origin %q: status = %d, ожидалось %d", origin, resp.StatusCode, want)
		}
	}
}
