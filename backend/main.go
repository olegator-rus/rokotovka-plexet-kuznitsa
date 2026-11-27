// API PLEXeT SearchOps — умный поиск для B2B-магазина ПО. HTTP-сервер Fiber:
// handlers → services → repositories; раскладка и соглашения — AGENTS.md.
//
// `go run .` — HTTP-сервер; `go run . import` — импорт каталога из DATA_DIR в OpenSearch и PostgreSQL
// (то же, что POST /api/catalog/import), сводка — JSON в stdout; `go run . warmup` — прогрев кэша разборов
// частотными запросами; `go run . traffic` — генератор mock-трафика стенда (traffic_cli.go).
//
// CORS не подключён намеренно: SPA и /api на одном origin (Caddy в проде, прокси Vite в dev).
package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"kuznica/api/config"
	"kuznica/api/externalApi/llm"
	"kuznica/api/externalApi/ocr"
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
	// Генератор трафика — HTTP-клиент витрины: без конфигурации API, базы и миграции (traffic_cli.go).
	if len(os.Args) > 1 && os.Args[1] == "traffic" {
		os.Exit(runTraffic(os.Args[2:]))
	}

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
		PicturesFile:   picturesFile(cfg.DataMapping),
		IndexAlias:     cfg.IndexAlias,
		LLMMode:        cfg.LLM.Mode,

		IntentModel:    intentModel(cfg.LLM, cfg.LLM.IntentModel),
		IntentFallback: intentModel(cfg.LLM, cfg.LLM.FallbackModel),
		JudgeModel:     intentModel(cfg.LLM, cfg.LLM.JudgeModel),

		ConsultantModel: intentModel(cfg.LLM, cfg.LLM.ConsultantReplyModel),
		ConsultantLinks: services.ConsultantLinks{Payment: consultantLink("CONSULTANT_LINK_PAYMENT"),
			Delivery: consultantLink("CONSULTANT_LINK_DELIVERY"), Contacts: consultantLink("CONSULTANT_LINK_CONTACTS")},

		AgentModel: agentModel(cfg.LLM, cfg.Agent.Model),
		Agent: services.AgentOptions{MaxRegressionWorse: cfg.Agent.MaxRegressionWorse, RegressionSets: cfg.Agent.RegressionSets,
			FeedRegressionSets: cfg.Agent.FeedRegressionSets, Holdout: services.HoldoutFromEnv(), ReadOnly: cfg.Agent.ReadOnly,
			MaxParallel: cfg.Agent.MaxParallel,
			Budget: services.RunBudget{Steps: cfg.Agent.Budget.Steps, Tokens: cfg.Agent.Budget.Tokens,
				CostRub: cfg.Agent.Budget.CostRub, Duration: cfg.Agent.Budget.Duration},
			QueryDuration: cfg.Agent.QueryDuration,
			Triggers: services.AgentTriggers{Schedule: cfg.Agent.Schedule, ScheduleAt: cfg.Agent.ScheduleAt,
				Interval: cfg.Agent.Interval, ScheduleDays: cfg.Agent.ScheduleDays, Spike: cfg.Agent.Spike.Enabled,
				SpikeT: services.SpikeThresholds{Window: cfg.Agent.Spike.Window, MinSearches: cfg.Agent.Spike.MinSearches,
					EmptyShare: cfg.Agent.Spike.EmptyShare, ReformShare: cfg.Agent.Spike.ReformShare,
					QueryFreq: cfg.Agent.Spike.QueryFreq, Cooldown: cfg.Agent.Spike.Cooldown},
				Catalog: cfg.Agent.CatalogTrigger, Check: cfg.Agent.TriggerCheck}},

		DailyBudgetRub:  cfg.Ops.DailyBudgetRub,
		AgentReserveRub: cfg.Ops.AgentReserveRub,
		PerIPDailyRub:   cfg.Ops.PerIPDailyRub,

		Features: services.Features{Hardware: cfg.Features.Hardware, Companions: cfg.Features.Companions,
			MarginPolicy: cfg.Features.MarginPolicy, Solutions: cfg.Features.Solutions, ResultsV3: cfg.Features.ResultsV3,
			Panel: cfg.Features.Panel, PanelProducts: cfg.Features.PanelProducts, PanelMargin: cfg.Features.PanelMargin,
			PanelRecs: cfg.Features.PanelRecs, PanelViewed: cfg.Features.PanelViewed},
		NoFallbackBlock: !cfg.Features.FallbackBlock,
		UpsellTitle:     cfg.UpsellTitle,
		UpsellRatio:     cfg.UpsellSameVendorMaxRatio,
	})
	// Контур ТЗ: распознавание сканов ТЗ (Yandex Vision OCR); не настроено — скан получает причину ocr.
	// Запасной путь — локальный tesseract (в образе API), если Vision OCR отказал.
	if chain := tzOCRChain(config.LoadOCR(cfg.LLM)); len(chain) > 0 {
		svc.SetTZOCR(chain)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if len(os.Args) > 1 && os.Args[1] == "import" {
		runImport(ctx, log, svc)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "import-refs" {
		runImportRefs(ctx, log, svc)
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
	if len(os.Args) > 1 && os.Args[1] == "guide-eval" {
		svc.SetGuideMinFactStatus(cfg.GuideMinFactStatus) // порог показа фактов — как у API
		runGuideEval(ctx, log, svc, os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "judge-calib" {
		runJudgeCalib(ctx, log, svc, os.Args[2:])
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
	if len(os.Args) > 1 && os.Args[1] == "margin" {
		runMargin(ctx, log, svc, os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "suggest-build" {
		runSuggestBuild(ctx, log, svc, os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "suggest-bench" {
		runSuggestBench(ctx, log, svc, os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "warmup" {
		runWarmup(ctx, log, svc, os.Args[2:])
		return
	}
	svc.LoadCatalogState(ctx)
	// Контур подбора: порог показа фактов подбора.
	svc.SetGuideMinFactStatus(cfg.GuideMinFactStatus)
	go func() { _ = svc.WarmPanel(ctx) }() // словарь подсказок панели — в фоне, старт не ждёт
	svc.RecoverAgentRuns(ctx)
	svc.StartAgentTriggers(ctx)
	defer svc.Wait()
	h := handlers.NewHandlerManager(svc, handlers.Options{PublicOrigin: cfg.PublicOrigin,
		AdminEnabled: cfg.Ops.AdminEnabled, RatePerMinute: cfg.Ops.RatePerMinute, BackofficeToken: cfg.Ops.BackofficeToken,
		Auth: cfg.Auth, SecureCookie: cfg.IsProduction()})

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

// ocrTimeout — срок распознавания одной страницы скана.
const ocrTimeout = 40 * time.Second

// tzOCRChain — распознавание сканов ТЗ: Vision OCR (если настроен), затем tesseract (если установлен).
func tzOCRChain(oc config.OCRConfig) ocr.Chain {
	var chain ocr.Chain
	if oc.Online() {
		chain = append(chain, ocr.New(oc.URL, oc.APIKey, oc.FolderID, ocrTimeout))
	}
	if t := ocr.NewTesseract(); t != nil {
		chain = append(chain, t)
	}
	return chain
}

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

// runImportRefs — `go run . import-refs`: только справочники вне фида (связи, клиенты, шаблоны решений, правила)
// в базу, без индекса и векторов; сбой — код выхода 1.
func runImportRefs(ctx context.Context, log *zap.Logger, svc *services.Services) {
	res, err := svc.ImportReferences(ctx)
	if err != nil {
		log.Fatal("импорт справочников", zap.Error(err))
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

// picturesFile — картинки позиций фида (scripts/data/softline_feed.py пишет feed/pictures.json рядом с mapping.yaml);
// у mock и без файла — пусто, карточки показывают значок производителя.
func picturesFile(mapping string) string {
	if mapping == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(mapping), "feed", "pictures.json")
}

// consultantLink — адрес сервисной темы консультанта из окружения: только https://store.softline.ru/…; иначе пусто.
func consultantLink(key string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if strings.HasPrefix(v, "https://store.softline.ru/") {
		return v
	}
	return ""
}
