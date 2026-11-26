package services

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"kuznica/api/domain"
	"kuznica/api/externalApi/llm"
	"kuznica/api/in"
	"kuznica/api/out"

	"github.com/google/uuid"
)

// Контур автономии: триггеры, план, бюджет, память «не предлагать» с условием возврата, проверка эффекта и откат.

// emptySearches — n пустых поисков запроса q в отдельных сессиях, начиная с at (шаг — минута).
func emptySearches(q string, n int, at time.Time) []domain.SearchEvent {
	var res []domain.SearchEvent
	for i := 0; i < n; i++ {
		sess := uuid.NewString()
		ev := domain.SearchEvent{RequestID: "s" + sess[:12], Query: q, ResultsCount: 0, SessionID: &sess, Source: "traffic"}
		ev.CreatedAt = at.Add(time.Duration(i) * time.Minute)
		res = append(res, ev)
	}
	return res
}

func okSearches(q string, n int, at time.Time) []domain.SearchEvent {
	res := emptySearches(q, n, at)
	for i := range res {
		res[i].ResultsCount = 5
	}
	return res
}

func stepsOf(ms *memStore, runID string) []domain.AgentStep {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	var res []domain.AgentStep
	for _, st := range ms.steps {
		if st.RunID.String() == runID {
			res = append(res, st)
		}
	}
	return res
}

func hasStep(steps []domain.AgentStep, kind, sub string) bool {
	for _, st := range steps {
		if st.Kind == kind && strings.Contains(st.Title, sub) {
			return true
		}
	}
	return false
}

// kisPatchRun — сценарий модели: правка «кис» → прогон → в очередь.
func kisPatchTurns() [][]llm.ToolCall {
	patch := map[string]any{"diagnosis": "ALIAS_GAP", "hypothesis": "кис — KIS",
		"add": []map[string]any{{"alias": "кис", "canonical": "kaspersky internet security", "target": "family",
			"target_id": "kaspersky-internet-security", "term_vendor": "Kaspersky"}}}
	return [][]llm.ToolCall{
		{tc("create_sandbox_patch", patch)}, {tc("run_evaluation", map[string]any{"patch_id": "p1"})},
		{tc("submit_proposal", map[string]any{"patch_id": "p1", "diagnosis_text": "сокращение"})},
	}
}

