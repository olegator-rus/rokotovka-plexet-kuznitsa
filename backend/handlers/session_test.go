package handlers

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"kuznica/api/out"
	"kuznica/api/services"

	"github.com/gofiber/fiber/v2"
)

func metaSession(t *testing.T, app *fiber.App, header string) string {
	t.Helper()
	req := httptest.NewRequest(fiber.MethodGet, "/api/meta", nil)
	if header != "" {
		req.Header.Set(SessionHeader, header)
	}
	resp, err := app.Test(req, -1)
	if err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("meta: %v %v", resp, err)
	}
	var m out.Meta
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	return m.Session
}

// Сессию выдаёт /api/meta (ревью-2 n3): действующая продлевается, чужая или самодельная заменяется новой.
func TestMetaIssuesSignedSession(t *testing.T) {
	app := newTestApp()
	s1 := metaSession(t, app, "")
	if !strings.HasPrefix(s1, "s-") || len(s1) != 2+sessionIDHex+1+sessionSigHex {
		t.Fatalf("токен сессии: %q", s1)
	}
	if got := metaSession(t, app, s1); got != s1 {
		t.Fatalf("действующая сессия должна продлеваться: %q → %q", s1, got)
	}
	forged := s1[:len(s1)-1] + map[bool]string{true: "0", false: "1"}[s1[len(s1)-1] != '0']
	for _, bad := range []string{"s-0123456789abcdef", forged, "s-1"} {
		if got := metaSession(t, app, bad); got == bad || got == s1 {
			t.Fatalf("самодельная сессия %q не должна продлеваться: %q", bad, got)
		}
	}
}

// В журнал идёт только номер сессии с верной подписью этого сервера; ключ из BACKOFFICE_TOKEN стабилен.
func TestVerifySession(t *testing.T) {
	svc := services.NewServices(nil, services.Options{})
	const token = "0123456789abcdef0123456789abcdef" // gitleaks:allow — тестовая константа
	h := NewHandlerManager(svc, Options{BackofficeToken: token})
	same := NewHandlerManager(svc, Options{BackofficeToken: token})
	other := NewHandlerManager(svc, Options{BackofficeToken: token + "x"})
	dev := NewHandlerManager(svc, Options{})

	tok := h.issueSession()
	id, _, _ := strings.Cut(tok, ".")
	if h.verifySession(tok) != id || same.verifySession(tok) != id {
		t.Fatalf("своя подпись — номер сессии %q: %q", id, h.verifySession(tok))
	}
	for name, v := range map[string]*Handler{"другой токен": other, "dev, случайный ключ": dev} {
		if v.verifySession(tok) != "" {
			t.Fatalf("%s: чужая подпись принята", name)
		}
	}
	for _, bad := range []string{"", id, id + ".", "s-1.abc", "s-zzzzzzzzzzzzzzzz." + strings.Repeat("0", sessionSigHex),
		id + "." + strings.Repeat("0", sessionSigHex), "x-" + id[2:] + tok[len(id):]} {
		if got := h.verifySession(bad); got != "" {
			t.Fatalf("%q принят как %q", bad, got)
		}
	}

	// Тело важнее заголовка; без тела — заголовок X-Session.
	app := fiber.New()
	app.Post("/s", func(c *fiber.Ctx) error { return c.SendString(h.requestSession(c, c.Query("body"))) })
	for _, tc := range []struct{ body, header, want string }{
		{tok, "", id}, {"", tok, id}, {"s-forged", tok, ""}, {"", "", ""},
	} {
		req := httptest.NewRequest(fiber.MethodPost, "/s?body="+tc.body, nil)
		req.Header.Set(SessionHeader, tc.header)
		resp, _ := app.Test(req, -1)
		got, _ := io.ReadAll(resp.Body)
		if string(got) != tc.want {
			t.Fatalf("тело %q, заголовок %q → %q, ожидалось %q", tc.body, tc.header, got, tc.want)
		}
	}
}
