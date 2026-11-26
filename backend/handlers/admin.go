package handlers

import (
	"context"
	"strings"
	"time"

	"kuznica/api/pkg/errs"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
)

// Защита стенда (ARCHITECTURE §8): служебные методы за флагом, лимит частоты, срок обработки.

// MsgAdminDisabled — служебные методы выключены (ADMIN_ENABLED=off).
const MsgAdminDisabled = "Служебные методы выключены на этом стенде (ADMIN_ENABLED)"

// MsgRateLimited — лимит частоты запросов с одного адреса.
const MsgRateLimited = "Слишком много запросов с вашего адреса — подождите минуту и повторите"

// llmModeRequest — POST /api/admin/llm-mode.
type llmModeRequest struct {
	Mode string `json:"mode"`
}

// SetLLMMode — POST /api/admin/llm-mode {"mode":"on|off"}: режим LLM без перезапуска. Ответ — то же, что
// meta.llm.
func (h *Handler) SetLLMMode(c *fiber.Ctx) error {
	if !h.admin {
		return errs.NewForbiddenError(MsgAdminDisabled)
	}
	var req llmModeRequest
	if err := bindBody(c, &req); err != nil {
		return err
	}
	res, err := h.services.SetLLMMode(c.UserContext(), strings.ToLower(strings.TrimSpace(req.Mode)))
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// RateLimit — лимит запросов к /api в минуту с одного адреса (c.IP(): за Caddy — X-Real-IP от доверенного
// прокси). Проверка живости и поток SSE агента не считаются. perMinute <= 0 — без лимита.
func RateLimit(perMinute int) fiber.Handler {
	if perMinute <= 0 {
		return func(c *fiber.Ctx) error { return c.Next() }
	}
	return limiter.New(limiter.Config{
		Max:        perMinute,
		Expiration: time.Minute,
		Next: func(c *fiber.Ctx) bool {
			p := c.Path()
			return p == "/api/health" || strings.HasSuffix(p, "/stream")
		},
		LimitReached: func(c *fiber.Ctx) error {
			return errs.NewTooManyError(MsgRateLimited)
		},
	})
}

// RequestDeadline — срок обработки запроса /api: база, OpenSearch и модель получают контекст со сроком,
// зависший внешний вызов не держит обработчик бесконечно. Поток SSE живёт по своему сроку (≤25 с), импорт
// каталога считает векторы минутами — у них срока нет.
func RequestDeadline(d time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if p := c.Path(); strings.HasSuffix(p, "/stream") || p == "/api/catalog/import" {
			return c.Next()
		}
		ctx, cancel := context.WithTimeout(c.UserContext(), d)
		defer cancel()
		c.SetUserContext(ctx)
		return c.Next()
	}
}

// APIDeadline — срок обработки запроса /api по умолчанию: дольше записи ответа сервера (30 с) не имеет смысла.
const APIDeadline = 25 * time.Second
