package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"kuznica/api/services"

	"go.uber.org/zap"
)

// runSuggestBuild — `go run . suggest-build`: индекс подсказок контура выдачи — новая версия
// config_versions kind = suggestions и строки в suggestions. Работающий API подхватывает её за ≤ 30 с.
func runSuggestBuild(ctx context.Context, log *zap.Logger, svc *services.Services, _ []string) {
	svc.LoadCatalogState(ctx)
	defer svc.Wait()
	info, err := svc.SuggestBuild(ctx)
	if err != nil {
		log.Fatal("подсказки: сборка", zap.Error(err))
	}
	raw, _ := json.MarshalIndent(info, "", "  ")
	fmt.Println(string(raw))
}

// runSuggestBench — `go run . suggest-bench --file prefixes.json`: задержки панели на сервере без HTTP и без записи
// трасс. Для каждого префикса — промах кэша товаров (NoCache), затем ответ из прогретого кэша. Файл — массив строк
// или набор §11.6 (`[{"prefix": …}]`). Проверки AC: вызовы моделей, дубли семейств в слотах, точный артикул
// первым, подсказки с пустой текстовой веткой.
func runSuggestBench(ctx context.Context, log *zap.Logger, svc *services.Services, args []string) {
	fs := flag.NewFlagSet("suggest-bench", flag.ExitOnError)
	file := fs.String("file", "../data/mock/prefixes.json", "префиксы: массив строк или [{\"prefix\": …}]")
	asJSON := fs.Bool("json", false, "итог в JSON")
	_ = fs.Parse(args)
	raw, err := os.ReadFile(*file)
	if err != nil {
		log.Fatal("подсказки: файл префиксов", zap.Error(err))
	}
	var prefixes []string
	if json.Unmarshal(raw, &prefixes) != nil {
		prefixes = nil
		var items []struct {
			Prefix string `json:"prefix"`
		}
		if err := json.Unmarshal(raw, &items); err != nil {
			log.Fatal("подсказки: формат префиксов", zap.Error(err))
		}
		for _, it := range items {
			prefixes = append(prefixes, it.Prefix)
		}
	}
	svc.LoadCatalogState(ctx)
	defer svc.Wait()
	if err := svc.WarmPanel(ctx); err != nil {
		log.Fatal("подсказки: словарь", zap.Error(err))
	}
	res, err := svc.BenchPanel(ctx, prefixes)
	if err != nil {
		log.Fatal("подсказки: замер", zap.Error(err))
	}
	if *asJSON {
		out, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(out))
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Панель: %d префиксов · словарь v%d (%d строк) · %s\n", res.Prefixes, res.SuggestionsVersion, res.DictEntries,
		time.Now().Format(time.RFC3339))
	keys := make([]string, 0, len(res.Latency))
	for k := range res.Latency {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		l := res.Latency[k]
		fmt.Fprintf(&b, "  %-22s p50 %7.2f мс · p95 %7.2f мс · max %7.2f мс (n=%d)\n", k, l.P50, l.P95, l.Max, l.N)
	}
	fmt.Fprintf(&b, "  частичных (товары не успели): %d · llm %d · embed %d · rerank %d\n", res.Partial, res.LLMCalls, res.EmbedCalls, res.RerankCalls)
	fmt.Fprintf(&b, "  дублей семейств в слотах: %d · точный артикул не первым: %d из %d · подсказок без результатов: %d из %d\n",
		res.FamilyDupes, res.ExactShifted, res.ExactChecked, res.EmptySuggestions, res.SuggestionsChecked)
	_, _ = os.Stdout.WriteString(b.String())
}
