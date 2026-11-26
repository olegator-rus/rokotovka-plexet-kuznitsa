package handlers

import (
	"time"

	"kuznica/api/out"

	"github.com/gofiber/fiber/v2"
)

// Health — GET /health и GET /api/health. Всегда 200, раз процесс ответил; лежащие база, поиск и
// эмбеддер видны в полях, а не статусом. Причин отказа и деталей окружения наружу нет.
func (h *Handler) Health(c *fiber.Ctx) error {
	c.Set(fiber.HeaderCacheControl, "no-store")
	ctx := c.UserContext()
	return c.JSON(out.Health{
		Status:   "ok",
		DB:       out.OkDown(h.services.DBReady(ctx)),
		Search:   out.OkDown(h.services.SearchReady(ctx)),
		Embedder: h.services.EmbedderStatus(ctx),
		Time:     h.services.Now().UTC().Format(time.RFC3339),
	})
}
