package handlers

import (
	"strconv"

	"kuznica/api/in"
	"kuznica/api/pkg/errs"

	"github.com/gofiber/fiber/v2"
)

// Evaluations — GET /api/evaluations?set=test: последний прогон каждой конфигурации на наборе.
func (h *Handler) Evaluations(c *fiber.Ctx) error {
	resp, err := h.services.Evaluations(c.UserContext(), c.Query("set"))
	if err != nil {
		return err
	}
	return c.JSON(resp)
}

// RunEvaluation — POST /api/evaluations/run: прогон в фоне, 202 и задание. Пустое тело — test, B0–B3.
func (h *Handler) RunEvaluation(c *fiber.Ctx) error {
	var req in.EvalRunRequest
	if len(c.Body()) > 0 {
		if err := bindBody(c, &req); err != nil {
			return err
		}
	}
	job, err := h.services.StartEvaluation(req.Set, req.Configs)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusAccepted).JSON(job)
}

// Analytics — GET /api/analytics?days=30&role=ecom: плитки, воронка, проблемные запросы.
func (h *Handler) Analytics(c *fiber.Ctx) error {
	days := 0
	if v := c.Query("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return errs.NewValidationError(errs.MsgInvalidFields).WithDetails(
				errs.FieldError{Field: "days", Code: errs.CodeInvalidFormat, Message: "Период — число дней"})
		}
		days = n
	}
	resp, err := h.services.Analytics(c.UserContext(), days, c.Query("role"))
	if err != nil {
		return err
	}
	return c.JSON(resp)
}
