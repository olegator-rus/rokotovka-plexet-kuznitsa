package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"
	"unicode/utf8"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"

	"go.uber.org/zap"
)

// Прогон панели по набору префиксов: `make eval SET=panel`. Два режима через ту же
// функцию, что отвечает покупателю (SuggestPanel, без записи трассы):
//   - old — прежние подсказки контура коммерции (флаг панели выключен в контексте; ответ — groups);
//   - new — панель контура выдачи (suggestions, products, meta).
// Цели набора — из позднего периода журнала (отложенная неделя агента), индекс подсказок контура выдачи строится на
// раннем; прежние подсказки собираются из всего журнала, как работают на витрине (это в пользу «до»).
// Подсказки в пустую выдачу проверяются текстовой веткой поиска (B1, без LLM и векторов).

const (
	panelEvalOld = "old"
	panelEvalNew = "new"
	// panelEvalCheckConfig — чем проверяется, что подсказка не ведёт в пустую выдачу (§11.4: «hits по текстовой
	// ветке»).
	panelEvalCheckConfig = domain.ConfigB1
	panelEvalIntentDays  = 30
	panelEvalExamples    = 10
)

var panelEvalLabels = map[string]string{
	panelEvalOld: "Прежние подсказки (контур коммерции)",
	panelEvalNew: "Панель контура выдачи",
}

// prefixLoader — источник набора префиксов (MockJSON).
type prefixLoader interface {
	LoadPrefixes(ctx context.Context) ([]domain.PanelPrefix, []byte, error)
}

// MsgPanelNoPrefixes — набора префиксов нет.
const MsgPanelNoPrefixes = "Набор префиксов панели не найден (data/mock/prefixes.json)"

// RunPanelEvaluation — прогон набора префиксов в двух режимах; persist — записать в evaluations (set = panel).
func (s *Services) RunPanelEvaluation(ctx context.Context, persist bool) (*out.PanelEvalRun, error) {
	started := time.Now()
	pl, ok := s.source.(prefixLoader)
	if !ok {
		return nil, errs.NewValidationError(MsgPanelNoPrefixes)
	}
	prefixes, raw, err := pl.LoadPrefixes(ctx)
	if err != nil {
		if isNoFile(err) {
			return nil, errs.NewValidationError(MsgPanelNoPrefixes)
		}
		return nil, errs.NewUnavailableError(MsgSourceBroken).WithCause(err)
	}
	sum := sha256.Sum256(raw)
	run := &out.PanelEvalRun{RunID: newEvalRunID(started), Set: domain.EvalSetPanel, SetHash: hex.EncodeToString(sum[:])[:16],
		Prefixes: len(prefixes), Commit: buildCommit(), StartedAt: started.UTC(), Modes: []out.PanelModeResult{},
		Changes: []out.PanelChange{}, Warnings: []string{}}
	if st := s.catalog.Load(); st != nil {
		run.CatalogVersion = st.version
	}
	run.Split = s.panelSplit(ctx, prefixes)
	checked := map[string]int{} // подсказка → результатов в B1 (-1 — ошибка поиска)
	var rows [2][]out.PanelPrefixResult
	for i, mode := range []string{panelEvalOld, panelEvalNew} {
		res, r := s.panelEvalMode(ctx, run.RunID, mode, prefixes, checked)
		rows[i] = r
		run.Modes = append(run.Modes, res)
		if !res.Available {
			run.Warnings = append(run.Warnings, res.Label+": "+res.Reason)
		}
	}
	if run.Modes[0].Available && run.Modes[1].Available {
		run.Changes = pevalChanges(prefixes, rows[0], rows[1])
	}
	for _, m := range run.Modes {
		if m.Metrics.Errors > 0 {
			run.Warnings = append(run.Warnings, fmt.Sprintf("%s: ошибок %d — эти префиксы в метрики не вошли", m.Label, m.Metrics.Errors))
		}
	}
	run.DurationMs = time.Since(started).Milliseconds()
	run.Repro = fmt.Sprintf("набор panel (%d префиксов) sha256:%s · каталог %s · commit %s", run.Prefixes, run.SetHash,
		run.CatalogVersion, run.Commit)
	zap.L().Info("оценка панели: прогон завершён", zap.String("run_id", run.RunID), zap.Int("prefixes", len(prefixes)),
		zap.Int64("ms", run.DurationMs))
	if persist && s.repos.Enabled() {
		if err := s.repos.SaveEvaluations(ctx, panelEvalRows(run)); err != nil {
			return run, fmt.Errorf("оценка панели: результат не записан: %w", err)
		}
	}
	return run, nil
}

