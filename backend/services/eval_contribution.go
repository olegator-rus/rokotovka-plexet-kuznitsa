package services

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"

	"go.uber.org/zap"
)

// Вклад компонентов (контур доводки, DEFENSE вопрос 1): что даёт каждый компонент поиска на замороженном наборе.
// Лестница добавляет по одному компоненту: точное совпадение (B0) → текст с нормализацией (B1) → + векторы (B2) →
// + разбор намерения, атрибуты, порог и бизнес-слой (B3 без реранкера) → + реранкер (B3, полный путь). Прогоны
// «с одним изменением» выключают в полном пути один компонент: векторную ветку, LLM, все модели сразу. Решение
// «оставить / удалить» по компоненту — полный путь против полного пути без него (ухудшения и улучшения по
// запросам), на mock-данных предварительное.

// contributionSpec — строка таблицы: конфигурация и что выключено.
type contributionSpec struct {
	id, kind, label, config, against string
	off                              searchModes
}

// Идентификаторы строк.
const (
	contribB3NoRerank = "B3-R"
	contribB3NoVec    = "B3-V"
	contribB3NoLLM    = "B3-L"
	contribB3Offline  = "B3-M"
	contribLadder     = "ladder"
	contribSingle     = "single_change"
)

var contributionSpecs = []contributionSpec{
	{id: domain.ConfigB0, kind: contribLadder, label: "Точное совпадение подстроки (B0)", config: domain.ConfigB0},
	{id: domain.ConfigB1, kind: contribLadder, label: "+ текст с нормализацией и словарями (B1)", config: domain.ConfigB1, against: domain.ConfigB0},
	{id: domain.ConfigB2, kind: contribLadder, label: "+ векторы BGE-M3 и RRF (B2)", config: domain.ConfigB2, against: domain.ConfigB1},
	{id: contribB3NoRerank, kind: contribLadder, label: "+ разбор намерения, атрибуты, порог, итоговое ранжирование (B3 без реранкера)",
		config: domain.ConfigB3, against: domain.ConfigB2, off: searchModes{noRerank: true}},
	{id: domain.ConfigB3, kind: contribLadder, label: "+ реранкер — полный путь (B3)", config: domain.ConfigB3, against: contribB3NoRerank},
	{id: contribB3NoVec, kind: contribSingle, label: "Полный путь без векторной ветки (kNN выключен, реранкер есть)",
		config: domain.ConfigB3, against: domain.ConfigB3, off: searchModes{noVectors: true}},
	{id: contribB3NoLLM, kind: contribSingle, label: "Полный путь без LLM (только правила, кэш разборов не читается)",
		config: domain.ConfigB3, against: domain.ConfigB3, off: searchModes{noLLM: true}},
	{id: contribB3Offline, kind: contribSingle, label: "Без моделей: LLM, векторы и реранкер выключены одной настройкой",
		config: domain.ConfigB3, against: domain.ConfigB3, off: searchModes{noLLM: true, noVectors: true, noRerank: true}},
}

// Пороги решения: на 148 запросах test разница 0,01–0,02 NDCG — один-два запроса (EVALUATION «Ограничения»).
const (
	contribKeepDelta   = 0.02
	contribRemoveDelta = 0.005
	contribHead        = 5
)

// Тексты.
const (
	MsgContribNoRun   = "Прогона вклада компонентов на этом наборе ещё не было: make eval SET=contribution"
	MsgContribRunning = "Прогон вклада компонентов уже идёт — дождитесь его окончания"
	MsgContribSet     = "Набор — tune, test или regression"
)

// contribSets — наборы, на которых считается таблица (замороженный test и наборы настройки и проверки правок).
var contribSets = []string{domain.EvalSetTest, domain.EvalSetTune, domain.EvalSetRegression}

func contribSet(set string) (string, error) {
	set = strings.ToLower(strings.TrimSpace(set))
	if set == "" {
		set = domain.EvalSetTest
	}
	for _, s := range contribSets {
		if s == set {
			return set, nil
		}
	}
	return "", errs.NewValidationError(errs.MsgInvalidFields).WithDetails(
		errs.FieldError{Field: "set", Code: errs.CodeInvalidFormat, Message: MsgContribSet})
}

