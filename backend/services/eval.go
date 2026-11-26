package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"

	"go.uber.org/zap"
)

// Оценка качества: прогон набора запросов через тот же
// сервис поиска, что отвечает покупателю (не через HTTP), метрики по разметке, ухудшения относительно
// предыдущей конфигурации и строка воспроизводимости. Тот же движок — для песочницы агента:
// RulesOverride подменяет действующие правила снимком.

// RulesSnapshot — снимок словарей и правил.
type RulesSnapshot = domain.RulesSnapshot

// EvalRequest — что прогонять. Пустой Configs — B0–B3; пустой QueryIDs — весь набор.
type EvalRequest struct {
	Set           string
	Configs       []string
	QueryIDs      []string
	RulesOverride *RulesSnapshot
	// Persist — записать результат в evaluations.
	Persist bool
	// Progress — вызывается после каждого запроса: сделано и всего (запросы × конфигурации); nil — не нужно.
	Progress func(done, total int)
	// Queries — готовый набор вместо queries.json (сборка наборов из журнала, evalsets.go); Set тогда — только
	// подпись прогона. В evaluations такой прогон не пишется.
	Queries []domain.EvalQuery
}

// EvalResult — результат прогона.
type EvalResult = out.EvalRun

// DefaultEvalConfigs — лестница по умолчанию для `make eval`.
var DefaultEvalConfigs = []string{domain.ConfigB0, domain.ConfigB1, domain.ConfigB2, domain.ConfigB3}

// evalSearchLimit — позиций в ответе на запрос прогона: нужно для Recall@50.
const evalSearchLimit = 50

// evalRequestPrefix — начало request_id поисков прогона (журнал они не пишут; аналитика их отсекает и
// на случай, если запишут).
const evalRequestPrefix = "eval-"

// Тексты отказов оценки.
const (
	MsgEvalNoQueries = "Наборы запросов с разметкой не найдены (queries.json)"
	MsgEvalRunning   = "Прогон уже идёт — дождитесь его окончания"
	MsgEvalEmptySet  = "В наборе нет запросов"
	MsgEvalSetField  = "Набор — tune, test, regression, demo или holdout_week"
	MsgEvalNoHoldout = "Отложенная неделя пуста: в журнале нет исторических поисков за это окно (make import)"
)

// errNoHoldoutLog — реплей отложенной недели не на чем строить.
var errNoHoldoutLog = errs.NewValidationError(MsgEvalNoHoldout)

// validEvalSet — набор из queries.json или реплей отложенной недели.
func validEvalSet(set string) bool {
	return slices.Contains(domain.EvalSets, set) || set == domain.EvalSetHoldoutWeek
}

// queryLoader — источник, который умеет отдать наборы запросов (MockJSON).
type queryLoader interface {
	LoadQueries(ctx context.Context) ([]domain.EvalQuery, error)
}

// RunEvaluation — прогон набора по конфигурациям. Ошибка одного запроса не роняет прогон: запрос
// помечается и в метрики не входит (число ошибок — в метриках).
func (s *Services) RunEvaluation(ctx context.Context, req EvalRequest) (*EvalResult, error) {
	started := time.Now()
	set, configs, queries, err := s.evalInput(ctx, req)
	if err != nil {
		return nil, err
	}
	cat, err := s.evalCatalog(ctx)
	if err != nil {
		return nil, err
	}
	run := &EvalResult{
		RunID: newEvalRunID(started), Set: set, SetHash: setHash(queries), Queries: len(queries),
		Commit: buildCommit(), StartedAt: started.UTC(), Configs: []out.EvalConfigResult{},
	}
	var prev []out.EvalQueryResult
	total, done := len(queries)*len(configs), 0
	tick := func() {
		done++
		if req.Progress != nil {
			req.Progress(done, total)
		}
	}
	if req.Progress != nil {
		req.Progress(0, total)
	}
	for i, cfg := range configs {
		res, err := s.evalConfig(ctx, run.RunID, cfg, queries, cat, req.RulesOverride, tick)
		if err != nil {
			return nil, err
		}
		res.Regressions, res.Improvements = []out.EvalChange{}, []out.EvalChange{}
		if i > 0 {
			res.Against = configs[i-1]
			res.Regressions, res.Improvements = evalCompare(prev, res.PerQuery, res.Against)
			rescued, lost := evalRescuedSearches(prev, res.PerQuery)
			res.Metrics.RescuedSearches, res.Metrics.LostSearches = &rescued, &lost
		}
		prev = res.PerQuery
		run.Configs = append(run.Configs, *res)
	}
	run.DurationMs = time.Since(started).Milliseconds()
	run.Repro = reproLine(run.Set, run.SetHash, run.Queries, run.Configs, run.Commit)
	zap.L().Info("оценка: прогон завершён", zap.String("run_id", run.RunID), zap.String("set", set),
		zap.Strings("configs", configs), zap.Int("queries", len(queries)), zap.Int64("ms", run.DurationMs))
	if req.Persist && req.Queries == nil && s.repos.Enabled() {
		if err := s.repos.SaveEvaluations(ctx, evaluationRows(run)); err != nil {
			return run, fmt.Errorf("оценка: результат не записан: %w", err)
		}
	}
	return run, nil
}

