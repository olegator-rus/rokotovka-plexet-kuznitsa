package services

import (
	"strings"
	"testing"
	"time"

	"kuznica/api/domain"
)

// evs — конструктор журнала: сессия, смещение в секундах от базы, тип, запрос или товар, позиция.
type logStep struct {
	session string
	sec     int
	typ     string
	arg     string
	pos     int
}

func logEvents(base time.Time, steps []logStep) []domain.LogEvent {
	var res []domain.LogEvent
	for _, s := range steps {
		e := domain.LogEvent{TS: base.Add(time.Duration(s.sec) * time.Second), SessionID: s.session, Type: s.typ}
		if s.typ == "search" {
			e.Query = s.arg
			n := 3
			if strings.HasPrefix(s.arg, "пусто") {
				n = 0
			}
			e.ResultsCount = &n
		} else {
			e.ProductID = s.arg
			if s.pos > 0 {
				p := s.pos
				e.Position = &p
			}
		}
		res = append(res, e)
	}
	return res
}

func TestImplicitGrades(t *testing.T) {
	base := time.Date(2026, 5, 12, 9, 0, 0, 0, time.UTC)
	evs := logEvents(base, []logStep{
		{"s1", 0, "search", "Касперский", 0}, {"s1", 10, "click", "kis-2dev-1y", 3}, {"s1", 40, "order", "kis-2dev-1y", 0},
		{"s2", 0, "search", "касперский", 0}, {"s2", 10, "add_to_cart", "kis-5dev-1y", 0},
		// Клик и через 20 с другой поиск — клик не помог.
		{"s3", 0, "search", "касперский", 0}, {"s3", 10, "click", "ksos-10dev-1y", 1}, {"s3", 30, "search", "ksos", 0},
		// Клик без переформулировки — помог.
		{"s4", 0, "search", "касперский", 0}, {"s4", 5, "click", "kis-5dev-1y", 2},
		// Пустая выдача и переформулировка дважды.
		{"s5", 0, "search", "пусто акад", 0}, {"s5", 20, "search", "autocad", 0}, {"s5", 30, "click", "acad-1y", 1},
		{"s6", 0, "search", "пусто акад", 0}, {"s6", 20, "search", "autocad", 0},
	})
	searches, behavior, _ := logRows(evs)
	lg := aggregateLog(searches, behavior)
	q := lg.queries["касперский"]
	if q == nil || q.freq != 4 || q.clicked != 4 || q.text == "" {
		t.Fatalf("запрос: %+v", q)
	}
	grade := func(pid string, click int) int {
		g, ok := implicitGrade(q.products[pid], q.freq, 1, click)
		if !ok {
			t.Fatalf("%s без оценки", pid)
		}
		return g
	}
	if grade("kis-2dev-1y", 2) != 3 || grade("kis-5dev-1y", 2) != 2 || grade("ksos-10dev-1y", 2) != 1 {
		t.Fatalf("оценки: заказ 3, корзина 2, неудачный клик 1: %+v", q.products)
	}
	if st := q.products["kis-2dev-1y"]; st.ipw < 1.7 || st.c3 != 1 {
		t.Fatalf("клик с 3-го места весит √3: %+v", st)
	}
	// Поддержка: при min-support 2 одиночный заказ оценки не даёт.
	if _, ok := implicitGrade(q.products["kis-2dev-1y"], q.freq, 2, 2); ok {
		t.Fatal("поддержка 2 не соблюдена")
	}
	// Строгая схема: помогший клик — 1.
	if g, _ := implicitGrade(&implicitStat{c1: 1, sat: 1, ipw: 1}, 10, 1, 1); g != 1 {
		t.Fatalf("строгая схема: %d", g)
	}
	a := lg.queries["пусто акад"]
	if a.empty != 2 || a.reform["autocad"] != 2 || a.reformHit["autocad"] != 1 {
		t.Fatalf("переформулировки: %+v", a)
	}
	if to, found := a.topReform(); to != "autocad" || !found {
		t.Fatalf("главная переформулировка: %s %v", to, found)
	}
}

func TestRuleTypes(t *testing.T) {
	lx, rules, _ := mockLexicon(t)
	for _, c := range []struct{ text, typ string }{
		{"касперский на 10 пк", domain.QueryLicense},
		{"rfcgthcrbq", domain.QueryLayout},
		{"счет на оплату", domain.QueryNonProduct},
		{"аналог фотошопа", domain.QueryAnalog},
		{"продлить dr.web", domain.QueryRenewal},
		{"123", domain.QueryNoise},
		{"kaspersky internet security", domain.QueryExact},
	} {
		typ, sure, _ := ruleType(c.text, lx, rules)
		if typ != c.typ || !sure {
			t.Errorf("%q: %s (уверенно %v), ждали %s", c.text, typ, sure, c.typ)
		}
	}
	// Непонятное слово — решает модель.
	if typ, sure, unknown := ruleType("ноушн", lx, rules); sure || len(unknown) == 0 {
		t.Fatalf("ноушн: %s %v %v", typ, sure, unknown)
	}
}

