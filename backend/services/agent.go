package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
	"kuznica/api/prompts"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Агент SearchOps (ARCHITECTURE §3): запуск по кнопке или расписанию, лента шагов в
// agent_steps и по SSE, остановка. Цикл одного кластера — agent_loop.go, инструменты — agent_tools.go,
// песочница — agent_eval.go. Агент ничего не применяет сам: предложения ждут человека.

// Ограничители (ARCHITECTURE §3.4).
const (
	agentToolCallsPerCluster = 15
	agentMaxHypotheses       = 3
	agentTokensPerRun        = 600_000
	agentRunTimeout          = 25 * time.Minute
	agentClusterTimeout      = 6 * time.Minute
	agentCallTimeout         = 60 * time.Second
	agentMaxTokensReply      = 1500
	agentDefaultDays         = 30
	agentDefaultClusters     = 5
	agentMaxClusters         = 12
	// agentCandidates — сколько кластеров по убыванию потерь смотреть, чтобы набрать max_clusters.
	agentCandidates = 40
)

// Тексты отказов.
const (
	MsgAgentNoModel  = "Модель агента не настроена (LLM_API_KEY, AGENT_MODEL): запуск невозможен"
	MsgAgentRunning  = "Агент уже работает — дождитесь окончания запуска или остановите его"
	MsgAgentNotFound = "Запуск не найден"
	MsgAgentLLMOff   = "Режим LLM выключен: агенту нужна модель — включите режим (POST /api/admin/llm-mode)"
	// MsgAgentPoolBusy — пул запусков агента занят (AGENT_MAX_PARALLEL).
	MsgAgentPoolBusy = "Пул агента занят: идёт %d из %d запусков — дождитесь окончания одного из них"
	// MsgAgentSameTrigger — запуск того же вида уже идёт.
	MsgAgentSameTrigger = "Запуск этого вида уже идёт — дождитесь его окончания или остановите его"
)

// Причины остановки запуска (run.stop_reason; пусто — разобраны все выбранные кластеры).
const (
	StopUser       = "user"
	StopStepLimit  = "step_limit"
	StopTokenLimit = "token_limit"
	// StopBudget — дневной бюджет LLM исчерпан посреди запуска (LLM_DAILY_BUDGET_RUB).
	StopBudget          = "budget_limit"
	StopTimeout         = "timeout"
	StopLLMError        = "llm_error"
	StopNoFailedQueries = "no_failed_queries"
	StopError           = "error"
)

// AgentOptions — пороги допуска правки и наборы регрессии (config.AgentConfig).
type AgentOptions struct {
	MaxRegressionWorse int
	RegressionSets     []string
	// FeedRegressionSets — наборы «по цели» на фиде заказчика (agent_eval.go); пусто — feed_regression, feed_tune.
	FeedRegressionSets []string
	// Holdout — что агенту видно из журнала (agent_holdout.go); нулевое — весь журнал.
	Holdout HoldoutOptions
	// Контур автономии (agent_p5.go): режим «только чтение», пул, бюджет запуска, триггеры.
	ReadOnly      bool
	MaxParallel   int
	Budget        RunBudget
	QueryDuration time.Duration
	Triggers      AgentTriggers
}

// agentService — запуски агента в процессе: один запуск за раз.
type agentService struct {
	s     *Services
	model ChatModel
	opts  AgentOptions

	// evalHook — прогон правки в тестах цикла; nil — песочница (agent_eval.go).
	evalHook func(ctx context.Context, c *failedCluster, p RulePatch) (*patchEval, error)

	mu sync.Mutex
	// runs — идущие запуски (пул не больше opts.MaxParallel); locks — ключ кластера → запуск, который его разбирает.
	runs  map[uuid.UUID]*runState
	locks map[string]uuid.UUID
	subs  map[uuid.UUID]map[chan agentEvent]struct{}
	// readOnly — режим «только чтение» (AGENT_READ_ONLY, POST /api/agent/settings).
	readOnly atomic.Bool
	// trig — состояние триггеров (agent_triggers.go).
	trig triggerState
}

// agentEvent — событие ленты для SSE: шаг или смена статуса запуска.
type agentEvent struct {
	Kind string // step | status | done
	Step *out.AgentStep
	Run  *out.AgentRun
}

// runState — запуск в работе.
type runState struct {
	run    *domain.AgentRun
	spec   runSpec
	budget RunBudget
	cancel context.CancelFunc
	mu     sync.Mutex
	seq    int
	stop   string
	// writes — фоновые записи llm_usage этого запуска (итог запуска считается после них).
	writes *runWrites
	// ended — событие done разослано, запуск снят с active.
	ended bool
}