// contribRowStats — метрики строки и стоимость из трасс.
type contribRowStats struct {
	per  []out.EvalQueryResult
	cost out.ContributionCost
	rv   int
	cv   string
}

// RunContribution — прогон таблицы вклада на наборе; persist — записать (GET /api/evaluations/contribution).
func (s *Services) RunContribution(ctx context.Context, set string, persist bool, progress func(done, total int)) (*out.ContributionRun, error) {
	started := time.Now()
	set, err := contribSet(set)
	if err != nil {
		return nil, err
	}
	queries, err := s.evalQueries(ctx, set, nil)
	if err != nil {
		return nil, err
	}
	cat, err := s.evalCatalog(ctx)
	if err != nil {
		return nil, err
	}
	run := &out.ContributionRun{RunID: newEvalRunID(started), Set: set, SetHash: setHash(queries), Queries: len(queries),
		Commit: buildCommit(), StartedAt: started.UTC(), Rows: []out.ContributionRow{}, Checks: []out.ContributionCheck{},
		Decisions: []out.ContributionDecision{}, Notes: []string{}}
	run.ParseCostRub, run.ParseCostSource = s.contribParseCost(ctx)
	total, done := len(queries)*len(contributionSpecs), 0
	if progress != nil {
		progress(0, total)
	}
	per := map[string][]out.EvalQueryResult{}
	for _, sp := range contributionSpecs {
		t := time.Now()
		st, err := s.contribRow(ctx, run.RunID, sp, queries, cat, func() {
			done++
			if progress != nil {
				progress(done, total)
			}
		})
		if err != nil {
			return nil, err
		}
		row := out.ContributionRow{ID: sp.id, Kind: sp.kind, Label: sp.label, Config: sp.config, Against: sp.against,
			Off:     out.ContributionModes{LLM: sp.off.noLLM, Vectors: sp.off.noVectors, Rerank: sp.off.noRerank},
			Metrics: evalAggregate(st.per), Cost: st.cost, RegressionHead: []out.EvalChange{}, ImprovementHead: []out.EvalChange{},
			DurationMs: time.Since(t).Milliseconds()}
		if !sp.off.noLLM {
			cold := round4(row.Cost.RouteDShare * run.ParseCostRub * 1000)
			row.Cost.ColdLLMRubPer1k = &cold
		}
		if prev, ok := per[sp.against]; ok {
			regs, imps := evalCompare(prev, st.per, sp.against)
			row.Regressions, row.Improvements = len(regs), len(imps)
			row.RegressionHead, row.ImprovementHead = contribHeadOf(regs), contribHeadOf(imps)
		}
		per[sp.id] = st.per
		if st.rv > 0 {
			run.RulesVersion = st.rv
		}
		if st.cv != "" {
			run.CatalogVersion = st.cv
		}
		run.Rows = append(run.Rows, row)
	}
	run.Checks = contribChecks(run.Rows)
	run.Decisions = contribDecisions(run.Rows)
	if !strings.HasPrefix(run.CatalogVersion, "softline") {
		run.Notes = append(run.Notes, "Прогон на mock-каталоге с разметкой по построению: выводы по реранкеру и векторам предварительные.")
	}
	run.Notes = append(run.Notes,
		"Стоимость LLM — по трассировкам прогона (кэш разборов как на стенде в момент прогона); верхняя граница без кэша — доля маршрута D × стоимость разбора.",
		"Эмбеддер и реранкер — свой сервис моделей на своём сервере: в ₽ за запрос не входят, это часть сервера (COSTS.md).",
		"Время — локальная машина, поиски последовательно; не замер стенда под нагрузкой.",
		"Строка «без LLM» — строгая нижняя граница: кэш разборов не читается. Режим стенда LLM_MODE=off кэш читает.")
	run.DurationMs = time.Since(started).Milliseconds()
	run.Repro = fmt.Sprintf("набор %s (%d запр.) sha256:%s · правила v%d · каталог %s · коммит %s · make eval SET=contribution CONTRIB_SET=%s",
		set, run.Queries, run.SetHash, run.RulesVersion, catalogLabel(run.CatalogVersion), run.Commit, set)
	zap.L().Info("вклад компонентов: прогон завершён", zap.String("run_id", run.RunID), zap.String("set", set),
		zap.Int("queries", len(queries)), zap.Int64("ms", run.DurationMs))
	if persist && s.repos.Enabled() {
		raw, _ := json.Marshal(run)
		row := &domain.Evaluation{RunID: run.RunID, SetHash: run.SetHash, Queries: run.Queries, Commit: run.Commit,
			RulesVersion: run.RulesVersion, CatalogVersion: run.CatalogVersion, Metrics: raw, DurationMs: int(run.DurationMs)}
		if err := s.repos.SaveContribution(ctx, set, row); err != nil {
			return run, fmt.Errorf("вклад компонентов: результат не записан: %w", err)
		}
	}
	return run, nil
}

