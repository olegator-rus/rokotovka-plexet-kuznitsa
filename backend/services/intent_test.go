package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"kuznica/api/domain"
	"kuznica/api/externalApi/llm"
	"kuznica/api/pkg/errs"
)

// Разбор намерения на фейковом OpenAI-совместимом сервере: без сети и без базы.

// fakeLLM — сервер, отдающий ответы по очереди (последний повторяется) и запоминающий тела запросов.
type fakeLLM struct {
	*httptest.Server
	mu      sync.Mutex
	replies []fakeReply
	bodies  []map[string]any
}

type fakeReply struct {
	status  int
	content string
	in, out int
	// delay — задержка ответа (параллельные одинаковые разборы успевают встретиться).
	delay time.Duration
}

func newFakeLLM(t *testing.T, replies ...fakeReply) *fakeLLM {
	t.Helper()
	f := &fakeLLM{replies: replies}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		i := len(f.bodies)
		f.bodies = append(f.bodies, body)
		rep := f.replies[min(i, len(f.replies)-1)]
		f.mu.Unlock()
		time.Sleep(rep.delay)
		if rep.status != 0 && rep.status != 200 {
			w.WriteHeader(rep.status)
			_, _ = w.Write([]byte(`{"error":{"message":"overloaded"}}`))
			return
		}
		content, _ := json.Marshal(rep.content)
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":` + string(content) + `}}],
			"usage":{"prompt_tokens":` + fakeItoa(rep.in) + `,"completion_tokens":` + fakeItoa(rep.out) + `,"total_tokens":0,"prompt_tokens_details":{"cached_tokens":0}}}`))
	}))
	t.Cleanup(f.Close)
	return f
}

func fakeItoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func (f *fakeLLM) calls() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.bodies) }

func (f *fakeLLM) client(model string) *llm.Client {
	return llm.New(f.URL, "gpt://b1gtest/"+model, "b1gtest", "sk-test", 2*time.Second)
}

// memCache — кэш разборов в памяти.
type memCache struct {
	mu   sync.Mutex
	rows map[string]domain.IntentCache
}

func (m *memCache) GetIntentCache(_ context.Context, key string, _ time.Time) (*domain.IntentCache, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[key]
	if !ok {
		return nil, nil
	}
	return &r, nil
}

func (m *memCache) PutIntentCache(_ context.Context, row *domain.IntentCache) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[row.Key] = *row
	return nil
}

// newIntentEnv — сервисы на фикстуре каталога и фейковой модели; cache != nil — кэш в памяти.
func newIntentEnv(t *testing.T, mode string, primary, fallback ChatModel, cache *memCache) *intentService {
	t.Helper()
	svc := NewServices(nil, Options{Source: fixtureSource(), LLMMode: mode, IntentModel: primary, IntentFallback: fallback})
	p := svc.intent.(*intentService)
	if cache != nil {
		p.cache = cache
	}
	t.Cleanup(svc.Wait)
	return p
}

const photoshopReply = "```json\n" + `{"intent":"find_alternative","reference_product":"Adobe Photoshop","category":"graphics",
"vendor":"adobe","origin":null,"constraints":{"license_quantity":20,"license_metric":"user"},
"preferences":{"license_duration_months":null},"unresolved":[]}` + "\n```"

func TestDecodeIntent(t *testing.T) {
	ok := []string{
		photoshopReply,
		`{"intent":"find_product"}`,
		"Вот разбор:\n```\n{\"intent\":\"renewal\",\"constraints\":{}}\n```",
		`{"intent":"find_product","constraints":{"license_quantity":"10"},"unresolved":"метрика"}`,
	}
	for _, c := range ok {
		if _, err := decodeIntent(c); err != nil {
			t.Errorf("%q: %v", c, err)
		}
	}
	bad := []string{"", "не знаю", "```json\n```", `{"reference_product":"x"}`, `{"intent":42}`, `{"intent":"find_product","constraints":[1]}`, `[{"intent":"x"}]`}
	for _, c := range bad {
		if _, err := decodeIntent(c); err == nil {
			t.Errorf("%q: ожидалась ошибка схемы", c)
		}
	}
}

