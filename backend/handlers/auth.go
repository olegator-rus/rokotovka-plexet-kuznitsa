package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"kuznica/api/config"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
	"kuznica/api/services"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"
)

// Вход по ролям. Сессия — подписанная HttpOnly-cookie без хранилища: роль, имя, клиент покупателя,
// демо-аккаунт и срок, подпись HMAC-SHA256 ключом, производным от ключа сессий покупателя (session.go). Выход стирает
// cookie; украденную cookie до истечения срока не отозвать — поэтому срок короткий (AUTH_SESSION_TTL, 12 ч).
//
// Кто действует (actor): Bearer/basic BACKOFFICE_TOKEN → system; иначе роль из сессии; иначе, если вход не обязателен
// и токена нет (development, AUTH_REQUIRED=off), → system; иначе guest.

// AuthCookie — имя cookie сессии. Path=/api: к статике она не уходит.
const AuthCookie = "plexet_auth"

const (
	authKeySalt     = "plexet-auth-v1"
	authTokenMaxLen = 2048
	viewerKey       = "viewer"
	actorKey        = "actor"
)

// Тексты отказов входа.
const (
	MsgLoginFailed  = "Неверный логин или пароль"
	MsgLoginEmpty   = "Введите логин и пароль"
	MsgLoginTooMany = "Слишком много попыток входа — подождите минуту"
	MsgDemoOff      = "Демо-аккаунты на этом стенде выключены"
	MsgDemoUnknown  = "Такого демо-аккаунта нет"
	MsgNeedLogin    = "Сначала войдите"
	MsgStaffOnly    = "Этот раздел — для сотрудников магазина: войдите под аккаунтом сотрудника"
)

// sessionClaims — содержимое cookie входа.
type sessionClaims struct {
	V       int    `json:"v"`
	Role    string `json:"r"`
	Name    string `json:"n"`
	Client  string `json:"c,omitempty"`
	Persona string `json:"p,omitempty"`
	Login   string `json:"l,omitempty"`
	Exp     int64  `json:"e"`
}

// viewer — вошедший пользователь (из проверенной cookie).
type viewer struct {
	sessionClaims
}

// authOptions — настройки входа из config.AuthConfig.
type authOptions struct {
	required  bool
	demoLogin bool
	accounts  map[string]config.Account
	ttl       time.Duration
	secure    bool
	key       []byte
	limiter   *loginLimiter
}

func newAuthOptions(a config.AuthConfig, sessionKey []byte, secure bool) authOptions {
	m := hmac.New(sha256.New, sessionKey)
	m.Write([]byte(authKeySalt))
	accounts := make(map[string]config.Account, len(a.Accounts))
	for _, acc := range a.Accounts {
		accounts[acc.Login] = acc
	}
	ttl := a.SessionTTL
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	return authOptions{required: a.Required, demoLogin: a.DemoLogin, accounts: accounts, ttl: ttl, secure: secure,
		key: m.Sum(nil), limiter: newLoginLimiter(a.LoginPerMin, time.Minute)}
}

// ── Токен ──

func (h *Handler) signClaims(cl sessionClaims) string {
	raw, _ := json.Marshal(cl)
	payload := base64.RawURLEncoding.EncodeToString(raw)
	return payload + "." + h.authSig(payload)
}

