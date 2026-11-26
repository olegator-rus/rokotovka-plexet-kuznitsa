package handlers

import (
	"strconv"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"

	"github.com/gofiber/fiber/v2"
)

// Evaluations — GET /api/evaluations?set=test: последний прогон каждой конфигурации на наборе. Маршрут открыт
// витрине («До / После»), но реплей отложенной недели собран из журнала поисков: без входа в бэк-офис его
// запросы отдаются без текстов и id (в id — клиент), только агрегаты и число изменений (ревью-2 n7).
func (h *Handler) Evaluations(c *fiber.Ctx) error {
	resp, err := h.services.Evaluations(c.UserContext(), c.Query("set"))
	if err != nil {
		return err
	}
	if resp.Set == domain.EvalSetHoldoutWeek && !h.backofficeAuthorized(c) {
		redactEvalTexts(resp)
	}
	return c.JSON(resp)
}

// redactEvalTexts — убрать тексты поисков и id запросов из изменений по запросам; метрики и счёт остаются.
func redactEvalTexts(resp *out.EvaluationsResponse) {
	for i := range resp.Configs {
		for _, list := range [][]out.EvalChange{resp.Configs[i].Regressions, resp.Configs[i].Improvements} {
			for j := range list {
				list[j].QueryID, list[j].Text = "", ""
			}
		}
	}
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

// AnalyticsCurrent — GET /api/analytics/current?days=&limit=: текущая выдача по проблемным запросам периода
// (поиск без записи в журнал на действующих правилах).
func (h *Handler) AnalyticsCurrent(c *fiber.Ctx) error {
	var bad []errs.FieldError
	days, limit := queryInt(c, "days", &bad), queryInt(c, "limit", &bad)
	if len(bad) > 0 {
		return errs.NewValidationError(errs.MsgInvalidFields).WithDetails(bad...)
	}
	resp, err := h.services.ProblemsNow(c.UserContext(), days, limit)
	if err != nil {
		return err
	}
	return c.JSON(resp)
}
