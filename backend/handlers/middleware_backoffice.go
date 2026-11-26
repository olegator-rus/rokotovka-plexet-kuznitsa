package handlers

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"strings"

	"kuznica/api/pkg/errs"
	"kuznica/api/services"

	"github.com/gofiber/fiber/v2"
)

// Вход в бэк-офис (ревью B2, вход по ролям —). Origin защищает от CSRF, но не от curl: заголовок
// подделывает любой не-браузерный клиент. Поэтому всё, кроме витрины и входа, требует одного из:
//   - сессии сотрудника (cookie входа, auth.go): ecom | product | content | head;
//   - Authorization: Bearer <BACKOFFICE_TOKEN> — скрипты и CLI (роль system, полный доступ); basic-auth с паролем =
//     токен тоже принимается (если на стенде включён basic_auth Caddy, deploy/Caddyfile);
//   - AUTH_REQUIRED=off без BACKOFFICE_TOKEN (только development) — бэк-офис открыт, как до входа по ролям.
// Покупателю бэк-офис — 403, гостю — 401 (браузер уходит на /login; окна пароля браузера нет: вызов — Bearer).
//
// Без входа (витрина и вход): GET /api/health, /api/meta, /api/suggest, /api/evaluations и /api/evaluations/commerce
// (сводные метрики вкладки «До / После»), /api/companions, /api/auth/config, /api/auth/me и POST /api/search,
// /api/events, /api/quotes, /api/auth/*. Остальное — по умолчанию закрыто: новый маршрут без записи здесь попадёт под
// вход, а не наружу. Права на решения (кто принимает какое предложение, откаты, маржа) — в обработчиках решений.

// MsgBackofficeAuth — запрос в бэк-офис без входа или с неверным токеном.
const MsgBackofficeAuth = "Нужен вход сотрудника: войдите на сайте или передайте токен (BACKOFFICE_TOKEN)"

// backofficeRealm — вызов на 401: Bearer, а не Basic — иначе браузер покажет своё окно пароля поверх сайта.
const backofficeRealm = `Bearer realm="PLEXeT SearchOps", charset="UTF-8"`

// publicRoutes — маршруты витрины покупателя и входа: метод + путь.
var publicRoutes = map[string]bool{
	fiber.MethodGet + " /api/health":      true,
	fiber.MethodGet + " /api/meta":        true,
	fiber.MethodGet + " /api/suggest":     true,
	fiber.MethodGet + " /api/evaluations": true,
	// Коммерческая лестница C0–C3 (вкладка «Коммерция» в «До / После»): сводные метрики, без цифр маржи позиций.
	fiber.MethodGet + " /api/evaluations/commerce": true,
	// Контур доводки: вклад компонентов (вкладка «Доказательства → Вклад компонентов»): свой набор запросов, без журнала.
	fiber.MethodGet + " /api/evaluations/contribution": true,
	// Хвосты контура доводки: сводка стоимости для гостя («Доказательства → Стоимость») — без журнала вызовов.
	fiber.MethodGet + " /api/costs/summary": true,
	fiber.MethodPost + " /api/search":       true,
	fiber.MethodPost + " /api/events":       true,
	fiber.MethodPost + " /api/quotes":       true,
	// «К этой позиции» в спецификации: компаньоны из справочника, без цифр маржи.
	fiber.MethodGet + " /api/companions": true,
	fiber.MethodGet + " /api/help":       true,
	// Контур выдачи: рекомендации панели поиска (витрина).
	fiber.MethodGet + " /api/suggest/recommendations": true,
	// Вход по ролям; свои КП — только вошедшему покупателю (проверяет обработчик).
	fiber.MethodGet + " /api/quotes/mine":       true,
	fiber.MethodGet + " /api/auth/config":       true,
	fiber.MethodGet + " /api/auth/me":           true,
	fiber.MethodPost + " /api/auth/login":       true,
	fiber.MethodPost + " /api/auth/demo-login":  true,
	fiber.MethodPost + " /api/auth/demo-switch": true,
	fiber.MethodPost + " /api/auth/logout":      true,
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

// Backoffice — вход в бэк-офис: сотрудник, токен или открытый режим development (роль — Identify, auth.go).
func (h *Handler) Backoffice(c *fiber.Ctx) error {
	if isPublic(c.Method(), c.Path()) || services.IsStaffRole(actorRole(c)) {
		return c.Next()
	}
	if actorRole(c) == services.RoleBuyer {
		return errs.NewForbiddenError(MsgStaffOnly)
	}
	c.Set(fiber.HeaderWWWAuthenticate, backofficeRealm)
	return errs.NewUnauthorizedError(MsgBackofficeAuth)
}

// backofficeAuthorized — запрос от сотрудника, по токену или в открытом режиме development. Для открытых
// маршрутов, которые отдают больше данных вошедшему.
func (h *Handler) backofficeAuthorized(c *fiber.Ctx) bool {
	return services.IsStaffRole(actorRole(c))
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
