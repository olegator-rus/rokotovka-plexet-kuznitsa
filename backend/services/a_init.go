// Package services — бизнес-логика по ограниченным контекстам: один контекст — один файл
// (у поиска несколько файлов search*.go — по шагу на файл).
package services

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"kuznica/api/domain"
	"kuznica/api/externalApi/opensearch"
	"kuznica/api/externalApi/tei"
	"kuznica/api/repositories"
)

// Services — граф сервисов. Слой знает про repositories и externalApi; про Fiber и HTTP — не знает.
//
// Новый контекст приезжает полем этой структуры и строкой в NewServices — иначе хендлеру не через что
// звать логику.
type Services struct {
	repos    *repositories.Repos
	now      func() time.Time
	search   *opensearch.Client
	embedder *tei.Client
	reranker *tei.Client
	rerankW  float64
	// models — кэш векторов запросов и оценок реранкера (search_modelcache.go); nil — без кэша.
	models *modelCache
	source CatalogSource
	// llm — режим модели (переключается без перезапуска) и дневной бюджет (llmguard.go).
	llm *llmGuard
	// alias — алиас индекса каталога в OpenSearch.
	alias string
	// intent — разбор намерения на маршруте D (intent.go); не nil всегда: без модели — кэш и правила.
	intent domain.IntentParser
	// judge — модель LLM-судьи (eval_judge.go), другого семейства, чем разбор и агент; nil — судьи нет.
	judge ChatModel

	// catalog — справочники в памяти (клиенты, версия каталога и правил); меняется импортом целиком.
	catalog atomic.Pointer[catalogState]
	// index — _meta текущего индекса; nil — ещё не прочитан (OpenSearch мог стартовать позже API).
	index atomic.Pointer[cachedIndex]
	// importMu — один импорт за раз.
	importMu sync.Mutex
	// core — состояние ядра поиска B3: кеш правил, разборщик намерения, подсказки (search_core.go).
	core searchCore
	// writes — фоновые записи журнала поиска; Wait дожидается их при остановке.
	writes sync.WaitGroup
	// agent — агент SearchOps: запуски, лента, расписание (agent.go).
	agent *agentService
	// agentStoreOverride — хранилище агента в тестах; nil — repos.
	agentStoreOverride agentStore
	// features — флаги контура коммерции (commerce.go).
	features Features
	// noFallbackBlock — SEARCH_FALLBACK_BLOCK=off: без «Популярного в наличии» под сервисным запросом и бессмыслицей.
	noFallbackBlock bool
	// upsellTitle — заголовок блока допродажи; upsell — справочники допродажи контура фида (upsell_data.go).
	upsellTitle string
	upsell      upsellState
	// procurement — справка и реестр контура доводки из файлов источника (help_index.go, analogs_reestr.go).
	procurement procurementCache
}

// Options — настройки и клиенты сервисов из config. Нулевые клиенты — шаг выключен.
type Options struct {
	// Now — часы; nil — time.Now (тесты подставляют свои).
	Now      func() time.Time
	Search   *opensearch.Client
	Embedder *tei.Client
	Reranker *tei.Client
	// RerankWeight — доля оценки реранкера в релевантности на маршрутах C/D; 0 — реранкер не вызывается.
	RerankWeight float64
	// ModelCacheSize, ModelCacheTTL — LRU-кэш векторов запросов и оценок реранкера; 0 — без кэша.
	ModelCacheSize int
	ModelCacheTTL  time.Duration
	// Source — источник каталога для импорта; nil — импорт недоступен.
	Source CatalogSource
	// LLMMode — on | off (config.LLMConfig.Mode); пусто — off.
	LLMMode string
	// IntentModel — модель разбора намерения; nil — маршрут D на кэше и правилах.
	IntentModel ChatModel
	// IntentFallback — резервная модель разбора при сбое основной; nil — без резерва.
	IntentFallback ChatModel
	// JudgeModel — модель LLM-судьи оценки (qwen3.6-35b-a3b); nil — `go run . judge` недоступен.
	JudgeModel ChatModel
	// IndexAlias — алиас индекса каталога; пусто — products. Живой интеграционный тест берёт свой,
	// чтобы не трогать индекс разработки.
	IndexAlias string
	// AgentModel — модель агента SearchOps; nil — запуск агента недоступен.
	AgentModel ChatModel
	// Agent — пороги и наборы проверки правок агента.
	Agent AgentOptions
	// DailyBudgetRub — дневной бюджет LLM, ₽ (LLM_DAILY_BUDGET_RUB); 0 — без потолка.
	DailyBudgetRub float64
	// AgentReserveRub — часть дневного бюджета, недоступная разборам покупателей (AGENT_BUDGET_RESERVE_RUB).
	AgentReserveRub float64
	// PerIPDailyRub — суточная квота разборов маршрута D на адрес покупателя, ₽ (LLM_PER_IP_DAILY_RUB); 0 — без квоты.
	PerIPDailyRub float64
	// Features — флаги контура коммерции (FEATURE_*).
	Features Features
	// NoFallbackBlock — без блока «Популярное в наличии» под сервисным запросом и бессмыслицей (SEARCH_FALLBACK_BLOCK=off,
	// search_ladder.go). Нулевое значение — блок есть: так по умолчанию и в тестах.
	NoFallbackBlock bool
	// UpsellTitle — заголовок блока допродажи (UPSELL_BLOCK_TITLE); пусто — «Дополните решение».
	UpsellTitle string
}

// NewServices — граф сервисов. repos == nil — API без базы: health отдаёт db: down, поиск не пишет журнал.
func NewServices(repos *repositories.Repos, opts Options) *Services {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.LLMMode == "" {
		opts.LLMMode = "off"
	}
	if opts.IndexAlias == "" {
		opts.IndexAlias = defaultIndexAlias
	}
	s := &Services{
		repos:           repos,
		now:             opts.Now,
		search:          opts.Search,
		embedder:        opts.Embedder,
		reranker:        opts.Reranker,
		rerankW:         opts.RerankWeight,
		models:          newModelCache(opts.ModelCacheSize, opts.ModelCacheTTL, opts.Now),
		source:          opts.Source,
		alias:           opts.IndexAlias,
		judge:           opts.JudgeModel,
		features:        opts.Features,
		noFallbackBlock: opts.NoFallbackBlock,
		upsellTitle:     opts.UpsellTitle,
	}
	var load func(context.Context, time.Time) (float64, error)
	if repos.Enabled() {
		load = repos.LLMCostSince
	}
	s.llm = newLLMGuard(opts.LLMMode, opts.IntentModel != nil || opts.AgentModel != nil, opts.DailyBudgetRub, opts.Now, load)
	s.llm.reserve, s.llm.perAddr = opts.AgentReserveRub, opts.PerIPDailyRub
	s.agent = newAgentService(s, opts.AgentModel, opts.Agent)
	s.intent = newIntentService(s, opts.IntentModel, opts.IntentFallback)
	// Маршруту D разборщик нужен, если есть модель или кэш разборов в базе; без того и другого он вернул
	// бы разбор правил как есть — тогда маршрут D честно работает как C (parser_not_connected).
	if opts.IntentModel != nil || repos.Enabled() {
		s.SetIntentParser(s.intent)
	}
	return s
}

// Wait — дождаться фоновых записей (журнал поиска) перед остановкой процесса.
func (s *Services) Wait() { s.writes.Wait() }
