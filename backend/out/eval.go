package out

import "time"

// Оценка качества: прогоны конфигураций на наборах запросов (GET /api/evaluations, `go run . eval`).
// Доли — 0..1; nil — метрику не на чем считать (в наборе нет запросов нужного типа).

// EvalMetrics — метрики конфигурации на наборе или на одном типе запросов.
type EvalMetrics struct {
	Queries int `json:"queries"`
	// EmptyRate — доля пустых выдач; EmptyRateWeighted — она же, взвешенная по freq.
	EmptyRate         *float64 `json:"empty_rate"`
	EmptyRateWeighted *float64 `json:"empty_rate_weighted"`
	// NoRelevantTop10 — выдача есть, а релевантного (оценка ≥2) в топ-10 нет; среди запросов, у которых
	// релевантное в разметке есть.
	NoRelevantTop10 *float64 `json:"no_relevant_top10"`
	// RelevantTop10 — есть релевантное (≥2) в топ-10; среди запросов, у которых оно есть в разметке.
	RelevantTop10 *float64 `json:"relevant_top10"`
	NDCG10        *float64 `json:"ndcg10"`
	MRR10         *float64 `json:"mrr10"`
	// Recall50 — доля релевантных (≥2) семейств разметки, попавших в топ-50.
	Recall50 *float64 `json:"recall50"`
	// ExactAccuracy — exact/sku: на 1-м месте позиция с оценкой 3 (по артикулу: оценка самой показанной позиции).
	ExactAccuracy *float64 `json:"exact_accuracy"`
	// NDCG10Family — NDCG@10 по семействам: оценка семейства — максимум по его артикулам, в выдаче — первое вхождение
	// семейства; при разметке с явными нулями неразмеченные семейства пропускаются (не считаются нерелевантными).
	// NDCG10 — прежний счёт: оценка показанного артикула, неразмеченное — 0.
	NDCG10Family *float64 `json:"ndcg10_family"`
	// ExactAccuracyFamily — exact/sku: у семейства первой позиции оценка 3 («верное семейство первым»); запросы, для
	// которых в каталоге нет ни одного товара (ExpectEmpty), в знаменатель не входят. ExactQueries — знаменатель.
	ExactAccuracyFamily *float64 `json:"exact_accuracy_family"`
	ExactQueries        int      `json:"exact_queries,omitempty"`
	// ExpectEmptyOK — запросы «ожидаем пусто или альтернативу» (absent и exact/sku без единой оценки > 0 в разметке):
	// доля с пустой выдачей, ответом «нет в каталоге» или пометкой аналога. ExpectEmptyQueries — сколько их.
	ExpectEmptyOK      *float64 `json:"expect_empty_ok"`
	ExpectEmptyQueries int      `json:"expect_empty_queries,omitempty"`
	// RescuedRate — проблемные типы (typo/layout/slang/alias_gap/license/analog): релевантное в топ-10.
	RescuedRate *float64 `json:"rescued_rate"`
	// FalseRescueRate — absent/noise: выдача непустая, без «нет в каталоге» и без пометки аналога.
	FalseRescueRate *float64 `json:"false_rescue_rate"`
	// ViolationRate — license/renewal: в топ-10 есть позиция нужного продукта с не той метрикой,
	// количеством или типом лицензии; ViolationItems — сколько таких позиций всего.
	ViolationRate  *float64 `json:"violation_rate"`
	ViolationItems int      `json:"violation_items"`
	// Errors — запросы, на которых поиск вернул ошибку (в метрики не входят).
	Errors int `json:"errors"`
	// Searches — сумма freq запросов (поисков); JudgedSearches — из них с релевантным в разметке;
	// RelevantTop10Weighted — доля таких поисков с релевантным в топ-10 (реплей отложенной недели).
	Searches              int      `json:"searches,omitempty"`
	JudgedSearches        int      `json:"judged_searches,omitempty"`
	RelevantTop10Weighted *float64 `json:"relevant_top10_weighted,omitempty"`
	// RescuedSearches / LostSearches — поиски (с весом freq), где у предыдущей конфигурации прогона
	// релевантного в топ-10 не было, а у этой есть (и наоборот); только размеченные. nil у первой.
	RescuedSearches *int                   `json:"rescued_searches,omitempty"`
	LostSearches    *int                   `json:"lost_searches,omitempty"`
	Latency         EvalLatency            `json:"latency"`
	ByRoute         map[string]EvalLatency `json:"by_route,omitempty"`
	// Target — метрики набора «по цели» на фиде (eval_target.go); у наборов с оценками 0–3 — nil.
	Target *TargetMetrics `json:"target,omitempty"`
	// Guide — оценка режима подбора (guide_eval.go); у прочих наборов — nil.
	Guide *GuideEvalMetrics `json:"guide,omitempty"`
}

