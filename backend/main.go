// API PLEXeT SearchOps — умный поиск для B2B-магазина ПО. HTTP-сервер Fiber:
// handlers → services → repositories; раскладка и соглашения — AGENTS.md.
//
// `go run .` — HTTP-сервер; `go run . import` — импорт каталога из DATA_DIR в OpenSearch и PostgreSQL
// (то же, что POST /api/catalog/import), сводка — JSON в stdout; `go run . warmup` — прогрев кэша разборов
// частотными запросами.
//
// CORS не подключён намеренно: SPA и /api на одном origin (Caddy в проде, прокси Vite в dev).
package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"kuznica/api/config"
	"kuznica/api/externalApi/llm"
	"kuznica/api/externalApi/opensearch"
	"kuznica/api/externalApi/tei"
	"kuznica/api/handlers"
	"kuznica/api/repositories"
	"kuznica/api/services"

	"github.com/gofiber/contrib/fiberzap/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/joho/godotenv"
	"go.uber.org/zap"
)

func main() {
	_ = godotenv.Load() // в проде файла нет, ошибка игнорируется намеренно

	log := newLogger(os.Getenv("APP_ENV"))
	defer func() { _ = log.Sync() }()
	zap.ReplaceGlobals(log)

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("конфигурация", zap.Error(err))
	}
	for _, line := range cfg.Summary() {
		log.Info(line)
	}

	// Без DATABASE_URL (только development) — старт с WARN: health отдаёт db: down.
	repos, err := repositories.InitRepos(cfg.DatabaseURL)
	if err != nil {
		log.Fatal("база: DATABASE_URL задан, но не открывается — make db?", zap.Error(err))
	}
	defer func() { _ = repos.Close() }()
	if err := repos.MigrateDB(); err != nil {
		log.Fatal("миграция", zap.Error(err))
	}

	src := dataSource(log, cfg)
	svc := services.NewServices(repos, services.Options{
		Search:         opensearch.New(cfg.OpenSearchURL, searchTimeout),
		Embedder:       tei.New(cfg.EmbedderURL, modelsTimeout),
		Reranker:       tei.New(cfg.RerankerURL, modelsTimeout),
		RerankWeight:   cfg.RerankWeight,
		ModelCacheSize: cfg.ModelCacheSize,
		ModelCacheTTL:  cfg.ModelCacheTTL,
		Source:         src,
		IndexAlias:     cfg.IndexAlias,
		LLMMode:        cfg.LLM.Mode,

		IntentModel:    intentModel(cfg.LLM, cfg.LLM.IntentModel),
		IntentFallback: intentModel(cfg.LLM, cfg.LLM.FallbackModel),
		JudgeModel:     intentModel(cfg.LLM, cfg.LLM.JudgeModel),

		AgentModel: agentModel(cfg.LLM, cfg.Agent.Model),
		Agent: services.AgentOptions{MaxRegressionWorse: cfg.Agent.MaxRegressionWorse, RegressionSets: cfg.Agent.RegressionSets,
			Holdout: services.HoldoutFromEnv()},

		DailyBudgetRub:  cfg.Ops.DailyBudgetRub,
		AgentReserveRub: cfg.Ops.AgentReserveRub,
		PerIPDailyRub:   cfg.Ops.PerIPDailyRub,

		Features: services.Features{Hardware: cfg.Features.Hardware, Companions: cfg.Features.Companions,
			MarginPolicy: cfg.Features.MarginPolicy, Solutions: cfg.Features.Solutions},
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if len(os.Args) > 1 && os.Args[1] == "import" {
		runImport(ctx, log, svc)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "import-logs" {
		runImportLogs(ctx, log, svc)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "eval" {
		runEval(ctx, log, svc, os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "judge" {
		runJudge(ctx, log, svc, os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "buildsets" {
		runBuildSets(ctx, log, svc, setsDir(cfg, src), intentModel(cfg.LLM, cfg.LLM.IntentModel), os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "audit" {
		runAudit(ctx, log, svc, os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "proposals" {
		runProposals(ctx, log, svc, os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "warmup" {
		runWarmup(ctx, log, svc, os.Args[2:])
		return
	}
	svc.LoadCatalogState(ctx)
	svc.RecoverAgentRuns(ctx)
	if cfg.Agent.Schedule {
		svc.StartAgentScheduler(ctx, cfg.Agent.Interval)
	}
	defer svc.Wait()
	h := handlers.NewHandlerManager(svc, handlers.Options{PublicOrigin: cfg.PublicOrigin,
		AdminEnabled: cfg.Ops.AdminEnabled, RatePerMinute: cfg.Ops.RatePerMinute, BackofficeToken: cfg.Ops.BackofficeToken})

	app := fiber.New(handlers.WithTrustedProxies(handlers.ServerConfig(handlers.DefaultTimeouts), cfg.TrustedProxies))
	// Порядок: recover (самый внешний) → номер запроса → fiberzap → routes.
	// Номер должен появиться до журнала запросов, чтобы request_id был в контексте раньше.
	app.Use(recover.New(recover.Config{
		EnableStackTrace: true,
		StackTraceHandler: func(c *fiber.Ctx, e any) {
			log.Error("panic", zap.String("request_id", handlers.TraceID(c)), zap.Any("panic", e),
				zap.ByteString("stack", debug.Stack()))
		},
	}))
	app.Use(handlers.RequestID)
	// path, а не url: строка запроса (поисковые фразы покупателей) в журнал не попадает.
	app.Use(fiberzap.New(fiberzap.Config{
		Logger: log,
		Fields: []string{"ip", "latency", "status", "method", "path"},
		FieldsFunc: func(c *fiber.Ctx) []zap.Field {
			return []zap.Field{zap.String("request_id", handlers.TraceID(c))}
		},
		Messages: []string{"http: ошибка сервера", "http: ошибка клиента", "http"},
	}))
	handlers.RegisterRoutes(app, h)

	go func() {
		<-ctx.Done()
		log.Info("остановка: сигнал получен")
		_ = app.ShutdownWithTimeout(10 * time.Second)
	}()

	log.Info("HTTP слушает", zap.String("addr", cfg.Addr))
	if err := app.Listen(cfg.Addr); err != nil {
		log.Fatal("HTTP", zap.Error(err))
	}
}

// Сроки вызовов внешних сервисов. Импорт считает векторы пачками по 32 — первый вызов прогревает модель,
// поэтому срок сервиса моделей длинный; поиск ставит на вектор запроса свой короткий срок.
const (
	searchTimeout = 30 * time.Second
	modelsTimeout = 120 * time.Second
)

// llmTimeout — срок HTTP-вызова модели; разбор ставит свой, более короткий срок через контекст.
const llmTimeout = 30 * time.Second

// intentModel — клиент модели разбора; nil, если модель не настроена или имя пустое.
func intentModel(c config.LLMConfig, model string) services.ChatModel {
	if !c.Online() || model == "" {
		return nil
	}
	return llm.New(c.BaseURL, model, c.FolderID, c.APIKey, llmTimeout)
}

// agentLLMTimeout — срок HTTP-вызова модели агента: ответ с вызовами инструментов длиннее разбора.
const agentLLMTimeout = 90 * time.Second

// agentModel — клиент модели агента; nil, если модель не настроена.
func agentModel(c config.LLMConfig, model string) services.ChatModel {
	if !c.Online() || model == "" {
		return nil
	}
	return llm.New(c.BaseURL, model, c.FolderID, c.APIKey, agentLLMTimeout)
}

// runImport — `go run . import`: импорт и сводка в stdout; сбой — код выхода 1.
func runImport(ctx context.Context, log *zap.Logger, svc *services.Services) {
	res, err := svc.ImportCatalog(ctx)
	if err != nil {
		log.Fatal("импорт", zap.Error(err))
	}
	// Исторические логи поведения — для аналитики и агента (сводка — в журнале).
	if _, err := svc.ImportSearchLogs(ctx); err != nil {
		log.Fatal("импорт логов", zap.Error(err))
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	_ = enc.Encode(res)
}

// newLogger — журнал процесса: в production JSON (production-конфиг zap), иначе консольный вывод
// development-конфига без стека на каждом предупреждении (стек паники пишет recover сам).
func newLogger(env string) *zap.Logger {
	if env == config.EnvProduction {
		return zap.Must(zap.NewProduction())
	}
	c := zap.NewDevelopmentConfig()
	c.DisableStacktrace = true
	return zap.Must(c.Build())
}
