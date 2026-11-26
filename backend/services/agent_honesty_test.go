package services

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"kuznica/api/domain"
	"kuznica/api/externalApi/catalogsource"
	"kuznica/api/externalApi/llm"
	"kuznica/api/in"

	"github.com/google/uuid"
)

// Ревью 2026-05-27: B1 (разрез журнала и test), M2 (перепроверка при принятии), M3 (паника и recover),
// M4 (производитель термина и canonical — кодом, кластеры по сессиям), SSE (done и пустые подписки).

// qSource — источник фикстуры с наборами запросов.
type qSource struct {
	CatalogSource
	qs []domain.EvalQuery
}

func (q qSource) LoadQueries(context.Context) ([]domain.EvalQuery, error) { return q.qs, nil }

// failSearches — n пустых поисков текста: каждая — своя сессия (sessions=true) или одна на все.
func failSearches(text string, n int, at time.Time, sessions bool) []domain.SearchEvent {
	var res []domain.SearchEvent
	one := uuid.NewString()
	for i := 0; i < n; i++ {
		sess := one
		if sessions {
			sess = uuid.NewString()
		}
		ev := domain.SearchEvent{RequestID: uuid.NewString()[:30], Query: text, ResultsCount: 0, SessionID: &sess,
			Source: domain.EventSourceMockLog}
		ev.CreatedAt = at.Add(time.Duration(i) * time.Minute)
		res = append(res, ev)
	}
	return res
}

// B1: агент не видит отложенную неделю и тексты test.
func TestAgentHoldoutAndTestExcluded(t *testing.T) {
	s, ms := agentEnv(t, &scriptModel{})
	now := s.now()
	ms.searches = append(ms.searches, failSearches("акад", 3, now.Add(-10*24*time.Hour), true)...)
	ms.searches = append(ms.searches, failSearches("зуп", 4, now.Add(-10*24*time.Hour), true)...)
	s.source = qSource{CatalogSource: s.source, qs: []domain.EvalQuery{{ID: "q-1", Text: "ЗУП", Set: domain.EvalSetTest}}}

	h := HoldoutOptions{Enabled: true, From: now.Add(-5 * 24 * time.Hour), ExcludeTest: true}
	st, info, err := s.agentQueryStats(t.Context(), 30, h)
	if err != nil {
		t.Fatal(err)
	}
	if st.byText["акад"] == nil {
		t.Fatal("поиски до отложенного окна агент видит")
	}
	if st.byText["кис"] != nil || st.byText["kaspersky internet security"] != nil {
		t.Fatal("поиски отложенной недели агенту не видны")
	}
	if st.byText["зуп"] != nil {
		t.Fatal("тексты test агенту не видны")
	}
	if info.HiddenEvents != 6 || info.TestTexts != 1 || info.HoldoutFrom == nil {
		t.Fatalf("сводка разреза: %+v", info)
	}
	// Без разреза — весь журнал (для сравнения).
	st, _, _ = s.agentQueryStats(t.Context(), 30, HoldoutOptions{})
	if st.byText["кис"] == nil || st.byText["зуп"] == nil {
		t.Fatal("без разреза агент видит весь журнал")
	}
}

// B1: переформулировка в текст test тоже скрыта.
func TestQueryStatsExcludeReformulations(t *testing.T) {
	st := &queryStats{byText: map[string]*clusterQuery{
		"зп":  {Text: "зп", ReformulatedTo: []reformTarget{{Text: "зуп", Count: 2}, {Text: "1с зарплата", Count: 1}}},
		"зуп": {Text: "зуп"},
	}, clickedProduct: map[string]map[string]int{"зуп": {"x": 1}}}
	if n := st.exclude(map[string]bool{"зуп": true}); n != 1 {
		t.Fatalf("убрано %d", n)
	}
	if len(st.byText["зп"].ReformulatedTo) != 1 || st.byText["зп"].ReformulatedTo[0].Text != "1с зарплата" || st.clickedProduct["зуп"] != nil {
		t.Fatalf("переформулировка в test должна исчезнуть: %+v", st.byText["зп"])
	}
}

