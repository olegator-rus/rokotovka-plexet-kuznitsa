package handlers

import (
	"kuznica/api/in"

	"github.com/gofiber/fiber/v2"
)

// CreateQuote — POST /api/quotes: запрос КП по спецификации, 201 (витрина: без входа; Origin и лимит частоты —
// общие для /api).
func (h *Handler) CreateQuote(c *fiber.Ctx) error {
	var req in.QuoteRequest
	if err := bindBody(c, &req); err != nil {
		return err
	}
	req.SessionID = h.requestSession(c, req.SessionID)
	res, err := h.services.CreateQuote(c.UserContext(), req)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(res)
}

// Quotes — GET /api/quotes: последние запросы КП (бэк-офис).
func (h *Handler) Quotes(c *fiber.Ctx) error {
	res, err := h.services.Quotes(c.UserContext())
	if err != nil {
		return err
	}
	return c.JSON(res)
}