func TestIntentValidate(t *testing.T) {
	p := newIntentEnv(t, "off", nil, nil, nil)
	dict := p.dictionary(t.Context())
	// В промпт идёт срез справочника по словам запроса (intent_context.go), а не весь каталог.
	prompt := dict.promptFor("аналог фотошопа для дизайна от kaspersky", domain.Intent{})
	if dict == nil || !strings.Contains(prompt, "graphics — ") || !strings.Contains(prompt, "kaspersky — Kaspersky") {
		t.Fatalf("кандидаты не подставлены в промпт")
	}
	if strings.Contains(prompt, "{{") {
		t.Fatal("в промпте остались плейсхолдеры")
	}

	raw, _ := decodeIntent(photoshopReply)
	in := dict.validate(*raw, "аналог фотошопа на 20 пользователей")
	if in.Kind != domain.IntentFindAlternative || in.ReferenceFamily != "adobe-photoshop" || in.CategoryID != "graphics" || in.VendorID != "" {
		t.Fatalf("разбор: %s", intentSummary(in))
	}
	if !hasConstraint(in.Constraints, domain.FieldLicenseQuantity, "20") || !hasConstraint(in.Constraints, domain.FieldLicenseMetric, "user") {
		t.Fatalf("условия: %v", in.Constraints)
	}
	if !hasDrop(in.Dropped, "vendor", dropReferenceVendor) {
		t.Fatalf("вендор референса должен быть отброшен: %v", in.Dropped)
	}

	// «отечественный» вместо вендора, выдуманные количество и категория, неизвестное поле, мусорный вид.
	raw, _ = decodeIntent(`{"intent":"search","reference_product":"Microsoft Office","category":"office_suite",
		"vendor":"отечественный","constraints":{"license_quantity":50,"license_metric":"компы","license_type":"perpetual","color":"red"},
		"preferences":{"license_duration_months":12},"unresolved":["метрика"]}`)
	in = dict.validate(*raw, "отечественный офис на 30 компов бессрочка")
	if in.Kind != domain.IntentFindProduct || in.Origin != "ru" || in.ReferenceFamily != "microsoft-office-2021" {
		t.Fatalf("разбор: %s", intentSummary(in))
	}
	for _, d := range [][2]string{
		{"intent", dropUnknownKind}, {"category", dropUnknownCategory}, {"vendor", dropOriginWord},
		{domain.FieldLicenseQuantity, dropNotInQuery}, {domain.FieldLicenseMetric, dropInvalidValue},
		{"constraints.color", dropUnknownField}, {domain.FieldLicenseDuration, dropNotInQuery},
	} {
		if !hasDrop(in.Dropped, d[0], d[1]) {
			t.Errorf("нет отброса %s/%s: %v", d[0], d[1], in.Dropped)
		}
	}
	if !hasConstraint(in.Constraints, domain.FieldLicenseType, domain.LicensePerpetual) || len(in.Unresolved) != 1 {
		t.Fatalf("условия %v, неразрешённое %v", in.Constraints, in.Unresolved)
	}

	// Референс, которого нет в каталоге, и вендор по алиасу.
	raw, _ = decodeIntent(`{"intent":"find_product","reference_product":"SolidWorks","vendor":"касперский"}`)
	in = dict.validate(*raw, "solidworks")
	if in.ReferenceFamily != "" || in.VendorID != "kaspersky" || !hasDrop(in.Dropped, "reference_product", dropNotInCatalog) {
		t.Fatalf("разбор: %s", intentSummary(in))
	}
	// Референс по токенам и по алиасу внутри.
	for ref, want := range map[string]string{"Photoshop": "adobe-photoshop", "фотошоп cc": "adobe-photoshop", "kaspersky small office": "kaspersky-small-office-security", "adobe-photoshop": "adobe-photoshop"} {
		if got := dict.resolveFamily(ref); got != want {
			t.Errorf("resolveFamily(%q) = %q, want %q", ref, got, want)
		}
	}
}

