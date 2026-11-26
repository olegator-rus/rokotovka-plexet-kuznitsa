package handlers

import (
	"kuznica/api/pkg/errs"

	"github.com/gofiber/fiber/v2"
	"github.com/oklog/ulid/v2"
)

// requestIDKey — ключ номера запроса в c.Locals. Пишет RequestID, читает TraceID; держим их
// рядом, иначе связь живёт только в совпадении строки.
const requestIDKey = "requestid"

// RequestID — номер запроса для логов и поля traceId в ответе об ошибке.
// Входящий X-Request-Id ИГНОРИРУЕМ и всегда генерируем свой: иначе клиент задаёт номер сам —
// любой длины, повторяющийся, чужой, — и номер перестаёт быть зацепкой в логах. Штатный
// requestid-мидлвар доверяет входящему, поэтому свой.
func RequestID(c *fiber.Ctx) error {
	id := ulid.Make().String()
	c.Locals(requestIDKey, id)
	c.Set("X-Request-Id", id)
	return c.Next()
}

// TraceID — номер запроса, проставленный RequestID (пусто, если мидлвар не отработал).
func TraceID(c *fiber.Ctx) string {
	s, _ := c.Locals(requestIDKey).(string)
	return s
}

// NoStore — на каждом ответе /api: ответы не кешируются ни браузером, ни прокси, а тип содержимого
// браузер не угадывает.
func NoStore(c *fiber.Ctx) error {
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set(fiber.HeaderXContentTypeOptions, "nosniff")
	return c.Next()
}

// MsgOrigin — отказ мутации с чужого Origin.
const MsgOrigin = "Запрос пришёл не со страницы магазина"

// Origin — изменяющие запросы (всё, кроме GET, HEAD и OPTIONS) принимаются только с
// Origin = PUBLIC_ORIGIN: защита от CSRF, когда появятся cookie (корзина, вход). Пустой Origin и
// "null" (sandbox, file://) — отказ всегда.
func (h *Handler) Origin(c *fiber.Ctx) error {
	switch c.Method() {
	case fiber.MethodGet, fiber.MethodHead, fiber.MethodOptions:
		return c.Next()
	}
	if o := c.Get(fiber.HeaderOrigin); o == "" || o == "null" || o != h.origin {
		return errs.NewForbiddenError(MsgOrigin)
	}
	return c.Next()
}
