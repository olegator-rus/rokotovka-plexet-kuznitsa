package handlers

import "github.com/gofiber/fiber/v2"

// RegisterRoutes — вся таблица маршрутов API в одном месте.
func RegisterRoutes(app *fiber.App, h *Handler) {
	// Liveness для healthcheck compose и мониторинга: процесс жив.
	app.Get("/health", h.Health)

	// Плоскость приложения: фронт ходит сюда через тот же origin (Caddy в проде, прокси Vite в dev).
	// NoStore — на всех ответах; лимит частоты — до всего остального; Origin — на всех мутациях, до разбора
	// маршрута и тела; срок обработки — на всё, кроме потока SSE.
	api := app.Group("/api", NoStore, RateLimit(h.rate), h.Origin, RequestDeadline(APIDeadline))

	api.Get("/health", h.Health)
	api.Get("/meta", h.Meta)
	api.Post("/search", h.Search)
	api.Get("/suggest", h.Suggest)
	api.Post("/events", h.Events)
	api.Post("/catalog/import", h.ImportCatalog)
	api.Get("/costs", h.Costs)
	api.Get("/costs/export", h.CostsExport)
	api.Get("/effect", h.Effect)
	api.Get("/traces/:request_id", h.Trace)
	api.Get("/evaluations", h.Evaluations)
	api.Post("/evaluations/run", h.RunEvaluation)
	api.Get("/analytics", h.Analytics)

	// Агент SearchOps, очередь предложений, версии правил.
	api.Post("/agent/runs", h.StartAgentRun)
	api.Get("/agent/runs", h.AgentRuns)
	api.Get("/agent/runs/:id", h.AgentRun)
	api.Get("/agent/runs/:id/stream", h.AgentRunStream)
	api.Post("/agent/runs/:id/stop", h.StopAgentRun)
	api.Get("/agent/memory", h.AgentMemory)
	api.Get("/proposals", h.Proposals)
	api.Get("/proposals/:id", h.Proposal)
	api.Post("/proposals/:id/approve", h.ApproveProposal)
	api.Post("/proposals/:id/reject", h.RejectProposal)
	api.Get("/configs", h.Configs)
	api.Post("/configs/rollback", h.RollbackConfig)

	// Служебное: режим LLM без перезапуска (ADMIN_ENABLED).
	api.Post("/admin/llm-mode", h.SetLLMMode)
}
