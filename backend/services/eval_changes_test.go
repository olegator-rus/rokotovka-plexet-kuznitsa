package services

import (
	"testing"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// evalChangesRun — прогон двух конфигураций, как его строит RunEvaluation: списки изменений — из evalCompare.
// Запросы: a — только хуже, b — только лучше, m — и лучше (NDCG), и хуже (нарушение), s и t — без изменений,
// e — ошибка у новой конфигурации, p — ошибка у предыдущей, n — есть только у новой.
func evalChangesRun() *EvalResult {
	f := func(v float64) *float64 { return &v }
	prev := []out.EvalQueryResult{
		{ID: "a", NDCG10: f(1), MRR10: f(1), HasRelevant: true},
		{ID: "b", NDCG10: f(0), MRR10: f(0), Empty: true},
		{ID: "m", NDCG10: f(0.2), MRR10: f(0.2), HasRelevant: true},
		{ID: "s", NDCG10: f(0.7), MRR10: f(1), HasRelevant: true},
		{ID: "t", NDCG10: f(0.7), MRR10: f(1), HasRelevant: true},
		{ID: "e", NDCG10: f(1), MRR10: f(1), HasRelevant: true},
		{ID: "p", Error: "boom"},
	}
	cur := []out.EvalQueryResult{
		{ID: "a", NDCG10: f(0.5), MRR10: f(0.5), HasRelevant: true},
		{ID: "b", NDCG10: f(0.8), MRR10: f(1), HasRelevant: true},
		{ID: "m", NDCG10: f(0.9), MRR10: f(1), HasRelevant: true, Violations: 1},
		{ID: "s", NDCG10: f(0.7), MRR10: f(1), HasRelevant: true},
		{ID: "t", NDCG10: f(0.72), MRR10: f(1), HasRelevant: true}, // сдвиг меньше порога 0.05
		{ID: "e", Error: "boom"},
		{ID: "p", NDCG10: f(1), MRR10: f(1), HasRelevant: true},
		{ID: "n", NDCG10: f(1), MRR10: f(1), HasRelevant: true},
	}
	regs, imps := evalCompare(prev, cur, "B0")
	return &EvalResult{RunID: "r1", Set: domain.EvalSetTest, Queries: 8, Configs: []out.EvalConfigResult{
		{Config: "B0", PerQuery: prev, Regressions: []out.EvalChange{}, Improvements: []out.EvalChange{}},
		{Config: "B1", Against: "B0", PerQuery: cur, Regressions: regs, Improvements: imps},
	}}
}

// Счёт пары: запрос из обоих списков — ухудшение, и только оно; запросы с ошибкой и без пары не сравнивались;
// сумма трёх групп равна числу сравненных.
func TestEvalChangeCounts(t *testing.T) {
	run := evalChangesRun()
	b1 := run.Configs[1]
	if len(b1.Regressions) != 2 || len(b1.Improvements) != 2 {
		t.Fatalf("списки evalCompare: хуже %d, лучше %d", len(b1.Regressions), len(b1.Improvements))
	}
	rows := evaluationRows(run)
	c := evalRowChangeCounts(rows[1].Set, rows[0].PerQuery, rows[1].PerQuery, b1.Regressions, b1.Improvements)
	if c == nil {
		t.Fatal("счёт не посчитан")
	}
	// Сравнивались a, b, m, s, t; хуже — a и m, лучше — только b, без изменений — s и t.
	if *c != (out.EvalChangeCounts{ComparedN: 5, ImprovedN: 1, UnchangedN: 2, RegressedN: 2}) {
		t.Fatalf("счёт: %+v", *c)
	}
	if c.ImprovedN+c.UnchangedN+c.RegressedN != c.ComparedN {
		t.Fatalf("сумма групп не равна числу сравненных: %+v", *c)
	}
	// Длины списков сложить нельзя: m учтён бы дважды.
	if c.RegressedN != len(b1.Regressions) || c.ImprovedN != len(b1.Improvements)-1 {
		t.Fatalf("запрос из обоих списков должен считаться только ухудшением: %+v", *c)
	}
}

// Без изменений по спискам — все сравненные запросы «без изменений»; повтор id в списке не удваивает счёт.
func TestEvalChangeCountsEdges(t *testing.T) {
	ids := map[string]struct{}{"a": {}, "b": {}, "c": {}}
	if c := evalChangeCounts(ids, nil, nil); c == nil || *c != (out.EvalChangeCounts{ComparedN: 3, UnchangedN: 3}) {
		t.Fatalf("пустые списки: %+v", c)
	}
	dup := []out.EvalChange{{QueryID: "a"}, {QueryID: "a"}}
	if c := evalChangeCounts(ids, dup, dup); c == nil || *c != (out.EvalChangeCounts{ComparedN: 3, UnchangedN: 2, RegressedN: 1}) {
		t.Fatalf("повтор id: %+v", c)
	}
	// Списки от другого прогона (запроса нет среди сравненных) — счёта нет, а не отрицательное «без изменений».
	if c := evalChangeCounts(ids, []out.EvalChange{{QueryID: "x"}}, nil); c != nil {
		t.Fatalf("чужой запрос в ухудшениях: %+v", c)
	}
	if c := evalChangeCounts(ids, nil, []out.EvalChange{{QueryID: "x"}}); c != nil {
		t.Fatalf("чужой запрос в улучшениях: %+v", c)
	}
	// Нет сохранённых запросов одной из конфигураций — счёта нет.
	if c := evalRowChangeCounts(domain.EvalSetTest, nil, domain.JSONB(`[{"id":"a"}]`), nil, nil); c != nil {
		t.Fatalf("без запросов предыдущей: %+v", c)
	}
	if c := evalRowChangeCounts(domain.EvalSetTest, domain.JSONB(`{`), domain.JSONB(`[{"id":"a"}]`), nil, nil); c != nil {
		t.Fatalf("запросы не разбираются: %+v", c)
	}
}

// Наборы «по цели»: сравниваются только запросы с вердиктом у обеих конфигураций (как в TargetChanges).
func TestEvalChangeCountsTargetSet(t *testing.T) {
	set := domain.TargetSets[0]
	prev := domain.JSONB(`[{"id":"a","target":{"strict":true}},{"id":"b","target":{"strict":false}},{"id":"c"}]`)
	cur := domain.JSONB(`[{"id":"a","target":{"strict":false}},{"id":"b","target":{"strict":false}},{"id":"c","target":{"strict":true}}]`)
	c := evalRowChangeCounts(set, prev, cur, []out.EvalChange{{QueryID: "a"}}, nil)
	if c == nil || *c != (out.EvalChangeCounts{ComparedN: 2, UnchangedN: 1, RegressedN: 1}) {
		t.Fatalf("набор по цели: %+v", c)
	}
	// На наборе с оценками 0–3 вердикт не нужен: сравниваются все три.
	if c := evalRowChangeCounts(domain.EvalSetTest, prev, cur, []out.EvalChange{{QueryID: "a"}}, nil); c == nil || c.ComparedN != 3 {
		t.Fatalf("набор test: %+v", c)
	}
}

// Счёт пары у первой конфигурации прогона не считается; предыдущая берётся из того же прогона.
func TestEvalPairCountsRows(t *testing.T) {
	run := evalChangesRun()
	rows := evaluationRows(run)
	s := &Services{}
	if c := s.evalPairCounts(t.Context(), rows[0], rows, out.EvaluationView{}); c != nil {
		t.Fatalf("у первой конфигурации счёта нет: %+v", c)
	}
	v := out.EvaluationView{Regressions: run.Configs[1].Regressions, Improvements: run.Configs[1].Improvements}
	c := s.evalPairCounts(t.Context(), rows[1], rows, v)
	if c == nil || *c != (out.EvalChangeCounts{ComparedN: 5, ImprovedN: 1, UnchangedN: 2, RegressedN: 2}) {
		t.Fatalf("счёт пары из строк: %+v", c)
	}
}
