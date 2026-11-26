package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"kuznica/api/config"
	"kuznica/api/externalApi/catalogsource"
	"kuznica/api/out"
	"kuznica/api/services"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"
)

// authApp — маршруты с входом по ролям: каталог фикстуры (клиенты guest, client-a, client-b) без базы и поиска.
func authApp(t *testing.T, auth config.AuthConfig, token string) *fiber.App {
	app, _ := authAppH(t, auth, token)
	return app
}

func authAppH(t *testing.T, auth config.AuthConfig, token string) (*fiber.App, *Handler) {
	t.Helper()
	svc := services.NewServices(nil, services.Options{Now: func() time.Time { return fixedNow },
		Source: catalogsource.MockJSON{Dir: "../testdata/catalog"}})
	svc.LoadCatalogState(t.Context())
	app := fiber.New(ServerConfig(DefaultTimeouts))
	app.Use(RequestID)
	h := NewHandlerManager(svc, Options{PublicOrigin: testOrigin, BackofficeToken: token, Auth: auth})
	RegisterRoutes(app, h)
	return app, h
}

var demoAuth = config.AuthConfig{Required: true, DemoLogin: true, SessionTTL: time.Hour, LoginPerMin: 5}

type authResp struct {
	code   int
	body   string
	cookie *http.Cookie
	header http.Header
}

