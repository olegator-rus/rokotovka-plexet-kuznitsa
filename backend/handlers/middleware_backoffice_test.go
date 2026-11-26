package handlers

import (
	"encoding/base64"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"kuznica/api/out"
	"kuznica/api/services"

	"github.com/gofiber/fiber/v2"
)

const testBackofficeToken = "bo-token-0123456789abcdef0123"

func backofficeApp(token string) *fiber.App {
	app := fiber.New(ServerConfig(DefaultTimeouts))
	app.Use(RequestID)
	RegisterRoutes(app, NewHandlerManager(services.NewServices(nil, services.Options{}),
		Options{PublicOrigin: testOrigin, AdminEnabled: true, BackofficeToken: token}))
	return app
}

func doAuth(t *testing.T, app *fiber.App, method, path, auth string) (int, string, string) {
	t.Helper()
	body := ""
	if method == fiber.MethodPost {
		body = "{}"
	}
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", testOrigin)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw), resp.Header.Get(fiber.HeaderWWWAuthenticate)
}

func basic(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

// С токеном: витрина покупателя без входа; бэк-офис (чтение и мутации) — только с Bearer или basic-auth.
func TestBackofficeTokenRequired(t *testing.T) {
	app := backofficeApp(testBackofficeToken)
	public := []struct{ method, path string }{
		{"GET", "/health"}, {"GET", "/api/health"}, {"GET", "/api/meta"}, {"GET", "/api/suggest?q=1c"},
		{"GET", "/api/evaluations"}, {"POST", "/api/search"}, {"POST", "/api/events"},
		{"GET", "/api/evaluations/commerce"}, {"GET", "/api/companions?product_id=x"},
	}
	for _, r := range public {
		if code, body, _ := doAuth(t, app, r.method, r.path, ""); code == fiber.StatusUnauthorized {
			t.Fatalf("%s %s — витрина, вход не нужен: %d %s", r.method, r.path, code, body)
		}
	}
	closed := []struct{ method, path string }{
		{"GET", "/api/analytics"}, {"GET", "/api/traces/01ABC"}, {"GET", "/api/costs"}, {"GET", "/api/costs/export"},
		{"GET", "/api/effect"}, {"GET", "/api/agent/runs"}, {"GET", "/api/agent/runs/x/stream"}, {"GET", "/api/agent/memory"},
		{"GET", "/api/proposals"}, {"GET", "/api/configs"},
		{"POST", "/api/configs/rollback"}, {"POST", "/api/proposals/x/approve"}, {"POST", "/api/proposals/x/reject"},
		{"POST", "/api/agent/runs"}, {"POST", "/api/agent/runs/x/stop"}, {"POST", "/api/catalog/import"},
		{"POST", "/api/evaluations/run"}, {"POST", "/api/admin/llm-mode"},
		{"GET", "/api/relations"}, {"GET", "/api/relations/versions"}, {"POST", "/api/relations/rollback"},
		{"GET", "/api/no-such-route"}, {"DELETE", "/api/search"}, {"GET", "/api/analytics/"},
	}
	for _, r := range closed {
		code, body, challenge := doAuth(t, app, r.method, r.path, "")
		if code != fiber.StatusUnauthorized || !strings.Contains(body, `"code":"`) || !strings.Contains(challenge, `realm="PLEXeT SearchOps"`) {
			t.Fatalf("%s %s без входа: %d %q %s", r.method, r.path, code, challenge, body)
		}
		for _, bad := range []string{"Bearer wrong", basic("manager", "wrong"), "Token " + testBackofficeToken,
			"Bearer " + testBackofficeToken + "x", basic(testBackofficeToken, ""), "Basic !!!"} {
			if code, _, _ := doAuth(t, app, r.method, r.path, bad); code != fiber.StatusUnauthorized {
				t.Fatalf("%s %s с неверным %q: %d", r.method, r.path, bad, code)
			}
		}
	}
	// С токеном проходят дальше (сервисы без базы отвечают своими кодами, но не 401).
	for _, auth := range []string{"Bearer " + testBackofficeToken, "bearer " + testBackofficeToken, basic("manager", testBackofficeToken),
		basic("", testBackofficeToken)} {
		if code, body, _ := doAuth(t, app, "GET", "/api/analytics", auth); code != fiber.StatusOK {
			t.Fatalf("аналитика с %q: %d %s", auth, code, body)
		}
		if code, body, _ := doAuth(t, app, "POST", "/api/admin/llm-mode", auth); code == fiber.StatusUnauthorized {
			t.Fatalf("мутация с %q: %d %s", auth, code, body)
		}
	}
}

// Мутация с токеном, но с чужого Origin — по-прежнему 403: вход не отменяет защиту от CSRF.
func TestBackofficeKeepsOriginCheck(t *testing.T) {
	app := backofficeApp(testBackofficeToken)
	req := httptest.NewRequest("POST", "/api/configs/rollback", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Authorization", "Bearer "+testBackofficeToken)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("чужой Origin: %d", resp.StatusCode)
	}
}

// Без токена (development по умолчанию) — как раньше: бэк-офис без входа.
func TestBackofficeOpenWithoutToken(t *testing.T) {
	app := backofficeApp("")
	for _, p := range []string{"/api/analytics", "/api/costs"} {
		if code, body, _ := doAuth(t, app, "GET", p, ""); code == fiber.StatusUnauthorized {
			t.Fatalf("%s без токена в конфиге: %d %s", p, code, body)
		}
	}
	if code, body, _ := doAuth(t, app, "POST", "/api/admin/llm-mode", ""); code == fiber.StatusUnauthorized {
		t.Fatalf("мутация без токена в конфиге: %d %s", code, body)
	}
}

// Ревью-2 n7: holdout_week без входа — без текстов и id запросов; с токеном или без токена в development — целиком.
func TestEvaluationsHoldoutRedacted(t *testing.T) {
	h := &Handler{token: hashToken(testBackofficeToken)}
	app := fiber.New()
	app.Get("/x", h.Identify, func(c *fiber.Ctx) error {
		if h.backofficeAuthorized(c) {
			return c.SendString("in")
		}
		return c.SendString("out")
	})
	for auth, want := range map[string]string{"": "out", "Bearer wrong": "out", "Bearer " + testBackofficeToken: "in",
		basic("manager", testBackofficeToken): "in"} {
		req := httptest.NewRequest("GET", "/x", nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		resp, _ := app.Test(req)
		body, _ := io.ReadAll(resp.Body)
		if string(body) != want {
			t.Errorf("%q: %s, ждали %s", auth, body, want)
		}
	}
	h.token = nil // development: вход выключен
	if resp, _ := app.Test(httptest.NewRequest("GET", "/x", nil)); resp != nil {
		if body, _ := io.ReadAll(resp.Body); string(body) != "in" {
			t.Error("без токена (development) вход не нужен")
		}
	}
	resp := &out.EvaluationsResponse{Set: "holdout_week", Configs: []out.EvaluationView{{
		Regressions:  []out.EvalChange{{QueryID: "h-0001-client-a", Text: "1с для ооо ромашка"}},
		Improvements: []out.EvalChange{{QueryID: "h-0002", Text: "зуп корп", NDCGDelta: 0.1}}}}}
	redactEvalTexts(resp)
	c := resp.Configs[0]
	if len(c.Regressions) != 1 || c.Regressions[0].Text != "" || c.Regressions[0].QueryID != "" ||
		c.Improvements[0].Text != "" || c.Improvements[0].NDCGDelta != 0.1 {
		t.Fatalf("после скрытия: %+v", c)
	}
}
