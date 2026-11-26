package handlers

import "github.com/gofiber/fiber/v2"

// Help — GET /api/help (витрина): статьи справки магазина для страницы помощи (контур доводки).
func (h *Handler) Help(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"items": h.services.HelpArticles()})
}
