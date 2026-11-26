package services

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"kuznica/api/domain"
	"kuznica/api/pkg/errs"

	"go.uber.org/zap"
)

// Триггеры агента (контур автономии): расписание (ночной разбор неудач за сутки), всплеск (доля пустых выдач или
// переформулировок за окно выше порога, либо новый неудачный запрос набрал частоту) и изменение каталога (новая
// версия каталога или справочника связей → проверка подсказок, связей и правил). Пороги — AGENT_SPIKE_*,
// AGENT_SCHEDULE_AT (config/agent.go). У каждого запуска записан trigger и вход триггера (run.input).

// catalogMinGap — между проверками каталога не меньше: принятая правка связей тоже новая версия справочника.
const catalogMinGap = 30 * time.Minute

// scheduleCatchUp — пропущенный ночной разбор (API был выключен в ЧЧ:ММ) догоняется, если с ЧЧ:ММ прошло не больше.
const scheduleCatchUp = 6 * time.Hour

// triggerState — память триггеров в процессе (между проверками).
type triggerState struct {
	mu          sync.Mutex
	catalogSeen string
	catalogInit bool
}

// StartAgentTriggers — расписание и проверка триггеров, пока жив ctx. Расписание без AGENT_SCHEDULE_AT — по
// интервалу, как раньше (StartAgentScheduler).
func (s *Services) StartAgentTriggers(ctx context.Context) {
	t := s.agent.opts.Triggers
	if t.Schedule && t.ScheduleAt == "" {
		s.StartAgentScheduler(ctx, t.Interval)
	}
	if !(t.Schedule && t.ScheduleAt != "") && !t.Spike && !t.Catalog {
		return
	}
	zap.L().Info("агент: триггеры включены", zap.String("schedule_at", t.ScheduleAt), zap.Bool("spike", t.Spike),
		zap.Bool("catalog", t.Catalog), zap.Duration("check", t.Check))
	go func() {
		tick := time.NewTicker(t.Check)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				s.triggerTick(ctx)
			}
		}
	}()
}

// triggerTick — одна проверка всех триггеров; паника не останавливает проверки и не роняет API.
func (s *Services) triggerTick(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			zap.L().Error("агент: паника в проверке триггеров", zap.Any("panic", r), zap.ByteString("stack", debug.Stack()))
		}
	}()
	t := s.agent.opts.Triggers
	now := s.now()
	if t.Schedule && t.ScheduleAt != "" {
		s.logTrigger("schedule", s.checkSchedule(ctx, now))
	}
	if t.Spike {
		s.logTrigger("spike", s.checkSpike(ctx, now))
	}
	if t.Catalog {
		s.logTrigger("catalog_change", s.checkCatalog(ctx, now))
	}
}

// logTrigger — запуск не стартовал: занятый пул и выключенная модель — обычное дело, не ошибка.
func (s *Services) logTrigger(kind string, err error) {
	if err == nil {
		return
	}
	var e *errs.Error
	if errors.As(err, &e) {
		zap.L().Info("агент: триггер сработал, запуск отложен", zap.String("trigger", kind), zap.String("reason", e.Message))
		return
	}
	zap.L().Warn("агент: проверка триггера не удалась", zap.String("trigger", kind), zap.Error(err))
}

// scheduleSlot — сегодняшнее ЧЧ:ММ по Москве для момента now.
func scheduleSlot(at string, now time.Time) (time.Time, bool) {
	t, err := time.Parse("15:04", at)
	if err != nil {
		return time.Time{}, false
	}
	m := now.In(mskZone)
	return time.Date(m.Year(), m.Month(), m.Day(), t.Hour(), t.Minute(), 0, 0, mskZone), true
}

// nextScheduleAt — следующий ночной разбор.
func (s *Services) nextScheduleAt(now time.Time) time.Time {
	t := s.agent.opts.Triggers
	if t.ScheduleAt == "" {
		return now.Add(t.Interval).UTC()
	}
	slot, ok := scheduleSlot(t.ScheduleAt, now)
	if !ok {
		return now.UTC()
	}
	if !now.Before(slot) {
		slot = slot.Add(24 * time.Hour)
	}
	return slot.UTC()
}

// checkSchedule — ночной разбор: после ЧЧ:ММ, если с ЧЧ:ММ ещё не было запуска по расписанию.
func (s *Services) checkSchedule(ctx context.Context, now time.Time) error {
	t := s.agent.opts.Triggers
	slot, ok := scheduleSlot(t.ScheduleAt, now)
	if !ok || now.Before(slot) || now.Sub(slot) > scheduleCatchUp {
		return nil
	}
	st := s.store()
	if st == nil {
		return nil
	}
	last, err := st.ListAgentRunsByTrigger(ctx, domain.TriggerSchedule, 1)
	if err != nil {
		return err
	}
	if len(last) > 0 && !last[0].StartedAt.Before(slot) {
		return nil
	}
	_, err = s.startRun(ctx, runSpec{Trigger: domain.TriggerSchedule, Days: t.ScheduleDays, MaxClusters: agentDefaultClusters,
		Mode: domain.RunModeSearch, Input: map[string]any{"at": t.ScheduleAt, "slot": slot.UTC(), "days": t.ScheduleDays,
			"why": fmt.Sprintf("ночной разбор неудачных запросов за %d сут.", t.ScheduleDays)}})
	return err
}

