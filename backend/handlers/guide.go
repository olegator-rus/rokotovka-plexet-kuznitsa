package handlers

import (
	"kuznica/api/in"
	"kuznica/api/services"

	"github.com/gofiber/fiber/v2"
)

// Режим подбора (контур подбора): витрина, без входа; Origin и лимит частоты — общие для /api.

// Guide — POST /api/guide: ход подбора.
func (h *Handler) Guide(c *fiber.Ctx) error {
	var req in.GuideRequest
	if err := bindBody(c, &req); err != nil {
		return err
	}
	req.SessionID = h.requestSession(c, req.SessionID)
	// Адрес покупателя — для суточной квоты модели (как у поиска).
	res, err := h.services.Guide(services.WithClientIP(c.UserContext(), c.IP()), TraceID(c), req)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// GuideRoute — POST /api/guide/route: состояние строки поиска (search | search_offer | guide).
func (h *Handler) GuideRoute(c *fiber.Ctx) error {
	var req in.GuideRouteRequest
	if err := bindBody(c, &req); err != nil {
		return err
	}
	return c.JSON(h.services.RouteGuide(c.UserContext(), req.Query))
}

// GuideSpec — POST /api/guide/spec: строка подбора в черновик спецификации (ключ операции, версия черновика).
func (h *Handler) GuideSpec(c *fiber.Ctx) error {
	var req in.GuideSpecRequest
	if err := bindBody(c, &req); err != nil {
		return err
	}
	res, err := h.services.UpdateGuideSpec(c.UserContext(), req)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// GuideGaps — GET /api/guide/gaps?days=: пробелы подбора для отчёта SearchOps (бэк-офис).
func (h *Handler) GuideGaps(c *fiber.Ctx) error {
	res, err := h.services.GuideGaps(c.UserContext(), c.QueryInt("days", 7))
	if err != nil {
		return err
	}
	return c.JSON(res)
}
