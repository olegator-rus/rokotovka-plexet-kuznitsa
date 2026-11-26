package handlers

import (
	"strconv"

	"kuznica/api/pkg/errs"

	"github.com/gofiber/fiber/v2"
)

// ProductCompanions — GET /api/companions?product_id=&quantity=: компаньоны к позиции спецификации (витрина).
func (h *Handler) ProductCompanions(c *fiber.Ctx) error {
	qty := 0
	if v := c.Query("quantity"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return errs.NewValidationError(errs.MsgInvalidQuery).WithDetails(errs.FieldError{Field: "quantity",
				Code: errs.CodeInvalidFormat, Message: "Количество — целое число"})
		}
		qty = n
	}
	res, err := h.services.ProductCompanions(c.UserContext(), c.Query("product_id"), qty)
	if err != nil {
		return err
	}
	return c.JSON(res)
}
