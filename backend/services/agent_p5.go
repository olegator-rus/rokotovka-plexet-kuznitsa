package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
)

// Контур автономии: агент запускается сам (триггеры — agent_triggers.go), строит план
// (agent_plan.go), держит бюджет запуска, помнит решения людей (agent_memory_p5.go) и проверяет эффект своих правок.
// Здесь — настройки, пул, бюджет и форма запуска в API.

// agentHardTimeoutGrace — запас жёсткого срока запуска над бюджетом времени: бюджет останавливает запуск мягко
// (итог, неразобранное), срок контекста — только страховка от зависшего вызова.
const agentHardTimeoutGrace = 90 * time.Second

// Причины остановки по бюджету запуска (run.stop_reason).
const (
	StopStepsBudget = "steps_budget"
	StopCostBudget  = "cost_budget"
	StopTimeBudget  = "time_budget"
)

// RunBudget — лимиты одного запуска: вызовы инструментов, токены, рубли, время.
type RunBudget struct {
	Steps    int
	Tokens   int
	CostRub  float64
	Duration time.Duration
}

// SpikeThresholds — пороги триггера «всплеск».
type SpikeThresholds struct {
	Window      time.Duration
	MinSearches int
	EmptyShare  float64
	ReformShare float64
	QueryFreq   int
	Cooldown    time.Duration
}

// AgentTriggers — когда агент запускается сам.
type AgentTriggers struct {
	// Schedule — ночной разбор: ежедневно в ScheduleAt (ЧЧ:ММ по Москве) или каждые Interval (ScheduleAt пусто).
	Schedule     bool
	ScheduleAt   string
	Interval     time.Duration
	ScheduleDays int
	Spike        bool
	SpikeT       SpikeThresholds
	Catalog      bool
	// Check — как часто проверять всплеск и изменение каталога.
	Check time.Duration
}

// withDefaults — значения по умолчанию для нулевых полей (тесты и CLI создают AgentOptions{}).
func (o AgentOptions) withDefaults() AgentOptions {
	if o.MaxParallel <= 0 {
		o.MaxParallel = 2
	}
	if o.Budget.Steps <= 0 {
		o.Budget.Steps = 80
	}
	if o.Budget.Tokens <= 0 {
		o.Budget.Tokens = agentTokensPerRun
	}
	if o.Budget.CostRub <= 0 {
		o.Budget.CostRub = 40
	}
	if o.Budget.Duration <= 0 {
		o.Budget.Duration = agentRunTimeout
	}
	if o.QueryDuration <= 0 {
		o.QueryDuration = 3 * time.Minute
	}
	t := &o.Triggers
	if t.ScheduleDays <= 0 {
		t.ScheduleDays = 1
	}
	if t.Interval <= 0 {
		t.Interval = 24 * time.Hour
	}
	if t.Check <= 0 {
		t.Check = time.Minute
	}
	sp := &t.SpikeT
	if sp.Window <= 0 {
		sp.Window = time.Hour
	}
	if sp.MinSearches <= 0 {
		sp.MinSearches = 60
	}
	if sp.EmptyShare <= 0 {
		sp.EmptyShare = 0.25
	}
	if sp.ReformShare <= 0 {
		sp.ReformShare = 0.12
	}
	if sp.QueryFreq <= 0 {
		sp.QueryFreq = 8
	}
	if sp.Cooldown <= 0 {
		sp.Cooldown = 3 * time.Hour
	}
	return o
}

// runSpec — что запускать: триггер, его вход и окно журнала.
type runSpec struct {
	Trigger     string
	Days        int
	MaxClusters int
	Mode        string
	// Query — запуск «Разобрать» на одном запросе.
	Query string
	// Input — вход триггера для API и плана (порог, окно, доли, новые запросы, версии каталога).
	Input map[string]any
	// Since — начало окна журнала (всплеск); нулевое — Days суток до запуска.
	Since time.Time
	// Focus — нормализованные тексты, на которые сработал триггер (всплеск): кластеры только из них.
	Focus []string
}

