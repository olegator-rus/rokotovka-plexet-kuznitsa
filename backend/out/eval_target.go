package out

// Оценка «по цели» на фиде (контур эталонов, services/eval_target*.go). Строки прогона пишутся в evaluations как у
// прочих наборов: метрики «по цели» — в EvalMetrics.Target, вердикт запроса — в EvalQueryResult.Target.

// TargetQueryResult — вердикт одного запроса: первая цель ответа, совпадение с ожидаемой, счётчики и трасса.
type TargetQueryResult struct {
	Group      string `json:"group,omitempty"`
	Vendor     string `json:"vendor,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Distortion string `json:"distortion,omitempty"`
	// Expect — ожидаемая цель коротко («family 121231», «vendor webinar», «category 86563», «none»).
	Expect string `json:"expect"`
	// Первая цель ответа: поле target, иначе первая карточка (URL семейства → оффера → вендора).
	FirstURL    string `json:"first_url"`
	FirstType   string `json:"first_type"`
	FirstSource string `json:"first_source"`
	FirstTitle  string `json:"first_title"`
	FirstFamily string `json:"first_family"`
	// ByCards — строка проверяется по первым N карточкам (набор понимания), а не по цели ответа: в Success@1 не входит.
	ByCards bool `json:"by_cards,omitempty"`
	// Strict — Success@1: первая цель совпала с ожидаемой. Top4 — она же или ожидаемое среди первых 4 карточек.
	// Rank — место первой карточки ожидаемой цели в первых 10 (1, если совпала цель); 0 — нет.
	Strict bool `json:"strict"`
	Top4   bool `json:"top4"`
	Rank   int  `json:"rank"`
	// Pass — Strict и дополнительные условия строки (нет исключённых семейств, есть альтернативы, маршрут);
	// Fails — какие не выполнены. MissReason — почему не совпала цель: «ссылка», «ранг N», «цель».
	Pass       bool     `json:"pass"`
	Fails      []string `json:"fails,omitempty"`
	MissReason string   `json:"miss_reason,omitempty"`
	// Счётчики ответа: карточки, разные семейства, разные SKU (карточки и их варианты), карточки «Можно купить
	// сейчас» с ценой; Violations — карточки, нарушающие жёсткие условия запроса.
	Cards        int  `json:"cards"`
	Families     int  `json:"families"`
	SKUs         int  `json:"skus"`
	Alternatives int  `json:"alternatives"`
	Violations   int  `json:"violations"`
	HasHard      bool `json:"has_hard,omitempty"`
	// Трасса: источник разбора, маршрут D без вызова модели (ничего не нашлось по тексту), вызовы модели и их
	// стоимость по трассе.
	ParseSource  string  `json:"parse_source,omitempty"`
	ZeroResults  bool    `json:"zero_results,omitempty"`
	LLMCalls     int     `json:"llm_calls,omitempty"`
	InputTokens  int     `json:"input_tokens,omitempty"`
	OutputTokens int     `json:"output_tokens,omitempty"`
	CostRub      float64 `json:"cost_rub,omitempty"`
}

// TargetMetrics — сводка прогона конфигурации на наборе «по цели».
type TargetMetrics struct {
	// Answers — ответов без ошибки; Judged — из них с проверкой по цели ответа (а не по первым N карточкам).
	Answers int `json:"answers"`
	Judged  int `json:"judged"`
	// Success1 — доля Success@1 среди Judged, Success1N — их число. Top4 — ожидаемое в первых 4. MRR10 — по месту
	// первой карточки ожидаемой цели в первых 10.
	Success1  *float64 `json:"success1"`
	Success1N int      `json:"success1_n"`
	Top4      *float64 `json:"top4"`
	Top4N     int      `json:"top4_n"`
	MRR10     *float64 `json:"mrr10"`
	// Pass — все условия строки (для наборов агента: ухудшение = Pass было и пропало).
	Pass  *float64 `json:"pass"`
	PassN int      `json:"pass_n"`
	// Доли ответов с ≥4 разными семействами, SKU и карточками (среди Answers).
	Families4  *float64 `json:"families4"`
	Families4N int      `json:"families4_n"`
	SKUs4      *float64 `json:"skus4"`
	SKUs4N     int      `json:"skus4_n"`
	Cards4     *float64 `json:"cards4"`
	Cards4N    int      `json:"cards4_n"`
	// Жёсткие условия: запросов с условиями, из них с нарушением, карточек-нарушений.
	HardQueries    int `json:"hard_queries"`
	HardViolated   int `json:"hard_violated"`
	ViolationItems int `json:"violation_items"`
	// Breakdown — Success@1 и Top4 по группам: vendor, distortion, group.
	Breakdown map[string]map[string]TargetCell `json:"breakdown,omitempty"`
	Cost      *TargetCost                      `json:"cost,omitempty"`
}

// TargetCell — строка разбивки.
type TargetCell struct {
	N        int `json:"n"`
	Success1 int `json:"success1"`
	Top4     int `json:"top4"`
}

// TargetCost — что стоил прогон по трассам и llm_usage: маршрут D по метке и по реальным вызовам
// модели, кэш разборов, токены. Mode — cold (кэш разборов и векторов не читается) или warm.
type TargetCost struct {
	Mode     string `json:"mode"`
	Searches int    `json:"searches"`
	// RouteD — поисков с меткой маршрута D; RouteDNoModel — из них без вызова модели и без кэша (метку ставит
	// «ничего не нашлось по тексту», zero_results).
	RouteD        int `json:"route_d"`
	RouteDNoModel int `json:"route_d_no_model"`
	// ModelSearches — поисков с вызовом модели; CacheSearches — с разбором из кэша.
	ModelSearches int `json:"model_searches"`
	CacheSearches int `json:"cache_searches"`
	// Из llm_usage по request_id прогона: вызовов (с повторами и резервом), токены, рубли.
	LLMCalls     int64   `json:"llm_calls"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	LLMRub       float64 `json:"llm_rub"`
	// TraceRub — та же стоимость по трассам ответов (сверка с llm_usage).
	TraceRub float64 `json:"trace_rub"`
	// UsageJoined — llm_usage прочитан (база доступна); false — рубли и токены только по трассам.
	UsageJoined bool `json:"usage_joined"`
}

