package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/services"

	"go.uber.org/zap"
)

// Оценка «по цели» на фиде заказчика (контур эталонов, services/eval_target*.go):
//
//	go run . eval --set etalon,tz,neighbors --cold --report ../data/softline/eval/report   # BM25, B1, B2, live
//	go run . eval --set etalon,tz,neighbors --configs live --report ../data/softline/eval/report
//	go run . eval --set closed --file <путь закрытого набора> --report …                    # один раз, при заморозке
//	go run . eval --summary ../data/softline/eval/report/REPORT.md --header "…"            # сводка S6 в Markdown

// targetSetsArg — наборы «по цели» из --set (через запятую); не «по цели» — nil.
func targetSetsArg(v string) []string {
	var sets []string
	for _, s := range strings.Split(v, ",") {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		if !domain.IsTargetSet(s) {
			return nil
		}
		sets = append(sets, s)
	}
	return sets
}

type targetEvalArgs struct {
	sets              []string
	file              string
	configs           []string
	cold              bool
	outDir, reportDir string
	persist           bool
	command           string
}

func runTargetEval(ctx context.Context, log *zap.Logger, svc *services.Services, a targetEvalArgs) {
	for _, set := range a.sets {
		if set == domain.TargetSetClosed && a.file == "" {
			log.Fatal(services.MsgTargetClosed)
		}
		file := ""
		if set == domain.TargetSetClosed || len(a.sets) == 1 {
			file = a.file
		}
		run, err := svc.RunTargetEvaluation(ctx, services.TargetEvalRequest{Set: set, File: file, Configs: a.configs, Cold: a.cold,
			Progress: func(s string) { fmt.Fprintln(os.Stderr, "eval: "+s) }})
		if err != nil {
			log.Fatal("оценка по цели", zap.String("set", set), zap.Error(err))
		}
		svc.Wait() // строки llm_usage пишутся в фоне
		if err := svc.FinishTargetEvaluation(ctx, run, a.persist); err != nil {
			log.Error("оценка по цели", zap.Error(err))
		}
		mode := services.TargetModeWarm
		if a.cold {
			mode = services.TargetModeCold
		}
		if a.outDir != "" {
			if err := os.MkdirAll(a.outDir, 0o755); err != nil {
				log.Fatal("оценка по цели: каталог результатов", zap.Error(err))
			}
			run.File = filepath.Join(a.outDir, "target-"+set+"-"+mode+"-"+run.RunID+".json")
			raw, err := json.MarshalIndent(run, "", "  ")
			if err == nil {
				err = os.WriteFile(run.File, raw, 0o644)
			}
			if err != nil {
				log.Fatal("оценка по цели: JSON прогона", zap.Error(err))
			}
		}
		if a.reportDir != "" {
			if err := os.MkdirAll(a.reportDir, 0o755); err != nil {
				log.Fatal("оценка по цели: каталог отчётов", zap.Error(err))
			}
			sum, _ := svc.FeedSummary(ctx)
			md := services.FormatTargetReport(run, a.command, sum.Catalog)
			path := filepath.Join(a.reportDir, "target-"+set+"-"+mode+".md")
			if err := os.WriteFile(path, []byte(md), 0o644); err != nil {
				log.Fatal("оценка по цели: отчёт", zap.Error(err))
			}
			fmt.Fprintln(os.Stderr, "eval: отчёт "+path)
		}
		fmt.Print(services.FormatTargetTable(run))
	}
}

// runFeedSummary — сводка «На данных заказчика» (те же числа, что GET /api/evaluations/feed) в Markdown.
func runFeedSummary(ctx context.Context, log *zap.Logger, svc *services.Services, path string, header []string) {
	sum, err := svc.FeedSummary(ctx)
	if err != nil {
		log.Fatal("сводка на фиде", zap.Error(err))
	}
	md := services.FormatFeedReport(sum, header)
	if err := os.WriteFile(path, []byte(md), 0o644); err != nil {
		log.Fatal("сводка на фиде: запись", zap.Error(err))
	}
	fmt.Print(md)
}