// EvalLatency — время ответа сервиса поиска, мс.
type EvalLatency struct {
	Count int     `json:"count"`
	P50   float64 `json:"p50"`
	P95   float64 `json:"p95"`
}

// EvalQueryResult — один запрос в одной конфигурации.
type EvalQueryResult struct {
	ID    string `json:"id"`
	Text  string `json:"text"`
	Type  string `json:"type"`
	Freq  int    `json:"freq"`
	Route string `json:"route,omitempty"`
	// Top — product_id первых 10 позиций выдачи, Grades — их оценки по разметке.
	Top         []string `json:"top"`
	Grades      []int    `json:"grades"`
	Total       int      `json:"total"`
	Empty       bool     `json:"empty"`
	NDCG10      *float64 `json:"ndcg10"`
	MRR10       *float64 `json:"mrr10"`
	Recall50    *float64 `json:"recall50"`
	HasRelevant bool     `json:"has_relevant"`
	ExactHit    *bool    `json:"exact_hit,omitempty"`
	// NDCG10Family, ExactHitFamily — то же по семействам (оценка семейства — максимум по артикулам).
	NDCG10Family   *float64 `json:"ndcg10_family"`
	ExactHitFamily *bool    `json:"exact_hit_family,omitempty"`
	// ExpectEmpty — запрос из группы «ожидаем пусто или альтернативу»; ExpectEmptyOK — выдача пустая или честно помечена.
	ExpectEmpty   bool    `json:"expect_empty,omitempty"`
	ExpectEmptyOK *bool   `json:"expect_empty_ok,omitempty"`
	Rescued       *bool   `json:"rescued,omitempty"`
	FalseRescue   *bool   `json:"false_rescue,omitempty"`
	Violations    int     `json:"violations"`
	Ms            float64 `json:"ms"`
	Error         string  `json:"error,omitempty"`
	// Target — вердикт запроса набора «по цели» (eval_target.go); у наборов с оценками 0–3 — nil.
	Target *TargetQueryResult `json:"target,omitempty"`
	// Guide — диалог режима подбора в прогоне (guide_eval.go).
	Guide []GuideDialogResult `json:"guide,omitempty"`
}

// EvalChange — запрос, по которому конфигурация хуже (или лучше) предыдущей в лестнице прогона.
type EvalChange struct {
	QueryID string `json:"query_id"`
	Text    string `json:"text"`
	Type    string `json:"type"`
	Freq    int    `json:"freq"`
	// Against — с какой конфигурацией сравнивали.
	Against   string   `json:"against"`
	NDCGFrom  *float64 `json:"ndcg_from"`
	NDCGTo    *float64 `json:"ndcg_to"`
	NDCGDelta float64  `json:"ndcg_delta"`
	// Reasons — что изменилось: ndcg | lost_relevant | became_empty | lost_exact | new_violation |
	// false_rescue (и обратные для улучшений: gained_relevant | no_longer_empty | gained_exact |
	// fixed_violation | false_rescue_fixed).
	Reasons []string `json:"reasons"`
}

