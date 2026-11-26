package services

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/externalApi/llm"
	"kuznica/api/in"
)

// 30 новых неудачных запросов (data/eval/agent_unseen.json): их нет в подготовленных данных, и запуск
// «Разобрать» на каждом проходит путь «память → действующий поиск → план → разбор» без ошибки. Модель здесь — по
// сценарию (закрывает кластер).
func TestAgentUnseenQueries(t *testing.T) {
	var set struct {
		Queries []struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		} `json:"queries"`
	}
	raw, err := os.ReadFile("../../data/eval/agent_unseen.json")
	if err != nil {
		t.Skip("нет data/eval/agent_unseen.json")
	}
	if err := json.Unmarshal(raw, &set); err != nil || len(set.Queries) != 30 {
		t.Fatalf("набор: %d запросов, %v", len(set.Queries), err)
	}
	known := map[string]bool{}
	var qs []struct {
		Text string `json:"text"`
	}
	if b, err := os.ReadFile("../../data/mock/queries.json"); err == nil {
		_ = json.Unmarshal(b, &qs)
	}
	for _, q := range qs {
		known[foldQuery(q.Text)] = true
	}
	if f, err := os.Open("../../data/mock/events.jsonl"); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			var e map[string]any
			if json.Unmarshal(sc.Bytes(), &e) == nil {
				for _, k := range []string{"query", "text", "q"} {
					if v, ok := e[k].(string); ok {
						known[foldQuery(v)] = true
					}
				}
			}
		}
		_ = f.Close()
	}
	model := &scriptModel{}
	s, ms := agentEnv(t, model)
	for _, q := range set.Queries {
		if known[foldQuery(q.Text)] {
			t.Fatalf("%s «%s» есть в подготовленных данных", q.ID, q.Text)
		}
		model.turns = [][]llm.ToolCall{{tc("close_cluster", map[string]any{"diagnosis": "NOISE", "reason": "сценарий"})}}
		r, err := s.StartAgentRun(t.Context(), in.AgentRunRequest{Query: q.Text})
		if err != nil {
			t.Fatalf("%s: %v", q.ID, err)
		}
		run := waitRun(t, s, r.RunID)
		steps := stepsOf(ms, r.RunID)
		if run.Status != domain.RunDone || !hasStep(steps, domain.StepMemoryRead, "Читаю память") {
			t.Fatalf("%s «%s»: %+v", q.ID, q.Text, run)
		}
		if run.StopReason != StopAlreadySolved && (run.ClustersDone != 1 || !hasStep(steps, domain.StepPlan, "План")) {
			t.Fatalf("%s «%s»: нет плана или кластера: %+v", q.ID, q.Text, run)
		}
	}
}
