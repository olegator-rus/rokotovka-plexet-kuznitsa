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
