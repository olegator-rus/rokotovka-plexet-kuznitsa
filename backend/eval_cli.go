package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kuznica/api/services"

	"go.uber.org/zap"
)

// runEval — `go run . eval --set test --configs B0,B1,B2,B3 [--out ../data/eval] [--no-persist]`:
// прогон набора через сервис поиска, таблица в stdout, запись в evaluations и JSON в --out.
func runEval(ctx context.Context, log *zap.Logger, svc *services.Services, args []string) {
	fs := flag.NewFlagSet("eval", flag.ExitOnError)
	set := fs.String("set", "test", "набор: tune | test | regression | demo")
	configs := fs.String("configs", strings.Join(services.DefaultEvalConfigs, ","), "конфигурации через запятую")
	outDir := fs.String("out", "../data/eval", "куда записать JSON прогона; пусто — не записывать")
	noPersist := fs.Bool("no-persist", false, "не записывать результат в базу")
	_ = fs.Parse(args)

	svc.LoadCatalogState(ctx)
	defer svc.Wait()
	run, err := svc.RunEvaluation(ctx, services.EvalRequest{
		Set: *set, Configs: strings.Split(*configs, ","), Persist: !*noPersist,
	})
	if err != nil {
		if run == nil {
			log.Fatal("оценка", zap.Error(err))
		}
		log.Error("оценка", zap.Error(err))
	}
	if *outDir != "" {
		if err := os.MkdirAll(*outDir, 0o755); err != nil {
			log.Fatal("оценка: каталог результатов", zap.Error(err))
		}
		run.File = filepath.Join(*outDir, run.Set+"-"+run.RunID+".json")
		raw, err := json.MarshalIndent(run, "", "  ")
		if err == nil {
			err = os.WriteFile(run.File, raw, 0o644)
		}
		if err != nil {
			log.Fatal("оценка: JSON прогона", zap.Error(err))
		}
	}
	fmt.Print(services.FormatEvalTable(run))
}

// runImportLogs — `go run . import-logs`: только исторические логи (events.jsonl) в журналы, без
// перестройки индекса. `go run . import` делает это сам после каталога.
func runImportLogs(ctx context.Context, log *zap.Logger, svc *services.Services) {
	res, err := svc.ImportSearchLogs(ctx)
	if err != nil {
		log.Fatal("импорт логов", zap.Error(err))
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(res)
}

// runJudge — `go run . judge --set test [--configs B0,B1,B2,B3] [--max-pairs 600] [--out ../data/eval]`: LLM-судья
// (другое семейство моделей) вслепую оценивает пул топ-10 конфигураций; согласие с разметкой — в stdout,
// в judge_runs (/api/evaluations → judge_agreement) и в JSON.
func runJudge(ctx context.Context, log *zap.Logger, svc *services.Services, args []string) {
	fs := flag.NewFlagSet("judge", flag.ExitOnError)
	set := fs.String("set", "test", "набор: tune | test | regression | demo")
	configs := fs.String("configs", strings.Join(services.DefaultEvalConfigs, ","), "конфигурации пула через запятую")
	maxPairs := fs.Int("max-pairs", 600, "пар в пуле, не больше 600")
	outDir := fs.String("out", "../data/eval", "куда записать JSON прогона; пусто — не записывать")
	noPersist := fs.Bool("no-persist", false, "не записывать результат в базу")
	_ = fs.Parse(args)

	svc.LoadCatalogState(ctx)
	defer svc.Wait()
	run, err := svc.RunJudge(ctx, services.JudgeRequest{
		Set: *set, Configs: strings.Split(*configs, ","), MaxPairs: *maxPairs, Persist: !*noPersist,
	})
	if err != nil {
		if run == nil {
			log.Fatal("судья", zap.Error(err))
		}
		log.Error("судья", zap.Error(err))
	}
	if *outDir != "" {
		if err := os.MkdirAll(*outDir, 0o755); err != nil {
			log.Fatal("судья: каталог результатов", zap.Error(err))
		}
		run.File = filepath.Join(*outDir, "judge-"+run.Set+"-"+run.RunID+".json")
		raw, err := json.MarshalIndent(run, "", "  ")
		if err == nil {
			err = os.WriteFile(run.File, raw, 0o644)
		}
		if err != nil {
			log.Fatal("судья: JSON прогона", zap.Error(err))
		}
	}
	fmt.Print(services.FormatJudgeTable(run))
}

// runWarmup — `go run . warmup [--limit 100] [--configs B3,B4] [--days 90]`: прогрев кэша разборов частотными
// запросами из журнала поиска и наборов; сводка — в stdout.
func runWarmup(ctx context.Context, log *zap.Logger, svc *services.Services, args []string) {
	fs := flag.NewFlagSet("warmup", flag.ExitOnError)
	limit := fs.Int("limit", 100, "сколько самых частотных запросов прогреть")
	configs := fs.String("configs", "B3,B4", "конфигурации через запятую")
	days := fs.Int("days", 90, "за сколько дней брать журнал поиска")
	_ = fs.Parse(args)

	svc.LoadCatalogState(ctx)
	defer svc.Wait()
	res, err := svc.Warmup(ctx, services.WarmupRequest{Limit: *limit, Configs: strings.Split(*configs, ","), Days: *days})
	if err != nil {
		if res == nil {
			log.Fatal("прогрев", zap.Error(err))
		}
		log.Error("прогрев", zap.Error(err))
	}
	fmt.Print(res.String())
}
