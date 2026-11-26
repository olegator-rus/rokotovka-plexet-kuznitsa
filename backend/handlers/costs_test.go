package handlers

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"kuznica/api/services"

	"github.com/gofiber/fiber/v2"
)

// GET /api/costs: неверные параметры — 400 с полем; без базы — 503 (журнал стоимости негде взять).
func TestCostsEndpoint(t *testing.T) {
	app := fiber.New(ServerConfig(DefaultTimeouts))
	app.Use(RequestID)
	RegisterRoutes(app, NewHandlerManager(services.NewServices(nil, services.Options{}), Options{PublicOrigin: testOrigin}))
	for path, want := range map[string]int{
		"/api/costs?limit=abc":  fiber.StatusBadRequest,
		"/api/costs?limit=1000": fiber.StatusBadRequest,
		"/api/costs?offset=-1":  fiber.StatusBadRequest,
		"/api/costs":            fiber.StatusServiceUnavailable,
	} {
		resp, err := app.Test(httptest.NewRequest("GET", path, nil), -1)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != want && !(want == fiber.StatusBadRequest && resp.StatusCode == fiber.StatusUnprocessableEntity) {
			t.Errorf("%s: %d %s", path, resp.StatusCode, raw)
		}
		if !strings.Contains(string(raw), `"error"`) {
			t.Errorf("%s: нет конверта ошибки: %s", path, raw)
		}
	}
}

// GET /api/costs/summary — открыт гостю (сводка «Доказательства → Стоимость» без журнала вызовов), а сам журнал
// GET /api/costs по-прежнему только сотруднику. Без базы сводке негде взять цифры — 503, но не 401.
func TestCostsSummaryOpenToGuest(t *testing.T) {
	app := authApp(t, demoAuth, "bo-token-0123456789abcdef0123")
	if r := authDo(t, app, "GET", "/api/costs/summary", "", ""); r.code == fiber.StatusUnauthorized ||
		r.code == fiber.StatusForbidden || r.code != fiber.StatusServiceUnavailable {
		t.Errorf("сводка гостю: %d %s", r.code, r.body)
	}
	if r := authDo(t, app, "GET", "/api/costs/summary?server_rub_20k=abc", "", ""); r.code != fiber.StatusBadRequest &&
		r.code != fiber.StatusUnprocessableEntity {
		t.Errorf("неверный параметр сводки: %d %s", r.code, r.body)
	}
	if r := authDo(t, app, "GET", "/api/costs", "", ""); r.code != fiber.StatusUnauthorized {
		t.Errorf("журнал гостю: %d", r.code)
	}
}
