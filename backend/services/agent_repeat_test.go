package services

import (
	"encoding/json"
	"strings"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/externalApi/llm"
	"kuznica/api/in"
)

// QA r2 №2: модель повторяла один и тот же отклонённый create_assortment_ticket 14 раз подряд. Повтор того же
// инструмента с теми же аргументами не выполняется, модель получает «уже отклонено», после maxRefusedRepeats
// повторов кластер закрывается с итогом repeat.
func TestAgentRefusedCallNotRepeated(t *testing.T) {
	model := &scriptModel{}
	s, ms := agentEnv(t, model)
	ticket := map[string]any{"diagnosis": "RANKING_FAILURE", "text": "стоит низко", "family_id": "kaspersky-internet-security"}
	for i := 0; i < 6; i++ {
		// Ключи в другом порядке — тот же вызов.
		if i%2 == 1 {
			raw := `{"text":"стоит низко","family_id":"kaspersky-internet-security","diagnosis":"RANKING_FAILURE"}`
			model.turns = append(model.turns, []llm.ToolCall{{Type: "function",
				Function: llm.FunctionCall{Name: "create_assortment_ticket", Arguments: raw}}})
			continue
		}
		model.turns = append(model.turns, []llm.ToolCall{tc("create_assortment_ticket", ticket)})
	}
	r, err := s.StartAgentRun(t.Context(), in.AgentRunRequest{MaxClusters: 1})
	if err != nil {
		t.Fatal(err)
	}
	run := waitRun(t, s, r.RunID)
	executed, repeats, closed := 0, 0, false
	for _, st := range ms.steps {
		if st.Tool == nil || *st.Tool != "create_assortment_ticket" {
			continue
		}
		switch {
		case st.Kind == domain.StepToolCall:
			executed++
		case st.Kind == domain.StepCheck && strings.Contains(st.Title, "повтор отклонённого вызова"):
			repeats++
			closed = closed || strings.Contains(st.Title, "группа закрыта")
		}
	}
	if executed != 1 || repeats != maxRefusedRepeats || !closed {
		t.Fatalf("ожидался 1 выполненный вызов и %d повтора с закрытием: выполнено %d, повторов %d, закрыт %v",
			maxRefusedRepeats, executed, repeats, closed)
	}
	if run.ToolCalls != 1 || run.ProposalsCreated != 0 {
		t.Fatalf("повторы не выполняются и не считаются в лимит: %+v", run)
	}
	// Модели вернулось понятное «уже отклонено»; лишних ходов после закрытия не было.
	model.mu.Lock()
	left, seen := len(model.turns), model.seen
	model.mu.Unlock()
	if left != 6-1-maxRefusedRepeats {
		t.Fatalf("после закрытия кластера модель не должна вызываться: осталось ходов %d", left)
	}
	last := seen[len(seen)-1]
	if msg := last[len(last)-1]; msg.Role != "tool" || !strings.Contains(msg.Content, "уже отклонён") ||
		!strings.Contains(msg.Content, "close_cluster") {
		t.Fatalf("модели нужен ответ «уже отклонено, сделай другое или заверши»: %+v", msg)
	}
	var ep struct {
		Clusters []struct {
			Outcome string `json:"outcome"`
		} `json:"clusters"`
	}
	_ = json.Unmarshal(run.Episode, &ep)
	if len(ep.Clusters) != 1 || ep.Clusters[0].Outcome != OutcomeRepeat {
		t.Fatalf("итог кластера — repeat: %s", run.Episode)
	}
}

// Отказ с другими аргументами — новый вызов, он выполняется.
func TestRefusedKeyCanonical(t *testing.T) {
	a := refusedKey("x", `{"a":1,"b":"в"}`)
	b := refusedKey("x", ` { "b":"в", "a":1 } `)
	c := refusedKey("x", `{"a":2,"b":"в"}`)
	if a != b || a == c || refusedKey("y", `{"a":1,"b":"в"}`) == a {
		t.Fatalf("канонический ключ: %q %q %q", a, b, c)
	}
}
