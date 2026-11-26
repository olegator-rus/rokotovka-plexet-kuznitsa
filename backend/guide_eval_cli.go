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

// runGuideEval — `go run . guide-eval [--runs 3] [--report ../data/softline/eval/results/REPORT.md]`: оценка режима
// подбора — диалоги набора через сервис подбора, автопроверки, сравнение с поиском и с подбором без проверок.
func runGuideEval(ctx context.Context, log *zap.Logger, svc *services.Services, args []string) {
	fs := flag.NewFlagSet("guide-eval", flag.ExitOnError)
	file := fs.String("file", "../data/softline/eval/guide-dialogs.jsonl", "набор диалогов (JSONL)")
	kb := fs.String("kb", "../data/softline/kb", "каталог базы знаний")
	runs := fs.Int("runs", 3, "прогонов полного подбора")
	report := fs.String("report", "", "куда записать Markdown-отчёт")
	outDir := fs.String("out", "../data/eval", "куда записать JSON прогона; пусто — не записывать")
	noPersist := fs.Bool("no-persist", false, "не записывать результат в базу")
	_ = fs.Parse(args)

	svc.LoadCatalogState(ctx)
	res, err := svc.RunGuideEval(ctx, services.GuideEvalRequest{File: *file, KBDir: *kb, Runs: *runs,
		Progress: func(s string) { fmt.Fprintln(os.Stderr, "guide-eval: "+s) }})
	if err != nil {
		log.Fatal("оценка подбора", zap.Error(err))
	}
	svc.Wait() // журнал хода и llm_usage пишутся в фоне
	if err := svc.FinishGuideEval(ctx, res, !*noPersist); err != nil {
		log.Error("оценка подбора", zap.Error(err))
	}
	md := services.FormatGuideReport(res, "go run . guide-eval "+strings.Join(args, " "))
	if *report != "" {
		if err := os.MkdirAll(filepath.Dir(*report), 0o755); err == nil {
			err = os.WriteFile(*report, []byte(md), 0o644)
		}
		if err != nil {
			log.Fatal("оценка подбора: отчёт", zap.Error(err))
		}
	}
	if *outDir != "" {
		_ = os.MkdirAll(*outDir, 0o755)
		raw, _ := json.MarshalIndent(res, "", "  ")
		_ = os.WriteFile(filepath.Join(*outDir, "guide-"+res.RunID+".json"), raw, 0o644)
	}
	fmt.Print(md)
}