// evalInput — набор, конфигурации и запросы прогона: из queries.json или готовые (req.Queries).
func (s *Services) evalInput(ctx context.Context, req EvalRequest) (string, []string, []domain.EvalQuery, error) {
	if req.Queries == nil {
		set, configs, err := validateEval(req)
		if err != nil {
			return "", nil, nil, err
		}
		queries, err := s.evalQueries(ctx, set, req.QueryIDs)
		return set, configs, queries, err
	}
	r := req
	r.Set = domain.EvalSetTest
	_, configs, err := validateEval(r)
	if err != nil {
		return "", nil, nil, err
	}
	if len(req.Queries) == 0 {
		return "", nil, nil, errs.NewValidationError(MsgEvalEmptySet)
	}
	queries := slices.Clone(req.Queries)
	sort.Slice(queries, func(i, j int) bool { return queries[i].ID < queries[j].ID })
	set := strings.TrimSpace(req.Set)
	if set == "" {
		set = "custom"
	}
	return set, configs, queries, nil
}

// validateEval — набор и конфигурации со значениями по умолчанию.
func validateEval(req EvalRequest) (string, []string, error) {
	var bad []errs.FieldError
	set := strings.ToLower(strings.TrimSpace(req.Set))
	if set == "" {
		set = domain.EvalSetTest
	}
	if !validEvalSet(set) {
		bad = append(bad, errs.FieldError{Field: "set", Code: errs.CodeInvalidFormat, Message: MsgEvalSetField})
	}
	configs := []string{}
	for _, c := range req.Configs {
		c = strings.ToUpper(strings.TrimSpace(c))
		if c == "" || slices.Contains(configs, c) {
			continue
		}
		if !slices.Contains(domain.SearchConfigs, c) {
			bad = append(bad, errs.FieldError{Field: "configs", Code: errs.CodeInvalidFormat, Message: "Конфигурация — одна из B0–B4"})
			continue
		}
		configs = append(configs, c)
	}
	if len(configs) == 0 {
		configs = slices.Clone(DefaultEvalConfigs)
	}
	if len(bad) > 0 {
		return "", nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(bad...)
	}
	return set, configs, nil
}