// B1: окно — последние 7 дней исторического журнала по Москве; живые поиски сегодня его не сдвигают.
func TestHoldoutRangeAuto(t *testing.T) {
	s := &Services{now: time.Now}
	last := time.Date(2026, 5, 26, 15, 0, 0, 0, mskZone)
	live := domain.SearchEvent{Source: domain.EventSourceLive}
	live.CreatedAt = time.Date(2026, 5, 27, 12, 0, 0, 0, mskZone)
	hist := domain.SearchEvent{Source: domain.EventSourceMockLog}
	hist.CreatedAt = last
	from, to := s.holdoutRange(HoldoutOptions{}, []domain.SearchEvent{hist, live})
	if !from.Equal(time.Date(2026, 5, 20, 0, 0, 0, 0, mskZone)) || !to.Equal(time.Date(2026, 5, 27, 0, 0, 0, 0, mskZone)) {
		t.Fatalf("окно %s – %s", from, to)
	}
	explicit := time.Date(2026, 5, 12, 0, 0, 0, 0, mskZone)
	if from, to = s.holdoutRange(HoldoutOptions{From: explicit, Days: 3}, nil); !from.Equal(explicit) || to.Sub(from) != 72*time.Hour {
		t.Fatalf("явное окно %s – %s", from, to)
	}
}

// B1: реплей недели — различные (текст, клиент), частота = числу поисков, разметка по тексту.
func TestBuildHoldoutQueries(t *testing.T) {
	from := time.Date(2026, 5, 20, 0, 0, 0, 0, mskZone)
	to := from.Add(7 * 24 * time.Hour)
	evs := append(failSearches("Акад", 3, from.Add(time.Hour), true), failSearches("акад", 1, from.Add(-time.Hour), true)...)
	evs = append(evs, failSearches("нечто", 2, from.Add(2*time.Hour), true)...)
	liveEv := failSearches("акад", 5, from.Add(3*time.Hour), true)
	for i := range liveEv {
		liveEv[i].Source = domain.EventSourceLive
	}
	evs = append(evs, liveEv...)
	judged := []domain.EvalQuery{{ID: "q-7", Text: "акад", Type: domain.QueryAliasGap, Judgments: map[string]int{"p": 3}}}
	qs := buildHoldoutQueries(evs, judged, from, to)
	if len(qs) != 2 {
		t.Fatalf("запросов: %+v", qs)
	}
	byID := map[string]domain.EvalQuery{}
	for _, q := range qs {
		byID[q.ID] = q
	}
	a := byID["h-q-7"]
	if a.Freq != 3 || a.Type != domain.QueryAliasGap || a.Judgments["p"] != 3 {
		t.Fatalf("размеченный запрос недели: %+v", a)
	}
	for id, q := range byID {
		if id != "h-q-7" && (q.Type != "unlabeled" || q.Freq != 2) {
			t.Fatalf("неразмеченный: %+v", q)
		}
	}
}

// M4: потерянный спрос — по различным сессиям: сотня поисков из одной сессии не выше пяти покупателей.
func TestClusterLostDemandBySessions(t *testing.T) {
	now := time.Now()
	evs := append(failSearches("фотошоп купить скидка", 100, now, false), failSearches("кис", 5, now, true)...)
	st := computeQueryStats(evs, nil)
	failed := st.failed()
	if len(failed) != 2 || failed[0].Text != "кис" || failed[0].FailSessions != 5 || failed[1].FailSessions != 1 {
		t.Fatalf("порядок неудачных: %+v", failed)
	}
	cs := clusterFailed(failed, nil, nil, nil)
	if cs[0].Label != "кис" || cs[0].LostDemand != 5 || cs[1].LostDemand != 1 || cs[1].Failures != 100 {
		t.Fatalf("кластеры: %+v", cs)
	}
}