// contribRow — одна строка по всем запросам, последовательно (время не искажается параллельными поисками).
func (s *Services) contribRow(ctx context.Context, runID string, sp contributionSpec, queries []domain.EvalQuery,
	cat *evalCatalog, tick func()) (*contribRowStats, error) {
	st := &contribRowStats{per: make([]out.EvalQueryResult, 0, len(queries))}
	m := sp.off
	m.noModelCache = true
	sctx := withSearchModes(ctx, m)
	routeD := 0
	for _, q := range queries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		sr := in.SearchRequest{Query: q.Text, Config: sp.config, Limit: evalSearchLimit, SessionID: evalRequestPrefix + runID}
		if q.ClientID != nil {
			sr.ClientID = *q.ClientID
		}
		t := time.Now()
		resp, err := s.SearchWith(sctx, evalRequestID(runID, sp.id, q.ID), sr, SearchOptions{NoLog: true})
		o := evalOutcome{Err: err, Ms: float64(time.Since(t).Microseconds()) / 1000}
		if err == nil {
			o.Total, o.EmptyMarker = resp.Total, resp.Empty != nil
			o.Items = evalFamilyItems(resp.Items)
			if tr := resp.Trace; tr != nil {
				o.Route = tr.Route
				if tr.TotalMs > 0 {
					o.Ms = tr.TotalMs
				}
				st.rv, st.cv = tr.RulesVersion, tr.CatalogVersion
				st.cost.LLMRubPer1k += tr.CostRub
				switch tr.ParseSource {
				case domain.ParseLLM:
					st.cost.LLMCalls++
				case domain.ParseCache:
					st.cost.CacheHits++
				}
				if tr.Route == domain.RouteIntent {
					routeD++
				}
				dense, rerank := traceRan(tr, domain.StepDense), traceRan(tr, domain.StepRerank)
				if dense {
					st.cost.EmbedQueries++
				}
				if rerank {
					st.cost.RerankQueries++
				}
			}
		}
		st.per = append(st.per, evalScoreQuery(q, o, cat))
		tick()
	}
	if n := len(queries); n > 0 {
		st.cost.LLMRubPer1k = round4(st.cost.LLMRubPer1k * 1000 / float64(n))
		st.cost.RouteDShare = round4(float64(routeD) / float64(n))
	}
	return st, nil
}

// evalFamilyItems — первая позиция каждого семейства (как evalSearch: метрики по семействам).
func evalFamilyItems(items []out.SearchItem) []evalItem {
	var res []evalItem
	seen := map[string]bool{}
	for _, it := range items {
		if it.FamilyID != "" {
			if seen[it.FamilyID] {
				continue
			}
			seen[it.FamilyID] = true
		}
		res = append(res, evalItem{ProductID: it.ProductID, FamilyID: it.FamilyID, Group: it.Group})
	}
	return res
}

// traceRan — шаг name выполнился (есть в трассе и не пропущен).
func traceRan(tr *domain.Trace, name string) bool {
	raw, err := json.Marshal(tr.Steps)
	if err != nil {
		return false
	}
	var steps []domain.StepBase
	if json.Unmarshal(raw, &steps) != nil {
		return false
	}
	for _, st := range steps {
		if st.Name == name {
			return !st.Skipped
		}
	}
	return false
}

