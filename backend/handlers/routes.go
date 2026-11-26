package handlers

import "github.com/gofiber/fiber/v2"

// RegisterRoutes — вся таблица маршрутов API в одном месте.
func RegisterRoutes(app *fiber.App, h *Handler) {
	// Liveness для healthcheck compose и мониторинга: процесс жив.
	app.Get("/health", h.Health)

	// Плоскость приложения: фронт ходит сюда через тот же origin (Caddy в проде, прокси Vite в dev).
	// NoStore — на всех ответах; лимит частоты — до всего остального; Origin — на всех мутациях, до разбора
	// маршрута и тела; кто смотрит — cookie входа и токен (auth.go); вход в бэк-офис — на всём, кроме витрины
	// покупателя и входа (middleware_backoffice.go); срок обработки — на всё, кроме потока SSE.
	api := app.Group("/api", NoStore, RateLimit(h.rate), h.Origin, h.Identify, h.Backoffice, RequestDeadline(APIDeadline))

	// Вход по ролям.
	api.Get("/auth/config", h.AuthConfig)
	api.Get("/auth/me", h.AuthMe)
	api.Post("/auth/login", h.Login)
	api.Post("/auth/demo-login", h.DemoLogin)
	api.Post("/auth/demo-switch", h.DemoSwitch)
	api.Post("/auth/logout", h.Logout)

	api.Get("/health", h.Health)
	api.Get("/meta", h.Meta)
	api.Post("/search", h.Search)
	api.Get("/suggest", h.Suggest)
	api.Get("/suggest/recommendations", h.SuggestRecommendations)
	api.Post("/events", h.Events)
	api.Post("/catalog/import", h.ImportCatalog)
	api.Get("/costs", h.Costs)
	api.Get("/costs/summary", h.CostsSummary)
	api.Get("/costs/export", h.CostsExport)
	api.Get("/traces/:request_id", h.Trace)
	api.Get("/evaluations", h.Evaluations)
	api.Post("/evaluations/run", h.RunEvaluation)
	api.Get("/analytics", h.Analytics)
	api.Get("/analytics/current", h.AnalyticsCurrent)

	// Агент SearchOps, очередь предложений, версии правил.
	api.Post("/agent/runs", h.StartAgentRun)
	api.Get("/agent/runs", h.AgentRuns)
	api.Get("/agent/runs/:id", h.AgentRun)
	api.Get("/agent/runs/:id/stream", h.AgentRunStream)
	api.Post("/agent/runs/:id/stop", h.StopAgentRun)
	api.Get("/agent/memory", h.AgentMemory)
	api.Post("/agent/memory/suppress/:id/remove", h.RemoveSuppress)
	api.Get("/agent/settings", h.AgentSettings)
	api.Post("/agent/settings", h.SetAgentSettings)
	api.Get("/proposals", h.Proposals)
	api.Get("/proposals/:id", h.Proposal)
	api.Post("/proposals/:id/approve", h.ApproveProposal)
	api.Post("/proposals/:id/reject", h.RejectProposal)
	api.Get("/configs", h.Configs)
	api.Post("/configs/rollback", h.RollbackConfig)

	// Контур коммерции: справочник связей и коммерческая лестница, бэк-офис.
	api.Get("/relations", h.Relations)
	api.Get("/relations/versions", h.RelationVersions)
	api.Post("/relations/rollback", h.RollbackRelations)
	api.Get("/evaluations/commerce", h.CommerceEvaluation)

	// Контур доводки: вклад компонентов.
	api.Get("/evaluations/contribution", h.Contribution)
	api.Post("/evaluations/contribution/run", h.RunContribution)

	// Политика маржи (контур коммерции, margin.go).
	api.Get("/margin/policy", h.MarginPolicy)
	api.Post("/margin/policy", h.ApplyMarginPolicy)
	api.Post("/margin/policy/rollback", h.RollbackMarginPolicy)
	api.Post("/margin/simulate", h.SimulateMargin)
	api.Get("/margin/simulate/status", h.MarginSimulationStatus)

	// Контур коммерции: спецификация и запрос КП; POST — витрина, GET — бэк-офис.
	api.Post("/quotes", h.CreateQuote)
	api.Get("/quotes", h.Quotes)
	api.Get("/quotes/mine", h.MyQuotes) // свои КП покупателя
	// Компаньоны к позиции спецификации (витрина).
	api.Get("/companions", h.ProductCompanions)
	// Страница товара (витрина).
	api.Get("/products/:id", h.Product)
	api.Get("/help", h.Help) // справка магазина (контур доводки)

	// Служебное: режим LLM без перезапуска (ADMIN_ENABLED).
	api.Post("/admin/llm-mode", h.SetLLMMode)
	// Контур доводки: режим без векторов и одна настройка «без моделей».
	api.Post("/admin/vectors-mode", h.SetVectorsMode)
	api.Post("/admin/offline-mode", h.SetOfflineMode)
}