func authDo(t *testing.T, app *fiber.App, method, path, body, cookie string) authResp {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", testOrigin)
	if cookie != "" {
		req.Header.Set("Cookie", AuthCookie+"="+cookie)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	r := authResp{code: resp.StatusCode, body: string(raw), header: resp.Header}
	for _, c := range resp.Cookies() {
		if c.Name == AuthCookie {
			r.cookie = c
		}
	}
	return r
}

// demoCookie — вход в демо-аккаунт, значение cookie сессии.
func demoCookie(t *testing.T, app *fiber.App, persona string) string {
	t.Helper()
	r := authDo(t, app, "POST", "/api/auth/demo-login", `{"persona":"`+persona+`"}`, "")
	if r.code != fiber.StatusOK || r.cookie == nil {
		t.Fatalf("демо-вход %s: %d %s", persona, r.code, r.body)
	}
	return r.cookie.Value
}

// Конфиг входа: демо-аккаунты — покупатели только на клиентах каталога (в фикстуре нет client-c и client-d) и все
// четыре роли сотрудников; без DEMO_LOGIN_BUTTONS — пусто, демо-вход — 404.
func TestAuthConfigPersonas(t *testing.T) {
	app := authApp(t, demoAuth, "")
	r := authDo(t, app, "GET", "/api/auth/config", "", "")
	var cfg out.AuthConfig
	if err := json.Unmarshal([]byte(r.body), &cfg); err != nil || r.code != 200 {
		t.Fatalf("%d %s", r.code, r.body)
	}
	got := map[string]string{}
	for _, p := range cfg.Personas {
		got[p.ID] = p.Role
		if p.Initials == "" || p.Name == "" || p.Note == "" || p.RoleLabel == "" {
			t.Errorf("пустые поля персоны: %+v", p)
		}
	}
	if !cfg.DemoLogin || !cfg.AuthRequired || cfg.PasswordLogin {
		t.Errorf("флаги: %+v", cfg)
	}
	want := map[string]string{"buyer-smb": "buyer", "staff-ecom": "ecom", "staff-product": "product",
		"staff-content": "content", "staff-head": "head"}
	if len(got) != len(want) {
		t.Fatalf("персоны: %v", got)
	}
	for id, role := range want {
		if got[id] != role {
			t.Errorf("%s: %q, ждали %q", id, got[id], role)
		}
	}

	off := authApp(t, config.AuthConfig{Required: true}, "")
	if r := authDo(t, off, "GET", "/api/auth/config", "", ""); !strings.Contains(r.body, `"personas":[]`) {
		t.Errorf("демо выключено: %s", r.body)
	}
	if r := authDo(t, off, "POST", "/api/auth/demo-login", `{"persona":"staff-head"}`, ""); r.code != fiber.StatusNotFound {
		t.Errorf("демо-вход при выключенных кнопках: %d", r.code)
	}
}

// Демо-вход: HttpOnly-cookie с SameSite=Lax на /api; me и meta показывают вошедшего; выход стирает cookie.
func TestDemoLoginSession(t *testing.T) {
	app := authApp(t, demoAuth, "")
	r := authDo(t, app, "POST", "/api/auth/demo-login", `{"persona":"buyer-smb"}`, "")
	if r.code != 200 || r.cookie == nil {
		t.Fatalf("%d %s", r.code, r.body)
	}
	if !r.cookie.HttpOnly || r.cookie.Path != "/api" || r.cookie.SameSite != http.SameSiteLaxMode || r.cookie.Secure {
		t.Errorf("cookie: %+v", r.cookie)
	}
	var me out.AuthMe
	_ = json.Unmarshal([]byte(r.body), &me)
	if me.Viewer.Role != "buyer" || me.Viewer.ClientID == nil || *me.Viewer.ClientID != "client-a" || me.Viewer.Persona == nil {
		t.Fatalf("viewer: %s", r.body)
	}
	cookie := r.cookie.Value
	if r := authDo(t, app, "GET", "/api/meta", "", cookie); !strings.Contains(r.body, `"viewer":{"role":"buyer"`) ||
		!strings.Contains(r.body, `"client_id":"client-a"`) {
		t.Errorf("meta: %s", r.body)
	}
	if r := authDo(t, app, "GET", "/api/auth/me", "", ""); !strings.Contains(r.body, `"role":"guest"`) {
		t.Errorf("гость: %s", r.body)
	}
	// Смена демо-аккаунта — только вошедшему.
	if r := authDo(t, app, "POST", "/api/auth/demo-switch", `{"persona":"staff-head"}`, ""); r.code != fiber.StatusUnauthorized {
		t.Errorf("смена без входа: %d", r.code)
	}
	sw := authDo(t, app, "POST", "/api/auth/demo-switch", `{"persona":"staff-head"}`, cookie)
	if sw.code != 200 || sw.cookie == nil || !strings.Contains(sw.body, `"role":"head"`) {
		t.Fatalf("смена: %d %s", sw.code, sw.body)
	}
	if r := authDo(t, app, "POST", "/api/auth/demo-login", `{"persona":"nobody"}`, ""); r.code != fiber.StatusUnprocessableEntity {
		t.Errorf("неизвестная персона: %d", r.code)
	}
	out := authDo(t, app, "POST", "/api/auth/logout", "", sw.cookie.Value)
	if out.code != fiber.StatusNoContent || out.cookie == nil || out.cookie.Value != "" || out.cookie.Expires.After(fixedNow) {
		t.Errorf("выход: %d %+v", out.code, out.cookie)
	}
}

// Подделанная, чужая и истёкшая cookie — гость.
func TestAuthCookieTampered(t *testing.T) {
	const token = "bo-token-0123456789abcdef0123"
	app, h := authAppH(t, demoAuth, token)
	cookie := demoCookie(t, app, "staff-content")
	payload, sig, _ := strings.Cut(cookie, ".")
	forged := h.signClaims(sessionClaims{V: 1, Role: "system", Name: "x", Exp: fixedNow.Add(time.Hour).Unix()})
	expired := h.signClaims(sessionClaims{V: 1, Role: "head", Name: "x", Exp: fixedNow.Add(-time.Second).Unix()})
	for name, bad := range map[string]string{"подпись": payload + "." + strings.Repeat("A", len(sig)), "мусор": "abc",
		"payload": "e30." + sig, "роль system": forged, "истекла": expired} {
		if r := authDo(t, app, "GET", "/api/auth/me", "", bad); !strings.Contains(r.body, `"role":"guest"`) {
			t.Errorf("%s: %s", name, r.body)
		}
	}
	if r := authDo(t, app, "GET", "/api/auth/me", "", cookie); !strings.Contains(r.body, `"role":"content"`) {
		t.Fatalf("своя cookie: %s", r.body)
	}
	// Ключ — от токена бэк-офиса: тот же токен (перезапуск) принимает cookie, другой — нет.
	if r := authDo(t, authApp(t, demoAuth, token), "GET", "/api/auth/me", "", cookie); !strings.Contains(r.body, `"role":"content"`) {
		t.Errorf("тот же токен: %s", r.body)
	}
	if r := authDo(t, authApp(t, demoAuth, token+"x"), "GET", "/api/auth/me", "", cookie); !strings.Contains(r.body, `"role":"guest"`) {
		t.Errorf("чужой ключ: %s", r.body)
	}
	// Демо-кнопки выключили — демо-сессии больше не действуют.
	if r := authDo(t, authApp(t, config.AuthConfig{Required: true}, token), "GET", "/api/auth/me", "", cookie); !strings.Contains(r.body, `"role":"guest"`) {
		t.Errorf("демо выключено: %s", r.body)
	}
}

// Бэк-офис при AUTH_REQUIRED: гость — 401 с вызовом Bearer (не Basic: без окна пароля браузера), покупатель — 403,
// сотрудник любой роли и Bearer — проходят; витрина и вход открыты всем.
func TestBackofficeRoles(t *testing.T) {
	const token = "bo-token-0123456789abcdef0123"
	app := authApp(t, demoAuth, token)
	buyer, content := demoCookie(t, app, "buyer-smb"), demoCookie(t, app, "staff-content")
	r := authDo(t, app, "GET", "/api/costs", "", "")
	if r.code != fiber.StatusUnauthorized || !strings.HasPrefix(r.header.Get(fiber.HeaderWWWAuthenticate), "Bearer ") {
		t.Errorf("гость: %d %q", r.code, r.header.Get(fiber.HeaderWWWAuthenticate))
	}
	if r := authDo(t, app, "GET", "/api/costs", "", buyer); r.code != fiber.StatusForbidden || !strings.Contains(r.body, "сотрудник") {
		t.Errorf("покупатель: %d %s", r.code, r.body)
	}
	if r := authDo(t, app, "GET", "/api/costs", "", content); r.code == fiber.StatusUnauthorized || r.code == fiber.StatusForbidden {
		t.Errorf("контент-менеджер: %d %s", r.code, r.body)
	}
	req := httptest.NewRequest("GET", "/api/costs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if resp, _ := app.Test(req, -1); resp.StatusCode == fiber.StatusUnauthorized || resp.StatusCode == fiber.StatusForbidden {
		t.Errorf("Bearer: %d", resp.StatusCode)
	}
	for _, path := range []string{"/api/meta", "/api/auth/config", "/api/auth/me", "/api/suggest?q=win"} {
		if r := authDo(t, app, "GET", path, "", ""); r.code != fiber.StatusOK {
			t.Errorf("%s гостю: %d", path, r.code)
		}
	}
}

// Права на решения: чужая зона — 403 «Это решение принимает <роль>» до разбора тела; своя зона и
// руководитель — дальше (здесь без базы — 422/503, но не 403).
func TestDecisionRights(t *testing.T) {
	app := authApp(t, demoAuth, "")
	cookies := map[string]string{}
	for _, p := range []string{"staff-ecom", "staff-product", "staff-content", "staff-head"} {
		cookies[p] = demoCookie(t, app, p)
	}
	cases := []struct {
		path, deny string
		allow      []string
	}{
		{"/api/configs/rollback", "Это решение принимает администратор поиска", []string{"staff-ecom", "staff-head"}},
		{"/api/margin/policy", "Это решение принимает администратор поиска", []string{"staff-ecom", "staff-head"}},
		{"/api/margin/policy/rollback", "Это решение принимает администратор поиска", []string{"staff-ecom", "staff-head"}},
		{"/api/relations/rollback", "Это решение принимает категорийный и лицензионный менеджер", []string{"staff-product", "staff-head"}},
	}
	for _, tc := range cases {
		for persona, cookie := range cookies {
			r := authDo(t, app, "POST", tc.path, `{}`, cookie)
			allowed := false
			for _, a := range tc.allow {
				allowed = allowed || a == persona
			}
			switch {
			case allowed && (r.code == fiber.StatusForbidden || r.code == fiber.StatusUnauthorized):
				t.Errorf("%s %s: %d %s", persona, tc.path, r.code, r.body)
			case !allowed && (r.code != fiber.StatusForbidden || !strings.Contains(r.body, tc.deny)):
				t.Errorf("%s %s: ждали 403 «%s», %d %s", persona, tc.path, tc.deny, r.code, r.body)
			}
		}
	}
	// Решение по предложению: роль предложения из очереди — без базы очередь недоступна (503), но не 403 у руководителя.
	if r := authDo(t, app, "POST", "/api/proposals/x/approve", `{}`, cookies["staff-head"]); r.code == fiber.StatusForbidden {
		t.Errorf("руководитель: %d %s", r.code, r.body)
	}
}

// Клиент витрины: покупатель — свой из сессии, гость — guest, сотрудник — выбранный.
func TestStorefrontClient(t *testing.T) {
	app, h := authAppH(t, demoAuth, "")
	buyer, ecom := demoCookie(t, app, "buyer-smb"), demoCookie(t, app, "staff-ecom")
	probe := fiber.New()
	probe.Get("/api/x", h.Identify, func(c *fiber.Ctx) error { return c.SendString(h.storefrontClient(c, c.Query("client"))) })
	for _, tc := range []struct{ cookie, client, want string }{
		{"", "client-b", "guest"}, {"", "", ""}, {buyer, "client-b", "client-a"}, {buyer, "", "client-a"},
		{ecom, "client-b", "client-b"}, {ecom, "", ""},
	} {
		req := httptest.NewRequest("GET", "/api/x?client="+tc.client, nil)
		if tc.cookie != "" {
			req.Header.Set("Cookie", AuthCookie+"="+tc.cookie)
		}
		resp, _ := probe.Test(req)
		body, _ := io.ReadAll(resp.Body)
		if string(body) != tc.want {
			t.Errorf("cookie=%t client=%q: %q, ждали %q", tc.cookie != "", tc.client, body, tc.want)
		}
	}
	// Без обязательного входа (прежний режим) гость выбирает клиента сам.
	_, open := authAppH(t, config.AuthConfig{}, "")
	probe2 := fiber.New()
	probe2.Get("/api/x", open.Identify, func(c *fiber.Ctx) error { return c.SendString(open.storefrontClient(c, "client-b")) })
	resp, _ := probe2.Test(httptest.NewRequest("GET", "/api/x", nil))
	if body, _ := io.ReadAll(resp.Body); string(body) != "client-b" {
		t.Errorf("открытый режим: %s", body)
	}
}

// Вход по паролю (AUTH_ACCOUNTS): верный — сессия роли аккаунта; неверный и несуществующий — один ответ 401; лимит
// попыток с адреса — 429 с Retry-After; успешный вход попытку возвращает.
func TestPasswordLogin(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("secret-pass"), bcrypt.MinCost)
	auth := demoAuth
	auth.LoginPerMin = 3
	auth.Accounts = []config.Account{{Login: "olga", Role: "buyer", ClientID: "client-a", Hash: string(hash)},
		{Login: "boss", Role: "head", Hash: string(hash)}}
	app := authApp(t, auth, "")
	ok := authDo(t, app, "POST", "/api/auth/login", `{"login":"Boss","password":"secret-pass"}`, "")
	if ok.code != 200 || ok.cookie == nil || !strings.Contains(ok.body, `"role":"head"`) {
		t.Fatalf("вход: %d %s", ok.code, ok.body)
	}
	if r := authDo(t, app, "GET", "/api/auth/config", "", ""); !strings.Contains(r.body, `"password_login":true`) {
		t.Errorf("config: %s", r.body)
	}
	wrong := authDo(t, app, "POST", "/api/auth/login", `{"login":"olga","password":"nope"}`, "")
	missing := authDo(t, app, "POST", "/api/auth/login", `{"login":"nobody","password":"nope"}`, "")
	if wrong.code != 401 || missing.code != 401 || !strings.Contains(wrong.body, MsgLoginFailed) || !strings.Contains(missing.body, MsgLoginFailed) {
		t.Errorf("неверный: %d %s / %d %s", wrong.code, wrong.body, missing.code, missing.body)
	}
	if r := authDo(t, app, "POST", "/api/auth/login", `{"login":"olga","password":"bad"}`, ""); r.code != 401 {
		t.Errorf("третья попытка: %d", r.code)
	}
	r := authDo(t, app, "POST", "/api/auth/login", `{"login":"olga","password":"secret-pass"}`, "")
	if r.code != fiber.StatusTooManyRequests || r.header.Get(fiber.HeaderRetryAfter) == "" {
		t.Errorf("лимит: %d %q %s", r.code, r.header.Get(fiber.HeaderRetryAfter), r.body)
	}
}

