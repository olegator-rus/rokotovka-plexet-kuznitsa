// Package services — бизнес-логика по ограниченным контекстам: один контекст — один файл
// (у поиска несколько файлов search*.go — по шагу на файл).
package services

import (
	"sync"
	"sync/atomic"
	"time"

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
	source   CatalogSource
	llmMode  string
	// alias — алиас индекса каталога в OpenSearch.
	alias string

	// catalog — справочники в памяти (клиенты, версия каталога и правил); меняется импортом целиком.
	catalog atomic.Pointer[catalogState]
	// index — _meta текущего индекса; nil — ещё не прочитан (OpenSearch мог стартовать позже API).
	index atomic.Pointer[cachedIndex]
	// importMu — один импорт за раз.
	importMu sync.Mutex
	// writes — фоновые записи журнала поиска; Wait дожидается их при остановке.
	writes sync.WaitGroup
}

// Options — настройки и клиенты сервисов из config. Нулевые клиенты — шаг выключен.
type Options struct {
	// Now — часы; nil — time.Now (тесты подставляют свои).
	Now      func() time.Time
	Search   *opensearch.Client
	Embedder *tei.Client
	Reranker *tei.Client
	// Source — источник каталога для импорта; nil — импорт недоступен.
	Source CatalogSource
	// LLMMode — on | off (config.LLMConfig.Mode); пусто — off.
	LLMMode string
	// IndexAlias — алиас индекса каталога; пусто — products. Живой интеграционный тест берёт свой,
	// чтобы не трогать индекс разработки.
	IndexAlias string
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
	return &Services{
		repos:    repos,
		now:      opts.Now,
		search:   opts.Search,
		embedder: opts.Embedder,
		reranker: opts.Reranker,
		source:   opts.Source,
		llmMode:  opts.LLMMode,
		alias:    opts.IndexAlias,
	}
}

// Wait — дождаться фоновых записей (журнал поиска) перед остановкой процесса.
func (s *Services) Wait() { s.writes.Wait() }
