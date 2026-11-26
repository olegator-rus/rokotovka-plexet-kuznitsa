package services

import (
	"testing"
	"time"

	"kuznica/api/domain"
	"kuznica/api/externalApi/llm"
	"kuznica/api/externalApi/opensearch"
	"kuznica/api/externalApi/opensearch/opensearchtest"
	"kuznica/api/externalApi/tei"
	"kuznica/api/externalApi/tei/teitest"
	"kuznica/api/in"
	"kuznica/api/out"
)

// Отказы моделей (контур доводки, DEFENSE вопрос 3): таймаут LLM, падение эмбеддера, падение реранкера — поиск
// отвечает. Полный путь B3 на поддельных OpenSearch, сервисе моделей и OpenAI-совместимом сервере.

// Поддельный OpenSearch не исполняет строгие фильтры B3, поэтому выдача здесь может быть пустой: проверяется, что
// поиск отвечает без ошибки и в срок, а шаги моделей пропущены с причиной. Качество без моделей — строка B3-M
// таблицы вклада на реальном индексе (docs/EVALUATION.md).

// failoverEnv — B3 с моделью разбора, эмбеддером и реранкером; llmDelay — задержка ответа модели (срок клиента 150 мс).
type failoverEnv struct {
	svc *Services
	tei *teitest.Server
	llm *fakeLLM
}

func newFailoverEnv(t *testing.T, llmDelay time.Duration) *failoverEnv {
	t.Helper()
	osrv := opensearchtest.New()
	t.Cleanup(osrv.Close)
	tsrv := teitest.New()
	t.Cleanup(tsrv.Close)
	f := newFakeLLM(t, fakeReply{content: photoshopReply, in: 100, out: 50, delay: llmDelay})
	model := llm.New(f.URL, "gpt://b1gtest/aliceai-llm-flash", "b1gtest", "sk-test", 150*time.Millisecond)
	models := tei.New(tsrv.URL, 0)
	svc := NewServices(nil, Options{Search: opensearch.New(osrv.URL, 0), Source: fixtureSource(), Embedder: models,
		Reranker: models, RerankWeight: 0.45, LLMMode: "on", IntentModel: model})
	t.Cleanup(svc.Wait)
	if _, err := svc.ImportCatalog(t.Context()); err != nil {
		t.Fatal(err)
	}
	return &failoverEnv{svc: svc, tei: tsrv, llm: f}
}

// failoverQueries — точный запрос, сленг и сложный запрос (маршрут D), как в сценариях защиты.
var failoverQueries = []string{"касперский", "Kaspersky Internet", "аналог фотошопа на 20 пользователей"}

func failoverSearch(t *testing.T, env *failoverEnv, q string) (*out.SearchResponse, map[string]map[string]any) {
	t.Helper()
	started := time.Now()
	resp, err := env.svc.SearchWith(t.Context(), "01FAIL", in.SearchRequest{Query: q, Config: domain.ConfigB3}, SearchOptions{NoLog: true})
	if err != nil {
		t.Fatalf("%q: поиск упал при отказе моделей: %v", q, err)
	}
	if d := time.Since(started); d > 5*time.Second {
		t.Fatalf("%q: ответ через %s — отказ модели держит поиск", q, d)
	}
	_, by := stepsByName(t, resp.Trace)
	return resp, by
}

// Эмбеддер и реранкер лежат (503), модель разбора отвечает дольше срока: поиск отвечает, шаги пропущены с причиной.
func TestFailoverAllModelsDown(t *testing.T) {
	env := newFailoverEnv(t, 600*time.Millisecond)
	env.tei.Down.Store(true)
	routeD := false
	for _, q := range failoverQueries {
		resp, by := failoverSearch(t, env, q)
		if by["dense"]["skipped"] != true || by["dense"]["reason"] != reasonEmbedderDown {
			t.Fatalf("%q: dense при лежащем эмбеддере: %v", q, by["dense"])
		}
		if by["rerank"]["skipped"] != true {
			t.Fatalf("%q: rerank при лежащем реранкере не пропущен: %v", q, by["rerank"])
		}
		if resp.Trace.Route == domain.RouteIntent {
			routeD = true
			if resp.Trace.ParseSource != domain.ParseRules {
				t.Fatalf("%q: таймаут модели — разбор правилами, а не %s", q, resp.Trace.ParseSource)
			}
		}
	}
	if !routeD {
		t.Fatal("ни один запрос не дошёл до маршрута D — таймаут модели не проверен")
	}
	if env.llm.calls() == 0 {
		t.Fatal("модель не вызывалась — таймаут не проверен")
	}
}

