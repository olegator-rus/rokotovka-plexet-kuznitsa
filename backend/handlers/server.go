package handlers

import (
	"strings"
	"time"

	"kuznica/api/services"

	"github.com/gofiber/fiber/v2"
)

// Timeouts — сроки HTTP-сервера API. Без них медленный или брошенный клиент держит соединение
// сколько угодно.
type Timeouts struct {
	Read  time.Duration // чтение запроса целиком (заголовки и тело), не работа обработчика
	Write time.Duration // отправка ответа; отсчёт — после обработчика
	Idle  time.Duration // ожидание следующего запроса в keep-alive
}

var DefaultTimeouts = Timeouts{Read: 15 * time.Second, Write: 30 * time.Second, Idle: 60 * time.Second}

// ServerConfig — конфигурация fiber для API: единый конверт ошибок и сроки соединений.
func ServerConfig(t Timeouts) fiber.Config {
	return fiber.Config{
		ErrorHandler: ErrorHandler,
		ReadTimeout:  t.Read,
		WriteTimeout: t.Write,
		IdleTimeout:  t.Idle,
		// Потолок тела на уровне сервера — для загрузки ТЗ (20 МиБ); остальным маршрутам LimitBody оставляет 1 МиБ.
		BodyLimit: MaxUploadBody,
	}
}

// MaxRequestBody — потолок тела запроса для всех маршрутов, кроме загрузок (uploadRoutes).
const MaxRequestBody = 1 << 20

// MaxUploadBody — потолок тела загрузки файла ТЗ (POST /api/tz): файл до 20 МиБ и обёртка multipart.
const MaxUploadBody = services.TZMaxFileBytes + 64<<10

// uploadRoutes — маршруты с телом больше MaxRequestBody: метод + путь.
var uploadRoutes = map[string]bool{fiber.MethodPost + " /api/tz": true}

// LimitBody — 413 для тела больше MaxRequestBody на всех маршрутах, кроме uploadRoutes. Сервер принимает до
// MaxUploadBody (fiber.Config.BodyLimit один на приложение), поэтому общий потолок держит этот мидлвар:
// по Content-Length, а без него (chunked) — по прочитанному телу.
func LimitBody(c *fiber.Ctx) error {
	if uploadRoutes[c.Method()+" "+strings.TrimRight(c.Path(), "/")] {
		return c.Next()
	}
	if n := c.Request().Header.ContentLength(); n > MaxRequestBody || len(c.Request().Body()) > MaxRequestBody {
		return fiber.ErrRequestEntityTooLarge
	}
	return c.Next()
}

// ProxyHeader — заголовок с адресом клиента, который ставит Caddy (deploy/Caddyfile:
// header_up X-Real-IP {remote_host}). Один адрес, не цепочка: X-Forwarded-For клиент может дописать
// сам, а X-Real-IP Caddy перезаписывает целиком.
const ProxyHeader = "X-Real-IP"

// WithTrustedProxies — c.IP() берёт адрес клиента из X-Real-IP, но только если соединение пришло с
// адреса из списка. Без этого за Caddy у всех клиентов был бы один адрес — адрес самого Caddy.
//
// Пустой список — конфигурация не меняется: в Fiber v2 при выключенной проверке прокси ProxyHeader
// доверяется ЛЮБОМУ отправителю, и заголовок подделал бы кто угодно. EnableIPValidation: без
// заголовка или с мусором в нём c.IP() возвращает адрес соединения, а не "".
func WithTrustedProxies(cfg fiber.Config, proxies []string) fiber.Config {
	if len(proxies) == 0 {
		return cfg
	}
	cfg.ProxyHeader = ProxyHeader
	cfg.EnableTrustedProxyCheck = true
	cfg.TrustedProxies = proxies
	cfg.EnableIPValidation = true
	return cfg
}