// EvalConfigResult — одна конфигурация в прогоне.
type EvalConfigResult struct {
	Config         string                 `json:"config"`
	RulesVersion   int                    `json:"rules_version"`
	RulesOverride  bool                   `json:"rules_override"`
	CatalogVersion string                 `json:"catalog_version"`
	Metrics        EvalMetrics            `json:"metrics"`
	ByType         map[string]EvalMetrics `json:"by_type"`
	// Against — предыдущая конфигурация прогона, с которой сравнивались ухудшения; "" у первой.
	Against      string            `json:"against,omitempty"`
	Regressions  []EvalChange      `json:"regressions"`
	Improvements []EvalChange      `json:"improvements"`
	PerQuery     []EvalQueryResult `json:"per_query,omitempty"`
	DurationMs   int64             `json:"duration_ms"`
	Warnings     []string          `json:"warnings,omitempty"`
}

// EvalRun — прогон: набор, конфигурации, строка воспроизводимости.
type EvalRun struct {
	RunID      string    `json:"run_id"`
	Set        string    `json:"set"`
	SetHash    string    `json:"set_hash"`
	Queries    int       `json:"queries"`
	Commit     string    `json:"commit"`
	StartedAt  time.Time `json:"started_at"`
	DurationMs int64     `json:"duration_ms"`
	// Repro — строка воспроизводимости: набор@хэш · правила vN · каталог · коммит.
	Repro   string             `json:"repro"`
	Configs []EvalConfigResult `json:"configs"`
	// File — куда записан JSON прогона (только CLI).
	File string `json:"file,omitempty"`
}

// EvaluationsResponse — GET /api/evaluations: последний прогон каждой конфигурации на наборе.
type EvaluationsResponse struct {
	Set     string           `json:"set"`
	Configs []EvaluationView `json:"configs"`
	Running *EvalJob         `json:"running"`
	LastJob *EvalJob         `json:"last_job"`
	// JudgeAgreement — последний прогон LLM-судьи на наборе; null — судья ещё не запускался.
	JudgeAgreement *JudgeAgreement `json:"judge_agreement"`
	// JudgeCalibration — последняя калибровка судьи на якорях (эталоны заказчика и их соседи, `go run . judge-calib`);
	// null — не запускалась.
	JudgeCalibration *JudgeCalibration `json:"judge_calibration"`
	// AgentCredit — честность правок агента в столбце B4 этого набора; null — B3 или B4 на наборе не прогонялись.
	AgentCredit *EvalAgentCredit `json:"agent_credit"`
	// Rescued — доля спасённых на реплее отложенной недели: только на holdout_week, иначе null.
	Rescued *EffectRescued `json:"rescued"`
}

// EvalAgentCredit — засчитан ли вклад агента (B3 → B4) и можно ли считать B4 независимой оценкой.
type EvalAgentCredit struct {
	// Counted — все принятые правки версии B4 сделаны запусками после разреза журнала.
	Counted bool `json:"counted"`
	// PreHoldout — среди правок B4 есть сделанные запусками до разреза: B4 видел тексты набора, его числа — не
	// независимая оценка.
	PreHoldout bool `json:"pre_holdout"`
	// Note — пояснение по-русски.
	Note string `json:"note"`
	// RulesVersion — версия правил столбца B4.
	RulesVersion int `json:"rules_version"`
}

// EvaluationView — сохранённый результат конфигурации.
type EvaluationView struct {
	RunID          string                 `json:"run_id"`
	Set            string                 `json:"set"`
	SetHash        string                 `json:"set_hash"`
	Queries        int                    `json:"queries"`
	Config         string                 `json:"config"`
	RulesVersion   int                    `json:"rules_version"`
	RulesOverride  bool                   `json:"rules_override"`
	CatalogVersion string                 `json:"catalog_version"`
	Commit         string                 `json:"commit"`
	Repro          string                 `json:"repro"`
	CreatedAt      time.Time              `json:"created_at"`
	DurationMs     int                    `json:"duration_ms"`
	Metrics        EvalMetrics            `json:"metrics"`
	ByType         map[string]EvalMetrics `json:"by_type"`
	Against        string                 `json:"against,omitempty"`
	Regressions    []EvalChange           `json:"regressions"`
	Improvements   []EvalChange           `json:"improvements"`
}