// FeedSummary — GET /api/evaluations/feed: экран «Доказательства → На данных заказчика» (). Все числа —
// из последних прогонов наборов «по цели» и трасс; чего не прогоняли — null.
type FeedSummary struct {
	Catalog FeedCatalog `json:"catalog"`
	// Etalon — эталоны заказчика у полного продукта; null — прогона не было.
	Etalon *FeedScore `json:"etalon"`
	// Closed — закрытая часть: Success@1 у BM25, гибрида и полного продукта; пустой — прогон не запускался.
	Closed []FeedScore `json:"closed"`
	// Four — доля ответов полного продукта с ≥4 разными семействами (и SKU) по последним прогонам наборов.
	Four *FeedFour `json:"four"`
	// Cost — стоимость запроса по формуле, холодный и тёплый прогон; null — прогонов полного продукта нет.
	Cost *FeedCost `json:"cost"`
	// Table — последний прогон каждой конфигурации на каждом наборе (кроме закрытого до его прогона).
	Table []FeedRow `json:"table"`
	// Configs — что сравнивается: id конфигурации и подпись.
	Configs []FeedConfig `json:"configs"`
	// MockRuns — есть прогоны mock-каталога (вкладка «Метрики», подпись «до данных заказчика»).
	MockRuns bool `json:"mock_runs"`
}

// FeedCatalog — каталог, на котором считаются числа: источник и число офферов.
type FeedCatalog struct {
	Source string `json:"source"` // feed | mock | unknown
	Label  string `json:"label"`
	Offers int    `json:"offers"`
}

// FeedConfig — конфигурация сравнения.
type FeedConfig struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Note  string `json:"note"`
}

// FeedScore — Success@1 конфигурации на наборе.
type FeedScore struct {
	Set      string   `json:"set"`
	Config   string   `json:"config"`
	Matched  int      `json:"matched"`
	Total    int      `json:"total"`
	Success1 *float64 `json:"success1"`
	Top4     *float64 `json:"top4"`
	RunTail  string   `json:"run_tail"`
}

// FeedFour — доля ответов с ≥4 товарами.
type FeedFour struct {
	Config     string   `json:"config"`
	Sets       []string `json:"sets"`
	Answers    int      `json:"answers"`
	Families4N int      `json:"families4_n"`
	SKUs4N     int      `json:"skus4_n"`
	Families4  *float64 `json:"families4"`
	SKUs4      *float64 `json:"skus4"`
}

// FeedCost — стоимость запроса: C = p × (1 − h) × c_llm + (инфраструктура + агент + сопровождение) / N.
type FeedCost struct {
	Formula  string       `json:"formula"`
	Requests int          `json:"requests"`
	Params   []CostsParam `json:"params"`
	Cold     *FeedCostRun `json:"cold"`
	Warm     *FeedCostRun `json:"warm"`
	// Journal — для сравнения: журнал витрины и трафика на этом каталоге (search_events + llm_usage).
	Journal *FeedCostRun `json:"journal"`
	Notes   []string     `json:"notes"`
}

// FeedCostRun — расчёт по одному прогону (холодному или тёплому) полного продукта.
type FeedCostRun struct {
	Mode string   `json:"mode"`
	Sets []string `json:"sets"`
	// Замеры прогона.
	Searches      int      `json:"searches"`
	RouteDLabel   float64  `json:"route_d_label"`    // доля поисков с меткой D
	RouteDNoModel int      `json:"route_d_no_model"` // из них без вызова модели (zero_results)
	P             float64  `json:"p"`                // доля поисков с разбором моделью (вызов или кэш)
	H             float64  `json:"h"`                // доля попаданий в кэш среди них
	CLLM          *float64 `json:"c_llm"`            // ₽ за поиск с вызовом модели (llm_usage / поисков с вызовом)
	LLMCalls      int64    `json:"llm_calls"`
	InputTokens   int64    `json:"input_tokens"`
	OutputTokens  int64    `json:"output_tokens"`
	LLMRub        float64  `json:"llm_rub"`
	P50Ms         float64  `json:"p50_ms"`
	P95Ms         float64  `json:"p95_ms"`
	// Расчёт на месяц.
	LLMPerRequestRub   float64 `json:"llm_per_request_rub"`
	FixedPerRequestRub float64 `json:"fixed_per_request_rub"`
	PerRequestRub      float64 `json:"per_request_rub"`
	MonthRub           float64 `json:"month_rub"`
	InfraShare         float64 `json:"infra_share"`
	AgentShare         float64 `json:"agent_share"`
	SupportShare       float64 `json:"support_share"`
	LLMShare           float64 `json:"llm_share"`
}

// FeedRow — строка таблицы «набор × конфигурация».
type FeedRow struct {
	Set     string        `json:"set"`
	Config  string        `json:"config"`
	Queries int           `json:"queries"`
	RunTail string        `json:"run_tail"`
	Mode    string        `json:"mode,omitempty"`
	Metrics TargetMetrics `json:"metrics"`
	P50Ms   float64       `json:"p50_ms"`
	P95Ms   float64       `json:"p95_ms"`
	// RubPerQuery — расход модели прогона на запрос (llm_usage, иначе трассы).
	RubPerQuery *float64 `json:"rub_per_query"`
}
