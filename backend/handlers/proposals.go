package handlers

import (
	"kuznica/api/in"
	"kuznica/api/services"

	"github.com/gofiber/fiber/v2"
)

// Очередь предложений агента.

// Proposals — GET /api/proposals?status=&role=.
func (h *Handler) Proposals(c *fiber.Ctx) error {
	res, err := h.services.Proposals(c.UserContext(), c.Query("status"), c.Query("role"), c.Query("cause"))
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Proposal — GET /api/proposals/{id}.
func (h *Handler) Proposal(c *fiber.Ctx) error {
	res, err := h.services.Proposal(c.UserContext(), c.Params("id"))
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// ApproveProposal — POST /api/proposals/{id}/approve {"comment"}: правка → новая версия правил; задача —
// «взята в работу».
func (h *Handler) ApproveProposal(c *fiber.Ctx) error {
	if err := h.requireProposalRole(c); err != nil {
		return err
	}
	var req in.ApproveRequest
	if len(c.Body()) > 0 {
		if err := bindBody(c, &req); err != nil {
			return err
		}
	}
	res, err := h.services.ApproveProposal(services.WithActor(c.UserContext(), actorRole(c)), c.Params("id"), req)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// RejectProposal — POST /api/proposals/{id}/reject {"reason","comment"}: причина обязательна.
func (h *Handler) RejectProposal(c *fiber.Ctx) error {
	if err := h.requireProposalRole(c); err != nil {
		return err
	}
	var req in.RejectRequest
	if err := bindBody(c, &req); err != nil {
		return err
	}
	res, err := h.services.RejectProposal(services.WithActor(c.UserContext(), actorRole(c)), c.Params("id"), req)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// requireProposalRole — решение по предложению принимает его роль (или руководитель, или скрипт по токену):
// иначе 403 «Это решение принимает <роль>» до разбора тела и без изменения состояния.
func (h *Handler) requireProposalRole(c *fiber.Ctx) error {
	role := actorRole(c)
	if role == services.RoleHead || role == services.RoleSystem {
		return nil
	}
	owner, err := h.services.ProposalRole(c.UserContext(), c.Params("id"))
	if err != nil {
		return err
	}
	return requireDecision(c, owner)
}