// windowStats — исходы поисков [since, until) по тем же правилам, что вход агента: в базе (SQL) или в Go.
func (s *Services) windowStats(ctx context.Context, since, until time.Time) (*queryStats, error) {
	st := s.store()
	scope := s.agentLogScope(ctx)
	if sq, ok := st.(agentStatsSQL); ok {
		agg, err := sq.AgentQueryStatsScoped(ctx, since, until, scope)
		if err != nil {
			return nil, err
		}
		return queryStatsFromSQL(agg), nil
	}
	searches, err := st.SearchEventsSince(ctx, since)
	if err != nil {
		return nil, err
	}
	behavior, err := st.BehaviorEventsSince(ctx, since)
	if err != nil {
		return nil, err
	}
	var ss []domain.SearchEvent
	for _, e := range filterSince(filterScope(scope, searches), since) {
		if e.CreatedAt.Before(until) {
			ss = append(ss, e)
		}
	}
	var bs []domain.BehaviorEvent
	for _, b := range filterBehaviorSince(behavior, since) {
		if b.CreatedAt.Before(until) {
			bs = append(bs, b)
		}
	}
	return computeQueryStats(ss, bs), nil
}

// spikeQuery — неудачный запрос окна.
type spikeQuery struct {
	Text     string `json:"text"`
	Freq     int    `json:"freq"`
	Failures int    `json:"failures"`
	New      bool   `json:"new"`
}

// spikeReport — что увидел триггер «всплеск» за окно.
type spikeReport struct {
	Window      string       `json:"window"`
	Since       time.Time    `json:"since"`
	Until       time.Time    `json:"until"`
	Searches    int          `json:"searches"`
	Empty       int          `json:"empty"`
	Reform      int          `json:"reformulated"`
	EmptyShare  float64      `json:"empty_share"`
	ReformShare float64      `json:"reform_share"`
	Thresholds  any          `json:"thresholds"`
	Fired       []string     `json:"fired"`
	NewQueries  []spikeQuery `json:"new_queries"`
	Failed      []spikeQuery `json:"failed"`
	Why         string       `json:"why"`
}

// evaluateSpike — сработал ли всплеск за окно, заканчивающееся в now. Доли — от поисков окна (без набора по префиксу),
// пустая выдача и переформулировка без клика — как во входе агента. Новый запрос — неудачный (пусто или
// переформулировка) не меньше QueryFreq раз за окно и без единого поиска за 30 суток до окна.
func (s *Services) evaluateSpike(ctx context.Context, now time.Time) (*spikeReport, error) {
	th := s.agent.opts.Triggers.SpikeT
	since := now.Add(-th.Window)
	win, err := s.windowStats(ctx, since, now)
	if err != nil {
		return nil, err
	}
	r := &spikeReport{Window: th.Window.String(), Since: since.UTC(), Until: now.UTC(), NewQueries: []spikeQuery{},
		Failed: []spikeQuery{}, Fired: []string{},
		Thresholds: map[string]any{"min_searches": th.MinSearches, "empty_share": th.EmptyShare,
			"reform_share": th.ReformShare, "query_freq": th.QueryFreq}}
	var cands []string
	for _, q := range win.byText {
		r.Searches += q.Freq
		r.Empty += q.Empty
		r.Reform += q.Reformulated
		if bad := q.Empty + q.Reformulated; bad > 0 {
			r.Failed = append(r.Failed, spikeQuery{Text: q.Text, Freq: q.Freq, Failures: bad})
			if bad >= th.QueryFreq {
				cands = append(cands, q.Text)
			}
		}
	}
	sort.Slice(r.Failed, func(i, j int) bool {
		if r.Failed[i].Failures != r.Failed[j].Failures {
			return r.Failed[i].Failures > r.Failed[j].Failures
		}
		return r.Failed[i].Text < r.Failed[j].Text
	})
	if len(r.Failed) > 20 {
		r.Failed = r.Failed[:20]
	}
	if r.Searches > 0 {
		r.EmptyShare = round3(float64(r.Empty) / float64(r.Searches))
		r.ReformShare = round3(float64(r.Reform) / float64(r.Searches))
	}
	var why []string
	if r.Searches >= th.MinSearches {
		if r.EmptyShare > th.EmptyShare {
			r.Fired = append(r.Fired, "empty_share")
			why = append(why, fmt.Sprintf("пустых выдач %.0f %% из %d поисков (порог %.0f %%)", r.EmptyShare*100, r.Searches, th.EmptyShare*100))
		}
		if r.ReformShare > th.ReformShare {
			r.Fired = append(r.Fired, "reform_share")
			why = append(why, fmt.Sprintf("переформулировок %.0f %% из %d поисков (порог %.0f %%)", r.ReformShare*100, r.Searches, th.ReformShare*100))
		}
	}
	if len(cands) > 0 {
		prior, err := s.windowStats(ctx, since.Add(-30*24*time.Hour), since)
		if err != nil {
			return nil, err
		}
		for _, t := range cands {
			if prior.byText[t] != nil {
				continue
			}
			q := win.byText[t]
			r.NewQueries = append(r.NewQueries, spikeQuery{Text: t, Freq: q.Freq, Failures: q.Empty + q.Reformulated, New: true})
		}
		sort.Slice(r.NewQueries, func(i, j int) bool { return r.NewQueries[i].Failures > r.NewQueries[j].Failures })
		if len(r.NewQueries) > 0 {
			r.Fired = append(r.Fired, "new_query")
			n := r.NewQueries[0]
			why = append(why, fmt.Sprintf("новый неудачный запрос «%s» — %d неудачных поисков за окно (порог %d)", n.Text, n.Failures, th.QueryFreq))
		}
	}
	r.Why = strings.Join(why, "; ")
	return r, nil
}

