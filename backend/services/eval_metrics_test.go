package services

import (
	"math"
	"strings"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/out"
)

func evalNear(a, b float64) bool { return math.Abs(a-b) < 1e-4 }

// NDCG@10 сверен вручную: выдача [3, 0, 2], идеал [3, 2, 1]:
// DCG = 7/1 + 0 + 3/log2(4) = 8.5; IDCG = 7 + 3/log2(3) + 1/log2(4) = 9.392789; NDCG = 0.904950.
func TestEvalNDCGHand(t *testing.T) {
	v := evalNDCG([]int{3, 0, 2}, []int{1, 3, 2}, 10)
	if v == nil || !evalNear(*v, 0.904950) {
		t.Fatalf("NDCG: %v", v)
	}
	if v := evalNDCG([]int{3, 2, 1}, []int{3, 2, 1}, 10); v == nil || !evalNear(*v, 1) {
		t.Fatalf("идеальная выдача: %v", v)
	}
	if v := evalNDCG([]int{0, 0}, []int{2}, 10); v == nil || *v != 0 {
		t.Fatalf("без релевантного: %v", v)
	}
	if v := evalNDCG([]int{0}, []int{0, 0}, 10); v != nil {
		t.Fatalf("идеал без пользы — nil: %v", *v)
	}
	// Позиции за k не считаются: релевантное на 11-м месте — 0.
	grades := make([]int, 11)
	grades[10] = 3
	if v := evalNDCG(grades, []int{3}, 10); v == nil || *v != 0 {
		t.Fatalf("за пределом k: %v", v)
	}
}

// MRR@10: первая позиция с оценкой ≥2 на 3-м месте — 1/3; оценка 1 релевантной не считается.
func TestEvalMRRHand(t *testing.T) {
	if v := evalMRR([]int{1, 0, 2, 3}, 10); !evalNear(v, 1.0/3) {
		t.Fatalf("MRR: %v", v)
	}
	if v := evalMRR([]int{3}, 10); v != 1 {
		t.Fatalf("MRR на 1-м: %v", v)
	}
	if v := evalMRR([]int{1, 1}, 10); v != 0 {
		t.Fatalf("MRR без релевантного: %v", v)
	}
}

func TestEvalPercentile(t *testing.T) {
	s := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	if evalPercentile(s, 50) != 5 || evalPercentile(s, 95) != 10 || evalPercentile(s[:1], 95) != 1 {
		t.Fatalf("процентили: %v %v", evalPercentile(s, 50), evalPercentile(s, 95))
	}
	if l := evalLatencyOf([]float64{30, 10, 20}); l.P50 != 20 || l.P95 != 30 || l.Count != 3 {
		t.Fatalf("время: %+v", l)
	}
}

// testEvalCatalog — два семейства антивируса: k (пакеты 1/10 устройств, продление) и p (поштучно).
func testEvalCatalog() *evalCatalog {
	dev, user := "device", "user"
	q1, q10 := 1, 10
	return newEvalCatalog([]domain.Product{
		{ID: "k-1", FamilyID: "k", License: domain.License{Type: "subscription", Metric: &dev, Quantity: &q1}},
		{ID: "k-10", FamilyID: "k", License: domain.License{Type: "subscription", Metric: &dev, Quantity: &q10}},
		{ID: "k-10-ren", FamilyID: "k", License: domain.License{Type: "renewal", Metric: &dev, Quantity: &q10}},
		{ID: "k-u10", FamilyID: "k", License: domain.License{Type: "subscription", Metric: &user, Quantity: &q10}},
		{ID: "p-1", FamilyID: "p", Attributes: domain.JSONB(`{"purchase":"поштучно"}`),
			License: domain.License{Type: "subscription", Metric: &dev, Quantity: &q1}},
		{ID: "x", FamilyID: "x"},
	})
}

func evalItems(ids ...string) []evalItem {
	cat := testEvalCatalog()
	res := make([]evalItem, len(ids))
	for i, id := range ids {
		res[i] = evalItem{ProductID: id, FamilyID: cat.familyOf(id), Group: domain.GroupMatch}
	}
	return res
}

