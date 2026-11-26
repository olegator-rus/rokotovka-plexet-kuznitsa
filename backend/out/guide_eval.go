package out

// Оценка режима подбора (контур подбора, services/guide_eval.go): 48 диалогов × 3 прогона, три варианта — полный подбор
// (config "gd"), поиск без подбора ("srch"), модель без проверок до показа ("nock", эмуляция по журналу удалённого).
// Строки — в evaluations (eval_set "guide"), метрики — EvalMetrics.Guide, диалоги — EvalQueryResult.Guide.

// GuideCheck — числитель и знаменатель одной автопроверки (нарушений из проверенного).
type GuideCheck struct {
	Label string `json:"label"`
	Bad   int    `json:"bad"`
	Of    int    `json:"of"`
}

// GuideEvalError — пример ошибки с причиной.
type GuideEvalError struct {
	Dialog string `json:"dialog"`
	Run    int    `json:"run"`
	Turn   int    `json:"turn"`
	Reason string `json:"reason"`
}

// GuideEvalMetrics — сводка варианта на наборе диалогов.
type GuideEvalMetrics struct {
	Variant string `json:"variant"`
	Runs    int    `json:"runs"`
	Dialogs int    `json:"dialogs"`
	// TaskAll — диалогов, решённых во всех прогонах; TaskPasses / TaskTotal — по прогонам.
	TaskAll    int `json:"task_all"`
	TaskPasses int `json:"task_passes"`
	TaskTotal  int `json:"task_total"`
	// Отдельно по диалогам вендоров и отрицательным.
	VendorAll   int `json:"vendor_all"`
	VendorOf    int `json:"vendor_of"`
	NegativeAll int `json:"negative_all"`
	NegativeOf  int `json:"negative_of"`
	// Гейт выдуманные идентификаторы и цены, нарушения правил лицензий, утверждения без факта.
	FabricatedIDs    int  `json:"fabricated_ids"`
	FabricatedPrices int  `json:"fabricated_prices"`
	LicenseViolation int  `json:"license_violations"`
	ClaimsNoFact     int  `json:"claims_without_fact"`
	Claims           int  `json:"claims"`
	Gate             bool `json:"gate"`
	// Checks — все автопроверки: ключ → нарушений из проверенного.
	Checks map[string]GuideCheck `json:"checks"`
	// Время хода (мс): первый вывод, проверенные строки, весь ход — p50/p95 по ходам; отдельно ходы с моделью.
	Turns      int         `json:"turns"`
	LLMTurns   int         `json:"llm_turns"`
	MsFirst    EvalLatency `json:"ms_first"`
	MsChecked  EvalLatency `json:"ms_checked"`
	MsTotal    EvalLatency `json:"ms_total"`
	MsTotalLLM EvalLatency `json:"ms_total_llm"`
	// Расход по журналу хода (guide_turns) и llm_usage.
	LLMCalls       int     `json:"llm_calls"`
	InputTokens    int     `json:"input_tokens"`
	OutputTokens   int     `json:"output_tokens"`
	TurnsCostRub   float64 `json:"turns_cost_rub"`
	UsageCostRub   float64 `json:"usage_cost_rub"`
	UsageCalls     int64   `json:"usage_calls"`
	JournalTurns   int     `json:"journal_turns"`
	DialogCostRub  float64 `json:"dialog_cost_rub"`     // средняя на диалог
	DialogLLMRub   float64 `json:"dialog_llm_cost_rub"` // средняя на диалог, где модель вызывалась
	DialogsWithLLM int     `json:"dialogs_with_llm"`
	// Removed — что сняли проверки до показа (по классам; у варианта «без проверок» это показано бы было).
	Removed map[string]int   `json:"removed,omitempty"`
	Errors  []GuideEvalError `json:"errors"`
	Notes   []string         `json:"notes,omitempty"`
}

// GuideDialogResult — диалог в одном прогоне.
type GuideDialogResult struct {
	Run       int      `json:"run"`
	Pass      bool     `json:"pass"`
	Fails     []string `json:"fails,omitempty"`
	Turns     int      `json:"turns"`
	Types     []string `json:"types"`
	LLM       int      `json:"llm_calls"`
	CostRub   float64  `json:"cost_rub"`
	Selection string   `json:"selection_id,omitempty"`
	// Fallbacks — причины ответа без модели по ходам (meta.fallback).
	Fallbacks []string `json:"fallbacks,omitempty"`
}

// FeedGuide — строка «Диалог подбора» сводки S6: последняя оценка полного подбора.
type FeedGuide struct {
	Dialogs        int     `json:"dialogs"`
	Runs           int     `json:"runs"`
	TaskAll        int     `json:"task_all"`
	Gate           bool    `json:"gate"`
	DialogCostRub  float64 `json:"dialog_cost_rub"`
	DialogLLMRub   float64 `json:"dialog_llm_cost_rub"`
	DialogsWithLLM int     `json:"dialogs_with_llm"`
	LLMCalls       int     `json:"llm_calls"`
	MsTotalP95     float64 `json:"ms_total_p95"`
	RunTail        string  `json:"run_tail"`
}