// M4: canonical должен называть цель.
func TestCheckCanonical(t *testing.T) {
	snap, err := fixtureSource().Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	c := newAgentCatalog(snap)
	ok := []struct{ target, id, canonical, vendor string }{
		{domain.AliasTargetFamily, "kaspersky-internet-security", "Kaspersky Internet Security", "kaspersky"},
		{domain.AliasTargetFamily, "1c-accounting", "1C:Бухгалтерия", "1c"},
		{domain.AliasTargetVendor, "kaspersky", "касперский", "kaspersky"},
	}
	for _, tc := range ok {
		v, err := c.checkCanonical(tc.target, tc.id, tc.canonical)
		if err != nil || v != tc.vendor {
			t.Fatalf("%s → %s: %v %q", tc.canonical, tc.id, err, v)
		}
	}
	bad := []struct{ target, id, canonical string }{
		{domain.AliasTargetFamily, "fotomaster", "Adobe Photoshop"},
		{domain.AliasTargetFamily, "kaspersky-internet-security", "kaspersky internet security фотомастер"},
		{domain.AliasTargetVendor, "kaspersky", "photoshop"},
		{domain.AliasTargetFamily, "нет-такого", "что-то"},
	}
	for _, tc := range bad {
		if _, err := c.checkCanonical(tc.target, tc.id, tc.canonical); err == nil {
			t.Fatalf("%s → %s должно отклоняться", tc.canonical, tc.id)
		}
	}
	if v := c.termVendor("фотомастер"); v != "amssoft" {
		t.Fatalf("производитель термина: %q", v)
	}
}

// M4: производитель термина — по данным; слово модели («term_vendor») не разрешает синоним на чужой товар,
// а без доказательств правка не проходит.
func TestAgentTermVendorFromData(t *testing.T) {
	model := &scriptModel{}
	s, ms := agentEnv(t, model)
	ms.searches = append(ms.searches, failSearches("фотошопчик", 6, time.Now().Add(-72*time.Hour), true)...)
	s.agent.evalHook = func(context.Context, *failedCluster, RulePatch) (*patchEval, error) {
		t.Fatal("прогон не должен вызываться")
		return nil, nil
	}
	// Кластер 1 — «фотошопчик» (6 сессий, без переформулировок): модель уверяет, что это ФотоМАСТЕР.
	model.turns = [][]llm.ToolCall{
		{tc("create_sandbox_patch", map[string]any{"diagnosis": "ALIAS_GAP", "hypothesis": "фотошопчик — это ФотоМАСТЕР",
			"add": []map[string]any{{"alias": "фотошопчик", "canonical": "ФотоМАСТЕР", "target": "family", "target_id": "fotomaster", "term_vendor": "AMS Software"}}})},
		{tc("create_sandbox_patch", map[string]any{"diagnosis": "ALIAS_GAP", "hypothesis": "термин",
			"add": []map[string]any{{"alias": "фотошопчик", "canonical": "фотомастер", "target": "term", "term_vendor": "AMS Software"}}})},
		{tc("close_cluster", map[string]any{"diagnosis": "UNKNOWN", "reason": "нет доказательств"})},
		// Кластер 2 — «кис» (клики после переформулировки в Kaspersky Internet Security): цель — Photoshop,
		// модель называет Adobe — код видит Kaspersky.
		{tc("create_sandbox_patch", map[string]any{"diagnosis": "ALIAS_GAP", "hypothesis": "кис — Photoshop",
			"add": []map[string]any{{"alias": "кис", "canonical": "Adobe Photoshop", "target": "family", "target_id": "adobe-photoshop", "term_vendor": "Adobe"}}})},
		// canonical не называет цель — отказ инструмента до гипотезы.
		{tc("create_sandbox_patch", map[string]any{"diagnosis": "ALIAS_GAP", "hypothesis": "подмена canonical",
			"add": []map[string]any{{"alias": "кис", "canonical": "Adobe Photoshop", "target": "family", "target_id": "kaspersky-internet-security"}}})},
		{tc("close_cluster", map[string]any{"diagnosis": "UNKNOWN", "reason": "нет правки"})},
	}
	r, err := s.StartAgentRun(t.Context(), in.AgentRunRequest{MaxClusters: 2})
	if err != nil {
		t.Fatal(err)
	}
	run := waitRun(t, s, r.RunID)
	if run.ProposalsCreated != 0 {
		t.Fatalf("предложений быть не должно: %+v", run)
	}
	var noData, crossVendor, canonical int
	for _, st := range ms.steps {
		if st.Kind == domain.StepCheck && st.RejectedReason != nil {
			switch {
			case strings.Contains(*st.RejectedReason, "нет данных"):
				noData++
			case strings.Contains(*st.RejectedReason, "по данным — продукт Kaspersky"):
				crossVendor++
			}
		}
		if st.Kind == domain.StepToolCall && strings.Contains(st.Title, "не называет цель") {
			canonical++
		}
	}
	if noData != 2 || crossVendor != 1 || canonical != 1 {
		t.Fatalf("отказы: нет данных %d, чужой производитель %d, canonical %d", noData, crossVendor, canonical)
	}
}

