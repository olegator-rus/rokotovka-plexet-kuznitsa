package handlers

import (
	"github.com/gofiber/fiber/v2"
)

// Product — GET /api/products/:id: страница товара (витрина, без входа). Ответ одинаков для всех
// ролей: ветки по роли нет.
func (h *Handler) Product(c *fiber.Ctx) error {
	res, err := h.services.Product(c.UserContext(), c.Params("id"))
	if err != nil {
		return err
	}
	return c.JSON(res)
}
