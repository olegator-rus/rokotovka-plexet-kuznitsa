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
	set := fs.String("set", "test", "набор: tune | test | regression | demo | holdout_week (реплей отложенной недели журнала) | panel (префиксы панели) | showcase (прогон витрины); «по цели» на фиде — etalon, tz, neighbors, understanding, feed_tune, feed_regression, closed (через запятую — несколько)")
	configs := fs.String("configs", strings.Join(services.DefaultEvalConfigs, ","), "конфигурации через запятую")
	outDir := fs.String("out", "../data/eval", "куда записать JSON прогона; пусто — не записывать")
	noPersist := fs.Bool("no-persist", false, "не записывать результат в базу")
	proposal := fs.String("with-proposal", "", "id ждущего предложения: эффект его правки в песочнице (B3, B4, B4+правка; в базу не пишется)")
	contribSet := fs.String("contrib-set", "test", "набор таблицы вклада компонентов при --set contribution: test | tune | regression")
	commerce := fs.Bool("commerce", true, "набор p2: коммерческая лестница C0–C3 и прогоны с одним изменением (false — обычная лестница B)")
	file := fs.String("file", "", "наборы «по цели»: явный путь JSONL (закрытый набор: --set closed --file …)")
	cold := fs.Bool("cold", false, "наборы «по цели»: холодный прогон — кэш разборов модели, векторов и реранкера не читается")
	report := fs.String("report", "", "наборы «по цели»: каталог Markdown-отчётов (target-<набор>-<режим>.md)")
	summary := fs.String("summary", "", "сводка «На данных заказчика» (S6) в Markdown по последним прогонам — путь файла")
	var header multiFlag
	fs.Var(&header, "header", "строка шапки сводки (--summary); можно несколько раз")
	_ = fs.Parse(args)
	cfgSet := false
	fs.Visit(func(f *flag.Flag) { cfgSet = cfgSet || f.Name == "configs" })

	svc.LoadCatalogState(ctx)
	defer svc.Wait()
	if *summary != "" {
		runFeedSummary(ctx, log, svc, *summary, header)
		return
	}
	if sets := targetSetsArg(*set); len(sets) > 0 {
		var cfgs []string
		if cfgSet {
			cfgs = strings.Split(*configs, ",")
		}
		runTargetEval(ctx, log, svc, targetEvalArgs{sets: sets, file: *file, configs: cfgs, cold: *cold, outDir: *outDir,
			reportDir: *report, persist: !*noPersist, command: "go run . eval " + strings.Join(args, " ")})
		return
	}
	if *set == "panel" {
		runPanelEval(ctx, log, svc, *outDir, !*noPersist)
		return
	}
	if *set == "showcase" {
		runShowcaseEval(ctx, log, svc, *outDir)
		return
	}
	if *set == "contribution" {
		runContributionEval(ctx, log, svc, *contribSet, *outDir, !*noPersist)
		return
	}
	if *set == "p2" && *commerce && *proposal == "" {
		runCommerceEval(ctx, log, svc, *outDir, !*noPersist)
		return
	}
	var run *services.EvalResult
	var err error
	if *proposal != "" {
		run, err = svc.RunProposalSandbox(ctx, *set, *proposal)
	} else {
		run, err = svc.RunEvaluation(ctx, services.EvalRequest{
			Set: *set, Configs: strings.Split(*configs, ","), Persist: !*noPersist,
		})
	}
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

// runCommerceEval — `make eval SET=p2`: коммерческая лестница C0–C3 — таблица, evaluations
// (GET /api/evaluations/commerce) и JSON в outDir.
func runCommerceEval(ctx context.Context, log *zap.Logger, svc *services.Services, outDir string, persist bool) {
	res, err := svc.RunCommerceEvaluation(ctx, persist)
	if err != nil {
		if res == nil {
			log.Fatal("коммерческая лестница", zap.Error(err))
		}
		log.Error("коммерческая лестница", zap.Error(err))
	}
	if outDir != "" {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			log.Fatal("коммерческая лестница: каталог результатов", zap.Error(err))
		}
		raw, err := json.MarshalIndent(res, "", "  ")
		if err == nil {
			err = os.WriteFile(filepath.Join(outDir, "commerce-"+res.RunID+".json"), raw, 0o644)
		}
		if err != nil {
			log.Fatal("коммерческая лестница: JSON прогона", zap.Error(err))
		}
	}
	fmt.Print(services.FormatCommerceTable(res))
}