// panelSplit — границы позднего периода (отложенная неделя агента) и число целей из него.
func (s *Services) panelSplit(ctx context.Context, prefixes []domain.PanelPrefix) out.PanelSplit {
	sp := out.PanelSplit{Note: "индекс подсказок контура выдачи — ранний период журнала (до отложенной недели агента), " +
		"цели набора — поздний; прежние подсказки — из всего журнала, как на витрине"}
	for _, p := range prefixes {
		if p.Source == "log_late" || p.Source == "log_empty" {
			sp.LateTargets++
		}
	}
	el, ok := s.source.(eventLoader)
	if !ok {
		return sp
	}
	evs, err := el.LoadEvents(ctx)
	if err != nil {
		return sp
	}
	var last time.Time
	for _, e := range evs {
		if e.Type == "search" && e.TS.After(last) {
			last = e.TS
		}
	}
	if last.IsZero() {
		return sp
	}
	h := HoldoutFromEnv()
	h.Enabled = true
	from, to := s.holdoutRangeAt(h, last, last)
	from, to = from.UTC(), to.UTC()
	sp.LateFrom, sp.LateTo = &from, &to
	return sp
}

// panelEvalMode — один режим: проход по префиксам (промах кэша), повтор (кэш), сэкономленные нажатия, проверка
// подсказок поиском.
func (s *Services) panelEvalMode(ctx context.Context, runID, mode string, prefixes []domain.PanelPrefix,
	checked map[string]int) (out.PanelModeResult, []out.PanelPrefixResult) {
	res := out.PanelModeResult{Mode: mode, Label: panelEvalLabels[mode], Available: true,
		ByType: map[string]out.PanelMetrics{}, PerPrefix: []out.PanelPrefixResult{}}
	obs := make([]pevalObs, len(prefixes))
	rows := make([]out.PanelPrefixResult, len(prefixes))
	for i, p := range prefixes {
		if err := ctx.Err(); err != nil {
			obs[i].err = err
		} else {
			o, avail := s.panelObserve(ctx, runID, mode, p.Prefix, true, true)
			if !avail {
				res.Available, res.Reason = false, "панель контура выдачи не отвечает секциями (FEATURE_PANEL выключен или /suggest контура выдачи не подключён)"
				res.Metrics.Prefixes = len(prefixes)
				return res, rows
			}
			obs[i] = o
		}
		rows[i] = scorePanelPrefix(p, obs[i])
	}
	var cached []float64
	for _, p := range prefixes {
		if ctx.Err() != nil {
			break
		}
		_, _ = s.panelObserve(ctx, runID, mode, p.Prefix, true, false) // прогрев кэша товаров
		if o, _ := s.panelObserve(ctx, runID, mode, p.Prefix, true, false); o.err == nil {
			cached = append(cached, o.ms)
		}
	}
	saved := s.panelSavedKeystrokes(ctx, runID, mode, prefixes)
	res.Metrics = aggregatePanel(prefixes, rows, obs, saved)
	res.Metrics.P50MsCached, res.Metrics.P95MsCached = pevalPercentile(cached, 50), pevalPercentile(cached, 95)
	s.panelCheckEmpty(ctx, runID, obs, checked, &res.Metrics)
	res.ByType = pevalByType(prefixes, rows, obs)
	res.PerPrefix = rows
	return res, rows
}

