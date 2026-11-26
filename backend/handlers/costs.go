package handlers

import (
	"strconv"

	"kuznica/api/pkg/errs"

	"github.com/gofiber/fiber/v2"
)

// Costs — GET /api/costs?limit=&offset=: стоимость по маршрутам, кэш, журнал вызовов модели.
func (h *Handler) Costs(c *fiber.Ctx) error {
	limit, err1 := costsQueryInt(c, "limit")
	offset, err2 := costsQueryInt(c, "offset")
	if err1 != nil || err2 != nil {
		var bad []errs.FieldError
		for field, err := range map[string]error{"limit": err1, "offset": err2} {
			if err != nil {
				bad = append(bad, errs.FieldError{Field: field, Code: errs.CodeInvalidFormat, Message: "Ожидается целое число"})
			}
		}
		return errs.NewValidationError(errs.MsgInvalidFields).WithDetails(bad...)
	}
	res, err := h.services.Costs(c.UserContext(), limit, offset)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// costsQueryInt — целый параметр строки запроса; нет параметра — 0.
func costsQueryInt(c *fiber.Ctx, name string) (int, error) {
	raw := c.Query(name)
	if raw == "" {
		return 0, nil
	}
	return strconv.Atoi(raw)
}