// Отклонённое не всплывает без новых условий; при наступившем условии агент предлагает снова и пишет, что изменилось;
// человек снимает запись с причиной — это решение.
func TestP5SuppressReturnCondition(t *testing.T) {
	model := &scriptModel{}
	s, ms := agentEnv(t, model)
	s.agent.evalHook = func(context.Context, *failedCluster, RulePatch) (*patchEval, error) { return evalResult(0, 1, 0), nil }
	model.turns = kisPatchTurns()
	r1, err := s.StartAgentRun(t.Context(), in.AgentRunRequest{MaxClusters: 1})
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, s, r1.RunID)
	list, _ := s.Proposals(t.Context(), domain.ProposalPending, "")
	if len(list.Items) != 1 {
		t.Fatalf("очередь: %+v", list)
	}
	if list.Items[0].Trigger != domain.TriggerManual || list.Items[0].RunStarted == nil {
		t.Fatalf("у предложения — триггер и запуск: %+v", list.Items[0])
	}
	if _, err := s.RejectProposal(WithActor(t.Context(), RoleEcom), list.Items[0].ID,
		in.RejectRequest{Reason: domain.RejectOther, Comment: "бесплатных версий не продаём"}); err != nil {
		t.Fatal(err)
	}
	if len(ms.suppress) != 1 || ms.suppress[0].FailuresAt != 3 || ms.suppress[0].ReturnFailures != 3 ||
		ms.suppress[0].Scope != domain.SuppressScopeCluster || ms.suppress[0].SourceRunID.String() != r1.RunID {
		t.Fatalf("запись «не предлагать»: %+v", ms.suppress)
	}

	// Второй запуск: условия возврата не наступили — пропуск шагом memory_skip, кластер не разбирается.
	model.turns = kisPatchTurns()
	r2, _ := s.StartAgentRun(t.Context(), in.AgentRunRequest{MaxClusters: 1})
	run := waitRun(t, s, r2.RunID)
	steps := stepsOf(ms, r2.RunID)
	if run.ProposalsCreated != 0 || !hasStep(steps, domain.StepMemorySkip, "условия возврата не наступили") ||
		!hasStep(steps, domain.StepMemorySkip, "бесплатных версий не продаём") {
		t.Fatalf("отклонённое всплыло без новых условий: %+v", steps)
	}
	v := s.agent.runView(run)
	if v.Outcome.SkippedMemory != 1 {
		t.Fatalf("итог: пропущено по памяти 1: %+v", v.Outcome)
	}
	if ms.suppress[0].CurrentFailures != 0 || ms.suppress[0].CheckedRunID == nil {
		t.Fatalf("текущее значение условия: %+v", ms.suppress[0])
	}

	// После отказа — ещё 3 неудачных поиска (столько же, сколько при отказе): условие наступило — агент предлагает
	// снова, «что изменилось». Поиски до отказа не считаются.
	ms.mu.Lock()
	ms.searches = append(ms.searches, emptySearches("кис", 3, time.Now().Add(-3*time.Minute))...)
	ms.mu.Unlock()
	s.now = func() time.Time { return time.Now().Add(5 * time.Minute) }
	ms.suppress[0].DecidedAt = time.Now().Add(-4 * time.Minute)
	model.turns = kisPatchTurns()
	r3, _ := s.StartAgentRun(t.Context(), in.AgentRunRequest{MaxClusters: 1})
	run = waitRun(t, s, r3.RunID)
	steps = stepsOf(ms, r3.RunID)
	if !hasStep(steps, domain.StepMemoryRead, "условие возврата наступило") || run.ProposalsCreated != 1 {
		t.Fatalf("условие наступило — предложить снова: %+v %+v", run, steps)
	}
	if ms.suppress[0].Status != domain.SuppressLifted || ms.suppress[0].LiftedRunID == nil {
		t.Fatalf("запись снята условием: %+v", ms.suppress[0])
	}
	list, _ = s.Proposals(t.Context(), domain.ProposalPending, "")
	if len(list.Items) != 1 || !strings.Contains(string(list.Items[0].Evidence), "what_changed") {
		t.Fatalf("в карточке — что изменилось: %+v", list.Items)
	}

	// Снятие записи человеком: причина обязательна, решение suppress_removed.
	id := ms.suppress[0].ID.String()
	if _, err := s.RemoveSuppress(t.Context(), id, in.SuppressRemoveRequest{}, RoleEcom); err == nil {
		t.Fatal("без причины — отказ")
	}
	if _, err := s.RemoveSuppress(t.Context(), id, in.SuppressRemoveRequest{Reason: "теперь продаём"}, RoleEcom); err != nil {
		t.Fatal(err)
	}
	last := ms.decisions[len(ms.decisions)-1]
	if last.Decision != domain.DecisionSuppressRemoved || last.Comment != "теперь продаём" || last.DecidedBy != RoleEcom {
		t.Fatalf("снятие — решение: %+v", last)
	}
	layer, err := s.AgentMemoryLayer(t.Context(), "suppress")
	if err != nil || len(layer.Suppress) != 1 || layer.Suppress[0].Status != domain.SuppressRemoved {
		t.Fatalf("слой «не предлагать»: %+v %v", layer, err)
	}
	dl, _ := s.AgentMemoryLayer(t.Context(), "decisions")
	if len(dl.Decisions) < 2 || dl.Decisions[0].RunID == nil {
		t.Fatalf("решения с «откуда»: %+v", dl)
	}
	el, _ := s.AgentMemoryLayer(t.Context(), "episodes")
	if len(el.Episodes) < 2 {
		t.Fatalf("эпизоды разборов: %+v", el)
	}
	if _, err := s.AgentMemoryLayer(t.Context(), "nope"); err == nil {
		t.Fatal("неизвестный слой — 422")
	}
}

