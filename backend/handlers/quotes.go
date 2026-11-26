package handlers

import (
	"kuznica/api/in"
	"kuznica/api/pkg/errs"
	"kuznica/api/services"

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
	req.ClientID = h.storefrontClient(c, req.ClientID)
	res, err := h.services.CreateQuote(c.UserContext(), req)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(res)
}

// MsgBuyerOnly — «свои КП» есть только у вошедшего покупателя.
const MsgBuyerOnly = "Свои запросы КП видит вошедший покупатель"

// MyQuotes — GET /api/quotes/mine: запросы КП покупателя; клиент — из сессии входа.
func (h *Handler) MyQuotes(c *fiber.Ctx) error {
	v := currentViewer(c)
	if v == nil {
		return errs.NewUnauthorizedError(MsgNeedLogin)
	}
	if v.Role != services.RoleBuyer {
		return errs.NewForbiddenError(MsgBuyerOnly)
	}
	res, err := h.services.ClientQuotes(c.UserContext(), v.Client)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Quotes — GET /api/quotes: последние запросы КП (бэк-офис).
func (h *Handler) Quotes(c *fiber.Ctx) error {
	res, err := h.services.Quotes(c.UserContext())
	if err != nil {
		return err
	}
	return c.JSON(res)
}