// evalQueries — запросы набора (по id, если заданы), по порядку id.
func (s *Services) evalQueries(ctx context.Context, set string, ids []string) ([]domain.EvalQuery, error) {
	if set == domain.EvalSetHoldoutWeek {
		return s.holdoutQueries(ctx)
	}
	loader, ok := s.source.(queryLoader)
	if !ok {
		return nil, errs.NewUnavailableError(MsgEvalNoQueries)
	}
	all, err := loader.LoadQueries(ctx)
	if err != nil {
		return nil, errs.NewUnavailableError(MsgEvalNoQueries).WithCause(err)
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var qs []domain.EvalQuery
	for _, q := range all {
		if q.Set == set && (len(want) == 0 || want[q.ID]) {
			qs = append(qs, q)
		}
	}
	if len(qs) == 0 {
		return nil, errs.NewValidationError(MsgEvalEmptySet)
	}
	sort.Slice(qs, func(i, j int) bool { return qs[i].ID < qs[j].ID })
	return qs, nil
}

// evalCatalog — позиции каталога: из базы (то, что проиндексировано), иначе из источника.
func (s *Services) evalCatalog(ctx context.Context) (*evalCatalog, error) {
	if s.repos.Enabled() {
		snap, err := s.repos.LoadCatalog(ctx)
		if err == nil && snap != nil {
			return newEvalCatalog(snap.Products), nil
		}
	}
	if s.source == nil {
		return nil, errs.NewUnavailableError(MsgNoSource)
	}
	snap, err := s.source.Load(ctx)
	if err != nil {
		return nil, errs.NewUnavailableError(MsgSourceBroken).WithCause(err)
	}
	return newEvalCatalog(snap.Products), nil
}

// evalConfig — одна конфигурация по всем запросам, последовательно: время ответа не искажается
// параллельными запросами.
func (s *Services) evalConfig(ctx context.Context, runID, cfg string, queries []domain.EvalQuery, cat *evalCatalog,
	override *RulesSnapshot, tick func()) (*out.EvalConfigResult, error) {
	started := time.Now()
	res := &out.EvalConfigResult{Config: cfg, RulesOverride: override != nil, PerQuery: make([]out.EvalQueryResult, 0, len(queries))}
	rulesSeen := map[int]bool{}
	for _, q := range queries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		o := s.evalSearch(ctx, runID, cfg, q, override)
		if o.rulesVersion > 0 {
			rulesSeen[o.rulesVersion] = true
			res.RulesVersion = o.rulesVersion
		}
		if o.catalogVersion != "" {
			res.CatalogVersion = o.catalogVersion
		}
		res.PerQuery = append(res.PerQuery, evalScoreQuery(q, o.evalOutcome, cat))
		tick()
	}
	res.Metrics = evalAggregate(res.PerQuery)
	res.ByType = evalByType(res.PerQuery)
	res.DurationMs = time.Since(started).Milliseconds()
	if len(rulesSeen) > 1 {
		res.Warnings = append(res.Warnings, "версия правил менялась во время прогона")
	}
	if override != nil && res.RulesVersion != override.Version {
		res.Warnings = append(res.Warnings, fmt.Sprintf("поиск не применил снимок правил v%d: в трассе v%d", override.Version, res.RulesVersion))
	}
	if res.Metrics.Errors > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("ошибок поиска: %d — эти запросы в метрики не вошли", res.Metrics.Errors))
	}
	return res, nil
}

// evalSearched — выдача и версии из трассы.
type evalSearched struct {
	evalOutcome
	rulesVersion   int
	catalogVersion string
}

// evalSearch — один запрос через сервис поиска: журнал не пишется, снимок правил — из песочницы.
func (s *Services) evalSearch(ctx context.Context, runID, cfg string, q domain.EvalQuery, override *RulesSnapshot) evalSearched {
	sr := in.SearchRequest{Query: q.Text, Config: cfg, Limit: evalSearchLimit, SessionID: evalRequestPrefix + runID}
	if q.ClientID != nil {
		sr.ClientID = *q.ClientID
	}
	t := time.Now()
	resp, err := s.SearchWith(ctx, evalRequestID(runID, cfg, q.ID), sr, SearchOptions{Rules: override, NoLog: true})
	ms := float64(time.Since(t).Microseconds()) / 1000
	if err != nil {
		return evalSearched{evalOutcome: evalOutcome{Err: err, Ms: ms}}
	}
	o := evalSearched{evalOutcome: evalOutcome{Total: resp.Total, EmptyMarker: resp.Empty != nil, Ms: ms}}
	if resp.Trace != nil {
		o.Route = resp.Trace.Route
		o.rulesVersion = resp.Trace.RulesVersion
		o.catalogVersion = resp.Trace.CatalogVersion
		// Время — по трассе сервиса (как у покупателя), без сборки ответа прогоном.
		if resp.Trace.TotalMs > 0 {
			o.Ms = resp.Trace.TotalMs
		}
	}
	for _, it := range resp.Items {
		o.Items = append(o.Items, evalItem{ProductID: it.ProductID, FamilyID: it.FamilyID, Group: it.Group})
	}
	return o
}