func TestParseClassify(t *testing.T) {
	got, err := parseClassify("```json\n{\"types\":[{\"id\":\"d-1\",\"type\":\"Absent\"},{\"id\":\"d-2\",\"type\":\"мусор\"}]}\n```")
	if err != nil || got["d-1"] != domain.QueryAbsent || len(got) != 1 {
		t.Fatalf("разбор: %v %v", got, err)
	}
	if _, err := parseClassify("не json"); err == nil {
		t.Fatal("не JSON принят")
	}
}

func TestAssignSetsByCluster(t *testing.T) {
	holdout := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	var qs []*DerivedQuery
	first := map[string]time.Time{}
	for i := range 40 {
		q := &DerivedQuery{EvalQuery: domain.EvalQuery{ID: derivedID(i + 1), Type: domain.QueryExact, Freq: 40 - i,
			Cluster: "c" + itoa(i/2)}}
		first[q.ID] = holdout.Add(-72 * time.Hour)
		qs = append(qs, q)
	}
	// Кластер только из отложенной недели.
	for _, id := range []string{"x-1", "x-2"} {
		qs = append(qs, &DerivedQuery{EvalQuery: domain.EvalQuery{ID: id, Type: domain.QueryAbsent, Freq: 5, Cluster: "new"}})
		first[id] = holdout.Add(time.Hour)
	}
	assignSets(qs, first, holdout)
	setOf := map[string]string{}
	count := map[string]int{}
	for _, q := range qs {
		if s, ok := setOf[q.Cluster]; ok && s != q.Set {
			t.Fatalf("кластер %s разрезан: %s и %s", q.Cluster, s, q.Set)
		}
		setOf[q.Cluster] = q.Set
		count[q.Set]++
	}
	if setOf["new"] != domain.EvalSetTest {
		t.Fatalf("кластер отложенной недели не в test: %s", setOf["new"])
	}
	if count[domain.EvalSetTune] < 14 || count[domain.EvalSetTune] > 18 || count[domain.EvalSetRegression] < 6 || count[domain.EvalSetRegression] > 10 {
		t.Fatalf("доли 40/40/20 по запросам: %v", count)
	}
}

func TestClusterQueries(t *testing.T) {
	qs := []*DerivedQuery{
		{EvalQuery: domain.EvalQuery{ID: "a", Text: "офис 2021", Judgments: map[string]int{"p1": 3, "p2": 2}}},
		{EvalQuery: domain.EvalQuery{ID: "b", Text: "2021 офис", Judgments: map[string]int{}}},
		{EvalQuery: domain.EvalQuery{ID: "c", Text: "office 2021", Judgments: map[string]int{"p1": 2, "p2": 3}}},
		{EvalQuery: domain.EvalQuery{ID: "d", Text: "винрар", Judgments: map[string]int{}}},
	}
	clusterQueries(qs, map[string]*setsLogQuery{}, nil, nil, nil)
	if qs[0].Cluster != qs[1].Cluster || qs[0].Cluster != qs[2].Cluster || qs[3].Cluster == qs[0].Cluster {
		t.Fatalf("кластеры: %s %s %s %s", qs[0].Cluster, qs[1].Cluster, qs[2].Cluster, qs[3].Cluster)
	}
}

