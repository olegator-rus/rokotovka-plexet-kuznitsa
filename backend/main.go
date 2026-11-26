// API магазина (шаблон хакатона «AI Кузница», кейс поиска). HTTP-сервер Fiber:
// handlers → services → repositories; раскладка и соглашения — AGENTS.md.
//
// CORS не подключён намеренно: SPA и /api на одном origin (Caddy в проде, прокси Vite в dev).
package main

import (
	"context"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"kuznica/api/config"
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

	svc := services.NewServices(repos, services.Options{})
	h := handlers.NewHandlerManager(svc, handlers.Options{PublicOrigin: cfg.PublicOrigin})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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
