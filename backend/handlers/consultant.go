package handlers

import (
	"bufio"
	"context"
	"encoding/json"

	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/services"

	"github.com/gofiber/fiber/v2"
)

// Консультант (контур консультанта): витрина, без входа. Ошибка валидации — обычный
// конверт до потока; дальше — поток application/x-ndjson, по строке JSON на событие.

// Consultant — POST /api/consultant: ход консультанта потоком событий.
func (h *Handler) Consultant(c *fiber.Ctx) error {
	var req in.ConsultantRequest
	if err := bindBody(c, &req); err != nil {
		return err
	}
	req.SessionID = h.requestSession(c, req.SessionID)
	if err := h.services.ValidateConsultant(&req); err != nil {
		return err
	}
	ctx := services.WithClientIP(context.WithoutCancel(c.UserContext()), c.IP())
	traceID := TraceID(c)
	c.Set(fiber.HeaderContentType, "application/x-ndjson; charset=utf-8")
	c.Set(fiber.HeaderCacheControl, "no-cache")
	c.Set("X-Accel-Buffering", "no")
	c.Status(fiber.StatusOK)
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		h.services.Consult(ctx, traceID, req, func(ev out.ConsultantEvent) {
			_ = enc.Encode(ev)
			_ = w.Flush()
		})
	})
	return nil
}