// TestBuildSets — сборка на фикстуре: наборы по кластерам, неявная оценка не перезаписывается судьёй, лимит и
// кэш судьи, сверка с эталоном и отчёт.
func TestBuildSets(t *testing.T) {
	env := imported(t, false)
	f := &fakeJudge{}
	env.svc.judge = f
	base := time.Date(2026, 5, 12, 9, 0, 0, 0, time.UTC)
	var steps []logStep
	for i := range 4 {
		s := "k" + itoa(i)
		steps = append(steps, logStep{s, i * 3600, "search", "Kaspersky Internet", 0}, logStep{s, i*3600 + 10, "click", "kis-2dev-1y", 1},
			logStep{s, i*3600 + 30, "order", "kis-2dev-1y", 0})
	}
	for i := range 3 {
		s := "p" + itoa(i)
		steps = append(steps, logStep{s, i * 3600, "search", "фотошоп", 0}, logStep{s, i*3600 + 10, "click", "photoshop-1u-1y", 2})
	}
	for i := range 2 {
		steps = append(steps, logStep{"a" + itoa(i), i * 3600, "search", "пусто asdf", 0})
	}
	for i, q := range []string{"офис", "антивирус", "бухгалтерия", "редактор фото"} {
		for r := range 2 {
			steps = append(steps, logStep{"o" + itoa(i) + itoa(r), i*600 + r*3600, "search", q, 0})
		}
	}
	steps = append(steps, logStep{"z", 0, "search", "Office", 0}) // частота 1 — не войдёт
	dir := t.TempDir()
	req := BuildSetsRequest{Events: logEvents(base, steps), JudgeMaxRub: 100, JudgeCalib: 1, CacheDir: dir,
		Reference: []domain.EvalQuery{{ID: "r1", Text: "kaspersky internet", Type: domain.QueryExact, Cluster: "kis",
			Judgments: map[string]int{"kis-2dev-1y": 3, "kis-5dev-1y": 3}}}}
	res, err := env.svc.BuildSets(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Queries) != 7 || res.Report.BelowFreq != 1 {
		t.Fatalf("отбор по частоте: %d запросов, ниже порога %d", len(res.Queries), res.Report.BelowFreq)
	}
	byText := map[string]*DerivedQuery{}
	for _, q := range res.Queries {
		byText[foldQuery(q.Text)] = q
		if q.Set != domain.EvalSetTune && q.Set != domain.EvalSetTest && q.Set != domain.EvalSetRegression {
			t.Fatalf("набор %q", q.Set)
		}
	}
	k := byText["kaspersky internet"]
	if k.Judgments["kis-2dev-1y"] != 3 || k.Sources["kis-2dev-1y"] != labelImplicit {
		t.Fatalf("неявная оценка перезаписана: %+v %+v", k.Judgments, k.Sources)
	}
	if byText["фотошоп"].Judgments["photoshop-1u-1y"] != 2 {
		t.Fatalf("помогший клик — 2: %+v", byText["фотошоп"].Judgments)
	}
	if e := byText["пусто asdf"]; e.Problem != problemEmpty || e.EmptyShare != 1 {
		t.Fatalf("проблемный запрос: %+v", e)
	}
	j := res.Report.Judge
	if j == nil || j.Calls == 0 || j.Filled == 0 || j.CostRub <= 0 || j.VsImplicit.Judged == 0 || j.PoolFamilies == 0 {
		t.Fatalf("судья: %+v", j)
	}
	if k.Sources["kis-5dev-1y"] == labelJudge && k.Judgments["kis-5dev-1y"] != 2 {
		t.Fatalf("оценка судьи не записана: %+v", k.Judgments)
	}
	ref := res.Report.Reference
	if ref == nil || ref.Matched != 1 || ref.TypeCorrect != 1 || ref.ImplicitVsRef.Judged != 1 || *ref.ImplicitVsRef.Exact != 1 {
		t.Fatalf("сверка: %+v", ref)
	}
	md := FormatSetsMarkdown(res.Report, "фикстура")
	for _, want := range []string{"## Наборы", "## Разметка", "## Сверка с разметкой по построению", "| implicit |"} {
		if !strings.Contains(md, want) {
			t.Fatalf("SETS.md без %q:\n%s", want, md)
		}
	}

	// Повтор — оценки судьи из кэша, вызовов нет.
	calls := f.calls
	again, err := env.svc.BuildSets(t.Context(), req)
	if err != nil || f.calls != calls || again.Report.Judge.Cached != j.Judged {
		t.Fatalf("кэш судьи: %v, вызовов %d→%d, из кэша %d из %d", err, calls, f.calls, again.Report.Judge.Cached, j.Judged)
	}

	// Лимит: дешевле одного вызова после первого — дальше не оценивается.
	req.CacheDir, req.JudgeMaxRub = "", 0.0001
	f.calls = 0
	capped, err := env.svc.BuildSets(t.Context(), req)
	if err != nil || capped.Report.Judge.PoolPairs <= judgeBatch || f.calls != 1 || capped.Report.Judge.Stopped == "" {
		t.Fatalf("лимит: %v, вызовов %d, %+v", err, f.calls, capped.Report.Judge)
	}
}

func TestBuildSetsNeedsJournal(t *testing.T) {
	env := imported(t, false)
	if _, err := env.svc.BuildSets(t.Context(), BuildSetsRequest{Events: []domain.LogEvent{}}); err == nil {
		t.Fatal("пустой журнал принят")
	}
}

// TestApplyTargetFamily — клик 2 → 3 только у exact/sku и только в семействе с наибольшей неявной поддержкой; при
// равенстве лидеров правило не применяется.
func TestApplyTargetFamily(t *testing.T) {
	fam := map[string]string{"a1": "A", "a2": "A", "b1": "B"}
	family := func(pid string) string { return fam[pid] }
	mk := func(typ string) *DerivedQuery {
		return &DerivedQuery{EvalQuery: domain.EvalQuery{Type: typ, Judgments: map[string]int{"a1": 2, "a2": 1, "b1": 2}},
			Sources: map[string]string{"a1": labelImplicit, "a2": labelImplicit, "b1": labelImplicit}}
	}
	q := mk(domain.QueryExact)
	if n := applyTargetFamily(q, map[string]int{"a1": 3, "a2": 1, "b1": 2}, family); n != 1 || q.TargetFamily != "A" ||
		q.Judgments["a1"] != 3 || q.Judgments["a2"] != 1 || q.Judgments["b1"] != 2 {
		t.Fatalf("цель A: %d %+v %s", n, q.Judgments, q.TargetFamily)
	}
	tie := mk(domain.QuerySKU)
	if n := applyTargetFamily(tie, map[string]int{"a1": 2, "b1": 2}, family); n != 0 || tie.TargetFamily != "" {
		t.Fatalf("равенство — без цели: %d %s", n, tie.TargetFamily)
	}
	other := mk(domain.QueryPurpose)
	if n := applyTargetFamily(other, map[string]int{"a1": 3}, family); n != 0 {
		t.Fatalf("не exact — правило не применяется: %d", n)
	}
}