// runContributionEval — `make eval SET=contribution [CONTRIB_SET=tune]`: таблица вклада компонентов (контур доводки) —
// stdout, evaluations (GET /api/evaluations/contribution) и JSON в outDir.
func runContributionEval(ctx context.Context, log *zap.Logger, svc *services.Services, set, outDir string, persist bool) {
	run, err := svc.RunContribution(ctx, set, persist, nil)
	if err != nil {
		if run == nil {
			log.Fatal("вклад компонентов", zap.Error(err))
		}
		log.Error("вклад компонентов", zap.Error(err))
	}
	if outDir != "" {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			log.Fatal("вклад компонентов: каталог результатов", zap.Error(err))
		}
		raw, err := json.MarshalIndent(run, "", "  ")
		if err == nil {
			err = os.WriteFile(filepath.Join(outDir, "contribution-"+run.Set+"-"+run.RunID+".json"), raw, 0o644)
		}
		if err != nil {
			log.Fatal("вклад компонентов: JSON прогона", zap.Error(err))
		}
	}
	fmt.Print(services.FormatContributionTable(run))
}

// runPanelEval — `make eval SET=panel`: прогон набора префиксов по прежним подсказкам и панели контура выдачи
// — таблица, evaluations (GET /api/evaluations?set=panel) и JSON в outDir.
func runPanelEval(ctx context.Context, log *zap.Logger, svc *services.Services, outDir string, persist bool) {
	run, err := svc.RunPanelEvaluation(ctx, persist)
	if err != nil {
		if run == nil {
			log.Fatal("оценка панели", zap.Error(err))
		}
		log.Error("оценка панели", zap.Error(err))
	}
	if outDir != "" {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			log.Fatal("оценка панели: каталог результатов", zap.Error(err))
		}
		raw, err := json.MarshalIndent(run, "", "  ")
		if err == nil {
			err = os.WriteFile(filepath.Join(outDir, "panel-"+run.RunID+".json"), raw, 0o644)
		}
		if err != nil {
			log.Fatal("оценка панели: JSON прогона", zap.Error(err))
		}
	}
	fmt.Print(services.FormatPanelTable(run))
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

// runProposals — `go run . proposals stale --run <id> [--run <id>] --reason "<текст>"`: ждущие предложения запусков
// до разреза журнала → stale с причиной pre_holdout (ревью-2 N2). Принятые не трогаются.
func runProposals(ctx context.Context, log *zap.Logger, svc *services.Services, args []string) {
	if len(args) == 0 || args[0] != "stale" {
		log.Fatal("proposals: известна только команда stale — go run . proposals stale --run <id> --reason \"<текст>\"")
	}
	fs := flag.NewFlagSet("proposals stale", flag.ExitOnError)
	var runs multiFlag
	fs.Var(&runs, "run", "id запуска агента (полный или первые 8+ знаков); можно несколько раз")
	reason := fs.String("reason", "", "почему предложения устарели (пишется в decided_comment)")
	_ = fs.Parse(args[1:])
	ids, err := svc.StaleProposalsOfRuns(ctx, runs, *reason)
	if err != nil {
		log.Fatal("proposals stale", zap.Error(err))
	}
	fmt.Printf("предложений переведено в stale (pre_holdout): %d\n", len(ids))
	for _, id := range ids {
		fmt.Println("  " + id)
	}
}

// multiFlag — повторяемый флаг.
type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

// runShowcaseEval — `make eval SET=showcase`: 31 запрос прогона витрины на mock-каталоге, доля верных классов
// ответа (контур доводки) — таблица в stdout и JSON в outDir; в базу не пишется.
func runShowcaseEval(ctx context.Context, log *zap.Logger, svc *services.Services, outDir string) {
	run, err := svc.RunShowcase(ctx)
	if err != nil {
		log.Fatal("набор витрины", zap.Error(err))
	}
	if outDir != "" {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			log.Fatal("набор витрины: каталог результатов", zap.Error(err))
		}
		raw, err := json.MarshalIndent(run, "", "  ")
		if err == nil {
			err = os.WriteFile(filepath.Join(outDir, "showcase-"+run.RunID+".json"), raw, 0o644)
		}
		if err != nil {
			log.Fatal("набор витрины: JSON прогона", zap.Error(err))
		}
	}
	fmt.Print(services.FormatShowcaseTable(run))
}
