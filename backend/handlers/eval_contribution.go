package handlers

import (
	"strings"

	"kuznica/api/pkg/errs"

	"github.com/gofiber/fiber/v2"
)

// Вклад компонентов и режимы без моделей (контур доводки).

// Contribution — GET /api/evaluations/contribution?set=test: последний прогон таблицы вклада (открыто, вкладка
// «Доказательства → Вклад компонентов»): сводные метрики и тексты запросов собственного набора, без журнала.
func (h *Handler) Contribution(c *fiber.Ctx) error {
	res, err := h.services.Contribution(c.UserContext(), c.Query("set"))
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// contributionRunRequest — POST /api/evaluations/contribution/run.
type contributionRunRequest struct {
	Set string `json:"set"`
}

// RunContribution — POST /api/evaluations/contribution/run: прогон в фоне, 202 и задание (бэк-офис).
func (h *Handler) RunContribution(c *fiber.Ctx) error {
	var req contributionRunRequest
	if len(c.Body()) > 0 {
		if err := bindBody(c, &req); err != nil {
			return err
		}
	}
	job, err := h.services.StartContribution(req.Set)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusAccepted).JSON(job)
}

// SetVectorsMode — POST /api/admin/vectors-mode {"mode":"on|off"}: эмбеддер и реранкер без перезапуска.
func (h *Handler) SetVectorsMode(c *fiber.Ctx) error {
	if !h.admin {
		return errs.NewForbiddenError(MsgAdminDisabled)
	}
	var req llmModeRequest
	if err := bindBody(c, &req); err != nil {
		return err
	}
	res, err := h.services.SetVectorsMode(strings.ToLower(strings.TrimSpace(req.Mode)))
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// SetOfflineMode — POST /api/admin/offline-mode {"mode":"on|off"}: одна настройка «без моделей» (LLM и векторы).
func (h *Handler) SetOfflineMode(c *fiber.Ctx) error {
	if !h.admin {
		return errs.NewForbiddenError(MsgAdminDisabled)
	}
	var req llmModeRequest
	if err := bindBody(c, &req); err != nil {
		return err
	}
	res, err := h.services.SetOfflineMode(c.UserContext(), strings.ToLower(strings.TrimSpace(req.Mode)))
	if err != nil {
		return err
	}
	return c.JSON(res)
}
