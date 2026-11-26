package services

import (
	"encoding/json"
	"strings"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/externalApi/llm"
	"kuznica/api/externalApi/opensearch"
	"kuznica/api/externalApi/opensearch/opensearchtest"
	"kuznica/api/in"
)

// Разбор намерения: телефон, почта и ИНН в тексте запроса до модели не доходят (тело запроса подменного сервера),
// в кэш разборов ключом и строкой идёт замаскированный текст, в трассе — «замаскировано: N». Тот же запрос с
// другим телефоном — тот же ключ: берётся из кэша без нового вызова.
func TestIntentMasksPIIBeforeModelAndCache(t *testing.T) {
	f := newFakeLLM(t, fakeReply{content: photoshopReply, in: 120, out: 95})
	cache := &memCache{rows: map[string]domain.IntentCache{}}
	p := newIntentEnv(t, "on", f.client("aliceai-llm-flash"), nil, cache)
	ctx := WithRequestID(t.Context(), "req-pii")

	q := "аналог фотошопа на 20 пользователей позвоните 8 916 123 45 67 или ivan.petrov@mail.ru инн 7707083893"
	got, err := p.Parse(ctx, q, domain.Intent{}, "smb")
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != domain.ParseLLM || got.PIIMasked != 3 {
		t.Fatalf("разбор: source=%s pii_masked=%d", got.Source, got.PIIMasked)
	}
	raw, _ := json.Marshal(f.bodies)
	for _, leak := range []string{"916", "45 67", "ivan.petrov", "mail.ru", "7707083893"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("в теле запроса к модели осталось %q", leak)
		}
	}
	for _, label := range []string{llm.PIIPhone, llm.PIIEmail, llm.PIIINN} {
		if !strings.Contains(string(raw), label) {
			t.Fatalf("в теле нет метки %s", label)
		}
	}
	step, _, _ := IntentTrace(got, 1)
	if step.PIIMasked != 3 {
		t.Fatalf("трасса: pii_masked = %d", step.PIIMasked)
	}

	p.s.Wait()
	if len(cache.rows) != 1 {
		t.Fatalf("строк кэша: %d", len(cache.rows))
	}
	for _, row := range cache.rows {
		if strings.Contains(row.Normalized, "916") || strings.Contains(row.Normalized, "@") || !strings.Contains(row.Normalized, llm.PIIPhone) {
			t.Fatalf("в кэш ушёл незамаскированный текст: %q", row.Normalized)
		}
	}
	other := strings.Replace(q, "8 916 123 45 67", "+7 (903) 765-43-21", 1)
	again, _ := p.Parse(ctx, other, domain.Intent{}, "smb")
	if again.Source != domain.ParseCache || f.calls() != 1 || again.PIIMasked != 3 {
		t.Fatalf("тот же запрос с другим телефоном — кэш: source=%s вызовов=%d pii=%d", again.Source, f.calls(), again.PIIMasked)
	}
}

// Весь путь поиска (B3, маршрут D): после нормализации ПДн в тело вызова модели не попадают, шаг intent трассы
// несёт pii_masked. Запрос без ПДн — поля нет (omitempty), как раньше.
func TestSearchRouteDMasksPII(t *testing.T) {
	f := newFakeLLM(t, fakeReply{content: photoshopReply, in: 900, out: 80})
	osrv := opensearchtest.New()
	t.Cleanup(osrv.Close)
	s := NewServices(nil, Options{Search: opensearch.New(osrv.URL, 0), Source: fixtureSource(), LLMMode: "on",
		IntentModel: f.client("aliceai-llm-flash")})
	t.Cleanup(s.Wait)
	if _, err := s.ImportCatalog(t.Context()); err != nil {
		t.Fatal(err)
	}
	q := "чем заменить фотошоп для дизайнеров, перезвоните +7 (916) 123-45-67, почта ivan.petrov@mail.ru"
	resp, err := s.SearchWith(t.Context(), "req-pii-d", in.SearchRequest{Query: q, Config: domain.ConfigB3}, SearchOptions{NoLog: true})
	if err != nil {
		t.Fatal(err)
	}
	if f.calls() == 0 {
		t.Skipf("запрос не ушёл на маршрут D (route=%s) — проверять нечего", resp.Trace.Route)
	}
	f.mu.Lock()
	raw, _ := json.Marshal(f.bodies)
	f.mu.Unlock()
	for _, leak := range []string{"916", "45-67", "45 67", "ivan", "petrov", "mail.ru", "mail ru"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("в теле запроса к модели осталось %q", leak)
		}
	}
	var masked int
	for _, st := range resp.Trace.Steps {
		b, _ := json.Marshal(st)
		var x struct {
			Name      string `json:"name"`
			PIIMasked int    `json:"pii_masked"`
		}
		if json.Unmarshal(b, &x) == nil && x.Name == domain.StepIntent {
			masked = x.PIIMasked
		}
	}
	if masked < 2 {
		t.Fatalf("шаг intent: pii_masked = %d, ждали ≥ 2", masked)
	}
}