func TestEvalScoreQuery(t *testing.T) {
	cat := testEvalCatalog()
	// «на 10 устройств»: k-10 и p-1 (поштучно) — 3, продление — 2, другое количество/метрика — 1.
	q := domain.EvalQuery{ID: "q1", Type: domain.QueryLicense, Freq: 5,
		Judgments: map[string]int{"k-10": 3, "p-1": 3, "k-10-ren": 2, "k-1": 1, "k-u10": 1}}

	// Выдача: k-1 (не то количество — нарушение), x (чужой продукт — не нарушение), p-1.
	r := evalScoreQuery(q, evalOutcome{Items: evalItems("k-1", "x", "p-1"), Total: 3}, cat)
	if r.Empty || r.HasRelevant != true || r.Violations != 1 || r.Rescued == nil || !*r.Rescued {
		t.Fatalf("license: %+v", r)
	}
	// Идеал по семействам: k=3, p=3 → [3,3]; выдача [1,0,3]: DCG = 1 + 0 + 7/2 = 4.5; IDCG = 7 + 7/log2(3).
	want := 4.5 / (7 + 7/math.Log2(3))
	if r.NDCG10 == nil || !evalNear(*r.NDCG10, want) || r.MRR10 == nil || !evalNear(*r.MRR10, 1.0/3) {
		t.Fatalf("NDCG/MRR: %v %v, want %v", *r.NDCG10, *r.MRR10, want)
	}
	if r.Recall50 == nil || *r.Recall50 != 1 {
		t.Fatalf("recall: %v", *r.Recall50)
	}
	// Не та метрика (на пользователя) — нарушение; продление (оценка 2) — нет.
	r = evalScoreQuery(q, evalOutcome{Items: evalItems("k-u10", "k-10-ren")}, cat)
	if r.Violations != 1 || !r.HasRelevant || *r.Recall50 != 0.5 {
		t.Fatalf("метрика: %+v", r)
	}

	// Точный запрос: 3 на первом месте.
	ex := domain.EvalQuery{ID: "q2", Type: domain.QueryExact, Judgments: map[string]int{"k-10": 3, "k-1": 3}}
	if r := evalScoreQuery(ex, evalOutcome{Items: evalItems("k-1")}, cat); r.ExactHit == nil || !*r.ExactHit {
		t.Fatalf("exact: %+v", r)
	}
	if r := evalScoreQuery(ex, evalOutcome{Items: evalItems("x", "k-1")}, cat); r.ExactHit == nil || *r.ExactHit {
		t.Fatalf("exact не на 1-м: %+v", r)
	}
	if r := evalScoreQuery(ex, evalOutcome{}, cat); !r.Empty || *r.ExactHit || r.HasRelevant {
		t.Fatalf("exact пусто: %+v", r)
	}

	// Шум: непустая выдача без «нет в каталоге» и без аналогов — ложное спасение.
	noise := domain.EvalQuery{ID: "q3", Type: domain.QueryNoise, Judgments: map[string]int{}}
	if r := evalScoreQuery(noise, evalOutcome{Items: evalItems("x")}, cat); r.FalseRescue == nil || !*r.FalseRescue || r.NDCG10 != nil {
		t.Fatalf("шум: %+v", r)
	}
	if r := evalScoreQuery(noise, evalOutcome{}, cat); *r.FalseRescue {
		t.Fatalf("шум, пусто: %+v", r)
	}
	analog := evalItems("p-1")
	analog[0].Group = domain.GroupAnalog
	absent := domain.EvalQuery{ID: "q4", Type: domain.QueryAbsent, Judgments: map[string]int{"p-1": 2}}
	if r := evalScoreQuery(absent, evalOutcome{Items: analog, EmptyMarker: true}, cat); *r.FalseRescue || !r.HasRelevant {
		t.Fatalf("absent с аналогом: %+v", r)
	}
}

func TestEvalAggregateAndCompare(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	b := func(v bool) *bool { return &v }
	prev := []out.EvalQueryResult{
		{ID: "a", Type: "exact", Freq: 10, NDCG10: f(1), MRR10: f(1), HasRelevant: true, ExactHit: b(true), Ms: 10, Route: "A"},
		{ID: "b", Type: "typo", Freq: 1, Empty: true, NDCG10: f(0), MRR10: f(0), Rescued: b(false), Ms: 20, Route: "B"},
		{ID: "c", Type: "noise", Freq: 1, Empty: true, FalseRescue: b(false), Ms: 30, Route: "A"},
	}
	cur := []out.EvalQueryResult{
		{ID: "a", Type: "exact", Freq: 10, NDCG10: f(0.5), MRR10: f(0.5), HasRelevant: true, ExactHit: b(false), Ms: 12, Route: "A"},
		{ID: "b", Type: "typo", Freq: 1, NDCG10: f(0.8), MRR10: f(1), HasRelevant: true, Rescued: b(true), Ms: 22, Route: "B"},
		{ID: "c", Type: "noise", Freq: 1, FalseRescue: b(true), Ms: 32, Route: "A"},
		{ID: "d", Type: "exact", Error: "boom"},
	}
	m := evalAggregate(prev)
	// Пустые 2 из 3; по частоте (1+1)/12.
	if m.Queries != 3 || !evalNear(*m.EmptyRate, 0.6667) || !evalNear(*m.EmptyRateWeighted, 0.1667) || *m.ExactAccuracy != 1 ||
		*m.RescuedRate != 0 || *m.FalseRescueRate != 0 || *m.NoRelevantTop10 != 0 || *m.RelevantTop10 != 0.5 {
		t.Fatalf("сводка: %+v", m)
	}
	mc := evalAggregate(cur)
	if mc.Errors != 1 || mc.Queries != 3 || *mc.EmptyRate != 0 || *mc.FalseRescueRate != 1 || mc.ViolationRate != nil ||
		mc.ByRoute["A"].Count != 2 || mc.Latency.P50 != 22 {
		t.Fatalf("сводка cur: %+v", mc)
	}
	bt := evalByType(cur)
	if bt["exact"].Queries != 1 || bt["exact"].Errors != 1 || *bt["typo"].RescuedRate != 1 {
		t.Fatalf("по типам: %+v", bt)
	}
	regs, imps := evalCompare(prev, cur, "B1")
	if len(regs) != 2 || regs[0].QueryID != "a" || !strings.Contains(strings.Join(regs[0].Reasons, ","), "lost_exact") ||
		regs[0].NDCGDelta != -0.5 || regs[1].QueryID != "c" || regs[1].Reasons[0] != "false_rescue" {
		t.Fatalf("ухудшения: %+v", regs)
	}
	if len(imps) != 1 || imps[0].QueryID != "b" || imps[0].Against != "B1" ||
		strings.Join(imps[0].Reasons, ",") != "ndcg,gained_relevant,no_longer_empty" {
		t.Fatalf("улучшения: %+v", imps)
	}
}

