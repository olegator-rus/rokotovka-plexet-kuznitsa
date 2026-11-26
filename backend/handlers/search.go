package handlers

import (
	"kuznica/api/in"

	"github.com/gofiber/fiber/v2"
)

// Search — POST /api/search: выдача и трасса.
func (h *Handler) Search(c *fiber.Ctx) error {
	var req in.SearchRequest
	if err := bindBody(c, &req); err != nil {
		return err
	}
	resp, err := h.services.Search(c.UserContext(), TraceID(c), req)
	if err != nil {
		return err
	}
	return c.JSON(resp)
}

// Meta — GET /api/meta: режим LLM, версии каталога и правил, конфигурации, клиенты.
func (h *Handler) Meta(c *fiber.Ctx) error {
	m := h.services.Meta()
	m.AdminEnabled, m.RateLimitPerMin = h.admin, h.rate
	return c.JSON(m)
}

// Events — POST /api/events: событие поведения, 204.
func (h *Handler) Events(c *fiber.Ctx) error {
	var req in.EventRequest
	if err := bindBody(c, &req); err != nil {
		return err
	}
	if err := h.services.RecordEvent(c.UserContext(), req); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ImportCatalog — POST /api/catalog/import: каталог из источника в индекс и базу; ответ — сводка импорта.
func (h *Handler) ImportCatalog(c *fiber.Ctx) error {
	res, err := h.services.ImportCatalog(c.UserContext())
	if err != nil {
		return err
	}
	return c.JSON(res)
}
