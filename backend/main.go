// API PLEXeT SearchOps — умный поиск для B2B-магазина ПО. HTTP-сервер Fiber:
// handlers → services → repositories; раскладка и соглашения — AGENTS.md.
//
// `go run .` — HTTP-сервер; `go run . import` — импорт каталога из DATA_DIR в OpenSearch и PostgreSQL
// (то же, что POST /api/catalog/import), сводка — JSON в stdout.
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
	"kuznica/api/externalApi/catalogsource"
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

	svc := services.NewServices(repos, services.Options{
		Search:   opensearch.New(cfg.OpenSearchURL, searchTimeout),
		Embedder: tei.New(cfg.EmbedderURL, modelsTimeout),
		Reranker: tei.New(cfg.RerankerURL, modelsTimeout),
		Source:   catalogsource.MockJSON{Dir: cfg.DataDir},
		LLMMode:  cfg.LLM.Mode,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if len(os.Args) > 1 && os.Args[1] == "import" {
		runImport(ctx, log, svc)
		return
	}
	svc.LoadCatalogState(ctx)
	defer svc.Wait()
	h := handlers.NewHandlerManager(svc, handlers.Options{PublicOrigin: cfg.PublicOrigin})

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

// runImport — `go run . import`: импорт и сводка в stdout; сбой — код выхода 1.
func runImport(ctx context.Context, log *zap.Logger, svc *services.Services) {
	res, err := svc.ImportCatalog(ctx)
	if err != nil {
		log.Fatal("импорт", zap.Error(err))
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
