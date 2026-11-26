package handlers

import (
	"kuznica/api/in"

	"github.com/gofiber/fiber/v2"
)

// Версии правил поиска.

// Configs — GET /api/configs: версии новые сверху, действующая, разница.
func (h *Handler) Configs(c *fiber.Ctx) error {
	res, err := h.services.Configs(c.UserContext())
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// RollbackConfig — POST /api/configs/rollback {"to_version","comment"} → новая версия = копия выбранной.
func (h *Handler) RollbackConfig(c *fiber.Ctx) error {
	var req in.RollbackRequest
	if err := bindBody(c, &req); err != nil {
		return err
	}
	res, err := h.services.RollbackConfig(c.UserContext(), req)
	if err != nil {
		return err
	}
	return c.JSON(res)
}
