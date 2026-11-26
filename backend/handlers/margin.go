package handlers

import (
	"kuznica/api/in"
	"kuznica/api/services"

	"github.com/gofiber/fiber/v2"
)

// Политика маржи (бэк-офис): рычаг, симуляция, применение и откат.

// MarginPolicy — GET /api/margin/policy.
func (h *Handler) MarginPolicy(c *fiber.Ctx) error {
	res, err := h.services.MarginPolicy(c.UserContext())
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// ApplyMarginPolicy — POST /api/margin/policy: новое положение рычага — новая версия.
func (h *Handler) ApplyMarginPolicy(c *fiber.Ctx) error {
	if err := requireDecision(c, services.AreaMargin); err != nil {
		return err
	}
	var req in.MarginPolicyRequest
	if err := bindBody(c, &req); err != nil {
		return err
	}
	res, err := h.services.ApplyMarginPolicy(c.UserContext(), req)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(res)
}

// RollbackMarginPolicy — POST /api/margin/policy/rollback.
func (h *Handler) RollbackMarginPolicy(c *fiber.Ctx) error {
	if err := requireDecision(c, services.AreaMargin); err != nil {
		return err
	}
	var req in.MarginRollbackRequest
	if err := bindBody(c, &req); err != nil {
		return err
	}
	res, err := h.services.RollbackMarginPolicy(c.UserContext(), req)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(res)
}

// SimulateMargin — POST /api/margin/simulate: положения рычага на размеченном наборе (tune по умолчанию).
func (h *Handler) SimulateMargin(c *fiber.Ctx) error {
	var req in.MarginSimulateRequest
	if len(c.Body()) > 0 {
		if err := bindBody(c, &req); err != nil {
			return err
		}
	}
	res, err := h.services.SimulateMargin(c.UserContext(), req)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// MarginSimulationStatus — GET /api/margin/simulate/status: прогресс сбора базовой выдачи.
func (h *Handler) MarginSimulationStatus(c *fiber.Ctx) error {
	return c.JSON(h.services.MarginSimulationStatus())
}
