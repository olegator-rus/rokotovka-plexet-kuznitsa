package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"kuznica/api/domain"
	"kuznica/api/externalApi/catalogsource"
	"kuznica/api/pkg/errs"
)

// evalSource — фикстура каталога плюс свой набор запросов.
type evalSource struct {
	catalogsource.MockJSON
	queries []domain.EvalQuery
}

func (e evalSource) LoadQueries(context.Context) ([]domain.EvalQuery, error) { return e.queries, nil }

func evalFixtureQueries() []domain.EvalQuery {
	return []domain.EvalQuery{
		{ID: "t-2", Text: "Kaspersky Internet", Type: domain.QueryExact, Set: domain.EvalSetTest, Freq: 10,
			Judgments: map[string]int{"kis-2dev-1y": 3, "kis-5dev-1y": 3}},
		{ID: "t-1", Text: "фотошоп", Type: domain.QuerySlang, Set: domain.EvalSetTest, Freq: 3,
			Judgments: map[string]int{"photoshop-1u-1y": 3}},
		{ID: "t-3", Text: "asdf", Type: domain.QueryNoise, Set: domain.EvalSetTest, Freq: 1, Judgments: map[string]int{}},
		{ID: "u-1", Text: "Office", Type: domain.QueryExact, Set: domain.EvalSetTune, Freq: 1,
			Judgments: map[string]int{"office-2021-hb": 3}},
	}
}

func evalEnv(t *testing.T) *testEnv {
	t.Helper()
	env := imported(t, false)
	env.svc.source = evalSource{MockJSON: fixtureSource(), queries: evalFixtureQueries()}
	return env
}

func TestRunEvaluation(t *testing.T) {
	env := evalEnv(t)
	run, err := env.svc.RunEvaluation(t.Context(), EvalRequest{Set: "test", Configs: []string{"b0", "B1", "B1"}, Persist: true})
	if err != nil {
		t.Fatal(err)
	}
	if run.Set != "test" || run.Queries != 3 || len(run.Configs) != 2 || run.Configs[0].Config != "B0" ||
		run.Configs[1].Against != "B0" || len(run.SetHash) != 12 || run.Commit == "" {
		t.Fatalf("прогон: %+v", run)
	}
	// Запросы — по порядку id, набор tune не попал.
	pq := run.Configs[0].PerQuery
	if len(pq) != 3 || pq[0].ID != "t-1" || pq[2].ID != "t-3" {
		t.Fatalf("запросы: %+v", pq)
	}
	b0 := run.Configs[0].Metrics
	// B0 — подстрока: «Kaspersky Internet» находится, «фотошоп» и «asdf» — нет.
	if b0.Queries != 3 || b0.Errors != 0 || *b0.ExactAccuracy != 1 || !evalNear(*b0.EmptyRate, 0.6667) || *b0.FalseRescueRate != 0 {
		t.Fatalf("B0: %+v", b0)
	}
	if run.Configs[0].CatalogVersion != "fixture-1" || run.Configs[0].RulesVersion == 0 || run.Configs[1].Regressions == nil ||
		run.Configs[0].Regressions == nil {
		t.Fatalf("версии/ухудшения: %+v", run.Configs[0])
	}
	if !strings.Contains(run.Repro, "sha256:"+run.SetHash) || !strings.Contains(run.Repro, "fixture-1") {
		t.Fatalf("воспроизводимость: %s", run.Repro)
	}
	table := FormatEvalTable(run)
	for _, want := range []string{"| Метрика | B0 | B1 |", "NDCG@10", "Ложное спасение", "B1 против B0"} {
		if !strings.Contains(table, want) {
			t.Fatalf("таблица без %q:\n%s", want, table)
		}
	}

	// Хэш набора не зависит от прогона; подмножество по id — другой хэш.
	again, err := env.svc.RunEvaluation(t.Context(), EvalRequest{Set: "test", Configs: []string{"B0"}})
	if err != nil || again.SetHash != run.SetHash || again.RunID == run.RunID {
		t.Fatalf("повтор: %v %+v", err, again)
	}
	one, err := env.svc.RunEvaluation(t.Context(), EvalRequest{Set: "test", Configs: []string{"B0"}, QueryIDs: []string{"t-2"}})
	if err != nil || one.Queries != 1 || one.SetHash == run.SetHash {
		t.Fatalf("подмножество: %v %+v", err, one)
	}

	// Снимок правил: либо поиск его применил (версия в трассе = 99), либо прогон предупреждает — тихой
	// подмены нет.
	ov, err := env.svc.RunEvaluation(t.Context(), EvalRequest{Set: "test", Configs: []string{"B0"},
		RulesOverride: &RulesSnapshot{Version: 99}})
	if err != nil || !ov.Configs[0].RulesOverride || (ov.Configs[0].RulesVersion != 99) == (len(ov.Configs[0].Warnings) == 0) {
		t.Fatalf("снимок: %v %+v", err, ov.Configs[0])
	}
}

