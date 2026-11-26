package services

import (
	"testing"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Решения и проверки таблицы вклада: пороги шума, зеркальный счёт улучшений, блокирующие проверки.
func TestContributionDecisionsAndChecks(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	row := func(id string, ndcg, exact, fr, p95 float64, imps, regs int) out.ContributionRow {
		return out.ContributionRow{ID: id, Improvements: imps, Regressions: regs, Metrics: out.EvalMetrics{NDCG10: f(ndcg),
			ExactAccuracy: f(exact), FalseRescueRate: f(fr), Latency: out.EvalLatency{P95: p95}}}
	}
	rows := []out.ContributionRow{
		row(domain.ConfigB2, 0.86, 0.95, 1, 30, 0, 0),
		row(contribB3NoRerank, 0.85, 1, 0.2, 40, 0, 0),
		row(domain.ConfigB3, 0.89, 1, 0.17, 150, 9, 2), // реранкер: +0,04, 9 улучшений против B3-R
		row(contribB3NoVec, 0.888, 1, 0.17, 140, 1, 1), // без kNN почти то же — шум
		row(contribB3NoLLM, 0.80, 1, 0.3, 30, 1, 12),   // без LLM: 12 запросов лучше у полного пути
		row(contribB3Offline, 0.78, 1, 0.3, 20, 0, 14),
	}
	got := map[string]string{}
	for _, d := range contribDecisions(rows) {
		got[d.Component] = d.Verdict
		if !d.Preliminary || d.DeltaNDCG == nil {
			t.Fatalf("%s: %+v", d.Component, d)
		}
		if d.Component == "llm" && (d.Improvements != 12 || d.Regressions != 1) {
			t.Fatalf("улучшения от LLM — ухудшения строки без него: %+v", d)
		}
	}
	if got["rerank"] != "keep" || got["vectors"] != "remove" || got["llm"] != "keep" {
		t.Fatalf("решения: %v", got)
	}
	checks := contribChecks(rows)
	for _, c := range checks {
		if !c.Passed {
			t.Fatalf("проверка не пройдена: %+v", c)
		}
	}
	rows[2].Metrics.ExactAccuracy = f(0.97)
	failed := 0
	for _, c := range contribChecks(rows) {
		if !c.Passed && c.Blocking {
			failed++
		}
	}
	if failed != 2 { // точные в полном пути и «реранкер не сдвигает точные»
		t.Fatalf("блокирующих провалов %d, want 2", failed)
	}
}
