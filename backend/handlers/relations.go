package handlers

import (
	"strings"

	"kuznica/api/in"

	"github.com/gofiber/fiber/v2"
)

// Справочник связей и коммерческая лестница (контур коммерции): бэк-офис.

// Relations — GET /api/relations?family=&status=&type=&source=.
func (h *Handler) Relations(c *fiber.Ctx) error {
	q := in.RelationsQuery{Family: strings.TrimSpace(c.Query("family")), Status: strings.TrimSpace(c.Query("status")),
		Type: strings.TrimSpace(c.Query("type")), Source: strings.TrimSpace(c.Query("source"))}
	res, err := h.services.Relations(c.UserContext(), q)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// RelationVersions — GET /api/relations/versions.
func (h *Handler) RelationVersions(c *fiber.Ctx) error {
	res, err := h.services.RelationVersions(c.UserContext())
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// RollbackRelations — POST /api/relations/rollback {"to_version","comment"} → новая версия = копия выбранной.
func (h *Handler) RollbackRelations(c *fiber.Ctx) error {
	var req in.RollbackRequest
	if err := bindBody(c, &req); err != nil {
		return err
	}
	res, err := h.services.RollbackRelations(c.UserContext(), req)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// CommerceEvaluation — GET /api/evaluations/commerce?set=p2: последний прогон коммерческой лестницы C0–C3.
func (h *Handler) CommerceEvaluation(c *fiber.Ctx) error {
	res, err := h.services.CommerceEvaluation(c.UserContext(), strings.TrimSpace(c.Query("set")))
	if err != nil {
		return err
	}
	return c.JSON(res)
}