func TestMergeIntent(t *testing.T) {
	rules := domain.Intent{Kind: domain.IntentRenewal, VendorID: "kaspersky",
		Constraints: []domain.AttrConstraint{{Field: domain.FieldLicenseQuantity, Value: "30"}, {Field: domain.FieldLicenseMetric, Value: "device"}},
		Unresolved:  []string{domain.FieldLicenseType}}
	m := domain.Intent{Kind: domain.IntentFindProduct, VendorID: "drweb", Origin: "ru", CategoryID: "security",
		Constraints: []domain.AttrConstraint{{Field: domain.FieldLicenseMetric, Value: "user"}, {Field: domain.FieldLicenseType, Value: "perpetual"}},
		Dropped:     []domain.DroppedField{{Field: "vendor", Value: "x", Reason: dropUnknownVendor}}}
	out := mergeIntent(rules, m)
	if out.Kind != domain.IntentRenewal || out.VendorID != "kaspersky" || out.CategoryID != "security" || out.Origin != "ru" {
		t.Fatalf("слияние: %s", intentSummary(out))
	}
	if !hasConstraint(out.Constraints, domain.FieldLicenseMetric, "device") || hasConstraint(out.Constraints, domain.FieldLicenseMetric, "user") {
		t.Fatalf("метрика правил не должна подменяться: %v", out.Constraints)
	}
	if !hasConstraint(out.Constraints, domain.FieldLicenseType, "perpetual") || !hasConstraint(out.Constraints, domain.FieldOrigin, "ru") {
		t.Fatalf("модель дополняет пустое: %v", out.Constraints)
	}
	if len(out.Unresolved) != 0 || len(out.Dropped) != 1 {
		t.Fatalf("неразрешённое %v, отброшено %v", out.Unresolved, out.Dropped)
	}
}

func TestIntentParseLLMAndCache(t *testing.T) {
	f := newFakeLLM(t, fakeReply{content: photoshopReply, in: 120, out: 95})
	cache := &memCache{rows: map[string]domain.IntentCache{}}
	p := newIntentEnv(t, "on", f.client("aliceai-llm-flash"), nil, cache)
	ctx := WithRequestID(t.Context(), "req-1")
	rules := domain.Intent{Constraints: []domain.AttrConstraint{{Field: domain.FieldLicenseQuantity, Value: "20"}}}

	in, err := p.Parse(ctx, "аналог фотошопа на 20 пользователей", rules, "smb")
	if err != nil {
		t.Fatal(err)
	}
	if in.Source != domain.ParseLLM || in.ReferenceFamily != "adobe-photoshop" || in.LLM == nil {
		t.Fatalf("разбор: %s", intentSummary(in))
	}
	// 120 × 100 + 95 × 200 = 31 000 ₽ за 1 млн → 0,031 ₽.
	if in.LLM.Model != "aliceai-llm-flash" || in.LLM.InputTokens != 120 || in.LLM.OutputTokens != 95 || in.LLM.CostRub != 0.031 || in.LLM.Calls != 1 {
		t.Fatalf("расход: %+v", in.LLM)
	}
	body := f.bodies[0]
	if body["temperature"].(float64) != 0 || body["response_format"].(map[string]any)["type"] != "json_object" || body["reasoning_effort"] != nil {
		t.Fatalf("тело запроса: %v", body)
	}
	msgs := body["messages"].([]any)
	user := msgs[1].(map[string]any)["content"].(string)
	if !strings.HasPrefix(user, `<query>"аналог фотошопа на 20 пользователей"</query>`) || !strings.Contains(user, "smb") {
		t.Fatalf("запрос должен уйти как данные: %q", user)
	}
	step, tl, cost := IntentTrace(in, 2100)
	if tl == nil || cost != 0.031 || step.Source != domain.ParseLLM || step.Fields["reference_family"] != "adobe-photoshop" {
		t.Fatalf("трасса: %+v %+v %v", step, tl, cost)
	}

	p.s.Wait() // запись в кэш — в фоне
	in2, _ := p.Parse(ctx, "аналог фотошопа на 20 пользователей", rules, "smb")
	if in2.Source != domain.ParseCache || in2.LLM != nil || in2.ReferenceFamily != "adobe-photoshop" || f.calls() != 1 {
		t.Fatalf("повтор должен взять кэш без вызова модели: %s, вызовов %d", intentSummary(in2), f.calls())
	}
	if _, tl, cost := IntentTrace(in2, 1); tl != nil || cost != 0 {
		t.Fatal("из кэша — стоимость 0 и trace.llm = null")
	}
	// Другой сегмент — другой ключ: модель вызывается снова.
	if in3, _ := p.Parse(ctx, "аналог фотошопа на 20 пользователей", rules, "gov"); in3.Source != domain.ParseLLM || f.calls() != 2 {
		t.Fatalf("другой сегмент: %s, вызовов %d", in3.Source, f.calls())
	}
}