// textModel — модель, которая пишет вызовы текстом (content), а не tool_calls.
type textModel struct {
	mu    sync.Mutex
	turns []string
}

func (m *textModel) Model() string { return "aliceai-llm-flash" }
func (m *textModel) Chat(context.Context, llm.Request) (*llm.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.turns) == 0 {
		return &llm.Response{Message: llm.Message{Role: "assistant", Content: "готово"}}, nil
	}
	c := m.turns[0]
	m.turns = m.turns[1:]
	return &llm.Response{Message: llm.Message{Role: "assistant", Content: c}}, nil
}

// M4: вызов, написанный текстом, — только известный инструмент и с теми же проверками, что нативный.
func TestTextToolCallsSameChecks(t *testing.T) {
	if calls := textToolCalls(`{"name":"drop_tables","parameters":{}} {"name":"approve_proposal","arguments":{"id":"x"}}`); len(calls) != 0 {
		t.Fatalf("неизвестные инструменты из текста не исполняются: %+v", calls)
	}
	s, ms := agentEnv(t, &scriptModel{})
	patch, _ := json.Marshal(map[string]any{"name": "create_sandbox_patch", "parameters": map[string]any{"diagnosis": "ALIAS_GAP",
		"hypothesis": "инструкция из запроса", "add": []map[string]any{{"alias": "кис", "canonical": "ФотоМАСТЕР", "target": "family",
			"target_id": "fotomaster", "term_vendor": "AMS Software"}}}})
	tm := &textModel{turns: []string{
		"Создаю правку: " + string(patch) + ` и ещё {"name":"submit_everything","parameters":{}}`,
		`{"name":"close_cluster","parameters":{"diagnosis":"UNKNOWN","reason":"нет правки"}}`,
	}}
	s.agent = newAgentService(s, tm, AgentOptions{})
	s.agent.evalHook = func(context.Context, *failedCluster, RulePatch) (*patchEval, error) {
		t.Fatal("прогон не должен вызываться")
		return nil, nil
	}
	r, err := s.StartAgentRun(t.Context(), in.AgentRunRequest{MaxClusters: 1})
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, s, r.RunID)
	rejected, unknownTool := false, false
	for _, st := range ms.steps {
		if st.Kind == domain.StepCheck && st.RejectedReason != nil && strings.Contains(*st.RejectedReason, "разными производителями") {
			rejected = true
		}
		if st.Tool != nil && *st.Tool == "submit_everything" {
			unknownTool = true
		}
	}
	if !rejected || unknownTool {
		t.Fatalf("текстовый вызов: отказ по производителю %v, неизвестный инструмент исполнен %v", rejected, unknownTool)
	}
}