// runWrites — счётчик фоновых записей учёта одного запуска. В отличие от sync.WaitGroup, add после
// начала wait допустим: ожидание — опрос с потолком, без паники.
type runWrites struct{ n atomic.Int64 }

type runWritesKey struct{}

// withRunWrites — вызовы модели в ctx учитываются в счётчике запуска.
func withRunWrites(ctx context.Context, w *runWrites) context.Context {
	return context.WithValue(ctx, runWritesKey{}, w)
}

// runWritesOf — счётчик запуска из ctx; nil — вызов не из запуска агента.
func runWritesOf(ctx context.Context) *runWrites {
	w, _ := ctx.Value(runWritesKey{}).(*runWrites)
	return w
}

func (w *runWrites) add() {
	if w != nil {
		w.n.Add(1)
	}
}

func (w *runWrites) done() {
	if w != nil {
		w.n.Add(-1)
	}
}

// wait — дождаться записей не дольше d; false — не дождались (итог запуска — по счётчикам в памяти).
func (w *runWrites) wait(d time.Duration) bool {
	if w == nil {
		return true
	}
	deadline := time.Now().Add(d)
	for w.n.Load() > 0 {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
	return true
}

func newAgentService(s *Services, model ChatModel, opts AgentOptions) *agentService {
	if len(opts.RegressionSets) == 0 {
		opts.RegressionSets = []string{domain.EvalSetRegression, domain.EvalSetTune}
	}
	if len(opts.FeedRegressionSets) == 0 {
		opts.FeedRegressionSets = []string{domain.TargetSetFeedRegression, domain.TargetSetFeedTune}
	}
	opts = opts.withDefaults()
	a := &agentService{s: s, model: model, opts: opts, subs: map[uuid.UUID]map[chan agentEvent]struct{}{},
		runs: map[uuid.UUID]*runState{}, locks: map[string]uuid.UUID{}}
	a.readOnly.Store(opts.ReadOnly)
	return a
}

// agentRequestID — request_id поиска агента (≤32 символов, в журнал поиска не пишется).
func agentRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "agent-" + hex.EncodeToString(b)
}

// StartAgentRun — POST /api/agent/runs: запуск в фоне, 202 и id. {"query": "…"} — запуск «Разобрать» на одном запросе.
func (s *Services) StartAgentRun(ctx context.Context, req in.AgentRunRequest) (*out.AgentRunStarted, error) {
	days, clusters := req.Days, req.MaxClusters
	var bad []errs.FieldError
	if days == 0 {
		days = agentDefaultDays
	}
	if days < 1 || days > 365 {
		bad = append(bad, errs.FieldError{Field: "days", Code: errs.CodeInvalidFormat, Message: "Период — от 1 до 365 дней"})
	}
	if clusters == 0 {
		clusters = agentDefaultClusters
	}
	if clusters < 1 || clusters > agentMaxClusters {
		bad = append(bad, errs.FieldError{Field: "max_clusters", Code: errs.CodeInvalidFormat, Message: "Кластеров — от 1 до 12"})
	}
	trigger := req.Trigger
	if trigger == "" {
		trigger = domain.TriggerManual
	}
	if trigger != domain.TriggerManual && trigger != domain.TriggerSchedule {
		bad = append(bad, errs.FieldError{Field: "trigger", Code: errs.CodeInvalidFormat, Message: "Запуск — manual или schedule"})
	}
	mode := req.Mode
	if mode == "" {
		mode = domain.RunModeSearch
	}
	if mode != domain.RunModeSearch && mode != domain.RunModeCommerce && mode != domain.RunModeGaps {
		bad = append(bad, errs.FieldError{Field: "mode", Code: errs.CodeInvalidFormat, Message: "Режим — search, commerce или gaps"})
	}
	if mode == domain.RunModeGaps && trigger != domain.TriggerManual {
		bad = append(bad, errs.FieldError{Field: "mode", Code: errs.CodeInvalidFormat, Message: "Пробелы подбора — только manual"})
	}
	query := strings.Join(strings.Fields(req.Query), " ")
	if query != "" {
		if n := len([]rune(query)); n > 200 {
			bad = append(bad, errs.FieldError{Field: "query", Code: errs.CodeInvalidFormat, Message: "Запрос — не длиннее 200 символов"})
		}
		if mode != domain.RunModeSearch || trigger != domain.TriggerManual {
			bad = append(bad, errs.FieldError{Field: "query", Code: errs.CodeInvalidFormat, Message: "Запуск на запросе — только manual и search"})
		}
		clusters = 1
	}
	if len(bad) > 0 {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(bad...)
	}
	spec := runSpec{Trigger: trigger, Days: days, MaxClusters: clusters, Mode: mode, Query: query}
	if query != "" {
		spec.Input = map[string]any{"query": query}
	} else if trigger == domain.TriggerSchedule {
		spec.Input = map[string]any{"days": days, "source": "api"}
	}
	return s.startRun(ctx, spec)
}

