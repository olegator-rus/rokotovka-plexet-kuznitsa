//go:build llmlive

package services

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"kuznica/api/config"
	"kuznica/api/domain"
	"kuznica/api/externalApi/catalogsource"
	"kuznica/api/externalApi/llm"
	"kuznica/api/repositories"

	"github.com/joho/godotenv"
)

// Живой прогон разбора на Yandex AI Studio: 10 сложных запросов из data/mock/queries.json, каждый дважды
// (второй раз — из кэша в PostgreSQL). Только вручную, тратит деньги:
//
//	cd backend && go test -tags llmlive ./services/ -run LiveIntent -count=1 -v
//
// Другая модель разбора — LLM_INTENT_MODEL=qwen3.6-35b-a3b; часть запросов — LIVE_QUERY_IDS=q-0004,q-0005.
//
// Ключ и каталог — из backend/.env; в вывод не попадают.
var liveQueryIDs = []string{"q-0004", "q-0005", "q-0241", "q-0244", "q-0250", "q-0254", "q-0247", "q-0120", "q-0266", "q-0320"}

func TestLiveIntent(t *testing.T) {
	_ = godotenv.Load("../.env")
	cfg, err := config.LoadLLM()
	if err != nil || !cfg.Online() {
		t.Skip("модель не настроена (LLM_API_KEY, LLM_FOLDER_ID в backend/.env)")
	}
	repos, err := repositories.InitRepos(os.Getenv("DATABASE_URL"))
	if err != nil || !repos.Enabled() {
		t.Skip("нет базы (DATABASE_URL)")
	}
	defer func() { _ = repos.Close() }()
	if err := repos.MigrateDB(); err != nil {
		t.Fatal(err)
	}
	var fallback ChatModel
	if cfg.FallbackModel != "" {
		fallback = llm.New(cfg.BaseURL, cfg.FallbackModel, cfg.FolderID, cfg.APIKey, 30*time.Second)
	}
	svc := NewServices(repos, Options{
		Source: catalogsource.MockJSON{Dir: "../../data/mock"}, LLMMode: "on",
		IntentModel:    llm.New(cfg.BaseURL, cfg.IntentModel, cfg.FolderID, cfg.APIKey, 30*time.Second),
		IntentFallback: fallback,
	})
	defer svc.Wait()

	raw, err := os.ReadFile("../../data/mock/queries.json")
	if err != nil {
		t.Fatal(err)
	}
	var all []struct{ ID, Text string }
	if err := json.Unmarshal(raw, &all); err != nil {
		t.Fatal(err)
	}
	ids := liveQueryIDs
	if v := os.Getenv("LIVE_QUERY_IDS"); v != "" { // LIVE_QUERY_IDS=q-0004,q-0005 — часть набора
		ids = strings.Split(v, ",")
	}
	texts := map[string]string{}
	for _, q := range all {
		texts[q.ID] = q.Text
	}

	var lat []float64
	var in, out, calls, ok, cached int
	var cost float64
	drops := map[string]int{}
	for _, id := range ids {
		q := texts[id]
		ctx := WithRequestID(t.Context(), "live-"+id)
		first, _ := svc.intent.Parse(ctx, q, domain.Intent{}, "")
		svc.Wait()
		second, _ := svc.intent.Parse(ctx, q, domain.Intent{}, "")
		if first.Source == domain.ParseLLM {
			ok++
		}
		if second.Source == domain.ParseCache {
			cached++
		}
		if l := first.LLM; l != nil {
			lat = append(lat, l.Ms)
			in += l.InputTokens
			out += l.OutputTokens
			calls += l.Calls
			cost += l.CostRub
		}
		for _, d := range first.Dropped {
			drops[d.Field+":"+d.Reason]++
		}
		t.Logf("%s | %s → %s | повтор: %s | %s", id, q, first.Source, second.Source, intentSummary(first))
		if first.LLM != nil {
			t.Logf("    %s, %d/%d токенов, %.0f мс, %.4f ₽, вызовов %d, откат: %q", first.LLM.Model,
				first.LLM.InputTokens, first.LLM.OutputTokens, first.LLM.Ms, first.LLM.CostRub, first.LLM.Calls, first.FallbackReason)
		}
	}
	sort.Float64s(lat)
	pct := func(p float64) float64 {
		if len(lat) == 0 {
			return 0
		}
		return lat[min(len(lat)-1, int(p*float64(len(lat))))]
	}
	var dl []string
	for k, v := range drops {
		dl = append(dl, k+"×"+itoaLive(v))
	}
	sort.Strings(dl)
	n := len(ids)
	t.Logf("ИТОГО: разобрано моделью %d/%d, из кэша на повторе %d/%d, вызовов %d, токены %d/%d (ср. %d/%d), "+
		"задержка p50 %.0f мс, макс %.0f мс, стоимость %.4f ₽ (ср. %.4f ₽ на разбор), отброшено: %s",
		ok, n, cached, n, calls, in, out, in/max(1, calls), out/max(1, calls), pct(0.5), pct(1), cost, cost/float64(max(1, ok)),
		strings.Join(dl, ", "))
	if ok < n*7/10 {
		t.Fatalf("успешных разборов меньше 70%%: %d/%d", ok, n)
	}
}

func itoaLive(n int) string { b, _ := json.Marshal(n); return string(b) }
