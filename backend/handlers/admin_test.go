package handlers

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"kuznica/api/services"

	"github.com/gofiber/fiber/v2"
)

func adminApp(admin bool, rate int) *fiber.App {
	app := fiber.New(ServerConfig(DefaultTimeouts))
	app.Use(RequestID)
	RegisterRoutes(app, NewHandlerManager(services.NewServices(nil, services.Options{}),
		Options{PublicOrigin: testOrigin, AdminEnabled: admin, RatePerMinute: rate}))
	return app
}

func do(t *testing.T, app *fiber.App, method, path, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", testOrigin)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

// POST /api/admin/llm-mode: за флагом ADMIN_ENABLED; off включается всегда, on — только с моделью.
func TestAdminLLMMode(t *testing.T) {
	code, body := do(t, adminApp(false, 0), "POST", "/api/admin/llm-mode", `{"mode":"off"}`)
	if code != fiber.StatusForbidden || !strings.Contains(body, "ADMIN_ENABLED") {
		t.Fatalf("выключено: %d %s", code, body)
	}
	app := adminApp(true, 0)
	if code, body = do(t, app, "POST", "/api/admin/llm-mode", `{"mode":"off"}`); code != 200 || !strings.Contains(body, `"mode":"off"`) {
		t.Fatalf("off: %d %s", code, body)
	}
	if code, body = do(t, app, "POST", "/api/admin/llm-mode", `{"mode":"on"}`); code != fiber.StatusConflict {
		t.Fatalf("on без модели: %d %s", code, body)
	}
	if code, body = do(t, app, "GET", "/api/meta", ""); code != 200 || !strings.Contains(body, `"llm":{"mode":"off"`) ||
		!strings.Contains(body, `"banner":"Режим без LLM`) || !strings.Contains(body, `"admin_enabled":true`) {
		t.Fatalf("meta: %d %s", code, body)
	}
}

// Лимит частоты: сверх лимита — 429 в конверте ошибки; проверка живости не считается.
func TestRateLimit(t *testing.T) {
	app := adminApp(false, 3)
	for i := 0; i < 3; i++ {
		if code, _ := do(t, app, "GET", "/api/meta", ""); code != 200 {
			t.Fatalf("запрос %d: %d", i, code)
		}
	}
	code, body := do(t, app, "GET", "/api/meta", "")
	if code != fiber.StatusTooManyRequests || !strings.Contains(body, `"TOO_MANY_REQUESTS"`) {
		t.Fatalf("сверх лимита: %d %s", code, body)
	}
	if code, _ := do(t, app, "GET", "/api/health", ""); code != 200 {
		t.Fatalf("health вне лимита: %d", code)
	}
}

// GET /api/effect без базы — 200 с причинами, почему сценариев нет; неверные допущения — ошибка поля.
func TestEffectEndpoint(t *testing.T) {
	app := adminApp(false, 0)
	if code, body := do(t, app, "GET", "/api/effect", ""); code != 200 || !strings.Contains(body, `"missing":["нет поисков`) {
		t.Fatalf("effect: %d %s", code, body)
	}
	if code, body := do(t, app, "GET", "/api/effect?order_prob=abc", ""); code != 422 && code != 400 || !strings.Contains(body, "order_prob") {
		t.Fatalf("effect bad: %d %s", code, body)
	}
	if code, _ := do(t, app, "GET", "/api/costs/export", ""); code != fiber.StatusServiceUnavailable {
		t.Fatalf("export без базы: %d", code)
	}
}
