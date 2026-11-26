package handlers

import (
	"strconv"
	"strings"

	"kuznica/api/pkg/errs"
	"kuznica/api/services"

	"github.com/gofiber/fiber/v2"
)

// Costs — GET /api/costs?limit=&offset=&server_rub_20k=&server_rub_200k=&agent_runs_month=&agent_runs_month_200k=: стоимость по
// маршрутам, кэш, журнал вызовов модели, месячные сценарии.
func (h *Handler) Costs(c *fiber.Ctx) error {
	var bad []errs.FieldError
	limit := queryInt(c, "limit", &bad)
	offset := queryInt(c, "offset", &bad)
	monthly := monthlyParams(c, &bad)
	if len(bad) > 0 {
		return errs.NewValidationError(errs.MsgInvalidFields).WithDetails(bad...)
	}
	res, err := h.services.Costs(c.UserContext(), services.CostsRequest{Limit: limit, Offset: offset, Monthly: monthly})
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// CostsSummary — GET /api/costs/summary?server_rub_20k=&…: открытая сводка стоимости (гость, «Доказательства →
// Стоимость»): цена запроса по маршрутам и средняя, доля без LLM, кэш, месячные сценарии — без журнала вызовов.
func (h *Handler) CostsSummary(c *fiber.Ctx) error {
	var bad []errs.FieldError
	monthly := monthlyParams(c, &bad)
	if len(bad) > 0 {
		return errs.NewValidationError(errs.MsgInvalidFields).WithDetails(bad...)
	}
	res, err := h.services.Costs(c.UserContext(), services.CostsRequest{Monthly: monthly, NoCalls: true})
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// CostsExport — GET /api/costs/export: данные расчёта стоимости (замеры, допущения, сценарии, прайс) в CSV
// для шаблона организатора. Допущения — те же параметры, что у /api/costs.
func (h *Handler) CostsExport(c *fiber.Ctx) error {
	var bad []errs.FieldError
	monthly := monthlyParams(c, &bad)
	if len(bad) > 0 {
		return errs.NewValidationError(errs.MsgInvalidFields).WithDetails(bad...)
	}
	raw, err := h.services.CostsExport(c.UserContext(), monthly)
	if err != nil {
		return err
	}
	c.Set(fiber.HeaderContentType, "text/csv; charset=utf-8")
	c.Set(fiber.HeaderContentDisposition, `attachment; filename="plexet-costs.csv"`)
	return c.Send(raw)
}

func monthlyParams(c *fiber.Ctx, bad *[]errs.FieldError) services.MonthlyParams {
	return services.MonthlyParams{
		ServerRub20k:       queryFloat(c, "server_rub_20k", bad),
		ServerRub200k:      queryFloat(c, "server_rub_200k", bad),
		AgentRunsMonth:     queryFloat(c, "agent_runs_month", bad),
		AgentRunsMonth200k: queryFloat(c, "agent_runs_month_200k", bad),
	}
}

// queryInt — целый параметр строки запроса; нет параметра — 0.
func queryInt(c *fiber.Ctx, name string, bad *[]errs.FieldError) int {
	raw := c.Query(name)
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		*bad = append(*bad, errs.FieldError{Field: name, Code: errs.CodeInvalidFormat, Message: "Ожидается целое число"})
	}
	return n
}

// queryFloat — дробный параметр (точка или запятая); нет параметра — nil.
func queryFloat(c *fiber.Ctx, name string, bad *[]errs.FieldError) *float64 {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return nil
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(raw, ",", "."), 64)
	if err != nil {
		*bad = append(*bad, errs.FieldError{Field: name, Code: errs.CodeInvalidFormat, Message: "Ожидается число"})
		return nil
	}
	return &v
}
