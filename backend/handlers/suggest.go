package handlers

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"kuznica/api/pkg/errs"
	"kuznica/api/services"

	"github.com/gofiber/fiber/v2"
)

// Suggest — GET /api/suggest?q=&intent_id=&session_id=&limit=&client_id=: прежние groups (не больше 8 строк) и
// секции панели контура выдачи. Номер ответа (X-Request-Id) — id трассы панели в GET /api/traces/{id}.
// Клиент в общий кэш не попадает: товары панели от клиента не зависят.
func (h *Handler) Suggest(c *fiber.Ctx) error {
	if !suggestLimit.allow(c.IP(), time.Now()) {
		return errs.NewTooManyError(MsgSuggestRate)
	}
	limit := 0
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return errs.NewValidationError(errs.MsgInvalidFields).WithDetails(errs.FieldError{Field: "limit",
				Code: errs.CodeInvalidFormat, Message: "Лимит — число"})
		}
		limit = n
	}
	resp, err := h.services.SuggestPanel(c.UserContext(), TraceID(c), services.SuggestRequest{
		Q: c.Query("q"), IntentID: c.Query("intent_id"), SessionID: h.requestSession(c, c.Query("session_id")),
		ClientID: h.storefrontClient(c, c.Query("client_id")), Limit: limit,
	}, services.SuggestOptions{})
	if err != nil {
		return err
	}
	return c.JSON(resp)
}

// SuggestRecommendations — GET /api/suggest/recommendations?session_id=&intent_id=&spec=pid1,pid2&exact=&viewed=&category=&q=
//: блок рекомендаций панели; оснований нет — null.
func (h *Handler) SuggestRecommendations(c *fiber.Ctx) error {
	var spec []string
	for _, p := range strings.Split(c.Query("spec"), ",") {
		if p = strings.TrimSpace(p); p != "" {
			spec = append(spec, p)
		}
	}
	res, err := h.services.SuggestRecommendations(c.UserContext(), services.RecsRequest{
		SessionID: h.requestSession(c, c.Query("session_id")), IntentID: c.Query("intent_id"), Spec: spec,
		Viewed: c.Query("viewed"), Category: c.Query("category"), Q: c.Query("q"), Exact: c.Query("exact"),
	})
	if err != nil {
		return err
	}
	if res == nil {
		return c.JSON(nil)
	}
	return c.JSON(res)
}

// MsgSuggestRate — подсказки запрашиваются чаще, чем печатает человек.
const MsgSuggestRate = "Слишком частые запросы подсказок — подождите секунду"

// Лимит частоты подсказок с адреса: окно в секунду. Клиент шлёт запрос не чаще раза в 150 мс (debounce), поэтому
// 20 в секунду с адреса — запас на несколько вкладок и офис за одним NAT, но не на перебор каталога.
const (
	suggestPerSecond = 20
	suggestWindow    = time.Second
)

var suggestLimit = &rateWindow{max: suggestPerSecond, window: suggestWindow, hits: map[string]*rateSlot{}}

type rateSlot struct {
	start time.Time
	n     int
}

// rateWindow — фиксированное окно на адрес; записи старше окна вычищаются по ходу.
type rateWindow struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string]*rateSlot
	swept  time.Time
}

func (r *rateWindow) allow(ip string, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if now.Sub(r.swept) > time.Minute {
		for k, v := range r.hits {
			if now.Sub(v.start) > r.window {
				delete(r.hits, k)
			}
		}
		r.swept = now
	}
	s := r.hits[ip]
	if s == nil || now.Sub(s.start) >= r.window {
		r.hits[ip] = &rateSlot{start: now, n: 1}
		return true
	}
	s.n++
	return s.n <= r.max
}
