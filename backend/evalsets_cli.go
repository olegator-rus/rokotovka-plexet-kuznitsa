package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"kuznica/api/domain"
	"kuznica/api/externalApi/catalogsource"
	"kuznica/api/services"

	"go.uber.org/zap"
)

// runBuildSets — `go run . buildsets --from-logs [--days N] [--out DIR]` или `--events FILE`: наборы оценки
// tune/test/regression и разметка из журнала (неявная обратная связь + LLM-судья). Пишет DIR/queries.json
//, DIR/SETS.md и DIR/sets-report.json. По умолчанию DIR — каталог данных (DATA_DIR): там их
// подхватят `make eval`, судья и исключение test из входа агента.
func runBuildSets(ctx context.Context, log *zap.Logger, svc *services.Services, dataDir string, classifier services.ChatModel, args []string) {
	fs := flag.NewFlagSet("buildsets", flag.ExitOnError)
	fromLogs := fs.Bool("from-logs", false, "журнал из базы: search_events и behavior_events (импорт логов), без live")
	events := fs.String("events", "", "журнал из файла events.jsonl вместо базы")
	live := fs.Bool("live", false, "с --from-logs: включить и живые поиски (source=live)")
	days := fs.Int("days", 0, "только последние N дней журнала; 0 — весь")
	minFreq := fs.Int("min-freq", 2, "запросы с частотой не ниже")
	minSupport := fs.Int("min-support", 1, "поисков с действием, с которых товар получает неявную оценку")
	clickGrade := fs.Int("click-grade", 2, "оценка «помогшего» клика без корзины: 2 или 1 (строгая схема)")
	judgeRub := fs.Float64("judge-rub", 100, "лимит стоимости судьи, ₽; 0 — без судьи")
	calib := fs.Int("calib", 100, "пар с неявной оценкой отдать судье для согласия")
	classifyRub := fs.Float64("classify-rub", 5, "лимит стоимости классификации типов, ₽")
	noLLM := fs.Bool("no-classify-llm", false, "типы только правилами")
	outDir := fs.String("out", dataDir, "куда записать queries.json, SETS.md, sets-report.json")
	reference := fs.String("reference", "", "queries.json с разметкой по построению для сверки (mock)")
	source := fs.String("source", "", "подпись источника в SETS.md; пусто — имя каталога --out")
	force := fs.Bool("force", false, "перезаписать queries.json, если он уже есть")
	_ = fs.Parse(args)

	if *fromLogs == (*events != "") {
		log.Fatal("buildsets: нужен ровно один источник журнала — --from-logs или --events FILE")
	}
	target := filepath.Join(*outDir, "queries.json")
	if _, err := os.Stat(target); err == nil && !*force {
		log.Fatal("buildsets: queries.json уже есть — укажите другой --out или --force", zap.String("path", target))
	}
	req := services.BuildSetsRequest{IncludeLive: *live, Days: *days, MinFreq: *minFreq, MinSupport: *minSupport, ClickGrade: *clickGrade,
		JudgeMaxRub: *judgeRub, JudgeCalib: *calib, ClassifyMaxRub: *classifyRub, CacheDir: *outDir,
		Progress: func(s string) { fmt.Fprintln(os.Stderr, "buildsets: "+s) }}
	if !*noLLM {
		req.Classifier = classifier
	}
	if *events != "" {
		dir := *events
		if st, err := os.Stat(dir); err == nil && !st.IsDir() {
			if filepath.Base(dir) != "events.jsonl" {
				log.Fatal("buildsets: --events — каталог с events.jsonl или сам файл events.jsonl")
			}
			dir = filepath.Dir(dir)
		}
		evs, err := catalogsource.MockJSON{Dir: dir}.LoadEvents(ctx)
		if err != nil {
			log.Fatal("buildsets: журнал", zap.Error(err))
		}
		req.Events = evs
	}
	if *reference != "" {
		var ref []domain.EvalQuery
		raw, err := os.ReadFile(*reference)
		if err == nil {
			err = json.Unmarshal(raw, &ref)
		}
		if err != nil {
			log.Fatal("buildsets: эталон", zap.Error(err))
		}
		req.Reference = ref
	}

	svc.LoadCatalogState(ctx)
	defer svc.Wait()
	res, err := svc.BuildSets(ctx, req)
	if err != nil {
		log.Fatal("buildsets", zap.Error(err))
	}
	if *source == "" {
		*source = filepath.Base(filepath.Clean(*outDir))
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatal("buildsets: каталог", zap.Error(err))
	}
	md := services.FormatSetsMarkdown(res.Report, *source)
	for name, v := range map[string]any{"queries.json": services.SetsQueriesJSON(res.Queries), "sets-report.json": res.Report} {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		err := enc.Encode(v)
		if err == nil {
			err = os.WriteFile(filepath.Join(*outDir, name), buf.Bytes(), 0o644)
		}
		if err != nil {
			log.Fatal("buildsets: запись "+name, zap.Error(err))
		}
	}
	if err := os.WriteFile(filepath.Join(*outDir, "SETS.md"), []byte(md), 0o644); err != nil {
		log.Fatal("buildsets: запись SETS.md", zap.Error(err))
	}
	fmt.Print(md)
}
