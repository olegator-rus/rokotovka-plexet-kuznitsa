package handlers

import (
	"time"

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
		BodyLimit:    MaxRequestBody,
	}
}

// MaxRequestBody — потолок тела запроса: JSON API без загрузки файлов. Появятся загрузки — поднять здесь.
const MaxRequestBody = 1 << 20

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
