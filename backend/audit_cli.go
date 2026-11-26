package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"kuznica/api/config"
	"kuznica/api/externalApi/catalogsource"
	"kuznica/api/services"

	"go.uber.org/zap"
)

// dataSource — источник каталога по DATA_SOURCE: mock (JSON контракта из DATA_DIR) или generic (любые
// таблицы по mapping.yaml, недостающее восстанавливает services.RestoreCatalog). Битый mapping.yaml —
// отказ при старте.
func dataSource(log *zap.Logger, cfg config.Config) services.CatalogSource {
	if cfg.DataSource != config.DataSourceGeneric {
		return catalogsource.MockJSON{Dir: cfg.DataDir}
	}
	g, err := catalogsource.NewGeneric(cfg.DataMapping, services.RestoreCatalog)
	if err != nil {
		log.Fatal("DATA_MAPPING", zap.Error(err))
	}
	return g
}

// setsDir — куда `go run . buildsets` пишет queries.json: каталог данных источника (DATA_DIR у mock,
// base_dir из mapping.yaml у generic) — там наборы читает метод LoadQueries того же источника.
func setsDir(cfg config.Config, src services.CatalogSource) string {
	if g, ok := src.(*catalogsource.Generic); ok {
		return g.DataDir()
	}
	return cfg.DataDir
}

// runAudit — `go run . audit [--out ../data] [--limit 300] [--no-search]`: аудит данных источника —
// AUDIT.md и AUDIT.json в --out. Поиск идёт по текущему индексу без журнала и без LLM (маршрут D — на
// кэше разборов и правилах): аудит ничего не тратит.
func runAudit(ctx context.Context, log *zap.Logger, svc *services.Services, args []string) {
	fs := flag.NewFlagSet("audit", flag.ExitOnError)
	outDir := fs.String("out", "../data", "куда записать AUDIT.md и AUDIT.json")
	limit := fs.Int("limit", 300, "сколько самых частых запросов прогнать через B0 и B3")
	noSearch := fs.Bool("no-search", false, "без прогона поиска (индекса ещё нет)")
	_ = fs.Parse(args)

	if _, err := svc.SetLLMMode(ctx, "off"); err != nil {
		log.Fatal("аудит: LLM не выключена", zap.Error(err))
	}
	svc.LoadCatalogState(ctx)
	defer svc.Wait()
	rep, err := svc.Audit(ctx, services.AuditOptions{Limit: *limit, NoSearch: *noSearch})
	if err != nil {
		log.Fatal("аудит", zap.Error(err))
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatal("аудит: каталог", zap.Error(err))
	}
	md := services.FormatAuditMarkdown(rep)
	raw, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		log.Fatal("аудит: JSON", zap.Error(err))
	}
	for name, body := range map[string][]byte{"AUDIT.md": []byte(md), "AUDIT.json": raw} {
		if err := os.WriteFile(filepath.Join(*outDir, name), body, 0o644); err != nil {
			log.Fatal("аудит: запись", zap.String("file", name), zap.Error(err))
		}
	}
	fmt.Print(md)
}