func TestRunEvaluationRejects(t *testing.T) {
	env := evalEnv(t)
	var e *errs.Error
	if _, err := env.svc.RunEvaluation(t.Context(), EvalRequest{Set: "prod"}); !errors.As(err, &e) || e.Type != errs.ErrTypeValidation {
		t.Fatalf("набор: %v", err)
	}
	if _, err := env.svc.RunEvaluation(t.Context(), EvalRequest{Configs: []string{"B9"}}); !errors.As(err, &e) || e.Type != errs.ErrTypeValidation {
		t.Fatalf("конфигурация: %v", err)
	}
	if _, err := env.svc.RunEvaluation(t.Context(), EvalRequest{Set: "regression"}); !errors.As(err, &e) || e.Type != errs.ErrTypeValidation {
		t.Fatalf("пустой набор: %v", err)
	}
	// Источник без наборов — 503.
	plain := imported(t, false)
	if _, err := plain.svc.RunEvaluation(t.Context(), EvalRequest{}); !errors.As(err, &e) || e.Type != errs.ErrTypeUnavailable {
		t.Fatalf("без наборов: %v", err)
	}
}

// Фоновый прогон: задание, второй параллельно — 409, по окончании — last_job со ссылкой на прогон.
func TestStartEvaluation(t *testing.T) {
	env := evalEnv(t)
	job, err := env.svc.StartEvaluation("test", []string{"B0"})
	if err != nil || job.Status != "running" || job.Set != "test" {
		t.Fatalf("задание: %v %+v", err, job)
	}
	var e *errs.Error
	if _, err := env.svc.StartEvaluation("test", nil); !errors.As(err, &e) || e.Type != errs.ErrTypeConflict {
		// Прогон мог успеть закончиться — тогда второй запуск законен.
		if err != nil {
			t.Fatalf("второй: %v", err)
		}
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, err := env.svc.Evaluations(t.Context(), "test")
		if err != nil {
			t.Fatal(err)
		}
		if resp.Running == nil && resp.LastJob != nil {
			if resp.LastJob.Status != "done" || resp.LastJob.RunID == "" || resp.LastJob.FinishedAt == nil {
				t.Fatalf("итог: %+v", resp.LastJob)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("фоновый прогон не закончился")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Без базы GET /api/evaluations — пустой список, не ошибка.
func TestEvaluationsWithoutDB(t *testing.T) {
	env := evalEnv(t)
	resp, err := env.svc.Evaluations(t.Context(), "")
	if err != nil || resp.Set != "test" || resp.Configs == nil || len(resp.Configs) != 0 || resp.Running != nil {
		t.Fatalf("пусто: %v %+v", err, resp)
	}
	if _, err := env.svc.Evaluations(t.Context(), "nope"); err == nil {
		t.Fatal("неизвестный набор — ошибка")
	}
}

func TestEvalRequestID(t *testing.T) {
	id := evalRequestID("t0-abcdef", "B3", "q-0123")
	if id != "eval-abcdef-B3-q-0123" || len(evalRequestID("x-abcdef", "B3", strings.Repeat("q", 40))) != 32 {
		t.Fatalf("request_id: %s", id)
	}
}
