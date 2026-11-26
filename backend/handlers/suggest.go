package handlers

import "github.com/gofiber/fiber/v2"

// Suggest — GET /api/suggest?q=…&client_id=…: подсказки по группам, не больше 8 строк.
func (h *Handler) Suggest(c *fiber.Ctx) error {
	resp, err := h.services.Suggest(c.UserContext(), c.Query("q"), c.Query("client_id"))
	if err != nil {
		return err
	}
	return c.JSON(resp)
}