// startRun — запуск в пуле агента: проверки модели и бюджета, место в пуле, один запуск каждого вида.
func (s *Services) startRun(ctx context.Context, spec runSpec) (*out.AgentRunStarted, error) {
	a := s.agent
	st := s.store()
	if st == nil {
		return nil, errs.NewUnavailableError(MsgNoAgentStore)
	}
	if a.model == nil {
		return nil, errs.NewUnavailableError(MsgAgentNoModel)
	}
	if s.LLMMode() != llmModeOn {
		return nil, errs.NewConflictError(MsgAgentLLMOff)
	}
	if s.guard().exceeded(ctx) {
		return nil, errs.NewUnavailableError(MsgBudgetExceeded)
	}
	a.mu.Lock()
	if len(a.runs) >= a.opts.MaxParallel {
		n := len(a.runs)
		a.mu.Unlock()
		return nil, errs.NewConflictError(fmt.Sprintf(MsgAgentPoolBusy, n, a.opts.MaxParallel))
	}
	for _, r := range a.runs {
		if r.spec.sameKind(spec) {
			a.mu.Unlock()
			if spec.Trigger == domain.TriggerManual && spec.Query == "" {
				return nil, errs.NewConflictError(MsgAgentRunning)
			}
			return nil, errs.NewConflictError(MsgAgentSameTrigger)
		}
	}
	p := prompts.Agent()
	if spec.Mode == domain.RunModeCommerce {
		p = prompts.AgentCommerce()
	}
	budget := a.opts.Budget
	if spec.Query != "" {
		budget.Duration = a.opts.QueryDuration
	}
	run := &domain.AgentRun{Status: domain.RunRunning, Trigger: spec.Trigger, Days: spec.Days, MaxClusters: spec.MaxClusters,
		StartedAt: s.now().UTC(), Model: llmModelName(a.model.Model()), PromptVersion: p.Version, Mode: spec.Mode,
		Query: spec.Query, ReadOnly: a.readOnly.Load(), Input: toJSON(nonNilMap(spec.Input)),
		Budget: toJSON(budgetView(budget, budgetUsed{}, ""))}
	if err := st.CreateAgentRun(ctx, run); err != nil {
		a.mu.Unlock()
		return nil, err
	}
	// Жёсткий срок — с запасом над бюджетом времени: бюджет останавливает запуск мягко, с итогом и неразобранным.
	rctx, cancel := context.WithTimeout(context.Background(), budget.Duration+agentHardTimeoutGrace)
	rs := &runState{run: run, spec: spec, budget: budget, cancel: cancel, writes: &runWrites{}}
	a.runs[run.ID] = rs
	a.mu.Unlock()
	// Горутина запуска — не в s.writes: в конце она ждёт только свои записи учёта (rs.writes). Паника
	// внутри запуска не роняет API: запуск помечается failed, подписчики получают done.
	go func() {
		defer cancel()
		defer func() {
			if r := recover(); r != nil {
				zap.L().Error("агент: паника в запуске", zap.String("run_id", run.ID.String()), zap.Any("panic", r),
					zap.ByteString("stack", debug.Stack()))
				a.failRun(rs, fmt.Sprint(r))
			}
		}()
		a.execute(withRunWrites(WithAgentRunID(rctx, run.ID.String()), rs.writes), rs)
	}()
	zap.L().Info("агент: запуск", zap.String("run_id", run.ID.String()), zap.String("trigger", spec.Trigger),
		zap.Int("days", spec.Days), zap.Int("max_clusters", spec.MaxClusters), zap.String("mode", spec.Mode),
		zap.String("query", spec.Query), zap.Bool("read_only", run.ReadOnly))
	return &out.AgentRunStarted{RunID: run.ID.String()}, nil
}