// evalRequestID — request_id поиска прогона: ≤32 символов (размер колонки search_events).
func evalRequestID(runID, cfg, queryID string) string {
	id := evalRequestPrefix + runID[len(runID)-6:] + "-" + cfg + "-" + queryID
	if len(id) > 32 {
		id = id[:32]
	}
	return id
}

// newEvalRunID — идентификатор прогона: время и случайный хвост.
func newEvalRunID(t time.Time) string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return t.UTC().Format("20060102T150405") + "-" + hex.EncodeToString(b)
}

// setHash — хэш набора: что прогоняли и с какой разметкой. Порядок ключей JSON стабилен.
func setHash(qs []domain.EvalQuery) string {
	type row struct {
		ID        string         `json:"id"`
		Text      string         `json:"text"`
		Type      string         `json:"type"`
		Freq      int            `json:"freq"`
		ClientID  *string        `json:"client_id"`
		Judgments map[string]int `json:"judgments"`
		Absent    bool           `json:"absent"`
	}
	rows := make([]row, len(qs))
	for i, q := range qs {
		rows[i] = row{q.ID, q.Text, q.Type, q.Freq, q.ClientID, q.Judgments, q.Absent}
	}
	raw, _ := json.Marshal(rows)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])[:12]
}

var (
	commitOnce sync.Once
	commitVal  string
)

