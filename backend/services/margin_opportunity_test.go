package services

import (
	"encoding/json"
	"math"
	"slices"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
)

// Вес по категории двигает только позиции своей категории; остальные — по общему весу (здесь 0,05 — на месте).
func TestMarginReorderCategoryWeights(t *testing.T) {
	cs := marginCands("g", []float64{0.9, 0.899, 0.898, 0.897}, []float64{0, 1, 0, 1})
	cs[0].cat, cs[1].cat, cs[2].cat, cs[3].cat = "a", "a", "b", "b"
	in := marginInput{cands: cs}
	r := marginReorderW(in, marginWeights{global: marginBaseWeight, cats: map[string]float64{"a": 0.15}}, marginMaxShift)
	// «a» с маржой 1 (поз. 2) обгоняет «a» с маржой 0; «b» (поз. 4) без надбавки остаётся ниже.
	if !sameIntOrder(r.order, []int{1, 0, 2, 3}) {
		t.Fatalf("вес по категории: %v", r.order)
	}
	if r := marginReorderW(in, marginWeights{global: marginBaseWeight, cats: map[string]float64{"a": marginBaseWeight}}, marginMaxShift); !sameIntOrder(r.order, []int{0, 1, 2, 3}) {
		t.Fatalf("все веса 0,05 — порядок контура поиска: %v", r.order)
	}
}

// testMarginCategory — первая по алфавиту категория фикстуры, кроме «office» (её занимает второй запрос).
func testMarginCategory(svc *Services) string {
	var cats []string
	for _, d := range svc.catalog.Load().lex.docs {
		if d.CategoryID != "" && d.CategoryID != "office" {
			cats = append(cats, d.CategoryID)
		}
	}
	slices.Sort(cats)
	return cats[0]
}

// marginTestCapture — базовая выдача из двух размеченных запросов: в категории cat позиция с большей маржой стоит
// второй при той же оценке (сдвиг не меняет NDCG), в другой категории — без разметки.
func marginTestCapture(cat string) *marginCapture {
	q1 := marginSimQuery{q: domain.EvalQuery{ID: "m-1", Text: "антивирус для бизнеса", Type: domain.QueryPurpose,
		Judgments: map[string]int{"p1": 2, "p2": 2}}, ideal: []int{2, 2}}
	c1 := marginCands("g", []float64{0.9, 0.899}, []float64{0.1, 0.9})
	c1[0].productID, c1[1].productID = "p1", "p2"
	c1[0].cat, c1[1].cat = cat, cat
	q1.in = marginInput{cands: c1}
	q2 := marginSimQuery{q: domain.EvalQuery{ID: "m-2", Text: "офис", Type: domain.QuerySlang,
		Judgments: map[string]int{"o1": 3}}, ideal: []int{3}}
	c2 := marginCands("g", []float64{0.9}, []float64{0.5})
	c2[0].productID, c2[0].cat = "o1", "office"
	q2.in = marginInput{cands: c2}
	return &marginCapture{key: "k", set: domain.EvalSetTune, setHash: "h", queries: []marginSimQuery{q1, q2},
		titles: map[string]string{}, catalog: "test", rules: 1}
}

