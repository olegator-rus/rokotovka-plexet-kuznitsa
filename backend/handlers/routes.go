package handlers

import "github.com/gofiber/fiber/v2"

// RegisterRoutes — вся таблица маршрутов API в одном месте.
func RegisterRoutes(app *fiber.App, h *Handler) {
	// Liveness для healthcheck compose и мониторинга: процесс жив.
	app.Get("/health", h.Health)

	// Плоскость приложения: фронт ходит сюда через тот же origin (Caddy в проде, прокси Vite в dev).
	// NoStore — на всех ответах; Origin — на всех мутациях, до разбора маршрута и тела.
	api := app.Group("/api", NoStore, h.Origin)

	api.Get("/health", h.Health)
}