// panelObserve — ответ панели на префикс. products = false — секция товаров выключена (сэкономленные нажатия);
// noCache — замер промаха кэша товаров. avail = false — режим new, но панель контура выдачи не отвечает.
func (s *Services) panelObserve(ctx context.Context, runID, mode, q string, products, noCache bool) (pevalObs, bool) {
	f := s.featuresFor(ctx)
	if mode == panelEvalOld {
		f.Panel = false
	} else if !products {
		f.PanelProducts = false
	}
	rid := evalRequestPrefix + runID[len(runID)-6:] + "-p-" + mode
	t := time.Now()
	resp, err := s.SuggestPanel(WithFeatures(ctx, f), rid, SuggestRequest{Q: q}, SuggestOptions{NoLog: true, NoCache: noCache})
	wall := float64(time.Since(t).Microseconds()) / 1000
	if err != nil {
		return pevalObs{err: err, ms: wall}, true
	}
	o := pevalObs{ms: wall}
	if mode == panelEvalOld {
		for _, g := range resp.Groups {
			for _, it := range g.Items {
				o.suggestions = append(o.suggestions, it.Text)
				if g.Type == suggestProduct && it.ID != nil {
					o.families = append(o.families, *it.ID)
				}
			}
		}
		return o, true
	}
	if resp.Mode == "" || resp.Mode == panelModeOff {
		return o, false
	}
	for _, sg := range resp.Suggestions {
		o.suggestions = append(o.suggestions, sg.Text)
	}
	for _, p := range resp.Products {
		o.families = append(o.families, p.FamilyID)
		o.products = append(o.products, p.ProductID)
	}
	if resp.Meta.Ms.Total > 0 {
		o.ms = resp.Meta.Ms.Total
	}
	o.productsMs, o.partial = resp.Meta.Ms.Products, resp.Meta.Partial
	o.llm, o.embed, o.rerank = resp.Meta.LLMCalls, resp.Meta.EmbedCalls, resp.Meta.RerankCalls
	return o, true
}

// panelSavedKeystrokes — по различным целям позднего периода (поиски с кликом): набор по символу, пока цель не
// окажется в первых 5 подсказках.
func (s *Services) panelSavedKeystrokes(ctx context.Context, runID, mode string, prefixes []domain.PanelPrefix) []pevalSaved {
	seen := map[string]bool{}
	var targets []string
	for _, p := range prefixes {
		if p.Source != "log_late" || seen[foldText(p.TargetQuery)] {
			continue
		}
		seen[foldText(p.TargetQuery)] = true
		targets = append(targets, p.TargetQuery)
	}
	sort.Strings(targets)
	res := make([]pevalSaved, 0, len(targets))
	for _, t := range targets {
		runes := []rune(t)
		sv := pevalSaved{length: len(runes)}
		for n := 1; n <= len(runes) && ctx.Err() == nil; n++ {
			o, _ := s.panelObserve(ctx, runID, mode, string(runes[:n]), false, false)
			if o.err == nil && pevalRank(o.suggestions, t, pevalTopSuggest, foldText) > 0 {
				sv.found = n
				break
			}
		}
		res = append(res, sv)
	}
	return res
}

// panelCheckEmpty — различные показанные подсказки (первые 5) проверяются текстовой веткой поиска; результат
// кэшируется на прогон (оба режима).
func (s *Services) panelCheckEmpty(ctx context.Context, runID string, obs []pevalObs, checked map[string]int, m *out.PanelMetrics) {
	seen := map[string]bool{}
	for _, o := range obs {
		for _, text := range pevalCap(o.suggestions, pevalTopSuggest) {
			key := foldText(text)
			if seen[key] || utf8.RuneCountInString(key) < 2 {
				continue
			}
			seen[key] = true
			n, ok := checked[key]
			if !ok {
				n = -1
				resp, err := s.SearchWith(ctx, evalRequestPrefix+runID[len(runID)-6:]+"-chk", in.SearchRequest{Query: text,
					Config: panelEvalCheckConfig, Limit: 1, SessionID: evalRequestPrefix + runID}, SearchOptions{NoLog: true})
				if err == nil {
					n = resp.Total
				}
				checked[key] = n
			}
			if n < 0 {
				continue
			}
			m.SuggestionsChecked++
			if n == 0 {
				m.SuggestionsToEmpty++
				if len(m.ToEmptyExamples) < panelEvalExamples {
					m.ToEmptyExamples = append(m.ToEmptyExamples, text)
				}
			}
		}
	}
}