// Навредившая правка → ночной запуск сам предлагает откат; принятие отката — новая версия тем же механизмом,
// исходная правка → rolled_back.
func TestP5HarmedPatchProposesRollback(t *testing.T) {
	model := &scriptModel{}
	s, ms := agentEnv(t, model)
	s.agent.evalHook = func(_ context.Context, _ *failedCluster, p RulePatch) (*patchEval, error) {
		if len(p.Remove) > 0 {
			// Прогон «без правки»: регрессионный запрос без неё лучше — правка навредила.
			ev := evalResult(0, 0, 0)
			ev.Regression.Better = 1
			ev.Regression.BetterQueries = []out.EvalChange{{Text: "касперский тотал"}}
			return ev, nil
		}
		return evalResult(0, 1, 0), nil
	}
	model.turns = kisPatchTurns()
	r1, _ := s.StartAgentRun(t.Context(), in.AgentRunRequest{MaxClusters: 1})
	waitRun(t, s, r1.RunID)
	list, _ := s.Proposals(t.Context(), domain.ProposalPending, "")
	orig := list.Items[0]
	if _, err := s.ApproveProposal(t.Context(), orig.ID, in.ApproveRequest{}); err != nil {
		t.Fatal(err)
	}
	r2, err := s.StartAgentRun(t.Context(), in.AgentRunRequest{Trigger: domain.TriggerSchedule, Days: 30, MaxClusters: 1})
	if err != nil {
		t.Fatal(err)
	}
	run := waitRun(t, s, r2.RunID)
	steps := stepsOf(ms, r2.RunID)
	if !hasStep(steps, domain.StepEffectCheck, "навредило") || !hasStep(steps, domain.StepProposal, "Предлагаю откат") {
		t.Fatalf("проверка эффекта и откат: %+v", steps)
	}
	v := s.agent.runView(run)
	if v.Outcome.Harmed != 1 || v.Outcome.Rollbacks != 1 || v.Outcome.EffectChecks != 1 {
		t.Fatalf("итог: %+v", v.Outcome)
	}
	list, _ = s.Proposals(t.Context(), domain.ProposalPending, "")
	var rb *out.Proposal
	for i := range list.Items {
		if list.Items[i].Action == domain.ActionRollback {
			rb = &list.Items[i]
		}
	}
	if rb == nil || rb.RollbackOf == nil || rb.RollbackOf.ProposalID != orig.ID || rb.Trigger != domain.TriggerSchedule ||
		!strings.Contains(string(rb.Evidence), "rollback_proposed_by_agent") {
		t.Fatalf("предложение отката: %+v", list.Items)
	}
	el, _ := s.AgentMemoryLayer(t.Context(), "episodes")
	harmed := false
	for _, e := range el.Episodes {
		if e.Kind == "effect_check" && e.Effect != nil && *e.Effect == domain.EffectHarmed {
			harmed = true
		}
	}
	if !harmed {
		t.Fatalf("эпизод «навредило»: %+v", el.Episodes)
	}
	// Повторно откат не предлагается, пока прежний ждёт решения.
	r3, _ := s.StartAgentRun(t.Context(), in.AgentRunRequest{Trigger: domain.TriggerSchedule, Days: 30, MaxClusters: 1})
	run = waitRun(t, s, r3.RunID)
	if s.agent.runView(run).Outcome.Rollbacks != 0 {
		t.Fatal("откат не дублируется")
	}
	res, err := s.ApproveProposal(t.Context(), rb.ID, in.ApproveRequest{Comment: "согласен"})
	if err != nil || res.ConfigVersion == nil || *res.ConfigVersion != 3 {
		t.Fatalf("принятие отката: %+v %v", res, err)
	}
	active, _ := s.activeRules(t.Context())
	if active.find("кис") != nil || active.find("винда") == nil {
		t.Fatalf("откат убрал только правку агента: %+v", active.Aliases)
	}
	if p := ms.proposals[uuid.MustParse(orig.ID)]; p.Status != domain.ProposalRolledBack {
		t.Fatalf("исходная правка → rolled_back: %s", p.Status)
	}
	if v := ms.versions[len(ms.versions)-1]; v.Source != "rollback" || v.Author != "agent+human" {
		t.Fatalf("версия отката: %+v", v)
	}
}

