package services

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
)

func search(t *testing.T, env *testEnv, cfg, query, client string) *out.SearchResponse {
	t.Helper()
	resp, err := env.svc.Search(t.Context(), "01TEST", in.SearchRequest{Query: query, Config: cfg, ClientID: client})
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// stepsByName — шаги трассы как JSON-объекты (так их видит фронт).
func stepsByName(t *testing.T, tr *domain.Trace) (names []string, by map[string]map[string]any) {
	t.Helper()
	raw, err := json.Marshal(tr.Steps)
	if err != nil {
		t.Fatal(err)
	}
	var steps []map[string]any
	if err := json.Unmarshal(raw, &steps); err != nil {
		t.Fatal(err)
	}
	by = map[string]map[string]any{}
	for _, s := range steps {
		names = append(names, s["name"].(string))
		by[s["name"].(string)] = s
	}
	return names, by
}

var allSteps = []string{"normalize", "attributes", "route", "intent", "bm25", "dense", "fusion", "rerank", "business", "relaxation"}

// B0: подстрока в названии, одна позиция на семейство, вторая лицензия — в variants.
func TestSearchB0(t *testing.T) {
	env := imported(t, true)
	resp := search(t, env, "B0", "Kaspersky Internet", "")
	if resp.Total != 1 || len(resp.Items) != 1 || resp.Items[0].FamilyID != "kaspersky-internet-security" ||
		len(resp.Items[0].Variants) != 1 || resp.Items[0].Group != domain.GroupExact || resp.Normalized != "kaspersky internet" {
		t.Fatalf("B0: %+v", resp)
	}
	names, by := stepsByName(t, resp.Trace)
	if !slices.Equal(names, allSteps) {
		t.Fatalf("шаги: %v", names)
	}
	if by["bm25"]["mode"] != "substring" || by["dense"]["skipped"] != true || by["fusion"]["skipped"] != true {
		t.Fatalf("шаги B0: %v", by)
	}
	if len(resp.Trace.Items) != 2 || resp.Trace.Items[0].BM25Rank == nil || resp.Trace.Items[0].DenseRank != nil {
		t.Fatalf("позиции трассы: %+v", resp.Trace.Items)
	}
	// B0 не понимает того, чего нет в названии буквально.
	if resp := search(t, env, "B0", "касперский", ""); len(resp.Items) != 0 || resp.Empty == nil || !resp.Empty.ShowManager {
		t.Fatalf("B0 по-русски: %+v", resp)
	}
}

// B1: алиасы в индексе находят «касперский»; dense и fusion пропущены как не входящие в конфигурацию.
func TestSearchB1(t *testing.T) {
	env := imported(t, true)
	resp := search(t, env, "B1", "касперский", "")
	if len(resp.Items) != 2 || resp.Items[0].Vendor != "Kaspersky" || resp.Items[0].Final != 1 {
		t.Fatalf("B1: %+v", resp.Items)
	}
	_, by := stepsByName(t, resp.Trace)
	if by["bm25"]["mode"] != "multi_match" || by["dense"]["reason"] != reasonNotInConfig || by["fusion"]["reason"] != reasonNotInConfig {
		t.Fatalf("шаги B1: %v", by)
	}
	if resp.Trace.EffectiveConfig != "B1" || resp.Trace.Route != "A" || resp.Trace.ParseSource != "rules" || resp.Trace.TotalMs <= 0 {
		t.Fatalf("трасса B1: %+v", resp.Trace)
	}
}

// B2: BM25 + kNN + RRF; у позиции в трассе оба ранга.
func TestSearchB2(t *testing.T) {
	env := imported(t, true)
	resp := search(t, env, "B2", "kaspersky internet security", "")
	_, by := stepsByName(t, resp.Trace)
	if by["dense"]["skipped"] == true || len(by["dense"]["top"].([]any)) == 0 || by["fusion"]["skipped"] == true || by["fusion"]["k"].(float64) != 60 {
		t.Fatalf("шаги B2: %v", by)
	}
	first := resp.Trace.Items[0]
	if resp.Items[0].FamilyID != "kaspersky-internet-security" || first.BM25Rank == nil || first.DenseRank == nil || first.Final <= 0 || first.Final > 1 {
		t.Fatalf("B2: %+v / %+v", resp.Items[0], first)
	}
}

// Без вектора запроса B2 честно работает на BM25: dense.skipped=true с причиной.
func TestSearchB2DenseSkipped(t *testing.T) {
	env := imported(t, false)
	_, by := stepsByName(t, search(t, env, "B2", "касперский", "").Trace)
	if by["dense"]["skipped"] != true || by["dense"]["reason"] != reasonEmbedderDisabled {
		t.Fatalf("эмбеддер не настроен: %v", by["dense"])
	}

	env = imported(t, true)
	env.tei.Down.Store(true)
	_, by = stepsByName(t, search(t, env, "B2", "касперский", "").Trace)
	if by["dense"]["skipped"] != true || by["dense"]["reason"] != reasonEmbedderDown || by["fusion"]["skipped"] == true {
		t.Fatalf("эмбеддер лежит: %v / %v", by["dense"], by["fusion"])
	}

	// Индекс собран без векторов, а эмбеддер потом поднялся — kNN не спрашиваем.
	env = newTestEnv(t, true)
	env.tei.Down.Store(true)
	if _, err := env.svc.ImportCatalog(t.Context()); err != nil {
		t.Fatal(err)
	}
	env.tei.Down.Store(false)
	_, by = stepsByName(t, search(t, env, "B2", "касперский", "").Trace)
	if by["dense"]["reason"] != reasonNoVectors {
		t.Fatalf("индекс без векторов: %v", by["dense"])
	}
}

// B3 — ядро поиска: все шаги трассы на месте, атрибуты и маршрут не пропущены, rerank выключен.
func TestSearchB3Steps(t *testing.T) {
	env := imported(t, true)
	resp := search(t, env, "", "касперский", "")
	// По умолчанию — витрина live: пайплайн B3 на действующих правилах (без базы — версия 1, то есть B3).
	if resp.Config != "live" || resp.Trace.Config != "live" || resp.Trace.EffectiveConfig != "B3" {
		t.Fatalf("конфигурация по умолчанию: %s / %+v", resp.Config, resp.Trace)
	}
	_, by := stepsByName(t, resp.Trace)
	for _, name := range []string{"normalize", "attributes", "route", "business", "relaxation"} {
		if by[name]["skipped"] == true {
			t.Fatalf("%s пропущен: %v", name, by[name])
		}
	}
	if by["rerank"]["skipped"] != true || by["intent"]["skipped"] != true {
		t.Fatalf("rerank/intent: %v / %v", by["rerank"], by["intent"])
	}
}

// Покупал ли клиент: позиция или другая лицензия того же семейства.
func TestSearchPurchasedBefore(t *testing.T) {
	env := imported(t, true)
	resp := search(t, env, "B1", "kaspersky small office", "client-a")
	if !resp.Items[0].PurchasedBefore || resp.Items[0].FamilyID != "kaspersky-small-office-security" {
		t.Fatalf("client-a: %+v", resp.Items[0])
	}
	if resp := search(t, env, "B1", "kaspersky small office", "client-b"); resp.Items[0].PurchasedBefore {
		t.Fatal("client-b ничего не покупал")
	}
}

// Массивы ответа и трассы — массивы даже пустые (фронт не проверяет null).
func TestSearchEmptyArrays(t *testing.T) {
	env := imported(t, true)
	raw, err := json.Marshal(search(t, env, "B0", "notion", ""))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"items":[]`, `"chips":[]`, `"relaxed":[]`, `"related_queries":[]`, `"changes":[]`,
		`"found":[]`, `"unresolved":[]`, `"signals":[]`, `"ladder":[]`, `"rejected":[]`, `"llm":null`,
		`"empty":{"message":"Товара «notion» нет в каталоге","show_manager":true}`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("нет %s в %s", want, raw)
		}
	}
}

func TestSearchValidation(t *testing.T) {
	env := imported(t, false)
	for name, req := range map[string]in.SearchRequest{
		"пустой":       {Query: "   "},
		"длинный":      {Query: strings.Repeat("я", maxQueryRunes+1)},
		"конфигурация": {Query: "x", Config: "B9"},
		"лимит":        {Query: "x", Limit: maxLimit + 1},
	} {
		_, err := env.svc.Search(t.Context(), "r", req)
		var e *errs.Error
		if !errors.As(err, &e) || e.Type != errs.ErrTypeValidation || len(e.Details) != 1 {
			t.Fatalf("%s: %v", name, err)
		}
	}
	resp, err := env.svc.Search(t.Context(), "r", in.SearchRequest{Query: "kaspersky", Config: "b1", Limit: 1})
	if err != nil || resp.Config != "B1" || len(resp.Items) != 1 || resp.Total != 2 {
		t.Fatalf("регистр конфигурации и лимит: %+v, %v", resp, err)
	}
}

// Без индекса или без OpenSearch — 503 с понятным текстом.
func TestSearchUnavailable(t *testing.T) {
	env := newTestEnv(t, false)
	_, err := env.svc.Search(t.Context(), "r", in.SearchRequest{Query: "x"})
	var e *errs.Error
	if !errors.As(err, &e) || e.Type != errs.ErrTypeUnavailable || e.Message != MsgNoIndex {
		t.Fatalf("без индекса: %v", err)
	}
	_, err = NewServices(nil, Options{}).Search(t.Context(), "r", in.SearchRequest{Query: "x"})
	if !errors.As(err, &e) || e.Message != MsgSearchDisabled {
		t.Fatalf("без OpenSearch: %v", err)
	}
}

// Ревью-2 n4: паника в dense (горутина вне recover Fiber) не роняет процесс — dense пропускается.
func TestDenseSafeRecovers(t *testing.T) {
	env := imported(t, true)
	r := env.svc.denseSafe(t.Context(), "фигма", nil) // nil-метаданные индекса — паника внутри dense
	if !r.step.Skipped || r.step.Reason != reasonKNNFailed || r.step.Top == nil {
		t.Fatalf("dense после паники: %+v", r.step)
	}
}
