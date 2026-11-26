package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"kuznica/api/out"

	"github.com/gofiber/fiber/v2"
)

// Без базы и данных аналитика и оценка отвечают пустыми структурами, а не ошибкой.
func TestAnalyticsAndEvaluationsEmpty(t *testing.T) {
	app := newTestApp()
	resp, err := app.Test(httptest.NewRequest("GET", "/api/analytics?days=30&role=ecom", nil))
	if err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("analytics: %v %v", err, resp.StatusCode)
	}
	var a out.AnalyticsResponse
	if err := json.NewDecoder(resp.Body).Decode(&a); err != nil || a.Days != 30 || a.Funnel == nil || a.Problems == nil {
		t.Fatalf("analytics: %v %+v", err, a)
	}
	resp, _ = app.Test(httptest.NewRequest("GET", "/api/analytics?days=abc", nil))
	if resp.StatusCode != fiber.StatusUnprocessableEntity {
		t.Fatalf("days=abc: %d", resp.StatusCode)
	}

	resp, err = app.Test(httptest.NewRequest("GET", "/api/evaluations", nil))
	if err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("evaluations: %v %v", err, resp.StatusCode)
	}
	var e out.EvaluationsResponse
	if err := json.NewDecoder(resp.Body).Decode(&e); err != nil || e.Set != "test" || e.Configs == nil {
		t.Fatalf("evaluations: %v %+v", err, e)
	}

	// Без источника наборов прогон недоступен — 503.
	req := httptest.NewRequest("POST", "/api/evaluations/run", strings.NewReader(`{"set":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", testOrigin)
	resp, _ = app.Test(req)
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("run: %d", resp.StatusCode)
	}
}