// contribParseCost — стоимость одного разбора моделью: замер «Экономики» (llm_usage живых поисков) или допущение.
func (s *Services) contribParseCost(ctx context.Context) (float64, string) {
	if s.repos.Enabled() {
		if c, err := s.Costs(ctx, CostsRequest{Limit: 1}); err == nil {
			for _, p := range c.Monthly.Params {
				if p.Key == "llm_parse_cost_rub" {
					return p.Value, p.Source
				}
			}
		}
	}
	return assumedParseCostRub, "assumed"
}

func contribRow(rows []out.ContributionRow, id string) *out.ContributionRow {
	for i := range rows {
		if rows[i].ID == id {
			return &rows[i]
		}
	}
	return nil
}

func pctText(v *float64) string {
	if v == nil {
		return "не посчитано"
	}
	return ruNum(fmt.Sprintf("%.1f %%", *v*100))
}

// ruNum — десятичная запятая в тексте для экрана.
func ruNum(s string) string { return strings.ReplaceAll(s, ".", ",") }

// contribChecks — блокирующие проверки: точные запросы, ошибки, работа без моделей; и информационные.
func contribChecks(rows []out.ContributionRow) []out.ContributionCheck {
	full, noRr, b2, off := contribRow(rows, domain.ConfigB3), contribRow(rows, contribB3NoRerank),
		contribRow(rows, domain.ConfigB2), contribRow(rows, contribB3Offline)
	if full == nil || noRr == nil || b2 == nil || off == nil {
		return []out.ContributionCheck{}
	}
	is1 := func(v *float64) bool { return v != nil && *v >= 0.9999 }
	exact := contribExact
	errsTotal := 0
	for _, r := range rows {
		errsTotal += r.Metrics.Errors
	}
	checks := []out.ContributionCheck{
		{ID: "exact_full", Label: "Точные запросы: верное семейство на 1-м месте в полном пути — 100 %", Blocking: true,
			Passed: is1(exact(full.Metrics)), Value: pctText(exact(full.Metrics))},
		{ID: "rerank_exact", Label: "Реранкер не сдвигает точные запросы", Blocking: true,
			Passed: exact(full.Metrics) != nil && exact(noRr.Metrics) != nil && *exact(full.Metrics) >= *exact(noRr.Metrics),
			Value:  pctText(exact(noRr.Metrics)) + " → " + pctText(exact(full.Metrics))},
		{ID: "no_errors", Label: "Ни одной ошибки поиска ни в одной строке", Blocking: true, Passed: errsTotal == 0,
			Value: fmt.Sprintf("ошибок %d", errsTotal)},
		{ID: "offline_answers", Label: "Без моделей поиск отвечает: ни одной ошибки", Blocking: true,
			Passed: off.Metrics.Errors == 0, Value: fmt.Sprintf("ошибок %d из %d", off.Metrics.Errors, off.Metrics.Queries+off.Metrics.Errors)},
		{ID: "offline_exact", Label: "Без моделей точные запросы: верное семейство на 1-м месте — 100 %",
			Passed: is1(exact(off.Metrics)), Value: pctText(exact(off.Metrics))},
		{ID: "false_rescue", Label: "Порог: ложное спасение полного пути не выше, чем у векторов без порога (B2)",
			Passed: full.Metrics.FalseRescueRate != nil && b2.Metrics.FalseRescueRate != nil && *full.Metrics.FalseRescueRate <= *b2.Metrics.FalseRescueRate,
			Value:  pctText(b2.Metrics.FalseRescueRate) + " → " + pctText(full.Metrics.FalseRescueRate)},
	}
	return checks
}

// contribExact, contribNDCG — метрики проверок и решений: по семействам; прогон до семейных метрик — по артикулу.
func contribExact(m out.EvalMetrics) *float64 {
	if m.ExactAccuracyFamily != nil {
		return m.ExactAccuracyFamily
	}
	return m.ExactAccuracy
}

func contribNDCG(m out.EvalMetrics) *float64 {
	if m.NDCG10Family != nil {
		return m.NDCG10Family
	}
	return m.NDCG10
}