// Находка MARGIN_OPPORTUNITY и POST /api/margin/simulate с category дают одни и те же числа.
func TestMarginOpportunityReproducible(t *testing.T) {
	svc, store := marginEnv(t)
	cat := testMarginCategory(svc)
	capture := marginTestCapture(cat)
	key := svc.marginSimKeyFor(domain.EvalSetTune)
	sim := &svc.margin().sim
	sim.mu.Lock()
	sim.captures, sim.pending, sim.errs = map[string]*marginCapture{key: capture}, map[string]chan struct{}{}, map[string]error{}
	sim.mu.Unlock()

	pol, _ := svc.activeMarginPolicy(t.Context())
	scan := svc.marginOpportunitiesOn(capture, pol)
	if len(scan.Found) != 1 {
		t.Fatalf("ожидалась одна возможность: %+v", scan)
	}
	o := scan.Found[0]
	if o.CategoryID != cat || o.From != marginBaseWeight || o.To != 0.08 || o.Gain <= 0 || o.NDCGDelta != 0 {
		t.Fatalf("возможность: %+v", o)
	}
	if scan.Skipped["no_judged_queries"]+scan.Skipped["no_shift"] == 0 {
		t.Fatalf("вторая категория должна быть отброшена: %+v", scan.Skipped)
	}
	res, err := svc.SimulateMargin(t.Context(), in.MarginSimulateRequest{Category: cat, Positions: []float64{o.From, o.To}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Scope == nil || res.Scope.CategoryID != cat || res.Scope.QueriesInScope != 1 || len(res.Items) != 2 {
		t.Fatalf("симуляция категории: %+v", res)
	}
	same := func(a, b out.MarginPosition) bool {
		return a.Weight == b.Weight && a.NDCG10 == b.NDCG10 && a.MarginIndex == b.MarginIndex &&
			a.MarginIndexRelevant == b.MarginIndexRelevant && a.ShareOverEps == b.ShareOverEps &&
			a.QueriesWithShifts == b.QueriesWithShifts && a.Top3Changed == b.Top3Changed && a.Available == b.Available
	}
	if !same(res.Items[0], o.before) || !same(res.Items[1], o.after) {
		t.Fatalf("числа не воспроизводятся:\n%+v\n%+v\n%+v\n%+v", res.Items[0], o.before, res.Items[1], o.after)
	}
	if d := res.Items[1].MarginIndex - res.Items[0].MarginIndex; math.Abs(evalRound4(d)-o.Gain) > 1e-9 {
		t.Fatalf("прирост индекса: %v vs %v", d, o.Gain)
	}

	// Применение веса категории — версия с categories; общий вес прежний; откат к 0 снимает.
	applied, err := svc.ApplyMarginPolicy(t.Context(), in.MarginPolicyRequest{Categories: map[string]float64{cat: o.To}})
	if err != nil || applied.Weight != marginBaseWeight || applied.Categories[cat] != o.To {
		t.Fatalf("применение категории: %+v %v", applied, err)
	}
	var r domain.MarginPolicyRules
	_ = json.Unmarshal(store.rows[len(store.rows)-1].Rules, &r)
	if r.Categories[cat] != o.To {
		t.Fatalf("версия без весов категорий: %s", store.rows[len(store.rows)-1].Rules)
	}
	p, _ := svc.MarginPolicy(t.Context())
	if p.Categories[cat] != o.To || p.CategoryNames[cat] == "" {
		t.Fatalf("GET policy: %+v", p)
	}
	// Действующий вес категории уже 0,08: следующий перебор начинается с него.
	pol, _ = svc.activeMarginPolicy(t.Context())
	if next := svc.marginOpportunitiesOn(capture, pol); slices.ContainsFunc(next.Found, func(x marginOpportunity) bool {
		return x.CategoryID == cat && x.From != o.To
	}) {
		t.Fatalf("перебор не учёл действующий вес категории: %+v", next.Found)
	}
	zero := 0
	back, err := svc.RollbackMarginPolicy(t.Context(), in.MarginRollbackRequest{ToVersion: &zero})
	if err != nil || len(back.Categories) != 0 {
		t.Fatalf("откат: %+v %v", back, err)
	}
	if _, err := svc.SimulateMargin(t.Context(), in.MarginSimulateRequest{Category: "нет-такой"}); err == nil {
		t.Fatalf("неизвестная категория принята")
	}
}

// Модель эффекта: тождество сходится в каждом сценарии, у каждого входа источник, непосчитанное — null.
func TestEffectCommerceIdentity(t *testing.T) {
	eff := &out.Effect{Measured: out.EffectInputs{Searches: 1000, OrdersMonth: 240},
		Assumptions: []out.EffectParam{{Key: "avg_check_rub", Value: 20000, Default: 20000, Source: "measured"}},
		Scenarios: []out.EffectScenario{{ID: "conservative", OrdersMonth: 2}, {ID: "base", OrdersMonth: 8},
			{ID: "optimistic", OrdersMonth: 15}}}
	m := ecMeasured{baseMargin: evalPtr(0.1), coverage: evalPtr(0.8), compSum: evalPtr(9000.0), compMargin: evalPtr(0.2),
		returnRate: evalPtr(0.02), mixPerOrder: evalPtr(12.5), refs: map[string]string{}, synthetic: true}
	res := effectCommerceFrom(eff, m, nil)
	if len(res.Scenarios) != 3 || !res.Synthetic {
		t.Fatalf("сценарии: %+v", res)
	}
	for _, in := range res.Inputs {
		if in.Source == "" || in.Value == nil && in.Key != ecMixPerOrder {
			t.Fatalf("вход без источника или значения: %+v", in)
		}
	}
	for _, sc := range res.Scenarios {
		if !sc.Complete || sc.Check == nil || !sc.Check.OK {
			t.Fatalf("тождество %s: %+v", sc.ID, sc.Check)
		}
	}
	base := res.Scenarios[1]
	// Спасённые: 8 × 20000 × 0,1.
	if *base.Terms[0].GPRub != 16000 {
		t.Fatalf("спасённые: %v", *base.Terms[0].GPRub)
	}
	// Смена входа пересчитывает: инкрементальность 0 — допродажи нет.
	zero := 0.0
	res2 := effectCommerceFrom(eff, m, map[string]*float64{ecIncrement: &zero})
	if *res2.Scenarios[1].Terms[1].GPRub != 0 || res2.Scenarios[1].DeltaGPRub >= base.DeltaGPRub {
		t.Fatalf("пересчёт: %+v", res2.Scenarios[1])
	}
	// Нет покрытия — допродажа и потеря якоря null, сценарий неполный, тождество по посчитанному.
	m.coverage = nil
	res3 := effectCommerceFrom(eff, m, nil)
	if sc := res3.Scenarios[1]; sc.Complete || sc.Terms[1].GPRub != nil || sc.Terms[3].GPRub != nil || !sc.Check.OK {
		t.Fatalf("без покрытия: %+v", sc)
	}
}

// Предложение по марже принимается только на экране «Маржа»: из очереди — 422; применение с proposal_id и той же
// настройкой — версия source=agent с proposal_id, предложение принято; другая настройка — отказ.
func TestMarginProposalAppliedViaScreen(t *testing.T) {
	svc, store := marginEnv(t)
	cat := testMarginCategory(svc)
	key := svc.marginSimKeyFor(domain.EvalSetTune)
	sim := &svc.margin().sim
	sim.mu.Lock()
	sim.captures, sim.pending, sim.errs = map[string]*marginCapture{key: marginTestCapture(cat)}, map[string]chan struct{}{}, map[string]error{}
	sim.mu.Unlock()
	ms := newMemStore(nil)
	svc.agentStoreOverride = ms
	patch := marginOppPatch{CategoryID: cat, WeightBefore: marginBaseWeight, Weight: 0.08, GlobalWeight: marginBaseWeight}
	p := &domain.Proposal{ClusterKey: marginOppKey(cat, 0.08), Diagnosis: domain.DiagMarginOpportunity,
		Action: domain.ActionMarginPolicy, Patch: toJSON(patch), Status: domain.ProposalPending}
	if err := ms.CreateProposal(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ApproveProposal(t.Context(), p.ID.String(), in.ApproveRequest{}); err == nil {
		t.Fatalf("предложение по марже принято из очереди")
	}
	if _, err := svc.ApplyMarginPolicy(t.Context(), in.MarginPolicyRequest{Categories: map[string]float64{cat: 0.12},
		ProposalID: p.ID.String()}); err == nil {
		t.Fatalf("настройка не как в предложении — принята")
	}
	res, err := svc.ApplyMarginPolicy(t.Context(), in.MarginPolicyRequest{Categories: map[string]float64{cat: 0.08},
		ProposalID: p.ID.String()})
	if err != nil || res.ProposalID == nil || *res.ProposalID != p.ID.String() {
		t.Fatalf("применение предложения: %+v %v", res, err)
	}
	v := store.rows[len(store.rows)-1]
	if v.Source != "agent" || v.ProposalID == nil || *v.ProposalID != p.ID.String() {
		t.Fatalf("версия: %+v", v)
	}
	got, _ := ms.GetProposal(t.Context(), p.ID)
	if got.Status != domain.ProposalAccepted || got.ConfigVersion == nil || *got.ConfigVersion != v.Version {
		t.Fatalf("предложение не принято: %+v", got)
	}
}
