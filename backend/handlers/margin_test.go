package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"kuznica/api/out"

	"github.com/gofiber/fiber/v2"
)

// Политика маржи без базы: исходное положение читается, применение — 503, неверный вес — 422.
func TestMarginPolicyRoutes(t *testing.T) {
	app := newTestApp()
	resp, err := app.Test(httptest.NewRequest("GET", "/api/margin/policy", nil))
	if err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("policy: %v %v", err, resp.StatusCode)
	}
	var p out.MarginPolicy
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil || p.Weight != 0.05 || len(p.Positions) != 6 || p.Epsilon != 0.01 {
		t.Fatalf("policy: %v %+v", err, p)
	}
	post := func(path, body string) int {
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", testOrigin)
		resp, _ := app.Test(req)
		return resp.StatusCode
	}
	if c := post("/api/margin/policy", `{"weight":0.07}`); c != fiber.StatusUnprocessableEntity {
		t.Fatalf("вес не из положений: %d", c)
	}
	if c := post("/api/margin/policy", `{"weight":0.08}`); c != fiber.StatusServiceUnavailable {
		t.Fatalf("без базы: %d", c)
	}
	if c := post("/api/margin/policy/rollback", `{"to_version":0}`); c != fiber.StatusServiceUnavailable {
		t.Fatalf("откат без базы: %d", c)
	}
	resp, _ = app.Test(httptest.NewRequest("GET", "/api/margin/simulate/status", nil))
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}
