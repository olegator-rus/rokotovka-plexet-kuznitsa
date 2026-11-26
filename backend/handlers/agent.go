package handlers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"kuznica/api/in"
	"kuznica/api/services"

	"github.com/gofiber/fiber/v2"
)

// Агент SearchOps: запуск, список, запуск с шагами, лента SSE, остановка, память.

// StartAgentRun — POST /api/agent/runs → 202 {"run_id"}. Пустое тело — 30 дней, 5 кластеров.
func (h *Handler) StartAgentRun(c *fiber.Ctx) error {
	var req in.AgentRunRequest
	if len(c.Body()) > 0 {
		if err := bindBody(c, &req); err != nil {
			return err
		}
	}
	res, err := h.services.StartAgentRun(c.UserContext(), req)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusAccepted).JSON(res)
}

// AgentRuns — GET /api/agent/runs?trigger=manual|schedule|spike|catalog_change.
func (h *Handler) AgentRuns(c *fiber.Ctx) error {
	res, err := h.services.AgentRuns(c.UserContext(), c.Query("trigger"))
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// AgentRun — GET /api/agent/runs/{id}.
func (h *Handler) AgentRun(c *fiber.Ctx) error {
	res, err := h.services.AgentRun(c.UserContext(), c.Params("id"))
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// StopAgentRun — POST /api/agent/runs/{id}/stop → 202.
func (h *Handler) StopAgentRun(c *fiber.Ctx) error {
	if err := h.services.StopAgentRun(c.UserContext(), c.Params("id")); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusAccepted)
}

// AgentMemory — GET /api/agent/memory: без layer — решения прежней формой; ?layer=decisions|suppress|episodes —
// слой памяти контура автономии.
func (h *Handler) AgentMemory(c *fiber.Ctx) error {
	if layer := c.Query("layer"); layer != "" {
		res, err := h.services.AgentMemoryLayer(c.UserContext(), layer)
		if err != nil {
			return err
		}
		return c.JSON(res)
	}
	res, err := h.services.AgentMemory(c.UserContext())
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// RemoveSuppress — POST /api/agent/memory/suppress/{id}/remove {"reason"}: снять запись «не предлагать» (владелец
// причины, руководитель или служебный доступ); это тоже решение.
func (h *Handler) RemoveSuppress(c *fiber.Ctx) error {
	role := actorRole(c)
	if role != services.RoleHead && role != services.RoleSystem {
		owner, err := h.services.SuppressOwner(c.UserContext(), c.Params("id"))
		if err != nil {
			return err
		}
		if err := requireDecision(c, owner); err != nil {
			return err
		}
	}
	var req in.SuppressRemoveRequest
	if err := bindBody(c, &req); err != nil {
		return err
	}
	res, err := h.services.RemoveSuppress(c.UserContext(), c.Params("id"), req, role)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// AgentSettings — GET /api/agent/settings: режим агента, расписание, пороги триггеров, пул, бюджет.
func (h *Handler) AgentSettings(c *fiber.Ctx) error {
	return c.JSON(h.services.AgentSettings(c.UserContext()))
}

// SetAgentSettings — POST /api/agent/settings {"read_only": bool}: режим «только чтение» (руководитель e-com).
func (h *Handler) SetAgentSettings(c *fiber.Ctx) error {
	if err := requireDecision(c, services.RoleHead); err != nil {
		return err
	}
	var req in.AgentSettingsRequest
	if err := bindBody(c, &req); err != nil {
		return err
	}
	res, err := h.services.SetAgentSettings(c.UserContext(), req)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// sseWindow — сколько держать один поток: срок записи сервера ограничен (DefaultTimeouts.Write), поэтому
// поток закрывается раньше, а EventSource переподключается сам с Last-Event-ID и получает шаги после него.
const sseWindow = 25 * time.Second

// AgentRunStream — GET /api/agent/runs/{id}/stream: SSE. event: step (data: Step, id: seq), event: status
// (data: Run), event: done (data: Run). Переподключение — заголовок Last-Event-ID или ?after=seq.
func (h *Handler) AgentRunStream(c *fiber.Ctx) error {
	after := 0
	if v := c.Get("Last-Event-ID"); v != "" {
		after, _ = strconv.Atoi(v)
	} else if v := c.Query("after"); v != "" {
		after, _ = strconv.Atoi(v)
	}
	run, steps, ch, cancel, err := h.services.AgentStream(c.UserContext(), c.Params("id"), max(after, 0))
	if err != nil {
		return err
	}
	c.Set(fiber.HeaderContentType, "text/event-stream; charset=utf-8")
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set(fiber.HeaderConnection, "keep-alive")
	c.Set("X-Accel-Buffering", "no")
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer cancel()
		send := func(event string, id int, v any) bool {
			b, _ := json.Marshal(v)
			if id > 0 {
				fmt.Fprintf(w, "id: %d\n", id)
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
			return w.Flush() == nil
		}
		fmt.Fprint(w, "retry: 1000\n\n")
		if !send("status", 0, run) {
			return
		}
		last := after
		for i := range steps {
			if !send("step", steps[i].Seq, steps[i]) {
				return
			}
			last = max(last, steps[i].Seq)
		}
		deadline := time.NewTimer(sseWindow)
		defer deadline.Stop()
		ping := time.NewTicker(10 * time.Second)
		defer ping.Stop()
		for {
			select {
			case ev, ok := <-ch:
				if !ok {
					// Запуск закончился (или не идёт): итоговый статус и done.
					final := run
					if ev.Run != nil {
						final = ev.Run
					}
					send("done", 0, final)
					return
				}
				switch ev.Kind {
				case "step":
					// Повтор шага с тем же seq — обновление (отклонённая гипотеза): id не двигаем назад.
					id := 0
					if ev.Step.Seq > last {
						id, last = ev.Step.Seq, ev.Step.Seq
					}
					if !send("step", id, ev.Step) {
						return
					}
				case "status":
					run = ev.Run
					if !send("status", 0, ev.Run) {
						return
					}
				case "done":
					send("done", 0, ev.Run)
					return
				}
			case <-ping.C:
				fmt.Fprint(w, ": ping\n\n")
				if w.Flush() != nil {
					return
				}
			case <-deadline.C:
				return
			}
		}
	})
	return nil
}
