package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"sync"
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
}

// agentService — запуски агента в процессе: один запуск за раз.
type agentService struct {
	s     *Services
	model ChatModel
	opts  AgentOptions

	// evalHook — прогон правки в тестах цикла; nil — песочница (agent_eval.go).
	evalHook func(ctx context.Context, c *failedCluster, p RulePatch) (*patchEval, error)

	mu     sync.Mutex
	active *runState
	subs   map[uuid.UUID]map[chan agentEvent]struct{}
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
	cancel context.CancelFunc
	mu     sync.Mutex
	seq    int
	stop   string
}

func newAgentService(s *Services, model ChatModel, opts AgentOptions) *agentService {
	if len(opts.RegressionSets) == 0 {
		opts.RegressionSets = []string{domain.EvalSetRegression, domain.EvalSetTune}
	}
	return &agentService{s: s, model: model, opts: opts, subs: map[uuid.UUID]map[chan agentEvent]struct{}{}}
}

// agentRequestID — request_id поиска агента (≤32 символов, в журнал поиска не пишется).
func agentRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "agent-" + hex.EncodeToString(b)
}

// StartAgentRun — POST /api/agent/runs: запуск в фоне, 202 и id.
func (s *Services) StartAgentRun(ctx context.Context, req in.AgentRunRequest) (*out.AgentRunStarted, error) {
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
	if len(bad) > 0 {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(bad...)
	}
	a.mu.Lock()
	if a.active != nil {
		a.mu.Unlock()
		return nil, errs.NewConflictError(MsgAgentRunning)
	}
	p := prompts.Agent()
	run := &domain.AgentRun{Status: domain.RunRunning, Trigger: trigger, Days: days, MaxClusters: clusters,
		StartedAt: s.now().UTC(), Model: llmModelName(a.model.Model()), PromptVersion: p.Version}
	if err := st.CreateAgentRun(ctx, run); err != nil {
		a.mu.Unlock()
		return nil, err
	}
	rctx, cancel := context.WithTimeout(context.Background(), agentRunTimeout)
	rs := &runState{run: run, cancel: cancel}
	a.active = rs
	a.mu.Unlock()
	// Горутина запуска — не в s.writes: в конце она сама ждёт фоновые записи учёта (s.writes.Wait).
	go func() {
		defer cancel()
		a.execute(WithAgentRunID(rctx, run.ID.String()), rs)
		a.mu.Lock()
		a.active = nil
		a.mu.Unlock()
	}()
	zap.L().Info("агент: запуск", zap.String("run_id", run.ID.String()), zap.String("trigger", trigger),
		zap.Int("days", days), zap.Int("max_clusters", clusters))
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
	if a.active != nil && a.active.run.ID == run.ID {
		a.active.mu.Lock()
		a.active.stop = StopUser
		a.active.mu.Unlock()
		a.active.cancel()
	}
	return nil
}

// AgentRuns — GET /api/agent/runs.
func (s *Services) AgentRuns(ctx context.Context) (*out.AgentRunList, error) {
	st := s.store()
	if st == nil {
		return nil, errs.NewUnavailableError(MsgNoAgentStore)
	}
	rows, err := st.ListAgentRuns(ctx, 50)
	if err != nil {
		return nil, err
	}
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
	if a.active != nil && a.active.run.ID == uid {
		a.active.mu.Lock()
		cp := *a.active.run
		a.active.mu.Unlock()
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
	live := a.active != nil && a.active.run.ID == run.ID
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
		Episode: rawOrNull(r.Episode), MaxToolCalls: agentToolCallsPerCluster * max(r.ClustersTotal, 1),
		Limits: out.AgentLimits{ToolCallsPerCluster: agentToolCallsPerCluster, Hypotheses: agentMaxHypotheses,
			TokensPerRun: agentTokensPerRun, TimeoutSec: int(agentRunTimeout.Seconds())}}
	if r.CurrentClusterID != nil {
		v.CurrentCluster = &out.ClusterRef{ID: *r.CurrentClusterID, Label: r.CurrentLabel}
	}
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
				if _, err := s.StartAgentRun(ctx, in.AgentRunRequest{Trigger: domain.TriggerSchedule}); err != nil {
					var e *errs.Error
					if errors.As(err, &e) {
						zap.L().Info("агент: такт расписания пропущен", zap.String("reason", e.Message))
					} else {
						zap.L().Warn("агент: такт расписания не запустился", zap.Error(err))
					}
				}
			}
		}
	}()
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
