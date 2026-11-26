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
