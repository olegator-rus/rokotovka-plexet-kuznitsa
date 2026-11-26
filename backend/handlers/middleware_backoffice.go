package handlers

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"strings"

	"kuznica/api/pkg/errs"

	"github.com/gofiber/fiber/v2"
)

// Вход в бэк-офис (ревью B2). Origin защищает от CSRF, но не от curl: заголовок подделывает любой не-браузерный
// клиент. Поэтому при заданном BACKOFFICE_TOKEN всё, кроме витрины покупателя, требует токен:
//   - Authorization: Bearer <токен> — скрипты и CLI;
//   - Authorization: Basic <пользователь:токен> — браузер после входа на basic_auth Caddy (deploy/Caddyfile)
//     сам шлёт этот заголовок на все запросы того же origin, фронту ничего хранить не нужно.
//
// Без входа (витрина покупателя): GET /api/health, /api/meta, /api/suggest, /api/evaluations и /api/evaluations/commerce (сводные метрики
// вкладки «До / После») и POST /api/search, /api/events, /api/quotes. Остальное — по умолчанию закрыто: новый маршрут без
// записи здесь попадёт под вход, а не наружу.

// MsgBackofficeAuth — запрос в бэк-офис без входа или с неверным токеном.
const MsgBackofficeAuth = "Нужен вход в бэк-офис: войдите на странице SearchOps или передайте токен (BACKOFFICE_TOKEN)"

// backofficeRealm — область basic-auth: та же строка, что в Caddyfile, чтобы браузер не спрашивал дважды.
const backofficeRealm = `Basic realm="PLEXeT SearchOps", charset="UTF-8"`

// publicRoutes — маршруты витрины покупателя: метод + путь.
var publicRoutes = map[string]bool{
	fiber.MethodGet + " /api/health":      true,
	fiber.MethodGet + " /api/meta":        true,
	fiber.MethodGet + " /api/suggest":     true,
	fiber.MethodGet + " /api/evaluations": true,
	// Коммерческая лестница C0–C3 (вкладка «Коммерция» в «До / После»): сводные метрики, без цифр маржи позиций.
	fiber.MethodGet + " /api/evaluations/commerce": true,
	fiber.MethodPost + " /api/search":              true,
	fiber.MethodPost + " /api/events":              true,
	fiber.MethodPost + " /api/quotes":              true,
}

// isPublic — запрос витрины покупателя (HEAD — как GET; OPTIONS — без тела и действия).
func isPublic(method, path string) bool {
	switch method {
	case fiber.MethodHead:
		method = fiber.MethodGet
	case fiber.MethodOptions:
		return true
	}
	return publicRoutes[method+" "+strings.TrimRight(path, "/")]
}

// Backoffice — проверка токена бэк-офиса. Пустой токен (development по умолчанию) — без проверки.
func (h *Handler) Backoffice(c *fiber.Ctx) error {
	if len(h.token) == 0 || isPublic(c.Method(), c.Path()) {
		return c.Next()
	}
	if !validBackofficeAuth(c.Get(fiber.HeaderAuthorization), h.token) {
		c.Set(fiber.HeaderWWWAuthenticate, backofficeRealm)
		return errs.NewUnauthorizedError(MsgBackofficeAuth)
	}
	return c.Next()
}

// backofficeAuthorized — запрос несёт вход в бэк-офис (или вход выключен: пустой токен в development). Для
// открытых маршрутов, которые отдают больше данных вошедшему.
func (h *Handler) backofficeAuthorized(c *fiber.Ctx) bool {
	return len(h.token) == 0 || validBackofficeAuth(c.Get(fiber.HeaderAuthorization), h.token)
}

// validBackofficeAuth — заголовок Authorization несёт токен: Bearer <токен> или Basic с паролем = токен
// (имя пользователя любое). token — SHA-256 токена: сравнение за постоянное время без утечки длины.
func validBackofficeAuth(header string, token []byte) bool {
	scheme, cred, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok {
		return false
	}
	cred = strings.TrimSpace(cred)
	var secret string
	switch strings.ToLower(scheme) {
	case "bearer":
		secret = cred
	case "basic":
		raw, err := base64.StdEncoding.DecodeString(cred)
		if err != nil {
			return false
		}
		_, pass, ok := strings.Cut(string(raw), ":")
		if !ok {
			return false
		}
		secret = pass
	default:
		return false
	}
	sum := sha256.Sum256([]byte(secret))
	return subtle.ConstantTimeCompare(sum[:], token) == 1
}

// hashToken — SHA-256 токена бэк-офиса; пустой — nil (вход выключен).
func hashToken(token string) []byte {
	if token == "" {
		return nil
	}
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