// panelEvalRows — строки evaluations: по строке на режим (config = old | new).
func panelEvalRows(run *out.PanelEvalRun) []domain.Evaluation {
	rows := make([]domain.Evaluation, 0, len(run.Modes))
	for _, m := range run.Modes {
		row := domain.Evaluation{RunID: run.RunID, Set: run.Set, SetHash: run.SetHash, Queries: run.Prefixes,
			Config: m.Mode, CatalogVersion: run.CatalogVersion, Commit: run.Commit, DurationMs: int(run.DurationMs)}
		row.Metrics, _ = domain.ToJSONB(panelStored{Metrics: m.Metrics, Available: m.Available, Reason: m.Reason,
			Split: run.Split, Warnings: run.Warnings, StartedAt: run.StartedAt})
		row.ByType, _ = domain.ToJSONB(m.ByType)
		row.PerQuery, _ = domain.ToJSONB(m.PerPrefix)
		if m.Mode == panelEvalNew {
			row.Against = panelEvalOld
			row.Regressions, _ = domain.ToJSONB(run.Changes)
		}
		rows = append(rows, row)
	}
	return rows
}

// panelStored — что лежит в evaluations.metrics строки панели: метрики и сведения о прогоне.
type panelStored struct {
	Metrics   out.PanelMetrics `json:"metrics"`
	Available bool             `json:"available"`
	Reason    string           `json:"reason,omitempty"`
	Split     out.PanelSplit   `json:"split"`
	Warnings  []string         `json:"warnings"`
	StartedAt time.Time        `json:"started_at"`
}

// PanelEvaluations — GET /api/evaluations?set=panel: последний прогон и учёт намерений по событиям стенда.
func (s *Services) PanelEvaluations(ctx context.Context) (*out.PanelEvaluation, error) {
	resp := &out.PanelEvaluation{Set: domain.EvalSetPanel, IntentsDays: panelEvalIntentDays}
	if !s.repos.Enabled() {
		return resp, nil
	}
	rows, err := s.repos.LatestEvaluations(ctx, domain.EvalSetPanel)
	if err != nil {
		return nil, err
	}
	if run, err := panelRunFromRows(rows); err != nil {
		return nil, err
	} else {
		resp.Run = run
	}
	resp.Intents, err = s.intentAnalytics(ctx, s.now().UTC().Add(-panelEvalIntentDays*24*time.Hour))
	return resp, err
}