// Падение только реранкера (в поддельном сервисе нет /rerank): выдача по слиянию, причина reranker_error.
func TestFailoverRerankerDown(t *testing.T) {
	env := newFailoverEnv(t, 0)
	for _, q := range failoverQueries {
		resp, by := failoverSearch(t, env, q)
		if resp.Trace.Route == domain.RouteIntent || resp.Trace.Route == domain.RouteSemantic {
			if by["rerank"]["reason"] != rerankFailed && by["rerank"]["reason"] != rerankFew && by["rerank"]["reason"] != rerankAnalogs {
				t.Fatalf("%q: реранкер лежит, причина %v", q, by["rerank"])
			}
		}
		if by["dense"]["skipped"] == true {
			t.Fatalf("%q: эмбеддер жив, dense не должен пропускаться: %v", q, by["dense"])
		}
	}
}

// Одна настройка «без моделей»: LLM не зовётся, эмбеддер не зовётся, поиск отвечает; выключение возвращает векторы.
func TestOfflineModeOneSetting(t *testing.T) {
	env := newFailoverEnv(t, 0)
	st, err := env.svc.SetOfflineMode(t.Context(), "on")
	if err != nil || st.LLM.Mode != "off" || st.Vectors.Mode != "off" || !st.Vectors.Offline || st.Vectors.Banner == nil {
		t.Fatalf("режим без моделей: %+v %v", st, err)
	}
	embeds, calls := env.tei.EmbedCalls.Load(), env.llm.calls()
	for _, q := range failoverQueries {
		_, by := failoverSearch(t, env, q)
		if by["dense"]["reason"] != reasonVectorsOff || by["rerank"]["skipped"] != true {
			t.Fatalf("%q: dense/rerank в режиме без моделей: %v / %v", q, by["dense"], by["rerank"])
		}
	}
	if env.tei.EmbedCalls.Load() != embeds || env.llm.calls() != calls {
		t.Fatalf("в режиме без моделей были вызовы: эмбеддер %d→%d, модель %d→%d", embeds, env.tei.EmbedCalls.Load(), calls, env.llm.calls())
	}
	if m := env.svc.Meta(); m.Vectors.Mode != "off" || m.LLM.Mode != "off" {
		t.Fatalf("meta: %+v / %+v", m.Vectors, m.LLM)
	}
	if st, err = env.svc.SetOfflineMode(t.Context(), "off"); err != nil || st.Vectors.Mode != "on" || st.LLM.Mode != "on" {
		t.Fatalf("возврат: %+v %v", st, err)
	}
	_, by := failoverSearch(t, env, "касперский")
	if by["dense"]["skipped"] == true {
		t.Fatalf("после возврата dense пропущен: %v", by["dense"])
	}
	if _, err := env.svc.SetOfflineMode(t.Context(), "maybe"); err == nil {
		t.Fatal("неверный режим принят")
	}
}

// Выключатели одного поиска (прогон вклада) не меняют режим процесса.
func TestSearchModesPerRequest(t *testing.T) {
	env := newFailoverEnv(t, 0)
	ctx := withSearchModes(t.Context(), searchModes{noVectors: true, noLLM: true})
	resp, err := env.svc.SearchWith(ctx, "01M", in.SearchRequest{Query: "касперский", Config: domain.ConfigB3}, SearchOptions{NoLog: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, by := stepsByName(t, resp.Trace); by["dense"]["reason"] != reasonVectorsOff {
		t.Fatalf("dense: %v", by["dense"])
	}
	if env.svc.VectorsMode() != "on" || env.svc.LLMMode() != "on" {
		t.Fatal("выключатель поиска поменял режим процесса")
	}
	_, by := failoverSearch(t, env, "касперский")
	if by["dense"]["skipped"] == true {
		t.Fatalf("без выключателя dense пропущен: %v", by["dense"])
	}
}
