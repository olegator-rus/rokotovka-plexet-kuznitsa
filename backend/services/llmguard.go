package services

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"kuznica/api/out"
	"kuznica/api/pkg/errs"

	"go.uber.org/zap"
)

// Режим LLM и дневной бюджет (ARCHITECTURE §8). Режим переключается без перезапуска
// (POST /api/admin/llm-mode); бюджет — потолок расхода на модель за сутки по Москве: сверх него маршрут D
// работает на правилах и кэше (причина budget_exceeded в трассе), агент не запускается.

// Режимы LLM.
const (
	llmModeOn  = "on"
	llmModeOff = "off"
)

// fallbackBudget — причина отката разбора на правила: дневной бюджет LLM исчерпан.
const fallbackBudget = "budget_exceeded"

// budgetRefresh — как часто перечитывать расход за сутки из llm_usage: туда же пишут CLI (eval, judge,
// warmup), которых процесс API не видит. Между перечитываниями расход копится в памяти.
const budgetRefresh = 30 * time.Second

// budgetReadTimeout — чтение расхода из базы; не ответила — считаем по памяти.
const budgetReadTimeout = time.Second

// mskZone — сутки бюджета считаются по Москве (как счёт провайдера).
var mskZone = time.FixedZone("MSK", 3*60*60)

// Тексты.
const (
	MsgLLMModeInvalid  = "Режим LLM — on или off"
	MsgLLMNotOnline    = "Модель не настроена (LLM_API_KEY): включить режим LLM нельзя"
	MsgBudgetExceeded  = "Дневной бюджет LLM исчерпан: агент не запускается до завтра (LLM_DAILY_BUDGET_RUB)"
	bannerLLMOff       = "Режим без LLM: сложные запросы разбираются правилами и кэшем"
	bannerBudgetFormat = "Дневной бюджет LLM исчерпан (%.2f из %.0f ₽): сложные запросы разбираются правилами и кэшем"
)

// llmGuard — режим и бюджет модели в процессе.
type llmGuard struct {
	mode    atomic.Value // string: on | off
	online  bool         // модель настроена (ключ и провайдер)
	limit   float64      // ₽ в сутки; 0 — без потолка
	changed atomic.Pointer[time.Time]

	mu        sync.Mutex
	day       time.Time // начало суток (МСК), к которым относится spent
	spent     float64
	refreshed time.Time
	loading   bool
	// load — расход с момента since из llm_usage; nil — без базы (только память).
	load func(ctx context.Context, since time.Time) (float64, error)
	now  func() time.Time
}

func newLLMGuard(mode string, online bool, limit float64, now func() time.Time,
	load func(context.Context, time.Time) (float64, error)) *llmGuard {
	g := &llmGuard{online: online, limit: limit, load: load, now: now}
	if mode != llmModeOn {
		mode = llmModeOff
	}
	g.mode.Store(mode)
	return g
}

func (g *llmGuard) Mode() string { return g.mode.Load().(string) }

// dayStart — начало суток по Москве.
func dayStart(t time.Time) time.Time {
	m := t.In(mskZone)
	return time.Date(m.Year(), m.Month(), m.Day(), 0, 0, 0, 0, mskZone)
}

// spentToday — расход за сутки: из базы не чаще budgetRefresh, между чтениями — память. Чтение базы — в
// фоне, кроме первого раза и смены суток: путь поиска не ждёт базу дольше budgetReadTimeout.
func (g *llmGuard) spentToday(ctx context.Context) float64 {
	now := g.now()
	day := dayStart(now)
	g.mu.Lock()
	if !g.day.Equal(day) {
		g.day, g.spent, g.refreshed = day, 0, time.Time{}
	}
	stale := g.load != nil && !g.loading && now.Sub(g.refreshed) >= budgetRefresh
	first := g.refreshed.IsZero()
	spent := g.spent
	if stale {
		g.loading = true
	}
	g.mu.Unlock()
	if !stale {
		return spent
	}
	read := func(ctx context.Context) {
		rctx, cancel := context.WithTimeout(ctx, budgetReadTimeout)
		defer cancel()
		v, err := g.load(rctx, day)
		g.mu.Lock()
		defer g.mu.Unlock()
		g.loading = false
		if err != nil {
			zap.L().Warn("бюджет LLM: расход за сутки не прочитан — считаю по памяти", zap.Error(err))
			g.refreshed = g.now() // не долбить базу при каждом поиске
			return
		}
		if g.day.Equal(day) {
			g.spent, g.refreshed = v, g.now()
		}
	}
	if first {
		read(context.WithoutCancel(ctx))
		g.mu.Lock()
		defer g.mu.Unlock()
		return g.spent
	}
	go read(context.Background())
	return spent
}

// add — расход вызова (между чтениями базы).
func (g *llmGuard) add(cost float64) {
	if cost <= 0 {
		return
	}
	day := dayStart(g.now())
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.day.Equal(day) {
		g.day, g.spent, g.refreshed = day, 0, time.Time{}
	}
	g.spent += cost
}

// exceeded — дневной бюджет исчерпан.
func (g *llmGuard) exceeded(ctx context.Context) bool {
	return g.limit > 0 && g.spentToday(ctx) >= g.limit
}

// llmGuardFor — guard сервиса; в тестах, собранных без NewServices, — выключенный режим.
func (s *Services) guard() *llmGuard {
	if s.llm == nil {
		return newLLMGuard(llmModeOff, false, 0, time.Now, nil)
	}
	return s.llm
}

// LLMMode — действующий режим: on | off.
func (s *Services) LLMMode() string { return s.guard().Mode() }

// LLMState — режим, бюджет и плашка для /api/meta и /api/admin/llm-mode.
func (s *Services) LLMState(ctx context.Context) out.LLMState {
	g := s.guard()
	st := out.LLMState{Mode: g.Mode(), Online: g.online, ChangedAt: g.changed.Load()}
	b := out.LLMBudget{Day: dayStart(g.now()).Format(time.DateOnly)}
	if g.limit > 0 {
		lim := g.limit
		b.DailyRub = &lim
	}
	b.SpentTodayRub = math.Round(g.spentToday(ctx)*1e4) / 1e4
	b.Exceeded = g.limit > 0 && b.SpentTodayRub >= g.limit
	st.Budget = b
	switch {
	case st.Mode == llmModeOff:
		st.Banner = ptr(bannerLLMOff)
	case b.Exceeded:
		st.Banner = ptr(fmt.Sprintf(bannerBudgetFormat, b.SpentTodayRub, g.limit))
	}
	return st
}

// SetLLMMode — POST /api/admin/llm-mode: переключение без перезапуска. Включить можно только настроенную
// модель.
func (s *Services) SetLLMMode(ctx context.Context, mode string) (*out.LLMState, error) {
	g := s.guard()
	if mode != llmModeOn && mode != llmModeOff {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).
			WithDetails(errs.FieldError{Field: "mode", Code: errs.CodeInvalidFormat, Message: MsgLLMModeInvalid})
	}
	if mode == llmModeOn && !g.online {
		return nil, errs.NewConflictError(MsgLLMNotOnline)
	}
	prev := g.Mode()
	g.mode.Store(mode)
	if prev != mode {
		now := s.now().UTC()
		g.changed.Store(&now)
		zap.L().Info("режим LLM переключён", zap.String("from", prev), zap.String("to", mode))
	}
	st := s.LLMState(ctx)
	return &st, nil
}

func ptr[T any](v T) *T { return &v }