// StopAgentRun — POST /api/agent/runs/{id}/stop.
func (s *Services) StopAgentRun(ctx context.Context, id string) error {
	run, err := s.loadRun(ctx, id)
	if err != nil {
		return err
	}
	a := s.agent
	a.mu.Lock()
	defer a.mu.Unlock()
	if rs := a.runs[run.ID]; rs != nil {
		rs.mu.Lock()
		rs.stop = StopUser
		rs.mu.Unlock()
		rs.cancel()
	}
	return nil
}

// AgentRuns — GET /api/agent/runs?trigger=: последние 50 запусков (фильтр по триггеру).
func (s *Services) AgentRuns(ctx context.Context, trigger ...string) (*out.AgentRunList, error) {
	st := s.store()
	if st == nil {
		return nil, errs.NewUnavailableError(MsgNoAgentStore)
	}
	tr := ""
	if len(trigger) > 0 {
		tr = strings.TrimSpace(trigger[0])
	}
	if tr != "" && !slices.Contains(domain.Triggers, tr) {
		return nil, errs.NewValidationError(errs.MsgInvalidQuery).WithDetails(errs.FieldError{Field: "trigger",
			Code: errs.CodeInvalidFormat, Message: "Триггер — manual, schedule, spike или catalog_change"})
	}
	rows, err := st.ListAgentRunsByTrigger(ctx, tr, 50)
	if err != nil {
		return nil, err
	}
	// Идущие запуски — из памяти: счётчики свежее строки в базе.
	s.agent.mu.Lock()
	for i := range rows {
		if rs := s.agent.runs[rows[i].ID]; rs != nil {
			rs.mu.Lock()
			rows[i] = *rs.run
			rs.mu.Unlock()
		}
	}
	s.agent.mu.Unlock()
	res := &out.AgentRunList{Items: make([]out.AgentRun, 0, len(rows))}
	for i := range rows {
		res.Items = append(res.Items, s.agent.runView(&rows[i]))
	}
	return res, nil
}

// AgentRun — GET /api/agent/runs/{id}: запуск и все шаги.
func (s *Services) AgentRun(ctx context.Context, id string) (*out.AgentRunDetail, error) {
	run, err := s.loadRun(ctx, id)
	if err != nil {
		return nil, err
	}
	steps, err := s.store().ListAgentSteps(ctx, run.ID, 0)
	if err != nil {
		return nil, err
	}
	res := &out.AgentRunDetail{Run: s.agent.runView(run), Steps: make([]out.AgentStep, 0, len(steps))}
	for i := range steps {
		res.Steps = append(res.Steps, stepView(&steps[i]))
	}
	return res, nil
}

func (s *Services) loadRun(ctx context.Context, id string) (*domain.AgentRun, error) {
	st := s.store()
	if st == nil {
		return nil, errs.NewUnavailableError(MsgNoAgentStore)
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errs.NewValidationError(errs.MsgInvalidID)
	}
	// Идущий запуск — из памяти: счётчики свежее строки в базе.
	a := s.agent
	a.mu.Lock()
	if rs := a.runs[uid]; rs != nil {
		rs.mu.Lock()
		cp := *rs.run
		rs.mu.Unlock()
		a.mu.Unlock()
		return &cp, nil
	}
	a.mu.Unlock()
	run, err := st.GetAgentRun(ctx, uid)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, errs.NewNotFoundError(MsgAgentNotFound)
	}
	return run, nil
}

// AgentStream — подписка на ленту запуска для SSE: шаги после afterSeq из базы, затем живые события.
// Закрытый канал — запуск закончился (или подписка отменена).
func (s *Services) AgentStream(ctx context.Context, id string, afterSeq int) (*out.AgentRun, []out.AgentStep, <-chan agentEvent, func(), error) {
	run, err := s.loadRun(ctx, id)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	a := s.agent
	ch := make(chan agentEvent, 256)
	a.mu.Lock()
	live := a.runs[run.ID] != nil
	if live {
		if a.subs[run.ID] == nil {
			a.subs[run.ID] = map[chan agentEvent]struct{}{}
		}
		a.subs[run.ID][ch] = struct{}{}
	}
	a.mu.Unlock()
	cancel := func() {
		a.mu.Lock()
		if m := a.subs[run.ID]; m != nil {
			if _, ok := m[ch]; ok {
				delete(m, ch)
				close(ch)
			}
			if len(m) == 0 {
				delete(a.subs, run.ID)
			}
		}
		a.mu.Unlock()
	}
	if !live {
		close(ch)
	}
	steps, err := s.store().ListAgentSteps(ctx, run.ID, afterSeq)
	if err != nil {
		cancel()
		return nil, nil, nil, nil, err
	}
	views := make([]out.AgentStep, 0, len(steps))
	for i := range steps {
		views = append(views, stepView(&steps[i]))
	}
	rv := a.runView(run)
	return &rv, views, ch, cancel, nil
}