// Пороги всплеска: доля пустых, мало поисков — молчит, новый неудачный запрос с частотой.
func TestP5SpikeThresholds(t *testing.T) {
	s, ms := agentEnv(t, &scriptModel{})
	s.agent.opts.Triggers.SpikeT = SpikeThresholds{Window: time.Hour, MinSearches: 60, EmptyShare: 0.25, ReformShare: 0.12,
		QueryFreq: 8, Cooldown: 3 * time.Hour}
	now := time.Now()
	at := now.Add(-50 * time.Minute)
	// 59 поисков, 20 пустых: доля 34 %, но поисков меньше порога — по долям молчит; «скетчап» 20 раз — новый запрос.
	ms.searches = append(okSearches("windows 11", 39, at), emptySearches("скетчап", 20, at)...)
	r, err := s.evaluateSpike(t.Context(), now)
	if err != nil {
		t.Fatal(err)
	}
	if r.Searches != 59 || len(r.Fired) != 1 || r.Fired[0] != "new_query" || r.NewQueries[0].Text != "скетчап" {
		t.Fatalf("мало поисков — только новый запрос: %+v", r)
	}
	// Тот же запрос был в журнале до окна — не «новый»; 60 поисков — доля пустых выше порога.
	ms.searches = append(append(okSearches("windows 11", 40, at), emptySearches("скетчап", 20, at)...),
		emptySearches("скетчап", 1, now.Add(-5*24*time.Hour))...)
	r, _ = s.evaluateSpike(t.Context(), now)
	if len(r.Fired) != 1 || r.Fired[0] != "empty_share" || r.EmptyShare <= 0.25 || len(r.NewQueries) != 0 {
		t.Fatalf("доля пустых выше порога: %+v", r)
	}
	// Ниже порога — молчит.
	ms.searches = append(okSearches("windows 11", 55, at), emptySearches("скетчап", 5, at)...)
	ms.searches = append(ms.searches, emptySearches("скетчап", 1, now.Add(-5*24*time.Hour))...)
	r, _ = s.evaluateSpike(t.Context(), now)
	if len(r.Fired) != 0 {
		t.Fatalf("ниже порогов: %+v", r)
	}
	// Сработал — запуск spike с входом триггера и фокусом; повтор до паузы не стартует.
	ms.searches = append(okSearches("windows 11", 40, at), emptySearches("скетчап", 20, at)...)
	if err := s.checkSpike(t.Context(), now); err != nil {
		t.Fatal(err)
	}
	runs, _ := ms.ListAgentRunsByTrigger(t.Context(), domain.TriggerSpike, 5)
	if len(runs) != 1 || !strings.Contains(string(runs[0].Input), "скетчап") {
		t.Fatalf("запуск по всплеску: %+v", runs)
	}
	waitRun(t, s, runs[0].ID.String())
	if err := s.checkSpike(t.Context(), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	runs, _ = ms.ListAgentRunsByTrigger(t.Context(), domain.TriggerSpike, 5)
	if len(runs) != 1 {
		t.Fatalf("пауза после всплеска: %d запусков", len(runs))
	}
	steps := stepsOf(ms, runs[0].ID.String())
	if !hasStep(steps, domain.StepPlan, "План") || !hasStep(steps, domain.StepToolCall, "окно всплеска") {
		t.Fatalf("запуск по всплеску: план и окно: %+v", steps)
	}
}

// Расписание: после ЧЧ:ММ — один запуск schedule, второй проверкой — нет; до ЧЧ:ММ — нет.
func TestP5ScheduleAt(t *testing.T) {
	s, ms := agentEnv(t, &scriptModel{})
	s.agent.opts.Triggers.Schedule, s.agent.opts.Triggers.ScheduleAt = true, "03:00"
	slot, _ := scheduleSlot("03:00", time.Now())
	if err := s.checkSchedule(t.Context(), slot.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if runs, _ := ms.ListAgentRunsByTrigger(t.Context(), domain.TriggerSchedule, 5); len(runs) != 0 {
		t.Fatal("до ЧЧ:ММ запуска нет")
	}
	s.now = func() time.Time { return slot.Add(5 * time.Minute) }
	if err := s.checkSchedule(t.Context(), slot.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	runs, _ := ms.ListAgentRunsByTrigger(t.Context(), domain.TriggerSchedule, 5)
	if len(runs) != 1 || runs[0].Days != 1 {
		t.Fatalf("ночной разбор за сутки: %+v", runs)
	}
	waitRun(t, s, runs[0].ID.String())
	_ = s.checkSchedule(t.Context(), slot.Add(10*time.Minute))
	if runs, _ = ms.ListAgentRunsByTrigger(t.Context(), domain.TriggerSchedule, 5); len(runs) != 1 {
		t.Fatalf("второй раз за ночь не запускается: %d", len(runs))
	}
	if next := s.nextScheduleAt(slot.Add(time.Hour)); !next.Equal(slot.Add(24 * time.Hour).UTC()) {
		t.Fatalf("следующий разбор: %v", next)
	}
}

// Бюджет останавливает запуск с причиной и списком неразобранного.
func TestP5BudgetStopsRun(t *testing.T) {
	model := &scriptModel{}
	s, ms := agentEnv(t, model)
	s.agent.opts.Budget.Tokens = 1000
	model.turns = [][]llm.ToolCall{{tc("search_catalog", map[string]any{"query": "кис"})},
		{tc("search_catalog", map[string]any{"query": "kis"})}}
	r, _ := s.StartAgentRun(t.Context(), in.AgentRunRequest{MaxClusters: 1})
	run := waitRun(t, s, r.RunID)
	if run.StopReason != StopTokenLimit {
		t.Fatalf("остановка по токенам: %+v", run)
	}
	if !hasStep(stepsOf(ms, r.RunID), domain.StepBudget, "Бюджет запуска: исчерпан лимит токенов") {
		t.Fatalf("шаг бюджета: %+v", stepsOf(ms, r.RunID))
	}
	var b struct {
		Exhausted string `json:"exhausted"`
		Used      struct {
			Tokens int `json:"tokens"`
		} `json:"used"`
	}
	_ = json.Unmarshal(run.Budget, &b)
	if b.Exhausted != StopTokenLimit || b.Used.Tokens < 1000 {
		t.Fatalf("бюджет в запуске: %s", run.Budget)
	}
	// Бюджет шагов.
	s.agent.opts.Budget.Tokens, s.agent.opts.Budget.Steps = 600_000, 1
	model.turns = [][]llm.ToolCall{{tc("search_catalog", map[string]any{"query": "кис"})},
		{tc("search_catalog", map[string]any{"query": "kis"})}}
	r, _ = s.StartAgentRun(t.Context(), in.AgentRunRequest{MaxClusters: 1})
	run = waitRun(t, s, r.RunID)
	if run.StopReason != StopStepsBudget || run.ToolCalls != 1 {
		t.Fatalf("остановка по шагам: %+v", run)
	}
}

// Запуск на одном запросе, которого нет в журнале: триггер manual, запрос в API, план и шаги.
func TestP5QueryRun(t *testing.T) {
	model := &scriptModel{}
	s, ms := agentEnv(t, model)
	model.turns = [][]llm.ToolCall{{tc("close_cluster", map[string]any{"diagnosis": "NOISE", "reason": "тест"})}}
	r, err := s.StartAgentRun(t.Context(), in.AgentRunRequest{Query: "  чертилка   для дома "})
	if err != nil {
		t.Fatal(err)
	}
	run := waitRun(t, s, r.RunID)
	v := s.agent.runView(run)
	if run.Trigger != domain.TriggerManual || v.Query == nil || *v.Query != "чертилка для дома" || run.ClustersDone != 1 {
		t.Fatalf("запуск на запросе: %+v", run)
	}
	steps := stepsOf(ms, r.RunID)
	if !hasStep(steps, domain.StepToolCall, "его нет — разбираю как новый") || !hasStep(steps, domain.StepPlan, "План") ||
		!hasStep(steps, domain.StepMemoryRead, "Читаю память") {
		t.Fatalf("шаги: %+v", steps)
	}
	if _, err := s.StartAgentRun(t.Context(), in.AgentRunRequest{Query: "x", Mode: domain.RunModeCommerce}); err == nil {
		t.Fatal("запрос — только в режиме search")
	}
	list, _ := s.AgentRuns(t.Context(), domain.TriggerManual)
	if len(list.Items) != 1 {
		t.Fatalf("фильтр по триггеру: %+v", list)
	}
	if _, err := s.AgentRuns(t.Context(), "bad"); err == nil {
		t.Fatal("неизвестный триггер — 422")
	}
}

// Только чтение: диагноз и черновик, в очереди пусто; режим виден в запуске.
func TestP5ReadOnly(t *testing.T) {
	model := &scriptModel{}
	s, ms := agentEnv(t, model)
	s.agent.evalHook = func(context.Context, *failedCluster, RulePatch) (*patchEval, error) { return evalResult(0, 1, 0), nil }
	ro := true
	if _, err := s.SetAgentSettings(t.Context(), in.AgentSettingsRequest{ReadOnly: &ro}); err != nil {
		t.Fatal(err)
	}
	model.turns = kisPatchTurns()
	r, _ := s.StartAgentRun(t.Context(), in.AgentRunRequest{MaxClusters: 1})
	run := waitRun(t, s, r.RunID)
	v := s.agent.runView(run)
	if !run.ReadOnly || len(ms.proposals) != 0 || v.Outcome.Drafts != 1 || v.Outcome.Proposals != 0 {
		t.Fatalf("только чтение: %+v %+v", run, v.Outcome)
	}
	if !hasStep(stepsOf(ms, r.RunID), domain.StepProposal, "Только чтение") {
		t.Fatal("черновик — шагом ленты")
	}
	if st := s.AgentSettings(t.Context()); !st.ReadOnly || !strings.Contains(st.ModeLabel, "Только чтение") {
		t.Fatalf("настройки: %+v", st)
	}
}

// Повтор при сбое модели — один раз; защита кластера от второго запуска; пул.
func TestP5RetryLockPool(t *testing.T) {
	model := &scriptModel{fail: 1}
	s, ms := agentEnv(t, model)
	model.turns = [][]llm.ToolCall{{tc("close_cluster", map[string]any{"diagnosis": "NOISE", "reason": "тест"})}}
	r, _ := s.StartAgentRun(t.Context(), in.AgentRunRequest{MaxClusters: 1})
	run := waitRun(t, s, r.RunID)
	if !hasStep(stepsOf(ms, r.RunID), domain.StepError, "повторяю вызов один раз") || run.Status != domain.RunDone {
		t.Fatalf("повтор: %+v", run)
	}
	a := s.agent
	r1, r2 := uuid.New(), uuid.New()
	a.mu.Lock()
	a.runs[r1] = &runState{run: &domain.AgentRun{}}
	a.mu.Unlock()
	if _, ok := a.lockCluster(r1, "кис"); !ok {
		t.Fatal("первый запуск берёт кластер")
	}
	if owner, ok := a.lockCluster(r2, "кис"); ok || owner != r1 {
		t.Fatal("второй запуск на тот же кластер — нет")
	}
	a.mu.Lock()
	a.runs[r2] = &runState{run: &domain.AgentRun{}, spec: runSpec{Trigger: domain.TriggerSpike}}
	a.mu.Unlock()
	if _, err := s.StartAgentRun(t.Context(), in.AgentRunRequest{}); err == nil || !strings.Contains(err.Error(), "Пул агента занят") {
		t.Fatalf("пул из двух занят: %v", err)
	}
	a.mu.Lock()
	delete(a.runs, r1)
	a.mu.Unlock()
	if err := s.startSpikeRun(t.Context(), &spikeReport{}); err == nil {
		t.Fatal("второй запуск по всплеску при идущем — нет")
	}
	a.mu.Lock()
	delete(a.runs, r2)
	a.mu.Unlock()
}

// Приоритет: частота × доля неудач × «новый».
func TestP5Priority(t *testing.T) {
	if priority(10, 5, false) != 5 || priority(10, 5, true) != 10 || priority(4, 8, false) != 4 {
		t.Fatal("приоритет")
	}
}

// Короткое окно (всплеск) при разрезе журнала: границы отложенной недели — по историческому журналу, а не по живым
// поискам окна; живой трафик после недели виден.
func TestP5SpikeWindowSeesLiveAfterHoldout(t *testing.T) {
	s, ms := agentEnv(t, &scriptModel{})
	now := time.Now()
	hist := emptySearches("старый запрос", 3, now.Add(-10*24*time.Hour))
	for i := range hist {
		hist[i].Source = "mock_log"
	}
	ms.searches = append(hist, emptySearches("скетчап", 9, now.Add(-30*time.Minute))...)
	stats, info, err := s.agentQueryStatsSince(t.Context(), now.Add(-time.Hour), HoldoutOptions{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if q := stats.byText["скетчап"]; q == nil || q.Empty != 9 || info.HoldoutFrom == nil {
		t.Fatalf("живые поиски окна после отложенной недели видны: %+v %+v", stats.byText, info)
	}
}