func TestEvalAggregateEmpty(t *testing.T) {
	m := evalAggregate(nil)
	if m.Queries != 0 || m.EmptyRate != nil || m.NDCG10 != nil || m.Latency.Count != 0 {
		t.Fatalf("пусто: %+v", m)
	}
}

// TestEvalFamilyMetrics — по семействам: первым показан другой артикул того же продукта (оценка 3 стоит у соседнего
// артикула) — по артикулу промах, по семейству попадание; exact без единой оценки > 0 уходит в группу
// «ожидаем пусто или альтернативу» и не входит в знаменатель «верное семейство первым».
func TestEvalFamilyMetrics(t *testing.T) {
	cat := &evalCatalog{family: map[string]string{"a1": "A", "a2": "A", "b1": "B"}}
	q := domain.EvalQuery{ID: "q", Type: domain.QueryExact, Judgments: map[string]int{"a2": 3, "b1": 1}}
	o := evalOutcome{Items: []evalItem{{ProductID: "a1", FamilyID: "A"}, {ProductID: "b1", FamilyID: "B"}}}
	r := evalScoreQuery(q, o, cat)
	if r.ExactHit == nil || *r.ExactHit || r.ExactHitFamily == nil || !*r.ExactHitFamily {
		t.Fatalf("артикул — промах, семейство — попадание: %v %v", r.ExactHit, r.ExactHitFamily)
	}
	if r.NDCG10Family == nil || !evalNear(*r.NDCG10Family, 1) || r.NDCG10 == nil || *r.NDCG10 >= 1 {
		t.Fatalf("NDCG: семейство %v, артикул %v", r.NDCG10Family, r.NDCG10)
	}
	none := domain.EvalQuery{ID: "n", Type: domain.QueryExact, Judgments: map[string]int{"b1": 0}}
	rn := evalScoreQuery(none, evalOutcome{Items: []evalItem{{ProductID: "b1", FamilyID: "B", Group: domain.GroupAnalog}}}, cat)
	if !rn.ExpectEmpty || rn.ExactHitFamily != nil || rn.ExpectEmptyOK == nil || !*rn.ExpectEmptyOK {
		t.Fatalf("ожидаем пусто: %+v", rn)
	}
	m := evalAggregate([]out.EvalQueryResult{r, rn})
	if m.ExactAccuracyFamily == nil || *m.ExactAccuracyFamily != 1 || m.ExactQueries != 1 || m.ExpectEmptyQueries != 1 ||
		m.ExactAccuracy == nil || *m.ExactAccuracy != 0 {
		t.Fatalf("сводка: %+v", m)
	}
}

// TestEvalFamilyNDCGJudgedOnly — разметка с явными нулями: неразмеченное семейство в выдаче не штрафует NDCG по
// семействам (пропускается), размеченный 0 — штрафует.
func TestEvalFamilyNDCGJudgedOnly(t *testing.T) {
	cat := &evalCatalog{family: map[string]string{"a": "A", "u": "U", "z": "Z"}}
	q := domain.EvalQuery{ID: "q", Type: domain.QueryPurpose, Judgments: map[string]int{"a": 3, "z": 0}}
	r := evalScoreQuery(q, evalOutcome{Items: []evalItem{{ProductID: "u", FamilyID: "U"}, {ProductID: "a", FamilyID: "A"}}}, cat)
	if r.NDCG10Family == nil || *r.NDCG10Family != 1 {
		t.Fatalf("неразмеченное пропускается: %v", r.NDCG10Family)
	}
	r = evalScoreQuery(q, evalOutcome{Items: []evalItem{{ProductID: "z", FamilyID: "Z"}, {ProductID: "a", FamilyID: "A"}}}, cat)
	if r.NDCG10Family == nil || *r.NDCG10Family >= 1 {
		t.Fatalf("размеченный 0 штрафует: %v", r.NDCG10Family)
	}
}