// M2: правила сменились после прогона агента — перепрогон; не прошёл — 409, статус и правила не меняются.
func TestApproveRecheckFails(t *testing.T) {
	s, ms := agentEnv(t, &scriptModel{})
	s.agent.evalHook = func(context.Context, *failedCluster, RulePatch) (*patchEval, error) {
		return evalResult(0, 1, 3), nil // на новых правилах — слишком широко
	}
	ms.versions = append(ms.versions, domain.ConfigVersion{Version: 2, Rules: ms.versions[0].Rules})
	p := &domain.Proposal{Status: domain.ProposalPending, Action: domain.ActionRulesPatch, ClusterLabel: "кис", BaseVersion: 1,
		ClusterQueries: toJSON([]clusterQuery{{Text: "кис", Freq: 3}}), Evaluation: toJSON(evalResult(0, 1, 0)),
		Patch: toJSON(RulePatch{Add: []PatchAlias{{Kind: "alias", Alias: "кис", Canonical: "Kaspersky Internet Security",
			Target: "family", TargetID: "kaspersky-internet-security"}}})}
	_ = ms.CreateProposal(t.Context(), p)
	_, err := s.ApproveProposal(t.Context(), p.ID.String(), in.ApproveRequest{})
	if !isConflict(err) || !strings.Contains(err.Error(), "слишком широко") {
		t.Fatalf("ожидался 409 с причиной: %v", err)
	}
	got, _ := ms.GetProposal(t.Context(), p.ID)
	if got.Status != domain.ProposalPending || !strings.Contains(string(got.Evidence), `"recheck"`) {
		t.Fatalf("статус не меняется, перепроверка записана: %s %s", got.Status, got.Evidence)
	}
	if active, _ := s.activeRules(t.Context()); active.Version != 2 || active.find("кис") != nil {
		t.Fatalf("правила не должны меняться: %+v", active)
	}
	// Прошёл — применяется, новый прогон в предложении, прежний — в evidence.initial_evaluation.
	s.agent.evalHook = func(context.Context, *failedCluster, RulePatch) (*patchEval, error) { return evalResult(0, 1, 0), nil }
	res, err := s.ApproveProposal(t.Context(), p.ID.String(), in.ApproveRequest{})
	if err != nil || res.ConfigVersion == nil || *res.ConfigVersion != 3 {
		t.Fatalf("принятие после успешной перепроверки: %+v %v", res, err)
	}
	got, _ = ms.GetProposal(t.Context(), p.ID)
	if got.BaseVersion != 2 || !strings.Contains(string(got.Evidence), "initial_evaluation") {
		t.Fatalf("новый прогон должен быть записан: база %d, %s", got.BaseVersion, got.Evidence)
	}
}

// M3: паника внутри запуска не роняет процесс; запуск — failed, не висит в running, done разослан.
func TestAgentRunPanicRecovered(t *testing.T) {
	model := &scriptModel{}
	s, _ := agentEnv(t, model)
	s.agent.evalHook = func(context.Context, *failedCluster, RulePatch) (*patchEval, error) {
		panic("сбой песочницы")
	}
	model.turns = [][]llm.ToolCall{
		{tc("create_sandbox_patch", map[string]any{"diagnosis": "ALIAS_GAP", "hypothesis": "кис — KIS",
			"add": []map[string]any{{"alias": "кис", "canonical": "Kaspersky Internet Security", "target": "family", "target_id": "kaspersky-internet-security"}}})},
		{tc("run_evaluation", map[string]any{"patch_id": "p1"})},
	}
	r, err := s.StartAgentRun(t.Context(), in.AgentRunRequest{MaxClusters: 1})
	if err != nil {
		t.Fatal(err)
	}
	run := waitRun(t, s, r.RunID)
	if run.Status != domain.RunFailed || run.StopReason != StopError || !strings.Contains(run.Summary, "сбой песочницы") || run.FinishedAt == nil {
		t.Fatalf("запуск после паники: %+v", run)
	}
	// Агент снова доступен.
	model.turns = nil
	r2, err := s.StartAgentRun(t.Context(), in.AgentRunRequest{MaxClusters: 1})
	if err != nil {
		t.Fatalf("после паники агент должен запускаться: %v", err)
	}
	waitRun(t, s, r2.RunID)
}