// buildCommit — коммит кода: APP_COMMIT (образ), иначе git; незакоммиченные правки — «+dirty».
func buildCommit() string {
	commitOnce.Do(func() {
		if v := strings.TrimSpace(os.Getenv("APP_COMMIT")); v != "" {
			commitVal = v
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		head, err := exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD").Output()
		if err != nil {
			commitVal = "unknown"
			return
		}
		commitVal = strings.TrimSpace(string(head))
		if st, err := exec.CommandContext(ctx, "git", "status", "--porcelain", "--untracked-files=no").Output(); err == nil &&
			len(strings.TrimSpace(string(st))) > 0 {
			commitVal += "+dirty"
		}
	})
	return commitVal
}

// reproLine — строка воспроизводимости прогона.
func reproLine(set, hash string, n int, cfgs []out.EvalConfigResult, commit string) string {
	rules, catalog := 0, ""
	override := false
	for _, c := range cfgs {
		rules = max(rules, c.RulesVersion)
		override = override || c.RulesOverride
		if c.CatalogVersion != "" {
			catalog = c.CatalogVersion
		}
	}
	r := fmt.Sprintf("набор %s (%d запр.) sha256:%s · правила v%d", set, n, hash, rules)
	if override {
		r += " (снимок песочницы)"
	}
	return r + fmt.Sprintf(" · каталог %s · коммит %s", catalog, commit)
}

// evaluationRows — строки evaluations из прогона.
func evaluationRows(run *EvalResult) []domain.Evaluation {
	rows := make([]domain.Evaluation, 0, len(run.Configs))
	for _, c := range run.Configs {
		row := domain.Evaluation{
			RunID: run.RunID, Set: run.Set, SetHash: run.SetHash, Queries: run.Queries, Config: c.Config,
			RulesVersion: c.RulesVersion, RulesOverride: c.RulesOverride, CatalogVersion: c.CatalogVersion,
			Commit: run.Commit, Against: c.Against, DurationMs: int(c.DurationMs),
		}
		row.Metrics, _ = domain.ToJSONB(c.Metrics)
		row.ByType, _ = domain.ToJSONB(c.ByType)
		row.Regressions, _ = domain.ToJSONB(c.Regressions)
		row.Improvements, _ = domain.ToJSONB(c.Improvements)
		row.PerQuery, _ = domain.ToJSONB(c.PerQuery)
		rows = append(rows, row)
	}
	return rows
}

// Evaluations — GET /api/evaluations: последний прогон каждой конфигурации на наборе и состояние
// асинхронного прогона. Без базы — пустой список.
func (s *Services) Evaluations(ctx context.Context, set string) (*out.EvaluationsResponse, error) {
	set = strings.ToLower(strings.TrimSpace(set))
	if set == "" {
		set = domain.EvalSetTest
	}
	if !validEvalSet(set) {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(
			errs.FieldError{Field: "set", Code: errs.CodeInvalidFormat, Message: MsgEvalSetField})
	}
	resp := &out.EvaluationsResponse{Set: set, Configs: []out.EvaluationView{}}
	resp.Running, resp.LastJob = s.evalJobs().snapshot()
	if !s.repos.Enabled() {
		return resp, nil
	}
	rows, err := s.repos.LatestEvaluations(ctx, set)
	if err != nil {
		return nil, err
	}
	if resp.JudgeAgreement, err = s.latestJudge(ctx, set); err != nil {
		return nil, err
	}
	for _, r := range rows {
		v := out.EvaluationView{
			RunID: r.RunID, Set: r.Set, SetHash: r.SetHash, Queries: r.Queries, Config: r.Config,
			RulesVersion: r.RulesVersion, RulesOverride: r.RulesOverride, CatalogVersion: r.CatalogVersion,
			Commit: r.Commit, CreatedAt: r.CreatedAt, DurationMs: r.DurationMs, Against: r.Against,
			ByType: map[string]out.EvalMetrics{}, Regressions: []out.EvalChange{}, Improvements: []out.EvalChange{},
		}
		for _, f := range []struct {
			raw domain.JSONB
			dst any
		}{{r.Metrics, &v.Metrics}, {r.ByType, &v.ByType}, {r.Regressions, &v.Regressions}, {r.Improvements, &v.Improvements}} {
			if len(f.raw) > 0 {
				if err := json.Unmarshal(f.raw, f.dst); err != nil {
					return nil, fmt.Errorf("evaluations %s: %w", r.RunID, err)
				}
			}
		}
		v.Repro = reproLine(r.Set, r.SetHash, r.Queries, []out.EvalConfigResult{{RulesVersion: r.RulesVersion,
			RulesOverride: r.RulesOverride, CatalogVersion: r.CatalogVersion}}, r.Commit)
		resp.Configs = append(resp.Configs, v)
	}
	sort.SliceStable(resp.Configs, func(i, j int) bool {
		return slices.Index(domain.SearchConfigs, resp.Configs[i].Config) < slices.Index(domain.SearchConfigs, resp.Configs[j].Config)
	})
	return resp, nil
}

// Асинхронные прогоны (POST /api/evaluations/run): один за раз на процесс.

const evalJobTimeout = 20 * time.Minute

type evalJobState struct {
	mu      sync.Mutex
	running *out.EvalJob
	last    *out.EvalJob
}

// evalJobsBy — состояние прогонов на каждый граф сервисов (в процессе он один; в тестах — свои).
var evalJobsBy sync.Map

func (s *Services) evalJobs() *evalJobState {
	v, _ := evalJobsBy.LoadOrStore(s, &evalJobState{})
	return v.(*evalJobState)
}

func (j *evalJobState) snapshot() (running, last *out.EvalJob) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.running != nil {
		c := *j.running
		running = &c
	}
	if j.last != nil {
		c := *j.last
		last = &c
	}
	return running, last
}

