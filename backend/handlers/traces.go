package handlers

import "github.com/gofiber/fiber/v2"

// Trace — GET /api/traces/:request_id: сохранённая трасса поиска и исходный запрос.
func (h *Handler) Trace(c *fiber.Ctx) error {
	res, err := h.services.Trace(c.UserContext(), c.Params("request_id"))
	if err != nil {
		return err
	}
	return c.JSON(res)
}