// 10 одинаковых параллельных промахов кэша — один вызов модели; расход в трассе у одного, остальные — как из кэша.
func TestIntentParseSingleflight(t *testing.T) {
	f := newFakeLLM(t, fakeReply{content: photoshopReply, in: 120, out: 95, delay: 300 * time.Millisecond})
	p := newIntentEnv(t, "on", f.client("aliceai-llm-flash"), nil, &memCache{rows: map[string]domain.IntentCache{}})
	rules := domain.Intent{Constraints: []domain.AttrConstraint{{Field: domain.FieldLicenseQuantity, Value: "20"}}}
	const n = 10
	var (
		wg    sync.WaitGroup
		start = make(chan struct{})
		res   = make([]domain.Intent, n)
	)
	for i := range n {
		wg.Go(func() {
			<-start
			res[i], _ = p.Parse(t.Context(), "аналог фотошопа на 20 пользователей", rules, "smb")
		})
	}
	close(start)
	wg.Wait()
	if f.calls() != 1 {
		t.Fatalf("вызовов модели %d, ожидался 1", f.calls())
	}
	owners, cost := 0, 0.0
	for _, in := range res {
		if in.ReferenceFamily != "adobe-photoshop" {
			t.Fatalf("разбор: %s", intentSummary(in))
		}
		if in.LLM != nil {
			owners++
			cost += in.LLM.CostRub
			if in.Source != domain.ParseLLM {
				t.Fatalf("с расходом — источник llm: %s", in.Source)
			}
		} else if in.Source != domain.ParseCache {
			t.Fatalf("ждущий без расхода — источник cache: %s", in.Source)
		}
	}
	if owners != 1 || cost != 0.031 {
		t.Fatalf("расход должен попасть в одну трассу: владельцев %d, ₽ %v", owners, cost)
	}
}

func TestIntentRetryAndFallback(t *testing.T) {
	rules := domain.Intent{Kind: domain.IntentFindProduct, VendorID: "kaspersky"}

	t.Run("повтор при ошибке схемы", func(t *testing.T) {
		f := newFakeLLM(t, fakeReply{content: "Конечно! Запрос про фотошоп.", in: 100, out: 10}, fakeReply{content: photoshopReply, in: 150, out: 90})
		p := newIntentEnv(t, "on", f.client("aliceai-llm-flash"), nil, nil)
		in, _ := p.Parse(t.Context(), "аналог фотошопа на 20 пользователей", rules, "")
		if in.Source != domain.ParseLLM || in.LLM.Calls != 2 || in.LLM.InputTokens != 250 || in.LLM.OutputTokens != 100 {
			t.Fatalf("разбор: %s %+v", intentSummary(in), in.LLM)
		}
		if msgs := f.bodies[1]["messages"].([]any); len(msgs) != 4 {
			t.Fatalf("повтор должен показать модели её ответ и ошибку: %d сообщений", len(msgs))
		}
	})

	t.Run("две ошибки схемы — правила, резерв не зовётся", func(t *testing.T) {
		f := newFakeLLM(t, fakeReply{content: "нет", in: 100, out: 2})
		reserve := newFakeLLM(t, fakeReply{content: photoshopReply})
		p := newIntentEnv(t, "on", f.client("aliceai-llm-flash"), reserve.client("qwen3.6-35b-a3b"), nil)
		in, err := p.Parse(t.Context(), "аналог фотошопа", rules, "")
		if err != nil || in.Source != domain.ParseRules || in.VendorID != "kaspersky" || in.FallbackReason != fallbackSchemaError {
			t.Fatalf("откат: %s %v", intentSummary(in), err)
		}
		if f.calls() != 2 || reserve.calls() != 0 || in.LLM == nil || in.LLM.Calls != 2 || in.LLM.CostRub != 0.0208 {
			t.Fatalf("вызовы %d/%d, расход %+v", f.calls(), reserve.calls(), in.LLM)
		}
	})

	t.Run("сбой основной — резервная модель без рассуждений", func(t *testing.T) {
		f := newFakeLLM(t, fakeReply{status: 503})
		reserve := newFakeLLM(t, fakeReply{content: photoshopReply, in: 118, out: 60})
		p := newIntentEnv(t, "on", f.client("aliceai-llm-flash"), reserve.client("qwen3.6-35b-a3b"), nil)
		in, _ := p.Parse(t.Context(), "аналог фотошопа на 20 пользователей", rules, "")
		if in.Source != domain.ParseLLM || in.LLM.Model != "qwen3.6-35b-a3b" || f.calls() != 1 || reserve.calls() != 1 {
			t.Fatalf("резерв: %s %+v", intentSummary(in), in.LLM)
		}
		if reserve.bodies[0]["reasoning_effort"] != "none" {
			t.Fatalf("Qwen без отключения рассуждений: %v", reserve.bodies[0])
		}
	})

	t.Run("сбой без резерва — правила", func(t *testing.T) {
		f := newFakeLLM(t, fakeReply{status: 500})
		p := newIntentEnv(t, "on", f.client("aliceai-llm-flash"), nil, nil)
		in, err := p.Parse(t.Context(), "аналог фотошопа", rules, "")
		if err != nil || in.Source != domain.ParseRules || in.FallbackReason != fallbackLLMError || f.calls() != 1 {
			t.Fatalf("откат: %s, вызовов %d", intentSummary(in), f.calls())
		}
	})
}