// publish — событие подписчикам запуска; медленный подписчик пропускает событие (догонит из базы).
func (a *agentService) publish(runID uuid.UUID, ev agentEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.broadcastLocked(runID, ev)
}

func (a *agentService) broadcastLocked(runID uuid.UUID, ev agentEvent) {
	for ch := range a.subs[runID] {
		select {
		case ch <- ev:
		default:
		}
	}
	if ev.Kind == "done" {
		for ch := range a.subs[runID] {
			close(ch)
		}
		delete(a.subs, runID)
	}
}

// finish — запуск снят с active и done разослан под одним a.mu: подписчик, пришедший после, видит
// законченный запуск (из базы), а не живой без done. Повторный вызов — без действия.
func (a *agentService) finish(rs *runState, v out.AgentRun) {
	a.mu.Lock()
	defer a.mu.Unlock()
	rs.mu.Lock()
	ended := rs.ended
	rs.ended = true
	rs.mu.Unlock()
	if ended {
		return
	}
	if a.runs[rs.run.ID] == rs {
		delete(a.runs, rs.run.ID)
	}
	for k, id := range a.locks {
		if id == rs.run.ID {
			delete(a.locks, k)
		}
	}
	a.broadcastLocked(rs.run.ID, agentEvent{Kind: "done", Run: &v})
}

// failRun — запуск упал с паникой: статус failed, итог в базу, done подписчикам. Сама запись тоже под
// recover — запуск не должен остаться running в памяти.
func (a *agentService) failRun(rs *runState, reason string) {
	defer func() {
		if r := recover(); r != nil {
			zap.L().Error("агент: итог упавшего запуска не записан", zap.Any("panic", r))
		}
		rs.mu.Lock()
		cp := *rs.run
		rs.mu.Unlock()
		a.finish(rs, a.runView(&cp))
	}()
	rs.mu.Lock()
	if rs.ended {
		rs.mu.Unlock()
		return
	}
	now := a.s.now().UTC()
	r := rs.run
	r.Status, r.StopReason, r.FinishedAt = domain.RunFailed, StopError, &now
	r.CurrentClusterID, r.CurrentLabel = nil, ""
	r.Summary = "Внутренняя ошибка запуска: " + shortText(reason, 200)
	rs.mu.Unlock()
	a.saveRun(context.Background(), rs)
}

// step — шаг ленты: в базу и подписчикам.
func (a *agentService) step(ctx context.Context, rs *runState, st *domain.AgentStep) {
	rs.mu.Lock()
	rs.seq++
	st.Seq = rs.seq
	st.RunID = rs.run.ID
	rs.mu.Unlock()
	if st.ID == uuid.Nil {
		_ = st.BeforeCreate(nil)
	}
	if st.Args == nil {
		st.Args = domain.JSONB("{}")
	}
	if st.Result == nil {
		st.Result = domain.JSONB("{}")
	}
	if err := a.s.store().CreateAgentStep(context.WithoutCancel(ctx), st); err != nil {
		zap.L().Warn("агент: шаг не записан", zap.String("run_id", rs.run.ID.String()), zap.Error(err))
	}
	v := stepView(st)
	a.publish(rs.run.ID, agentEvent{Kind: "step", Step: &v})
}

// updateStep — перезапись шага (отклонённая гипотеза): в базу и подписчикам тем же id.
func (a *agentService) updateStep(ctx context.Context, rs *runState, st *domain.AgentStep) {
	if err := a.s.store().SaveAgentStep(context.WithoutCancel(ctx), st); err != nil {
		zap.L().Warn("агент: шаг не обновлён", zap.Error(err))
	}
	v := stepView(st)
	a.publish(rs.run.ID, agentEvent{Kind: "step", Step: &v})
}

// saveRun — счётчики запуска в базу и подписчикам.
func (a *agentService) saveRun(ctx context.Context, rs *runState) {
	rs.mu.Lock()
	cp := *rs.run
	rs.mu.Unlock()
	if err := a.s.store().SaveAgentRun(context.WithoutCancel(ctx), &cp); err != nil {
		zap.L().Warn("агент: запуск не записан", zap.Error(err))
	}
	v := a.runView(&cp)
	a.publish(rs.run.ID, agentEvent{Kind: "status", Run: &v})
}

