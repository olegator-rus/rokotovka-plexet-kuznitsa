package handlers

import (
	"kuznica/api/pkg/errs"
	"kuznica/api/services"

	"github.com/gofiber/fiber/v2"
)

// EffectCommerce — GET /api/effect/commerce?days=&<параметры /api/effect>&<входы модели>: модель эффекта контура коммерции
//. Входы — services.EffectCommerceKeys; без параметра — замер или допущение.
func (h *Handler) EffectCommerce(c *fiber.Ctx) error {
	var bad []errs.FieldError
	req := services.EffectCommerceRequest{
		Effect: services.EffectRequest{
			Days:          queryInt(c, "days", &bad),
			SessionFactor: queryFloat(c, "session_factor", &bad),
			OrderProb:     queryFloat(c, "order_prob", &bad),
			RescueFactor:  queryFloat(c, "rescue_factor", &bad),
			AvgCheckRub:   queryFloat(c, "avg_check_rub", &bad),
		},
		Overrides: map[string]*float64{},
	}
	for _, k := range services.EffectCommerceKeys {
		if v := queryFloat(c, k, &bad); v != nil {
			req.Overrides[k] = v
		}
	}
	if len(bad) > 0 {
		return errs.NewValidationError(errs.MsgInvalidFields).WithDetails(bad...)
	}
	res, err := h.services.EffectCommerce(c.UserContext(), req)
	if err != nil {
		return err
	}
	return c.JSON(res)
}