// M3: записи учёта запуска ждутся своим счётчиком; добавление во время ожидания не паникует.
func TestRunWritesWait(t *testing.T) {
	w := &runWrites{}
	w.add()
	go func() {
		time.Sleep(20 * time.Millisecond)
		w.add()
		w.done()
		w.done()
	}()
	if !w.wait(time.Second) {
		t.Fatal("записи должны дождаться")
	}
	w.add()
	if w.wait(30 * time.Millisecond) {
		t.Fatal("незавершённая запись — ожидание с потолком")
	}
	var nilW *runWrites
	nilW.add()
	nilW.done()
	if !nilW.wait(0) {
		t.Fatal("без счётчика ждать нечего")
	}
	ctx := withRunWrites(context.Background(), w)
	if runWritesOf(ctx) != w || runWritesOf(context.Background()) != nil {
		t.Fatal("счётчик в контексте")
	}
}

// SSE: пустая подписка удаляется; после окончания запуска подписчик получает закрытый канал, а не
// «живой» запуск без done.
func TestAgentStreamDoneAndEmptySubs(t *testing.T) {
	model := &scriptModel{}
	s, _ := agentEnv(t, model)
	release := make(chan struct{})
	s.agent.evalHook = func(context.Context, *failedCluster, RulePatch) (*patchEval, error) {
		<-release
		return evalResult(0, 0, 0), nil
	}
	model.turns = [][]llm.ToolCall{
		{tc("create_sandbox_patch", map[string]any{"diagnosis": "ALIAS_GAP", "hypothesis": "кис — KIS",
			"add": []map[string]any{{"alias": "кис", "canonical": "Kaspersky Internet Security", "target": "family", "target_id": "kaspersky-internet-security"}}})},
		{tc("run_evaluation", map[string]any{"patch_id": "p1"})},
		{tc("close_cluster", map[string]any{"diagnosis": "UNKNOWN", "reason": "нет правки"})},
	}
	r, err := s.StartAgentRun(t.Context(), in.AgentRunRequest{MaxClusters: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, _, ch, cancel, err := s.AgentStream(t.Context(), r.RunID, 0)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	s.agent.mu.Lock()
	_, left := s.agent.subs[uuid.MustParse(r.RunID)]
	s.agent.mu.Unlock()
	if left {
		t.Fatal("пустая карта подписок должна удаляться")
	}
	if _, open := <-ch; open {
		// После cancel канал закрыт; события до cancel допустимы — дочитываем.
		for range ch {
		}
	}
	_, _, ch2, cancel2, err := s.AgentStream(t.Context(), r.RunID, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel2()
	close(release)
	gotDone := false
	for ev := range ch2 {
		if ev.Kind == "done" {
			gotDone = true
			s.agent.mu.Lock()
			active := s.agent.active
			s.agent.mu.Unlock()
			if active != nil {
				t.Fatal("к рассылке done запуск уже снят с active")
			}
		}
	}
	if !gotDone {
		t.Fatal("подписчик живого запуска получает done")
	}
	_, _, ch3, cancel3, _ := s.AgentStream(t.Context(), r.RunID, 0)
	defer cancel3()
	if _, open := <-ch3; open {
		t.Fatal("после окончания запуска канал подписки сразу закрыт")
	}
}

// Ревью-2 n8: разметка не читается — агент не стартует, а не учится молча на текстах test.
func TestAgentStopsWithoutTestSet(t *testing.T) {
	s, _ := agentEnv(t, &scriptModel{})
	src := s.source
	s.source = qSourceErr{src, errors.New("queries.json: битая строка")}
	if _, _, err := s.agentQueryStats(t.Context(), 30, HoldoutOptions{Enabled: true, ExcludeTest: true}); err == nil {
		t.Fatal("при нечитаемой разметке вход агента должен давать ошибку")
	}
	// Без исключения test (AGENT_EXCLUDE_TEST=off) разметка не нужна.
	if _, _, err := s.agentQueryStats(t.Context(), 30, HoldoutOptions{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	// Файла разметки нет вовсе — исключать нечего, запуск идёт.
	s.source = qSourceErr{src, catalogsource.ErrNoFile}
	if _, _, err := s.agentQueryStats(t.Context(), 30, HoldoutOptions{Enabled: true, ExcludeTest: true}); err != nil {
		t.Fatal(err)
	}
}

type qSourceErr struct {
	CatalogSource
	err error
}

func (q qSourceErr) LoadQueries(context.Context) ([]domain.EvalQuery, error) { return nil, q.err }

// Ревью-2 n1: пока шёл перепрогон, предложение отклонили — проваленная перепроверка не возвращает его в pending.
func TestRecheckDoesNotResurrect(t *testing.T) {
	s, ms := agentEnv(t, &scriptModel{})
	ms.versions = append(ms.versions, domain.ConfigVersion{Version: 2, Rules: ms.versions[0].Rules})
	p := &domain.Proposal{Status: domain.ProposalPending, Action: domain.ActionRulesPatch, ClusterLabel: "кис", BaseVersion: 1,
		ClusterQueries: toJSON([]clusterQuery{{Text: "кис", Freq: 3}}), Evaluation: toJSON(evalResult(0, 1, 0)),
		Patch: toJSON(RulePatch{Add: []PatchAlias{{Kind: "alias", Alias: "кис", Canonical: "Kaspersky Internet Security",
			Target: "family", TargetID: "kaspersky-internet-security"}}})}
	_ = ms.CreateProposal(t.Context(), p)
	s.agent.evalHook = func(context.Context, *failedCluster, RulePatch) (*patchEval, error) {
		ms.mu.Lock()
		q := ms.proposals[p.ID]
		q.Status, q.DecidedReason = domain.ProposalRejected, domain.RejectNotNow
		ms.proposals[p.ID] = q
		ms.mu.Unlock()
		return evalResult(0, 1, 3), nil
	}
	if _, err := s.ApproveProposal(t.Context(), p.ID.String(), in.ApproveRequest{}); !isConflict(err) {
		t.Fatalf("ожидался 409: %v", err)
	}
	got, _ := ms.GetProposal(t.Context(), p.ID)
	if got.Status != domain.ProposalRejected || strings.Contains(string(got.Evidence), `"recheck"`) {
		t.Fatalf("решение затёрто перепроверкой: %s %s", got.Status, got.Evidence)
	}
}

// `proposals stale`: id запуска — uuid или префикс от 8 знаков, причина обязательна.
func TestStaleProposalsValidation(t *testing.T) {
	s := NewServices(nil, Options{})
	for _, c := range []struct {
		runs   []string
		reason string
	}{{nil, "до разреза"}, {[]string{"01a2"}, "до разреза"}, {[]string{"01a20fe2; drop"}, "до разреза"}, {[]string{"01a20fe2"}, " "}} {
		if _, err := s.StaleProposalsOfRuns(t.Context(), c.runs, c.reason); err == nil || isUnavailable(err) {
			t.Errorf("%v %q: ждали ошибку проверки, есть %v", c.runs, c.reason, err)
		}
	}
	if _, err := s.StaleProposalsOfRuns(t.Context(), []string{"01a20fe2", "01A21002-aaaa"}, "до разреза"); !isUnavailable(err) {
		t.Errorf("верные аргументы без базы — 503: %v", err)
	}
}

func TestProposalViewStaleReason(t *testing.T) {
	now := time.Now()
	v := proposalView(&domain.Proposal{Status: domain.ProposalStale, DecidedAt: &now, DecidedReason: domain.ProposalStalePreHoldout})
	if v.StaleReason == nil || *v.StaleReason != domain.ProposalStalePreHoldout {
		t.Fatalf("stale_reason: %v", v.StaleReason)
	}
	if v := proposalView(&domain.Proposal{Status: domain.ProposalPending}); v.StaleReason != nil {
		t.Fatal("у ждущего stale_reason — null")
	}
}

// Фильтр очереди по stale допустим (вкладка устаревших предложений).
func TestListProposalsStale(t *testing.T) {
	s, ms := agentEnv(t, &scriptModel{})
	p := &domain.Proposal{Status: domain.ProposalStale, DecidedReason: domain.ProposalStalePreHoldout}
	_ = ms.CreateProposal(t.Context(), p)
	res, err := s.Proposals(t.Context(), domain.ProposalStale, "")
	if err != nil || len(res.Items) != 1 || res.Items[0].StaleReason == nil {
		t.Fatalf("stale: %+v %v", res, err)
	}
}