// contribDecisions — предварительные решения по реранкеру, векторной ветке и LLM.
func contribDecisions(rows []out.ContributionRow) []out.ContributionDecision {
	full := contribRow(rows, domain.ConfigB3)
	if full == nil {
		return []out.ContributionDecision{}
	}
	var res []out.ContributionDecision
	for _, c := range []struct{ comp, label, without string }{
		{"rerank", "Реранкер", contribB3NoRerank},
		{"vectors", "Векторная ветка (kNN)", contribB3NoVec},
		{"llm", "LLM-разбор маршрута D", contribB3NoLLM},
	} {
		wo := contribRow(rows, c.without)
		if wo == nil {
			continue
		}
		d := out.ContributionDecision{Component: c.comp, Label: c.label, Preliminary: true,
			DeltaP95Ms: math.Round((full.Metrics.Latency.P95-wo.Metrics.Latency.P95)*10) / 10}
		// Улучшения и ухудшения от компонента: строка без него против полного пути — зеркально.
		if c.without == contribB3NoRerank {
			d.Improvements, d.Regressions = full.Improvements, full.Regressions
		} else {
			d.Improvements, d.Regressions = wo.Regressions, wo.Improvements
		}
		if f, w := contribNDCG(full.Metrics), contribNDCG(wo.Metrics); f != nil && w != nil {
			v := round4(*f - *w)
			d.DeltaNDCG = &v
		}
		switch {
		case d.DeltaNDCG == nil:
			d.Verdict, d.Text = "borderline", "NDCG не посчитан — решения нет"
		case *d.DeltaNDCG >= contribKeepDelta && d.Improvements > d.Regressions:
			d.Verdict = "keep"
			d.Text = ruNum(fmt.Sprintf("Оставить: NDCG@10 %+.3f, улучшено запросов %d, ухудшено %d, P95 %+.1f мс", *d.DeltaNDCG, d.Improvements, d.Regressions, d.DeltaP95Ms))
		case *d.DeltaNDCG <= contribRemoveDelta:
			d.Verdict = "remove"
			d.Text = ruNum(fmt.Sprintf("Прироста нет: NDCG@10 %+.3f (порог шума %.3f), улучшено %d, ухудшено %d, P95 %+.1f мс — кандидат на удаление",
				*d.DeltaNDCG, contribRemoveDelta, d.Improvements, d.Regressions, d.DeltaP95Ms))
		default:
			d.Verdict = "borderline"
			d.Text = ruNum(fmt.Sprintf("В пределах шума одного-двух запросов: NDCG@10 %+.3f, улучшено %d, ухудшено %d, P95 %+.1f мс — решать на данных задачи",
				*d.DeltaNDCG, d.Improvements, d.Regressions, d.DeltaP95Ms))
		}
		res = append(res, d)
	}
	return res
}

// Contribution — GET /api/evaluations/contribution?set=test: последний записанный прогон и состояние фонового.
func (s *Services) Contribution(ctx context.Context, set string) (*out.ContributionResponse, error) {
	set, err := contribSet(set)
	if err != nil {
		return nil, err
	}
	resp := &out.ContributionResponse{Set: set, Notes: []string{},
		Modes: out.ModelsState{LLM: s.LLMState(ctx), Vectors: s.VectorsState()}}
	resp.Running, resp.LastJob = s.contribJobs().snapshot()
	if !s.repos.Enabled() {
		resp.Notes = append(resp.Notes, "Сервис запущен без базы: записанных прогонов нет")
		return resp, nil
	}
	row, err := s.repos.LatestContribution(ctx, set)
	if err != nil {
		return nil, err
	}
	if row == nil {
		resp.Notes = append(resp.Notes, MsgContribNoRun)
		return resp, nil
	}
	var run out.ContributionRun
	if err := json.Unmarshal(row.Metrics, &run); err != nil {
		return nil, fmt.Errorf("вклад компонентов %s: %w", row.RunID, err)
	}
	resp.Run = &run
	return resp, nil
}

// Фоновый прогон (POST /api/evaluations/contribution/run): один за раз на процесс, свой, не общий с B0–B4.

var contribJobsBy sync.Map

func (s *Services) contribJobs() *evalJobState {
	v, _ := contribJobsBy.LoadOrStore(s, &evalJobState{})
	return v.(*evalJobState)
}