// addUsage — расход вызова модели в счётчики запуска.
func (rs *runState) addUsage(in, outTok int, cost float64) {
	rs.mu.Lock()
	rs.run.InputTokens += in
	rs.run.OutputTokens += outTok
	rs.run.CostRub = math.Round((rs.run.CostRub+cost)*1e4) / 1e4
	rs.mu.Unlock()
}

func (rs *runState) tokens() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.run.InputTokens + rs.run.OutputTokens
}

func (rs *runState) stopReason() string {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.stop
}

// runView — запуск в форме ответа API.
func (a *agentService) runView(r *domain.AgentRun) out.AgentRun {
	v := out.AgentRun{ID: r.ID.String(), Status: r.Status, Trigger: r.Trigger, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt,
		Days: r.Days, MaxClusters: r.MaxClusters, ClustersTotal: r.ClustersTotal, ClustersDone: r.ClustersDone,
		ProposalsCreated: r.ProposalsCreated, ToolCalls: r.ToolCalls, InputTokens: r.InputTokens, OutputTokens: r.OutputTokens,
		CostRub: r.CostRub, StopReason: r.StopReason, Summary: r.Summary, Model: r.Model, RulesVersion: r.RulesVersion,
		Mode:    firstNonEmpty(r.Mode, domain.RunModeSearch),
		Episode: rawOrNull(r.Episode), MaxToolCalls: agentToolCallsPerCluster * max(r.ClustersTotal, 1),
		Limits: out.AgentLimits{ToolCallsPerCluster: agentToolCallsPerCluster, Hypotheses: agentMaxHypotheses,
			TokensPerRun: agentTokensPerRun, TimeoutSec: int(agentRunTimeout.Seconds())}}
	if r.CurrentClusterID != nil {
		v.CurrentCluster = &out.ClusterRef{ID: *r.CurrentClusterID, Label: r.CurrentLabel}
	}
	a.p5View(r, &v)
	return v
}

// stepView — шаг в форме ответа API.
func stepView(st *domain.AgentStep) out.AgentStep {
	return out.AgentStep{ID: st.ID.String(), RunID: st.RunID.String(), Seq: st.Seq, TS: st.CreatedAt, Kind: st.Kind,
		ClusterID: st.ClusterID, Tool: st.Tool, Title: st.Title, Args: rawOrNull(st.Args), Result: rawOrNull(st.Result),
		OK: st.OK, HypothesisNo: st.HypothesisNo, RejectedReason: st.RejectedReason, Ms: st.Ms}
}

// toJSON — значение в JSONB (ошибка сериализации — пустой объект: шаг всё равно пишется).
func toJSON(v any) domain.JSONB {
	b, err := json.Marshal(v)
	if err != nil {
		return domain.JSONB("{}")
	}
	return b
}

// StartAgentScheduler — запуск агента по расписанию (AGENT_SCHEDULE=on): каждые interval, пока жив ctx.
// Занятый агент — пропуск такта.
func (s *Services) StartAgentScheduler(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	zap.L().Info("агент: расписание включено", zap.Duration("interval", interval))
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.scheduleTick(ctx)
			}
		}
	}()
}

// scheduleTick — один такт расписания; паника такта не останавливает расписание и не роняет API.
func (s *Services) scheduleTick(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			zap.L().Error("агент: паника в такте расписания", zap.Any("panic", r), zap.ByteString("stack", debug.Stack()))
		}
	}()
	if _, err := s.StartAgentRun(ctx, in.AgentRunRequest{Trigger: domain.TriggerSchedule}); err != nil {
		var e *errs.Error
		if errors.As(err, &e) {
			zap.L().Info("агент: такт расписания пропущен", zap.String("reason", e.Message))
		} else {
			zap.L().Warn("агент: такт расписания не запустился", zap.Error(err))
		}
	}
}

// RecoverAgentRuns — запуски, оборванные падением процесса, помечаются failed (вызывается при старте).
func (s *Services) RecoverAgentRuns(ctx context.Context) {
	if st := s.store(); st != nil {
		if err := st.FailStaleAgentRuns(ctx, "процесс API перезапущен во время запуска", s.now().UTC()); err != nil {
			zap.L().Warn("агент: оборванные запуски не помечены", zap.Error(err))
		}
	}
}

// shortText — строка для заголовка шага.
func shortText(s string, n int) string {
	return truncateRunes(strings.TrimSpace(strings.Join(strings.Fields(s), " ")), n)
}