func (h *Handler) authSig(payload string) string {
	m := hmac.New(sha256.New, h.auth.key)
	m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// parseClaims — проверенная и неистёкшая сессия; false — нет, подделана, истекла или аккаунт больше не действует.
func (h *Handler) parseClaims(token string) (sessionClaims, bool) {
	var cl sessionClaims
	if token == "" || len(token) > authTokenMaxLen {
		return cl, false
	}
	payload, sig, ok := strings.Cut(token, ".")
	if !ok || subtle.ConstantTimeCompare([]byte(sig), []byte(h.authSig(payload))) != 1 {
		return cl, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil || json.Unmarshal(raw, &cl) != nil || cl.V != 1 {
		return cl, false
	}
	if h.services.Now().Unix() >= cl.Exp {
		return cl, false
	}
	switch {
	case cl.Role != services.RoleBuyer && !services.IsStaffRole(cl.Role), cl.Role == services.RoleSystem:
		return cl, false
	case cl.Persona != "" && !h.auth.demoLogin:
		// Демо-вход выключили — демо-сессии больше не действуют.
		return cl, false
	case cl.Login != "":
		acc, ok := h.auth.accounts[cl.Login]
		if !ok || acc.Role != cl.Role || acc.ClientID != cl.Client {
			return cl, false
		}
	}
	return cl, true
}

// setAuthCookie — cookie входа: HttpOnly (скрипты страницы её не видят), SameSite=Lax (запросы с чужих сайтов её не
// несут; мутации дополнительно режет Origin), Secure в production.
func (h *Handler) setAuthCookie(c *fiber.Ctx, cl sessionClaims) {
	c.Cookie(&fiber.Cookie{Name: AuthCookie, Value: h.signClaims(cl), Path: "/api", HTTPOnly: true, Secure: h.auth.secure,
		SameSite: fiber.CookieSameSiteLaxMode, Expires: time.Unix(cl.Exp, 0),
		MaxAge: max(1, int(cl.Exp-h.services.Now().Unix()))})
}

func (h *Handler) clearAuthCookie(c *fiber.Ctx) {
	c.Cookie(&fiber.Cookie{Name: AuthCookie, Value: "", Path: "/api", HTTPOnly: true, Secure: h.auth.secure,
		SameSite: fiber.CookieSameSiteLaxMode, Expires: time.Unix(1, 0), MaxAge: -1})
}

// ── Кто смотрит ──

// Identify — на каждом запросе /api: разбор cookie входа и токена бэк-офиса; кладёт в Locals вошедшего и роль actor.
func (h *Handler) Identify(c *fiber.Ctx) error {
	var v *viewer
	if cl, ok := h.parseClaims(c.Cookies(AuthCookie)); ok {
		v = &viewer{cl}
		c.Locals(viewerKey, v)
	}
	actor := services.RoleGuest
	switch {
	case len(h.token) > 0 && validBackofficeAuth(c.Get(fiber.HeaderAuthorization), h.token):
		actor = services.RoleSystem
	case v != nil:
		actor = v.Role
	case !h.auth.required && len(h.token) == 0:
		actor = services.RoleSystem
	}
	c.Locals(actorKey, actor)
	return c.Next()
}

// currentViewer — вошедший пользователь запроса; nil — гость (или скрипт по токену).
func currentViewer(c *fiber.Ctx) *viewer {
	v, _ := c.Locals(viewerKey).(*viewer)
	return v
}

// actorRole — кто действует: guest | buyer | ecom | product | content | head | system.
func actorRole(c *fiber.Ctx) string {
	if r, ok := c.Locals(actorKey).(string); ok {
		return r
	}
	return services.RoleGuest
}

// viewerOut — вошедший для ответа API.
func (h *Handler) viewerOut(c *fiber.Ctx) out.Viewer {
	v := currentViewer(c)
	if v == nil {
		role := actorRole(c)
		if role != services.RoleSystem {
			role = services.RoleGuest
		}
		return out.Viewer{Role: role, RoleLabel: services.RoleLabel(role)}
	}
	res := out.Viewer{Role: v.Role, RoleLabel: services.RoleLabel(v.Role), Name: v.Name}
	if v.Client != "" {
		id, name := v.Client, h.services.ClientName(v.Client)
		res.ClientID = &id
		if name != "" {
			res.ClientName = &name
		}
	}
	if v.Persona != "" {
		p := v.Persona
		res.Persona = &p
	}
	exp := time.Unix(v.Exp, 0).UTC()
	res.ExpiresAt = &exp
	return res
}

// storefrontClient — клиент витрины для поиска, подсказок, событий и КП: покупатель — свой из сессии (присланный
// игнорируется); сотрудник и служебный доступ — присланный («Смотреть глазами клиента»); гость при обязательном
// входе — guest (чужую историю покупок не подставить), иначе — присланный, как раньше.
func (h *Handler) storefrontClient(c *fiber.Ctx, requested string) string {
	role := actorRole(c)
	switch {
	case role == services.RoleBuyer:
		if v := currentViewer(c); v != nil {
			return v.Client
		}
		return ""
	case services.IsStaffRole(role), !h.auth.required:
		return requested
	case requested == "":
		return ""
	}
	return guestClientID
}

// guestClientID — клиент «Гость» витрины (так его шлёт фронт).
const guestClientID = "guest"

// requireDecision — решение в зоне роли owner: 403 «Это решение принимает <роль>», если actor не вправе.
func requireDecision(c *fiber.Ctx, owner string) error {
	if !services.CanDecide(actorRole(c), owner) {
		return errs.NewForbiddenError(services.DenyMessage(owner))
	}
	return nil
}

// ── Маршруты /api/auth ──

// AuthConfig — GET /api/auth/config: способы входа и демо-аккаунты.
func (h *Handler) AuthConfig(c *fiber.Ctx) error {
	res := out.AuthConfig{DemoLogin: h.auth.demoLogin, AuthRequired: h.auth.required, PasswordLogin: len(h.auth.accounts) > 0,
		Personas: []out.Persona{}}
	if h.auth.demoLogin {
		res.Personas = h.services.DemoPersonas(c.UserContext())
	}
	return c.JSON(res)
}

// AuthMe — GET /api/auth/me: кто смотрит (гость — role guest, 200).
func (h *Handler) AuthMe(c *fiber.Ctx) error {
	return c.JSON(out.AuthMe{Viewer: h.viewerOut(c)})
}

// Login — POST /api/auth/login {login, password}. Лимит попыток — до разбора тела (битые тела тоже считаются);
// успешный вход попытку возвращает. Неверный логин и неверный пароль — один ответ и одинаковое время (bcrypt
// считается и для несуществующего логина).
func (h *Handler) Login(c *fiber.Ctx) error {
	if err := h.limitLogin(c); err != nil {
		return err
	}
	var req in.Login
	if err := bindBody(c, &req); err != nil {
		return err
	}
	login := strings.ToLower(strings.TrimSpace(req.Login))
	if login == "" || req.Password == "" {
		return errs.NewValidationError(MsgLoginEmpty)
	}
	if len(login) > 64 || len(req.Password) > 72 {
		return errs.NewUnauthorizedError(MsgLoginFailed)
	}
	acc, ok := h.auth.accounts[login]
	hash := []byte(acc.Hash)
	if !ok {
		hash = dummyHash
	}
	if err := bcrypt.CompareHashAndPassword(hash, []byte(req.Password)); err != nil || !ok {
		return errs.NewUnauthorizedError(MsgLoginFailed)
	}
	h.auth.limiter.refund(c.IP())
	cl := sessionClaims{V: 1, Role: acc.Role, Name: acc.Login, Client: acc.ClientID, Login: acc.Login,
		Exp: h.services.Now().Add(h.auth.ttl).Unix()}
	return h.startSession(c, cl)
}

// dummyHash — bcrypt-хэш для несуществующего логина: время ответа не выдаёт, есть ли такой аккаунт.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("plexet-no-such-account"), bcrypt.DefaultCost)

// DemoLogin — POST /api/auth/demo-login {persona}: вход в демо-аккаунт одной кнопкой, пароль в браузер не уходит.
// Без DEMO_LOGIN_BUTTONS — 404.
func (h *Handler) DemoLogin(c *fiber.Ctx) error {
	if !h.auth.demoLogin {
		return errs.NewNotFoundError(MsgDemoOff)
	}
	if err := h.limitLogin(c); err != nil {
		return err
	}
	return h.demoSession(c)
}

// DemoSwitch — POST /api/auth/demo-switch {persona}: сменить демо-аккаунт из шапки (нужен вход). Без
// DEMO_LOGIN_BUTTONS — 404.
func (h *Handler) DemoSwitch(c *fiber.Ctx) error {
	if !h.auth.demoLogin {
		return errs.NewNotFoundError(MsgDemoOff)
	}
	if currentViewer(c) == nil {
		return errs.NewUnauthorizedError(MsgNeedLogin)
	}
	if err := h.limitLogin(c); err != nil {
		return err
	}
	return h.demoSession(c)
}

func (h *Handler) demoSession(c *fiber.Ctx) error {
	var req in.DemoLogin
	if err := bindBody(c, &req); err != nil {
		return err
	}
	p, ok := h.services.DemoPersona(strings.TrimSpace(req.Persona))
	if !ok {
		return errs.NewValidationError(MsgDemoUnknown).WithDetails(errs.FieldError{Field: "persona",
			Code: errs.CodeInvalidFormat, Message: MsgDemoUnknown})
	}
	h.auth.limiter.refund(c.IP())
	cl := sessionClaims{V: 1, Role: p.Role, Name: p.Name, Persona: p.ID, Exp: h.services.Now().Add(h.auth.ttl).Unix()}
	if p.ClientID != nil {
		cl.Client = *p.ClientID
	}
	return h.startSession(c, cl)
}

// startSession — cookie и ответ {viewer} с новой сессией.
func (h *Handler) startSession(c *fiber.Ctx, cl sessionClaims) error {
	h.setAuthCookie(c, cl)
	v := &viewer{cl}
	c.Locals(viewerKey, v)
	c.Locals(actorKey, cl.Role)
	return c.JSON(out.AuthMe{Viewer: h.viewerOut(c)})
}

// Logout — POST /api/auth/logout: стирает cookie, 204 (и без входа).
func (h *Handler) Logout(c *fiber.Ctx) error {
	h.clearAuthCookie(c)
	return c.SendStatus(fiber.StatusNoContent)
}

// limitLogin — попытка входа с адреса; исчерпано — 429 с Retry-After.
func (h *Handler) limitLogin(c *fiber.Ctx) error {
	if ok, wait := h.auth.limiter.hit(c.IP()); !ok {
		c.Set(fiber.HeaderRetryAfter, strconv.Itoa(max(1, int(wait.Round(time.Second).Seconds()))))
		return errs.NewTooManyError(MsgLoginTooMany)
	}
	return nil
}

// ── Лимит попыток входа ──

// loginLimiter — не больше max попыток за окно с адреса (фиксированное окно). max <= 0 — без лимита.
type loginLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	now    func() time.Time
	hits   map[string]*loginWindow
}

type loginWindow struct {
	count int
	reset time.Time
}

func newLoginLimiter(max int, window time.Duration) *loginLimiter {
	return &loginLimiter{max: max, window: window, now: time.Now, hits: map[string]*loginWindow{}}
}

// hit — учесть попытку; false и сколько ждать — лимит исчерпан.
func (l *loginLimiter) hit(ip string) (bool, time.Duration) {
	if l == nil || l.max <= 0 {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if len(l.hits) > 10_000 {
		for k, w := range l.hits {
			if !now.Before(w.reset) {
				delete(l.hits, k)
			}
		}
	}
	w := l.hits[ip]
	if w == nil || !now.Before(w.reset) {
		w = &loginWindow{reset: now.Add(l.window)}
		l.hits[ip] = w
	}
	if w.count >= l.max {
		return false, w.reset.Sub(now)
	}
	w.count++
	return true, 0
}

// refund — успешный вход возвращает попытку.
func (l *loginLimiter) refund(ip string) {
	if l == nil || l.max <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if w := l.hits[ip]; w != nil && w.count > 0 {
		w.count--
	}
}
