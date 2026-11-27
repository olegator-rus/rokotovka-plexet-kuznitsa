package handlers

import (
	"kuznica/api/in"
	"kuznica/api/services"

	"github.com/gofiber/fiber/v2"
)

// Search — POST /api/search: выдача и трасса.
func (h *Handler) Search(c *fiber.Ctx) error {
	var req in.SearchRequest
	if err := bindBody(c, &req); err != nil {
		return err
	}
	// Сессия — только выданная сервером (session.go); самодельная пишется как «без сессии».
	req.SessionID = h.requestSession(c, req.SessionID)
	// Клиент витрины: у покупателя — из сессии, у гостя — guest, у сотрудника — выбранный.
	req.ClientID = h.storefrontClient(c, req.ClientID)
	// Адрес покупателя — для суточной квоты разборов LLM (c.IP(): за Caddy — X-Real-IP от доверенного прокси).
	ctx := services.WithClientIP(c.UserContext(), c.IP())
	if testRun(c) {
		// Сквозные тесты: ответ как у витрины, но в журнал живого поиска не пишется.
		resp, err := h.services.SearchWith(ctx, TraceID(c), req, services.SearchOptions{NoLog: true, Storefront: true})
		if err != nil {
			return err
		}
		return c.JSON(resp)
	}
	resp, err := h.services.Search(ctx, TraceID(c), req)
	if err != nil {
		return err
	}
	return c.JSON(resp)
}

// TestRunHeader — заголовок прогона сквозных тестов (Playwright, e2e/playwright.config.ts): поиск и события такого
// запроса не пишутся в журнал живого поиска — тестовые запросы не попадают в аналитику и во вход агента.
const TestRunHeader = "X-Test-Run"

func testRun(c *fiber.Ctx) bool { return c.Get(TestRunHeader) == "e2e" }

// Meta — GET /api/meta: режим LLM, версии каталога и правил, конфигурации, клиенты.
func (h *Handler) Meta(c *fiber.Ctx) error {
	m := h.services.Meta()
	m.AdminEnabled, m.RateLimitPerMin = h.admin, h.rate
	// Сессия покупателя: действующая (X-Session с верной подписью) продлевается, иначе выдаётся новая.
	m.Session = c.Get(SessionHeader)
	if h.verifySession(m.Session) == "" {
		m.Session = h.issueSession()
	}
	m.Viewer = h.viewerOut(c)
	return c.JSON(m)
}

// Events — POST /api/events: событие поведения, 204.
func (h *Handler) Events(c *fiber.Ctx) error {
	var req in.EventRequest
	if err := bindBody(c, &req); err != nil {
		return err
	}
	req.SessionID = h.requestSession(c, req.SessionID)
	req.ClientID = h.storefrontClient(c, req.ClientID)
	if testRun(c) {
		return c.SendStatus(fiber.StatusNoContent) // событие сквозного теста в журнал не пишется
	}
	if err := h.services.RecordEvent(c.UserContext(), req); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ImportCatalog — POST /api/catalog/import: каталог из источника в индекс и базу; ответ — сводка импорта.
func (h *Handler) ImportCatalog(c *fiber.Ctx) error {
	res, err := h.services.ImportCatalog(c.UserContext())
	if err != nil {
		return err
	}
	return c.JSON(res)
}