// StartContribution — прогон таблицы вклада в фоне с записью; второй, пока идёт первый, — 409.
func (s *Services) StartContribution(set string) (*out.EvalJob, error) {
	set, err := contribSet(set)
	if err != nil {
		return nil, err
	}
	if _, ok := s.source.(queryLoader); !ok {
		return nil, errs.NewUnavailableError(MsgEvalNoQueries)
	}
	jobs := s.contribJobs()
	jobs.mu.Lock()
	if jobs.running != nil {
		jobs.mu.Unlock()
		return nil, errs.NewConflictError(MsgContribRunning)
	}
	ids := make([]string, len(contributionSpecs))
	for i, sp := range contributionSpecs {
		ids[i] = sp.id
	}
	job := &out.EvalJob{ID: newEvalRunID(s.now()), Set: set, Configs: ids, Status: "running", StartedAt: s.now().UTC()}
	jobs.running = job
	c := *job
	jobs.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), evalJobTimeout)
		defer cancel()
		finish := func(status, runID, msg string) {
			jobs.mu.Lock()
			defer jobs.mu.Unlock()
			done := s.now().UTC()
			job.FinishedAt, job.Status, job.RunID, job.Error = &done, status, runID, msg
			jobs.running, jobs.last = nil, job
		}
		defer func() {
			if r := recover(); r != nil {
				zap.L().Error("вклад компонентов: паника в фоновом прогоне", zap.Any("panic", r), zap.ByteString("stack", debug.Stack()))
				finish("failed", "", "Прогон упал — причина в журнале API")
			}
		}()
		run, err := s.RunContribution(ctx, set, true, func(done, total int) {
			jobs.mu.Lock()
			job.Done, job.Total = done, total
			jobs.mu.Unlock()
		})
		if err != nil {
			zap.L().Warn("вклад компонентов: фоновый прогон не удался", zap.Error(err))
			finish("failed", "", publicEvalError(err))
			return
		}
		finish("done", run.RunID, "")
	}()
	return &c, nil
}

// FormatContributionTable — таблица для stdout CLI.
func FormatContributionTable(r *out.ContributionRun) string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Вклад компонентов %s · %s\n\n", r.RunID, r.Repro)
	b.WriteString("| Строка | NDCG@10 (сем.) | Пустые | Нарушения условий | Точные (сем.) | Ложное спасение | P50 / P95, мс | ₽ LLM / 1000 (без кэша) | Улучш. / ухудш. |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, row := range r.Rows {
		m := row.Metrics
		ndcg := "—"
		if v := contribNDCG(m); v != nil {
			ndcg = fmt.Sprintf("%.3f", *v)
		}
		cold := "LLM выкл."
		if row.Cost.ColdLLMRubPer1k != nil {
			cold = fmt.Sprintf("%.2f", *row.Cost.ColdLLMRubPer1k)
		}
		vs := "—"
		if row.Against != "" {
			vs = fmt.Sprintf("+%d / −%d к %s", row.Improvements, row.Regressions, row.Against)
		}
		fmt.Fprintf(&b, "| %s %s | %s | %s | %s (%d поз.) | %s | %s | %.1f / %.1f | %.2f (%s) | %s |\n", row.ID, row.Label, ndcg,
			pctText(m.EmptyRate), pctText(m.ViolationRate), m.ViolationItems, pctText(contribExact(m)), pctText(m.FalseRescueRate),
			m.Latency.P50, m.Latency.P95, row.Cost.LLMRubPer1k, cold, vs)
	}
	b.WriteString("\nПроверки:\n")
	for _, c := range r.Checks {
		mark := "ок"
		if !c.Passed {
			mark = "НЕ ПРОЙДЕНА"
		}
		kind := ""
		if c.Blocking {
			kind = " [блокирующая]"
		}
		fmt.Fprintf(&b, "- %s%s: %s — %s\n", c.Label, kind, c.Value, mark)
	}
	b.WriteString("\nПредварительные решения (mock):\n")
	for _, d := range r.Decisions {
		fmt.Fprintf(&b, "- %s: %s\n", d.Label, d.Text)
	}
	fmt.Fprintf(&b, "\nСтоимость разбора для верхней границы: %.4f ₽ (%s)\n", r.ParseCostRub, r.ParseCostSource)
	return b.String()
}

// contribHeadOf — первые contribHead изменений.
func contribHeadOf(list []out.EvalChange) []out.EvalChange {
	if len(list) > contribHead {
		return list[:contribHead]
	}
	return list
}