// sameKind — запуск того же вида уже идёт: один ночной, один по всплеску, одна проверка каталога, один разбор по
// периоду вручную и один разбор каждого запроса.
func (sp runSpec) sameKind(o runSpec) bool {
	if sp.Trigger != o.Trigger {
		return false
	}
	if sp.Trigger != domain.TriggerManual {
		return true
	}
	return sp.Query == o.Query && sp.Mode == o.Mode
}

func nonNilMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

// budgetUsed — расход бюджета запуска.
type budgetUsed struct {
	Steps   int     `json:"steps"`
	Tokens  int     `json:"tokens"`
	CostRub float64 `json:"cost_rub"`
	Seconds int     `json:"seconds"`
}

// budgetView — run.budget: лимиты, расход и что исчерпано.
func budgetView(b RunBudget, u budgetUsed, exhausted string) map[string]any {
	return map[string]any{
		"limits": map[string]any{"steps": b.Steps, "tokens": b.Tokens, "cost_rub": b.CostRub,
			"seconds": int(b.Duration.Seconds())},
		"used": u, "exhausted": exhausted,
	}
}

// used — расход бюджета сейчас.
func (rs *runState) used(now time.Time) budgetUsed {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return budgetUsed{Steps: rs.run.ToolCalls, Tokens: rs.run.InputTokens + rs.run.OutputTokens, CostRub: rs.run.CostRub,
		Seconds: int(now.Sub(rs.run.StartedAt).Seconds())}
}

// exhausted — какой лимит бюджета исчерпан ("" — ни один). Причина — stop_reason запуска.
func (rs *runState) exhausted(now time.Time) string {
	u := rs.used(now)
	b := rs.budget
	switch {
	case b.Steps > 0 && u.Steps >= b.Steps:
		return StopStepsBudget
	case b.Tokens > 0 && u.Tokens >= b.Tokens:
		return StopTokenLimit
	case b.CostRub > 0 && u.CostRub >= b.CostRub:
		return StopCostBudget
	case b.Duration > 0 && now.Sub(rs.run.StartedAt) >= b.Duration:
		return StopTimeBudget
	}
	return ""
}

// budgetLabels — что исчерпано, по-русски.
var budgetLabels = map[string]string{
	StopStepsBudget: "лимит шагов", StopTokenLimit: "лимит токенов", StopCostBudget: "лимит стоимости",
	StopTimeBudget: "лимит времени",
}

// p5View — поля контура автономии в форме запуска.
func (a *agentService) p5View(r *domain.AgentRun, v *out.AgentRun) {
	if r.Query != "" {
		q := r.Query
		v.Query = &q
	}
	v.ReadOnly = r.ReadOnly
	v.Input = rawOrNull(r.Input)
	v.Plan = rawOrNull(r.Plan)
	v.Budget = rawOrNull(r.Budget)
	v.Unprocessed = json.RawMessage("[]")
	var ep struct {
		Outcome     out.RunOutcome  `json:"outcome"`
		Unprocessed json.RawMessage `json:"unprocessed"`
	}
	if len(r.Episode) > 0 && json.Unmarshal(r.Episode, &ep) == nil {
		v.Outcome = ep.Outcome
		if len(ep.Unprocessed) > 0 && string(ep.Unprocessed) != "null" {
			v.Unprocessed = ep.Unprocessed
		}
	}
	if r.Status == domain.RunRunning {
		// Идущий запуск: расход бюджета — на сейчас.
		a.mu.Lock()
		rs := a.runs[r.ID]
		a.mu.Unlock()
		if rs != nil {
			v.Budget = json.RawMessage(toJSON(budgetView(rs.budget, rs.used(a.s.now()), "")))
		}
	}
	if r.FinishedAt != nil {
		d := r.FinishedAt.Sub(r.StartedAt).Milliseconds()
		v.DurationMs = &d
	}
}