// EvalJob — асинхронный прогон (POST /api/evaluations/run).
type EvalJob struct {
	ID      string   `json:"id"`
	Set     string   `json:"set"`
	Configs []string `json:"configs"`
	Status  string   `json:"status"` // running | done | failed
	// Done и Total — сколько поисков прогона сделано и сколько всего (запросы × конфигурации); Total 0 —
	// набор ещё не загружен.
	Done       int        `json:"done"`
	Total      int        `json:"total"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	RunID      string     `json:"run_id,omitempty"`
	Error      string     `json:"error,omitempty"`
}

// JudgeAgreement — согласие LLM-судьи с разметкой набора (ARCHITECTURE §6, «Разметка»). Разметка mock-набора
// сделана по построению и служит прокси ручной: судье можно верить в вопросе «какая конфигурация лучше», но
// не в абсолютных числах.
type JudgeAgreement struct {
	RunID         string    `json:"run_id"`
	Set           string    `json:"set"`
	SetHash       string    `json:"set_hash"`
	Model         string    `json:"model"`
	PromptVersion string    `json:"prompt_version"`
	Configs       []string  `json:"configs"`
	CreatedAt     time.Time `json:"created_at"`
	// Pairs — пар запрос–товар в пуле; Judged — по скольким судья ответил (в согласие входят только они).
	Pairs  int `json:"pairs"`
	Judged int `json:"judged"`
	// Depth — глубина, до которой пул полный: топ-Depth каждой конфигурации оценён целиком (NDCG@Depth ниже).
	Depth int `json:"depth"`
	// Exact — доля точного совпадения оценки, Within1 — расхождение не больше 1, Kappa — каппа Коэна
	// с квадратичными весами (0 — как случайно, 1 — полное согласие).
	Exact   *float64 `json:"exact"`
	Within1 *float64 `json:"within1"`
	Kappa   *float64 `json:"kappa"`
	// Confusion — матрица 4×4: строка — оценка разметки, столбец — оценка судьи.
	Confusion [4][4]int `json:"confusion"`
	// ByConfig — NDCG@Depth конфигураций на пуле по разметке и по судье; SameOrder — порядок конфигураций
	// по судье тот же, что по разметке.
	ByConfig  []JudgeConfig `json:"by_config"`
	SameOrder bool          `json:"same_order"`
	Calls     int           `json:"calls"`
	CostRub   float64       `json:"cost_rub"`
	// DurationMs — время прогона судьи.
	DurationMs int `json:"duration_ms"`
}

// JudgeConfig — конфигурация глазами разметки и судьи.
type JudgeConfig struct {
	Config     string   `json:"config"`
	NDCGLabels *float64 `json:"ndcg_labels"`
	NDCGJudge  *float64 `json:"ndcg_judge"`
}

// JudgePair — оценка одной пары (JSON прогона судьи).
type JudgePair struct {
	QueryID   string `json:"query_id"`
	ProductID string `json:"product_id"`
	Label     int    `json:"label"`
	Judge     *int   `json:"judge"`
}

// JudgeRun — результат `go run . judge`: согласие и оценки пар.
type JudgeRun struct {
	JudgeAgreement
	Grades []JudgePair `json:"grades"`
	File   string      `json:"file,omitempty"`
}

// EffectRescued — доля спасённых поисков по реплею отложенной недели (holdout_week).
type EffectRescued struct {
	Set     string `json:"set"` // holdout_week
	SetHash string `json:"set_hash"`
	// Source — откуда доля, по-русски: набор, прогоны, засчитан ли вклад агента.
	Source   string `json:"source"`
	Baseline string `json:"baseline"` // B0
	// Config — до какой конфигурации засчитано: B3 (только поиск) или B4 (поиск и честные правки агента).
	Config string `json:"config"`
	// RunBaseline, RunConfig, RunAgent — прогоны B0, B3, B4 (evaluations.run_id); RunAgent пусто — B4 не прогонялся.
	RunBaseline       string `json:"run_baseline"`
	RunConfig         string `json:"run_config"`
	RunAgent          string `json:"run_agent"`
	RulesVersion      int    `json:"rules_version"`
	AgentRulesVersion int    `json:"agent_rules_version"`
	// Weight — единица счёта: searches (запрос весит числом своих поисков за неделю).
	Weight  string `json:"weight"`
	Failed  int    `json:"failed"`  // поисков, где B0 не дал релевантного в топ-10
	Rescued int    `json:"rescued"` // из них спасено (поиск + засчитанный вклад агента)
	// FailedQueries, RescuedQueries — то же в различных запросах.
	FailedQueries  int     `json:"failed_queries"`
	RescuedQueries int     `json:"rescued_queries"`
	Share          float64 `json:"share"`
	// SearchShare — вклад поиска (B0 → B3); AgentShare — вклад агента (B3 → B4), 0, если не засчитан.
	SearchShare float64 `json:"search_share"`
	AgentShare  float64 `json:"agent_share"`
	// AgentCounted — вклад агента засчитан: все принятые правки B4 — из запусков после разреза журнала.
	AgentCounted bool `json:"agent_counted"`
	// AgentPreHoldout — в B4 есть правки запусков до разреза журнала: числа B4 — не независимая оценка.
	AgentPreHoldout bool   `json:"agent_pre_holdout"`
	AgentNote       string `json:"agent_note"`
	// CILow, CIHigh — 95% интервал Уилсона (n — различные запросы): из него берутся консервативный и
	// оптимистичный сценарии.
	CILow  float64 `json:"ci_low"`
	CIHigh float64 `json:"ci_high"`
}

// JudgeCalibration — калибровка LLM-судьи на якорях фида: цели эталонов заказчика и их соседей должны получить 3,
// запрещённые соседи (цели якорей других вендоров) — не выше 1. Каппы против кликов здесь нет: она вырождена.
type JudgeCalibration struct {
	RunID         string    `json:"run_id"`
	Model         string    `json:"model"`
	PromptVersion string    `json:"prompt_version"`
	CreatedAt     time.Time `json:"created_at"`
	// WithFamily — в карточке товара судье передавалось семейство.
	WithFamily bool `json:"with_family"`
	// Targets — пар «якорь — товар целевого семейства»; TargetsHit — из них с оценкой 3; TargetRate — доля.
	Targets    int      `json:"targets"`
	TargetsHit int      `json:"targets_hit"`
	TargetRate *float64 `json:"target_rate"`
	// EtalonTargets, EtalonHit — то же только по 10 эталонам заказчика.
	EtalonTargets int `json:"etalon_targets"`
	EtalonHit     int `json:"etalon_hit"`
	// Forbidden — пар «якорь — товар цели другого вендора»; ForbiddenOK — из них с оценкой не выше 1.
	Forbidden     int      `json:"forbidden"`
	ForbiddenOK   int      `json:"forbidden_ok"`
	ForbiddenRate *float64 `json:"forbidden_rate"`
	// TargetGrades, ForbiddenGrades — распределение оценок судьи 0–3.
	TargetGrades    [4]int `json:"target_grades"`
	ForbiddenGrades [4]int `json:"forbidden_grades"`
	// Unjudged — пар без ответа судьи (в доли не входят).
	Unjudged   int              `json:"unjudged"`
	Calls      int              `json:"calls"`
	CostRub    float64          `json:"cost_rub"`
	DurationMs int              `json:"duration_ms"`
	Misses     []JudgeCalibMiss `json:"misses"`
}

// JudgeCalibMiss — промах калибровки: цель не получила 3 или запрещённый сосед получил больше 1.
type JudgeCalibMiss struct {
	Query   string `json:"query"`
	Product string `json:"product"`
	Family  string `json:"family"`
	// Expect — target | forbidden.
	Expect string `json:"expect"`
	Grade  int    `json:"grade"`
}