func round3(f float64) float64 { return float64(int(f*1000+0.5)) / 1000 }

// checkSpike — всплеск: после паузы AGENT_SPIKE_COOLDOWN от прошлого запуска по всплеску.
func (s *Services) checkSpike(ctx context.Context, now time.Time) error {
	st := s.store()
	if st == nil {
		return nil
	}
	th := s.agent.opts.Triggers.SpikeT
	last, err := st.ListAgentRunsByTrigger(ctx, domain.TriggerSpike, 1)
	if err != nil {
		return err
	}
	if len(last) > 0 && now.Sub(last[0].StartedAt) < th.Cooldown {
		return nil
	}
	r, err := s.evaluateSpike(ctx, now)
	if err != nil || len(r.Fired) == 0 {
		return err
	}
	return s.startSpikeRun(ctx, r)
}

// startSpikeRun — запуск по всплеску: кластеры только из неудачных запросов окна (новые — первыми).
func (s *Services) startSpikeRun(ctx context.Context, r *spikeReport) error {
	var focus []string
	seen := map[string]bool{}
	for _, q := range r.NewQueries {
		focus, seen[q.Text] = append(focus, q.Text), true
	}
	for _, q := range r.Failed {
		if !seen[q.Text] {
			focus, seen[q.Text] = append(focus, q.Text), true
		}
	}
	_, err := s.startRun(ctx, runSpec{Trigger: domain.TriggerSpike, Days: 1, MaxClusters: 3, Mode: domain.RunModeSearch,
		Since: r.Since, Focus: focus, Input: map[string]any{"spike": r, "why": r.Why}})
	return err
}

// catalogFingerprint — версия каталога и справочника связей одной строкой.
func (s *Services) catalogFingerprint(ctx context.Context) (string, map[string]any) {
	cv := ""
	if cs := s.catalog.Load(); cs != nil {
		cv = cs.version
	}
	rv := 0
	if ch, err := s.relationChain(ctx); err == nil && ch.Materialized {
		rv = ch.Version
	}
	return fmt.Sprintf("%s|relations:%d", cv, rv), map[string]any{"catalog_version": cv, "relations_version": rv}
}

// checkCatalog — изменение каталога: отпечаток (каталог + справочник связей) не тот, что у прошлой проверки.
// Первая проверка процесса сверяется с последним запуском catalog_change в базе; запусков не было — проверка
// каталога после развёртывания.
func (s *Services) checkCatalog(ctx context.Context, now time.Time) error {
	st := s.store()
	if st == nil || s.catalog.Load() == nil {
		return nil
	}
	fp, ver := s.catalogFingerprint(ctx)
	ts := &s.agent.trig
	ts.mu.Lock()
	prev, init := ts.catalogSeen, ts.catalogInit
	ts.mu.Unlock()
	last, err := st.ListAgentRunsByTrigger(ctx, domain.TriggerCatalogChange, 1)
	if err != nil {
		return err
	}
	if !init {
		prev = ""
		if len(last) > 0 {
			var in struct {
				Fingerprint string `json:"fingerprint"`
			}
			_ = jsonUnmarshal(last[0].Input, &in)
			prev = in.Fingerprint
		}
	}
	if prev == fp {
		ts.mu.Lock()
		ts.catalogSeen, ts.catalogInit = fp, true
		ts.mu.Unlock()
		return nil
	}
	if len(last) > 0 && now.Sub(last[0].StartedAt) < catalogMinGap {
		return nil
	}
	why := "проверка каталога после развёртывания: прошлых проверок нет"
	from := map[string]any{}
	if prev != "" {
		why = "новая версия каталога или справочника связей"
		parts := strings.SplitN(prev, "|relations:", 2)
		from["catalog_version"] = parts[0]
		if len(parts) == 2 {
			from["relations_version"] = parts[1]
		}
	}
	_, err = s.startRun(ctx, runSpec{Trigger: domain.TriggerCatalogChange, Days: 1, MaxClusters: 3, Mode: domain.RunModeCommerce,
		Input: map[string]any{"fingerprint": fp, "from": from, "to": ver, "why": why}})
	if err == nil {
		ts.mu.Lock()
		ts.catalogSeen, ts.catalogInit = fp, true
		ts.mu.Unlock()
	}
	return err
}