// ── Настройки агента ──

// AgentSettings — GET /api/agent/settings: режим, расписание, пороги, пул, бюджет — одной строкой и по полям.
func (s *Services) AgentSettings(ctx context.Context) *out.AgentSettings {
	a := s.agent
	o := a.opts
	t := o.Triggers
	a.mu.Lock()
	running := len(a.runs)
	a.mu.Unlock()
	res := &out.AgentSettings{ReadOnly: a.readOnly.Load(), CatalogTrigger: t.Catalog, MaxParallel: o.MaxParallel,
		Running:  running,
		Schedule: out.AgentSchedule{Enabled: t.Schedule, At: t.ScheduleAt, Interval: t.Interval.String(), Days: t.ScheduleDays},
		Spike: out.AgentSpike{Enabled: t.Spike, Window: t.SpikeT.Window.String(), MinSearches: t.SpikeT.MinSearches,
			EmptyShare: t.SpikeT.EmptyShare, ReformShare: t.SpikeT.ReformShare, QueryFreq: t.SpikeT.QueryFreq,
			Cooldown: t.SpikeT.Cooldown.String()},
		Budget: out.AgentBudgetLimits{Steps: o.Budget.Steps, Tokens: o.Budget.Tokens, CostRub: o.Budget.CostRub,
			DurationSec: int(o.Budget.Duration.Seconds())},
		QueryBudget: out.AgentBudgetLimits{Steps: o.Budget.Steps, Tokens: o.Budget.Tokens, CostRub: o.Budget.CostRub,
			DurationSec: int(o.QueryDuration.Seconds())}}
	res.ModeLabel = "Предлагает правки, применяет человек"
	if res.ReadOnly {
		res.ModeLabel = "Только чтение: диагноз и черновик предложения, без записи в очередь"
	}
	if t.Schedule {
		next := s.nextScheduleAt(s.now())
		res.Schedule.NextAt = &next
	}
	var parts []string
	switch {
	case !t.Schedule:
		parts = append(parts, "расписание выключено")
	case t.ScheduleAt != "":
		parts = append(parts, fmt.Sprintf("ночной разбор в %s МСК за %d сут.", t.ScheduleAt, t.ScheduleDays))
	default:
		parts = append(parts, "разбор каждые "+t.Interval.String())
	}
	if t.Spike {
		parts = append(parts, fmt.Sprintf("всплеск: за %s от %d поисков пустых > %.0f %% или переформулировок > %.0f %%, новый запрос ≥ %d",
			shortDuration(t.SpikeT.Window), t.SpikeT.MinSearches, t.SpikeT.EmptyShare*100, t.SpikeT.ReformShare*100, t.SpikeT.QueryFreq))
	} else {
		parts = append(parts, "всплеск выключен")
	}
	if t.Catalog {
		parts = append(parts, "изменение каталога — проверка подсказок, связей и правил")
	}
	res.Line = strings.Join(parts, " · ")
	res.Regression = s.RegressionStatus(ctx)
	return res
}

// SetAgentSettings — POST /api/agent/settings {"read_only": true|false}: режим «только чтение» без перезапуска.
func (s *Services) SetAgentSettings(ctx context.Context, req in.AgentSettingsRequest) (*out.AgentSettings, error) {
	if req.ReadOnly == nil {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(errs.FieldError{Field: "read_only",
			Code: errs.CodeInvalidFormat, Message: "Укажите read_only: true или false"})
	}
	s.agent.readOnly.Store(*req.ReadOnly)
	return s.AgentSettings(ctx), nil
}

// shortDuration — «1h0m0s» → «1 ч», «30m0s» → «30 мин».
func shortDuration(d time.Duration) string {
	switch {
	case d%time.Hour == 0:
		return fmt.Sprintf("%d ч", int(d.Hours()))
	case d%time.Minute == 0:
		return fmt.Sprintf("%d мин", int(d.Minutes()))
	}
	return d.String()
}