func TestIntentModeOff(t *testing.T) {
	f := newFakeLLM(t, fakeReply{content: photoshopReply})
	cache := &memCache{rows: map[string]domain.IntentCache{}}
	rules := domain.Intent{Kind: domain.IntentFindProduct, Constraints: []domain.AttrConstraint{{Field: domain.FieldLicenseQuantity, Value: "20"}}}

	p := newIntentEnv(t, "off", f.client("aliceai-llm-flash"), nil, cache)
	in, err := p.Parse(t.Context(), "аналог фотошопа на 20 пользователей", rules, "")
	if err != nil || in.Source != domain.ParseRules || in.LLM != nil || in.FallbackReason != fallbackLLMOff || f.calls() != 0 {
		t.Fatalf("LLM_MODE=off: %s, вызовов %d", intentSummary(in), f.calls())
	}
	if _, tl, cost := IntentTrace(in, 0); tl != nil || cost != 0 {
		t.Fatal("без модели trace.llm = null и стоимость 0")
	}

	// Без модели вовсе: тоже правила; кэш, заполненный раньше, при этом работает.
	on := newIntentEnv(t, "on", f.client("aliceai-llm-flash"), nil, cache)
	if in, _ := on.Parse(t.Context(), "аналог фотошопа на 20 пользователей", rules, ""); in.Source != domain.ParseLLM {
		t.Fatal(in.Source)
	}
	on.s.Wait()
	if in, _ := p.Parse(t.Context(), "аналог фотошопа на 20 пользователей", rules, ""); in.Source != domain.ParseCache {
		t.Fatalf("режим без LLM читает кэш: %s", in.Source)
	}
	none := newIntentEnv(t, "", nil, nil, nil)
	if in, _ := none.Parse(t.Context(), "x", rules, ""); in.Source != domain.ParseRules || in.FallbackReason != fallbackNoModel {
		t.Fatalf("без модели: %s %s", in.Source, in.FallbackReason)
	}
}

