package services

import (
	"context"
	"strings"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/externalApi/llm"
	"kuznica/api/in"
	"kuznica/api/out"
)

// Доводка поиска и оценки: мягкие условия разбора, «С этим покупают», источник compare, выбор лицензии
// семейства, LLM-судья (пул, разбор ответа, согласие).

func TestIntentPreferencesAreSoft(t *testing.T) {
	p := newIntentEnv(t, "off", nil, nil, nil)
	dict := p.dictionary(t.Context())
	raw, _ := decodeIntent(`{"intent":"find_alternative","reference_product":"Microsoft Office","category":"office-suite",
		"constraints":{},"preferences":{"origin":"ru","license_metric":"user"}}`)
	in := dict.validate(*raw, "замена офиса для госкомпании")
	if in.Origin != "" || hasConstraint(in.Constraints, domain.FieldOrigin, "ru") || !hasConstraint(in.Preferences, domain.FieldOrigin, "ru") {
		t.Fatalf("домысленное «отечественное» — только предпочтение: %s", intentSummary(in))
	}
	lx, _, _ := mockLexicon(t)
	var attrs queryAttrs
	var ent queryEntities
	mergeParsed(in, &attrs, &ent, lx)
	if attrs.origin != "" || attrs.metric != "" || attrs.metricPref != domain.MetricUser {
		t.Fatalf("мягкие условия стали обязательными: %+v", attrs)
	}
	// То же, но названо прямо — обязательное.
	raw, _ = decodeIntent(`{"intent":"find_product","origin":"ru","constraints":{"origin":"ru"}}`)
	in = dict.validate(*raw, "отечественный офис")
	attrs = queryAttrs{}
	mergeParsed(in, &attrs, &ent, lx)
	if attrs.origin != "ru" {
		t.Fatalf("явное «отечественный» должно фильтровать: %+v", attrs)
	}
}

func TestComplementsOf(t *testing.T) {
	lx, _, _ := mockLexicon(t)
	got := complementsOf(lx, "windows-server-2022", map[string]bool{})
	if len(got) == 0 || got[0].FamilyID != "windows-server-cal" || got[0].Note == "" || got[0].ProductID == "" {
		t.Fatalf("Windows Server → CAL: %+v", got)
	}
	if got := complementsOf(lx, "windows-server-2022", map[string]bool{"windows-server-cal": true}); len(got) > 0 && got[0].FamilyID == "windows-server-cal" {
		t.Fatalf("семейство из выдачи не повторяется: %+v", got)
	}
	if got := complementsOf(lx, "1c-accounting", nil); len(got) > maxComplements {
		t.Fatalf("не больше %d: %d", maxComplements, len(got))
	}
	if got := complementsOf(lx, "нет-такого", nil); got == nil || len(got) != 0 {
		t.Fatalf("без связей — пустой массив, не nil: %#v", got)
	}
}

func TestSearchSourceValidation(t *testing.T) {
	for src, ok := range map[string]bool{"": true, "live": true, "Compare": true, "eval": false} {
		req := in.SearchRequest{Query: "windows", Source: src}
		_, _, err := validateSearch(&req)
		if (err == nil) != ok {
			t.Errorf("source %q: err %v", src, err)
		}
	}
}

// Бессрочная лицензия: семейство без неё ниже; «для дома» — слово категории, а не продукта.
func TestRankLicenseTypeAndGenericWords(t *testing.T) {
	lx, rules, _ := mockLexicon(t)
	nr := normalizeFull("nanocad бессрочная лицензия", lx, rules, false)
	a := extractAttrs(nr.tokens)
	e := findEntities(nr, a, lx, rules)
	rc := rankCtx{lx: lx, attrs: a, ent: e, purchased: map[string]bool{}, purchasedQty: map[string]int{},
		boughtVendors: map[string]bool{}, productRel: map[string]float64{}, refCats: map[string]bool{}}
	sub := indexDoc{LicenseType: domain.LicenseSubscription}
	perp := indexDoc{LicenseType: domain.LicensePerpetual}
	if rc.licenseFit(sub) >= rc.licenseFit(perp) {
		t.Fatalf("подписка при запросе бессрочной должна весить меньше")
	}
	// «для дома» не называет Microsoft 365 для дома: слово категории, а не продукта.
	nr = normalizeFull("антивирус для дома", lx, rules, false)
	e = findEntities(nr, extractAttrs(nr.tokens), lx, rules)
	if contains(e.families, "microsoft-365-home") {
		t.Fatalf("«для дома» → %v", e.families)
	}
}

func TestQuadraticKappa(t *testing.T) {
	var perfect [4][4]float64
	for i := range 4 {
		perfect[i][i] = 5
	}
	if k := quadraticKappa(perfect); k == nil || *k != 1 {
		t.Fatalf("полное согласие: %v", k)
	}
	// Пример: 2×2 из 0 и 3 с одним расхождением на 3 балла.
	var o [4][4]float64
	o[0][0], o[3][3], o[0][3] = 4, 4, 2
	k := quadraticKappa(o)
	if k == nil || *k <= 0 || *k >= 1 {
		t.Fatalf("каппа вне (0,1): %v", k)
	}
	var one [4][4]float64
	one[2][2] = 3
	if quadraticKappa(one) != nil {
		t.Fatal("одна категория — каппа не определена")
	}
}