// StartEvaluation — прогон в фоне с записью в evaluations; ответ — задание. Второй прогон, пока идёт
// первый, — 409.
func (s *Services) StartEvaluation(set string, configs []string) (*out.EvalJob, error) {
	set, configs, err := validateEval(EvalRequest{Set: set, Configs: configs})
	if err != nil {
		return nil, err
	}
	if _, ok := s.source.(queryLoader); !ok {
		return nil, errs.NewUnavailableError(MsgEvalNoQueries)
	}
	jobs := s.evalJobs()
	jobs.mu.Lock()
	if jobs.running != nil {
		jobs.mu.Unlock()
		return nil, errs.NewConflictError(MsgEvalRunning)
	}
	job := &out.EvalJob{ID: newEvalRunID(s.now()), Set: set, Configs: configs, Status: "running", StartedAt: s.now().UTC()}
	jobs.running = job
	c := *job
	jobs.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), evalJobTimeout)
		defer cancel()
		// Паника прогона не роняет API и не оставляет задание в running.
		defer func() {
			if r := recover(); r != nil {
				zap.L().Error("оценка: паника в фоновом прогоне", zap.String("job", job.ID), zap.Any("panic", r),
					zap.ByteString("stack", debug.Stack()))
				jobs.mu.Lock()
				defer jobs.mu.Unlock()
				done := s.now().UTC()
				job.FinishedAt = &done
				job.Status, job.Error = "failed", "Прогон упал — причина в журнале API"
				jobs.running, jobs.last = nil, job
			}
		}()
		progress := func(done, total int) {
			jobs.mu.Lock()
			job.Done, job.Total = done, total
			jobs.mu.Unlock()
		}
		run, err := s.RunEvaluation(ctx, EvalRequest{Set: set, Configs: configs, Persist: true, Progress: progress})
		jobs.mu.Lock()
		defer jobs.mu.Unlock()
		done := s.now().UTC()
		job.FinishedAt = &done
		switch {
		case err != nil:
			job.Status, job.Error = "failed", publicEvalError(err)
			zap.L().Warn("оценка: фоновый прогон не удался", zap.String("job", job.ID), zap.Error(err))
		default:
			job.Status, job.RunID = "done", run.RunID
		}
		jobs.running, jobs.last = nil, job
	}()
	return &c, nil
}

// publicEvalError — текст ошибки для экрана: свои отказы — как есть, прочее — общий текст.
func publicEvalError(err error) string {
	var e *errs.Error
	if errors.As(err, &e) {
		return e.Message
	}
	return "Прогон не удался — причина в журнале API"
}

// RunProposalSandbox — эффект ждущей правки в песочнице, без принятия и без записи в evaluations:
// набор прогоняется в B3, B4 (действующие правила) и «B4+правка» (снимок действующих правил с правкой
// предложения). Третий столбец сравнивается с B4: спасённые и потерянные поиски — эффект именно правки.
func (s *Services) RunProposalSandbox(ctx context.Context, set, proposalID string) (*EvalResult, error) {
	p, err := s.loadProposal(ctx, proposalID)
	if err != nil {
		return nil, err
	}
	if p.Action != domain.ActionRulesPatch {
		return nil, errs.NewValidationError("Это задача человеку, а не правка правил: прогонять нечего")
	}
	var patch RulePatch
	if err := json.Unmarshal(p.Patch, &patch); err != nil {
		return nil, errs.NewInternalError("Правка предложения не разбирается").WithCause(err)
	}
	cur, err := s.activeRules(ctx)
	if err != nil {
		return nil, err
	}
	run, err := s.RunEvaluation(ctx, EvalRequest{Set: set, Configs: []string{domain.ConfigB3, domain.ConfigB4}})
	if err != nil {
		return nil, err
	}
	cand := cur.applyPatch(patch)
	cand.Version = cur.Version
	sb, err := s.RunEvaluation(ctx, EvalRequest{Set: set, Configs: []string{domain.ConfigB4}, RulesOverride: cand.snapshot()})
	if err != nil {
		return nil, err
	}
	c := sb.Configs[0]
	c.Config = "B4+п"
	prev := run.Configs[len(run.Configs)-1]
	c.Against = prev.Config
	c.Regressions, c.Improvements = evalCompare(prev.PerQuery, c.PerQuery, c.Against)
	rescued, lost := evalRescuedSearches(prev.PerQuery, c.PerQuery)
	c.Metrics.RescuedSearches, c.Metrics.LostSearches = &rescued, &lost
	c.Warnings = append(c.Warnings, fmt.Sprintf("песочница: правка предложения %s (%s) поверх правил v%d, не принята", p.ID.String()[:8], patchLine(patch), cur.Version))
	run.Configs = append(run.Configs, c)
	run.Repro = reproLine(run.Set, run.SetHash, run.Queries, run.Configs, run.Commit) + " · правка " + p.ID.String()
	return run, nil
}