func TestLoginLimiter(t *testing.T) {
	now := fixedNow
	l := newLoginLimiter(2, time.Minute)
	l.now = func() time.Time { return now }
	if ok, _ := l.hit("a"); !ok {
		t.Fatal("1")
	}
	l.refund("a")
	for i := range 2 {
		if ok, _ := l.hit("a"); !ok {
			t.Fatalf("попытка %d", i)
		}
	}
	if ok, wait := l.hit("a"); ok || wait != time.Minute {
		t.Fatalf("лимит: %v %v", ok, wait)
	}
	if ok, _ := l.hit("b"); !ok {
		t.Fatal("другой адрес")
	}
	now = now.Add(time.Minute)
	if ok, _ := l.hit("a"); !ok {
		t.Fatal("новое окно")
	}
}

// Свои КП: гостю — 401, сотруднику — 403, покупателю — список его клиента (без базы — пустой).
func TestMyQuotes(t *testing.T) {
	app := authApp(t, demoAuth, "")
	if r := authDo(t, app, "GET", "/api/quotes/mine", "", ""); r.code != fiber.StatusUnauthorized {
		t.Errorf("гость: %d", r.code)
	}
	if r := authDo(t, app, "GET", "/api/quotes/mine", "", demoCookie(t, app, "staff-head")); r.code != fiber.StatusForbidden {
		t.Errorf("сотрудник: %d", r.code)
	}
	if r := authDo(t, app, "GET", "/api/quotes/mine", "", demoCookie(t, app, "buyer-smb")); r.code != 200 || !strings.Contains(r.body, `"items":[]`) {
		t.Errorf("покупатель: %d %s", r.code, r.body)
	}
	// Общий список КП — бэк-офис: покупателю 403.
	if r := authDo(t, app, "GET", "/api/quotes", "", demoCookie(t, app, "buyer-smb")); r.code != fiber.StatusForbidden {
		t.Errorf("общий список покупателю: %d", r.code)
	}
}