func TestJudgePoolAndGrades(t *testing.T) {
	q1 := domain.EvalQuery{ID: "q1", Text: "a", Judgments: map[string]int{"p1": 3, "p2": 1}}
	q2 := domain.EvalQuery{ID: "q2", Text: "b", Judgments: map[string]int{"p3": 2}}
	run := &out.EvalRun{Configs: []out.EvalConfigResult{
		{Config: "B1", PerQuery: []out.EvalQueryResult{{ID: "q1", Top: []string{"p1", "p2"}}, {ID: "q2", Top: []string{"p3", "p4"}}}},
		{Config: "B2", PerQuery: []out.EvalQueryResult{{ID: "q1", Top: []string{"p2", "p1"}}, {ID: "q2", Top: []string{"p4", "p3"}}}},
	}}
	pairs, depth := judgePool(run, []domain.EvalQuery{q1, q2}, 600)
	if len(pairs) != 4 || depth != 2 {
		t.Fatalf("пул без повторов, полная глубина 2: %d пар, глубина %d", len(pairs), depth)
	}
	if pairs, depth = judgePool(run, []domain.EvalQuery{q1, q2}, 3); len(pairs) != 3 || depth != 0 {
		t.Fatalf("лимит пар: %d, глубина %d", len(pairs), depth)
	}
	pairs, depth = judgePool(run, []domain.EvalQuery{q1, q2}, 600)
	byID := map[string]*judgePairIn{}
	for i, p := range pairs {
		byID[[]string{"x1", "x2", "x3", "x4"}[i]] = p
	}
	missing, err := applyJudgeGrades("```json\n{\"grades\":[{\"id\":\"x1\",\"grade\":3},{\"id\":\"x2\",\"grade\":\"1\"},{\"id\":\"x3\",\"grade\":7},{\"id\":\"zz\",\"grade\":2}]}\n```", byID)
	if err != nil || missing != 2 {
		t.Fatalf("разбор ответа: missing %d, err %v", missing, err)
	}
	if m, err := applyJudgeGrades(`{"/grades":[{"id":"x2","grade":1}]}`, byID); err != nil || m != 2 {
		t.Fatalf("испорченный ключ grades: missing %d, err %v", m, err)
	}
	if _, err := applyJudgeGrades("не json", byID); err == nil {
		t.Fatal("мусор — ошибка схемы")
	}
	_, _ = applyJudgeGrades(`{"grades":[{"id":"x3","grade":2},{"id":"x4","grade":0}]}`, byID)
	var grades []out.JudgePair
	for _, p := range pairs {
		grades = append(grades, out.JudgePair{QueryID: p.q.ID, ProductID: p.pid, Label: p.q.Judgments[p.pid], Judge: p.grade})
	}
	var a out.JudgeAgreement
	judgeAgreement(&a, grades)
	if a.Judged != 4 || a.Exact == nil || a.Within1 == nil || *a.Within1 < *a.Exact {
		t.Fatalf("согласие: %+v", a)
	}
	cfgs, _ := judgeByConfig(run, pairs, depth)
	if len(cfgs) != 2 || cfgs[0].NDCGLabels == nil || cfgs[0].NDCGJudge == nil {
		t.Fatalf("NDCG по конфигурациям: %+v", cfgs)
	}
}

// fakeJudge — судья, который ставит всем парам 2 и сообщает токены.
type fakeJudge struct{ calls int }

func (f *fakeJudge) Model() string { return "qwen3.6-35b-a3b" }
func (f *fakeJudge) Chat(_ context.Context, req llm.Request) (*llm.Response, error) {
	f.calls++
	if req.ReasoningEffort != "none" || strings.Contains(req.Messages[1].Content, "B3") {
		return nil, &llm.Error{Status: 400, Body: "судья видит конфигурацию или рассуждает"}
	}
	n := strings.Count(req.Messages[1].Content, `"id":"p`)
	var b strings.Builder
	b.WriteString(`{"grades":[`)
	for i := 1; i <= n; i++ {
		if i > 1 {
			b.WriteString(",")
		}
		b.WriteString(`{"id":"p` + itoa(i) + `","grade":2}`)
	}
	b.WriteString("]}")
	resp := &llm.Response{Message: llm.Message{Role: "assistant", Content: b.String()}}
	resp.Usage.PromptTokens, resp.Usage.CompletionTokens = 1000, 100
	return resp, nil
}

func TestJudgeBatchBlind(t *testing.T) {
	lx, _, _ := mockLexicon(t)
	f := &fakeJudge{}
	s := &Services{judge: f}
	q := domain.EvalQuery{ID: "q1", Text: "windows 11 pro", Judgments: map[string]int{}}
	var batch []*judgePairIn
	for _, pid := range lx.familyOrder[:3] {
		batch = append(batch, &judgePairIn{q: q, pid: lx.families[pid][0], rank: 1})
	}
	cost, calls, err := s.judgeBatch(t.Context(), "system", lx, batch)
	if err != nil || calls != 1 || cost <= 0 {
		t.Fatalf("пачка: cost %v calls %d err %v", cost, calls, err)
	}
	for _, p := range batch {
		if p.grade == nil || *p.grade != 2 {
			t.Fatalf("оценка не записана: %+v", p)
		}
	}
}