// panelRunFromRows — прогон из строк evaluations; режимы одного run_id (последнего), old перед new.
func panelRunFromRows(rows []domain.Evaluation) (*out.PanelEvalRun, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	latest := rows[0]
	for _, r := range rows[1:] {
		if r.CreatedAt.After(latest.CreatedAt) {
			latest = r
		}
	}
	run := &out.PanelEvalRun{RunID: latest.RunID, Set: latest.Set, SetHash: latest.SetHash, Prefixes: latest.Queries,
		Commit: latest.Commit, CatalogVersion: latest.CatalogVersion, DurationMs: int64(latest.DurationMs),
		Modes: []out.PanelModeResult{}, Changes: []out.PanelChange{}, Warnings: []string{}}
	for _, mode := range []string{panelEvalOld, panelEvalNew} {
		for _, r := range rows {
			if r.RunID != latest.RunID || r.Config != mode {
				continue
			}
			var st panelStored
			m := out.PanelModeResult{Mode: mode, Label: panelEvalLabels[mode], ByType: map[string]out.PanelMetrics{},
				PerPrefix: []out.PanelPrefixResult{}}
			for _, f := range []struct {
				raw domain.JSONB
				dst any
			}{{r.Metrics, &st}, {r.ByType, &m.ByType}, {r.PerQuery, &m.PerPrefix}} {
				if len(f.raw) > 0 {
					if err := json.Unmarshal(f.raw, f.dst); err != nil {
						return nil, fmt.Errorf("evaluations panel %s: %w", r.RunID, err)
					}
				}
			}
			m.Metrics, m.Available, m.Reason = st.Metrics, st.Available, st.Reason
			run.Split, run.StartedAt = st.Split, st.StartedAt
			if st.Warnings != nil {
				run.Warnings = st.Warnings
			}
			if mode == panelEvalNew && len(r.Regressions) > 0 {
				if err := json.Unmarshal(r.Regressions, &run.Changes); err != nil {
					return nil, fmt.Errorf("evaluations panel %s: %w", r.RunID, err)
				}
			}
			run.Modes = append(run.Modes, m)
		}
	}
	run.Repro = fmt.Sprintf("набор panel (%d префиксов) sha256:%s · каталог %s · commit %s", run.Prefixes, run.SetHash,
		run.CatalogVersion, run.Commit)
	return run, nil
}

// FormatPanelTable — таблица прогона панели для stdout.
func FormatPanelTable(run *out.PanelEvalRun) string {
	f := func(v *float64, pct bool) string {
		if v == nil {
			return "—"
		}
		if pct {
			return fmt.Sprintf("%.1f%%", *v*100)
		}
		return fmt.Sprintf("%.3f", *v)
	}
	ms := func(v *float64) string {
		if v == nil {
			return "—"
		}
		return fmt.Sprintf("%.2f", *v)
	}
	b := fmt.Sprintf("Прогон панели %s · %s\n", run.RunID, run.Repro)
	if run.Split.LateFrom != nil {
		b += fmt.Sprintf("Поздний период: %s — %s; префиксов с целью из него: %d\n",
			run.Split.LateFrom.Format(time.RFC3339), run.Split.LateTo.Format(time.RFC3339), run.Split.LateTargets)
	}
	b += fmt.Sprintf("%-28s %8s %8s %8s %8s %8s %8s %8s %6s %7s %7s %8s %8s %8s %8s %4s\n", "режим", "MRR@5п",
		"S@5п", "MRR@5т", "S@5т", "нажатий", "сэконом", "пустых", "дубли", "точн.выт", "в пусто", "p50", "p95",
		"p50кэш", "p95кэш", "LLM")
	for _, m := range run.Modes {
		if !m.Available {
			b += fmt.Sprintf("%-28s не прогнан: %s\n", m.Label, m.Reason)
			continue
		}
		x := m.Metrics
		b += fmt.Sprintf("%-28s %8s %8s %8s %8s %8s %8s %8s %6d %4d/%-3d %3d/%-3d %8s %8s %8s %8s %4d\n", m.Label,
			f(x.SuggestMRR5, false), f(x.SuggestSuccess5, true), f(x.ProductMRR5, false), f(x.ProductSuccess5, true),
			ms(x.SavedKeystrokes), f(x.SavedShare, true), f(x.EmptySuggestRate, true), x.FamilyDupPrefixes,
			x.ExactDisplaced, x.ExactChecks, x.SuggestionsToEmpty, x.SuggestionsChecked, ms(x.P50Ms), ms(x.P95Ms),
			ms(x.P50MsCached), ms(x.P95MsCached), x.LLMCalls+x.EmbedCalls+x.RerankCalls)
	}
	reg, imp := 0, 0
	for _, c := range run.Changes {
		if c.Kind == "regression" {
			reg++
		} else {
			imp++
		}
	}
	b += fmt.Sprintf("Изменения new против old: улучшений %d, ухудшений %d\n", imp, reg)
	for _, w := range run.Warnings {
		b += "! " + w + "\n"
	}
	return b
}