func TestIntentCacheKey(t *testing.T) {
	g := domain.Intent{Constraints: []domain.AttrConstraint{{Field: "b", Value: "1"}, {Field: "a", Value: "2"}}}
	g2 := domain.Intent{Constraints: []domain.AttrConstraint{{Field: "a", Value: "2"}, {Field: "b", Value: "1"}}}
	base := intentCacheKey("q", "smb", g, "mock-1", 1, "intent-v1+aa", "aliceai-llm-flash")
	if base != intentCacheKey("q", "smb", g2, "mock-1", 1, "intent-v1+aa", "aliceai-llm-flash") || len(base) != 64 {
		t.Fatal("ключ зависит от порядка условий")
	}
	for name, k := range map[string]string{
		"запрос":  intentCacheKey("q2", "smb", g, "mock-1", 1, "intent-v1+aa", "aliceai-llm-flash"),
		"сегмент": intentCacheKey("q", "gov", g, "mock-1", 1, "intent-v1+aa", "aliceai-llm-flash"),
		"правила": intentCacheKey("q", "smb", domain.Intent{}, "mock-1", 1, "intent-v1+aa", "aliceai-llm-flash"),
		"каталог": intentCacheKey("q", "smb", g, "mock-2", 1, "intent-v1+aa", "aliceai-llm-flash"),
		"версия":  intentCacheKey("q", "smb", g, "mock-1", 2, "intent-v1+aa", "aliceai-llm-flash"),
		"промпт":  intentCacheKey("q", "smb", g, "mock-1", 1, "intent-v2+bb", "aliceai-llm-flash"),
		"модель":  intentCacheKey("q", "smb", g, "mock-1", 1, "intent-v1+aa", "aliceai-llm"),
	} {
		if k == base {
			t.Errorf("%s не входит в ключ", name)
		}
	}
	if v := newIntentEnv(t, "off", nil, nil, nil).PromptVersion(); !strings.HasPrefix(v, "intent-v4+") {
		t.Fatalf("версия промпта: %q", v)
	}
}

func TestLLMCost(t *testing.T) {
	cases := []struct {
		model   string
		in, out int
		want    float64
	}{
		{"gpt://b1gtest/aliceai-llm-flash", 800, 200, 0.12}, // ARCHITECTURE §7: разбор на Flash — 0,12 ₽
		{"aliceai-llm-flash/latest", 20000, 3000, 2.6},
		{"gpt://b1gtest/qwen3.6-35b-a3b", 20000, 3000, 4.9},
		{"gpt://b1gtest/aliceai-llm/latest", 20000, 3000, 13.6},
		{"gpt://b1gtest/unknown", 1000, 1000, 0},
	}
	for _, c := range cases {
		if got := llmCost(c.model, c.in, c.out); got != c.want {
			t.Errorf("%s %d/%d = %v, want %v", c.model, c.in, c.out, got, c.want)
		}
	}
	if llmModelName("gpt://b1gfolder/aliceai-llm-flash/latest") != "aliceai-llm-flash" {
		t.Fatal("каталог должен вырезаться из имени модели")
	}
}

func TestCostsValidation(t *testing.T) {
	svc := NewServices(nil, Options{})
	for _, c := range [][2]int{{-1, 0}, {500, 0}, {10, -1}} {
		_, err := svc.Costs(t.Context(), CostsRequest{Limit: c[0], Offset: c[1]})
		var e *errs.Error
		if !errors.As(err, &e) || e.HTTPStatus() != http.StatusBadRequest && e.HTTPStatus() != http.StatusUnprocessableEntity {
			t.Errorf("%v: %v", c, err)
		}
	}
	_, err := svc.Costs(t.Context(), CostsRequest{})
	var e *errs.Error
	if !errors.As(err, &e) || e.HTTPStatus() != http.StatusServiceUnavailable {
		t.Fatalf("без базы — 503: %v", err)
	}
}

func hasConstraint(cs []domain.AttrConstraint, field, value string) bool {
	for _, c := range cs {
		if c.Field == field && c.Value == value {
			return true
		}
	}
	return false
}

func hasDrop(ds []domain.DroppedField, field, reason string) bool {
	for _, d := range ds {
		if d.Field == field && d.Reason == reason {
			return true
		}
	}
	return false
}

func TestLLMCallKind(t *testing.T) {
	str := func(s string) *string { return &s }
	cases := map[string]domain.LLMUsage{
		"search": {RequestID: str("01M87VH9WE8RRSHPZ3K0N770QG")},
		"eval":   {RequestID: str(evalRequestPrefix + "abc-B3-q-0004")},
		"agent":  {AgentRunID: str("run-1")},
		"judge":  {RequestID: str(judgeRequestPrefix + "abc123-001")},
		"other":  {RequestID: str("live-q-0004")},
	}
	for want, u := range cases {
		if got := llmCallKind(u); got != want {
			t.Errorf("%+v: %s, want %s", u, got, want)
		}
	}
}
