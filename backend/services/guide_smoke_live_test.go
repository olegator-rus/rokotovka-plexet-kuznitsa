//go:build llmlive

package services

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"kuznica/api/config"
	"kuznica/api/externalApi/llm"
	"kuznica/api/prompts"

	"github.com/joho/godotenv"
)

// дымовая проверка Yandex AI Studio для подбора — вызов функций, вывод по JSON-схеме, json_object,
// поток — на лёгкой и сильной модели; замер времени лёгкой модели на типовом входе подбора. Тратит деньги
// (около 20 вызовов), только вручную:
//
//	cd backend && go test -tags llmlive ./services/ -run LiveGuideSmoke -count=1 -v
//
// Ключ и каталог — из backend/.env; в вывод не попадают.
func TestLiveGuideSmoke(t *testing.T) {
	_ = godotenv.Load("../.env")
	cfg, err := config.LoadLLM()
	if err != nil || !cfg.Online() {
		t.Skip("модель не настроена")
	}
	for _, short := range []string{"aliceai-llm-flash", "aliceai-llm"} {
		model := cfg.FolderID
		model = "gpt://" + model + "/" + short
		t.Run(short, func(t *testing.T) {
			smokeTools(t, cfg, model)
			smokeSchema(t, cfg, model)
			smokeJSONObject(t, cfg, model)
			smokeStream(t, cfg, model)
		})
	}
	flash := "gpt://" + cfg.FolderID + "/aliceai-llm-flash"
	var total time.Duration
	for i := 0; i < 3; i++ {
		ms, ok, in, outTok := smokeTypical(t, cfg, flash)
		total += ms
		t.Logf("typical flash #%d: %v ok=%v tokens in=%d out=%d", i+1, ms.Round(time.Millisecond), ok, in, outTok)
	}
	t.Logf("typical flash avg: %v", (total / 3).Round(time.Millisecond))
}

func smokePost(t *testing.T, cfg config.LLMConfig, body map[string]any) (int, []byte, time.Duration) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, cfg.BaseURL+"/chat/completions", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("OpenAI-Project", cfg.FolderID)
	started := time.Now()
	resp, err := (&http.Client{Timeout: 40 * time.Second}).Do(req)
	if err != nil {
		t.Logf("  transport error: %v", strings.ReplaceAll(err.Error(), cfg.APIKey, "***"))
		return 0, nil, time.Since(started)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, []byte(strings.ReplaceAll(string(b), cfg.APIKey, "***")), time.Since(started)
}

type smokeResp struct {
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func smokeHead(b []byte) string {
	s := string(b)
	if len([]rune(s)) > 300 {
		return string([]rune(s)[:300]) + "…"
	}
	return s
}

func smokeTools(t *testing.T, cfg config.LLMConfig, model string) {
	tools := []map[string]any{
		{"type": "function", "function": map[string]any{
			"name": "search_catalog", "description": "Найти семейства и офферы в каталоге ПО",
			"parameters": json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"},"count":{"type":"integer"}},"required":["query"]}`),
		}},
		{"type": "function", "function": map[string]any{
			"name": "get_facts", "description": "Факты базы знаний о редакциях семейств",
			"parameters": json.RawMessage(`{"type":"object","properties":{"family_ids":{"type":"array","items":{"type":"string"}},"axis":{"type":"string"}},"required":["family_ids"]}`),
		}},
	}
	code, body, d := smokePost(t, cfg, map[string]any{
		"model": model, "max_tokens": 1000, "temperature": 0,
		"messages": []map[string]any{
			{"role": "system", "content": "Ты подбираешь лицензии ПО. Используй инструменты, чтобы найти товары."},
			{"role": "user", "content": "подбери касперского на 50 компьютеров"},
		},
		"tools": tools,
	})
	var r smokeResp
	_ = json.Unmarshal(body, &r)
	if code != 200 || len(r.Choices) == 0 {
		t.Logf("TOOLS: HTTP %d %v: %s", code, d.Round(time.Millisecond), smokeHead(body))
		return
	}
	calls := r.Choices[0].Message.ToolCalls
	names := []string{}
	for _, c := range calls {
		names = append(names, c.Function.Name+" "+c.Function.Arguments)
	}
	t.Logf("TOOLS: HTTP 200 %v finish=%s calls=%d %v content=%q", d.Round(time.Millisecond), r.Choices[0].FinishReason,
		len(calls), names, smokeHead([]byte(r.Choices[0].Message.Content)))
}

const smokeGuideSchema = `{"type":"object","properties":{
"type":{"type":"string","enum":["comparison","selection","clarification","no_data","out_of_scope"]},
"lead":{"type":"string"},
"claims":{"type":"array","items":{"type":"string"}},
"offer_ids":{"type":"array","items":{"type":"string"}},
"next":{"type":"array","items":{"type":"string","enum":["add_to_spec","change_conditions","request_quote"]}}
},"required":["type","lead","claims","offer_ids","next"],"additionalProperties":false}`

func smokeSchema(t *testing.T, cfg config.LLMConfig, model string) {
	code, body, d := smokePost(t, cfg, map[string]any{
		"model": model, "max_tokens": 1000, "temperature": 0,
		"messages": []map[string]any{
			{"role": "system", "content": "Ответь по схеме. Факты: kb_kes_adv_encryption — в Расширенном есть шифрование; kb_kes_std_encryption — в Стандартном нет шифрования. Офферы: 4986 (Стандартный), 4857 (Расширенный)."},
			{"role": "user", "content": "чем расширенный касперский отличается от стандартного"},
		},
		"response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{
			"name": "guide_answer", "strict": true, "schema": json.RawMessage(smokeGuideSchema)}},
	})
	var r smokeResp
	_ = json.Unmarshal(body, &r)
	if code != 200 || len(r.Choices) == 0 {
		t.Logf("SCHEMA: HTTP %d %v: %s", code, d.Round(time.Millisecond), smokeHead(body))
		return
	}
	var v map[string]any
	perr := json.Unmarshal([]byte(r.Choices[0].Message.Content), &v)
	t.Logf("SCHEMA: HTTP 200 %v parsed=%v keys=%d content=%s", d.Round(time.Millisecond), perr == nil, len(v), smokeHead([]byte(r.Choices[0].Message.Content)))
}

func smokeJSONObject(t *testing.T, cfg config.LLMConfig, model string) {
	code, body, d := smokePost(t, cfg, map[string]any{
		"model": model, "max_tokens": 1000, "temperature": 0,
		"messages": []map[string]any{
			{"role": "system", "content": `Верни JSON {"type": "...", "lead": "...", "claims": ["fact_id"]}. Факты: kb_kes_adv_encryption — в Расширенном есть шифрование.`},
			{"role": "user", "content": "есть ли шифрование в расширенном касперском"},
		},
		"response_format": map[string]any{"type": "json_object"},
	})
	var r smokeResp
	_ = json.Unmarshal(body, &r)
	if code != 200 || len(r.Choices) == 0 {
		t.Logf("JSON_OBJECT: HTTP %d %v: %s", code, d.Round(time.Millisecond), smokeHead(body))
		return
	}
	var v map[string]any
	perr := json.Unmarshal([]byte(r.Choices[0].Message.Content), &v)
	t.Logf("JSON_OBJECT: HTTP 200 %v parsed=%v content=%s", d.Round(time.Millisecond), perr == nil, smokeHead([]byte(r.Choices[0].Message.Content)))
}

func smokeStream(t *testing.T, cfg config.LLMConfig, model string) {
	raw, _ := json.Marshal(map[string]any{
		"model": model, "max_tokens": 200, "temperature": 0, "stream": true,
		"messages": []map[string]any{{"role": "user", "content": "Назови три российские ОС одной строкой."}},
	})
	req, _ := http.NewRequest(http.MethodPost, cfg.BaseURL+"/chat/completions", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("OpenAI-Project", cfg.FolderID)
	started := time.Now()
	resp, err := (&http.Client{Timeout: 40 * time.Second}).Do(req)
	if err != nil {
		t.Logf("STREAM: transport error")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Logf("STREAM: HTTP %d: %s", resp.StatusCode, smokeHead([]byte(strings.ReplaceAll(string(b), cfg.APIKey, "***"))))
		return
	}
	sc := bufio.NewScanner(resp.Body)
	chunks, first := 0, time.Duration(0)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "data:") && !strings.Contains(line, "[DONE]") {
			if chunks == 0 {
				first = time.Since(started)
			}
			chunks++
		}
	}
	t.Logf("STREAM: HTTP 200 chunks=%d first=%v total=%v ctype=%s", chunks, first.Round(time.Millisecond),
		time.Since(started).Round(time.Millisecond), resp.Header.Get("Content-Type"))
}

// smokeTypical — типовой вход хода подбора: системный промпт, состояние, найденные семейства и факты;
// ответ — json_object по контракту.
func smokeTypical(t *testing.T, cfg config.LLMConfig, model string) (time.Duration, bool, int, int) {
	sys := `Ты — режим подбора лицензий ПО в поиске магазина. Отвечай только JSON-объектом:
{"type":"comparison|selection|clarification|no_data|out_of_scope","lead":"одна-две фразы без цен и чисел","claims":["fact_id"],"family_ids":["id"],"offer_ids":["id"],"next":["add_to_spec|change_conditions|request_quote"]}
Используй только идентификаторы из данных. Не пиши цены, артикулы, слова «лучший», «самый».`
	user := `<state>{"objects":[{"kind":"workstation","count":30}],"operation":"new","term_months":12,"org_type":"commercial"}</state>
<families>[{"family_id":"3953","title":"Kaspersky Endpoint Security для бизнеса Стандартный"},{"family_id":"3935","title":"Kaspersky Endpoint Security для бизнеса Расширенный"}]</families>
<facts>[{"fact_id":"kb_kes_std_encryption","family_id":"3953","axis":"encryption","value":"no"},{"fact_id":"kb_kes_adv_encryption","family_id":"3935","axis":"encryption","value":"yes"},{"fact_id":"kb_kes_adv_patch","family_id":"3935","axis":"patch_management","value":"yes"},{"fact_id":"kb_kes_std_patch","family_id":"3953","axis":"patch_management","value":"no"}]</facts>
<offers>[{"offer_id":"4986","family_id":"3953","tier":"25-49"},{"offer_id":"4857","family_id":"3935","tier":"25-49"}]</offers>
<query>"чем расширенный касперский отличается от стандартного"</query>`
	code, body, d := smokePost(t, cfg, map[string]any{
		"model": model, "max_tokens": 1000, "temperature": 0,
		"messages":        []map[string]any{{"role": "system", "content": sys}, {"role": "user", "content": user}},
		"response_format": map[string]any{"type": "json_object"},
	})
	var r smokeResp
	_ = json.Unmarshal(body, &r)
	if code != 200 || len(r.Choices) == 0 {
		t.Logf("TYPICAL: HTTP %d %s", code, smokeHead(body))
		return d, false, 0, 0
	}
	var v map[string]any
	ok := json.Unmarshal([]byte(r.Choices[0].Message.Content), &v) == nil
	t.Logf("  typical content: %s", smokeHead([]byte(r.Choices[0].Message.Content)))
	return d, ok, r.Usage.PromptTokens, r.Usage.CompletionTokens
}

// TestLiveGuideSmokeShort — короткий выход (извлечение условий, ~30 токенов): сколько стоит один ход без фразы.
func TestLiveGuideSmokeShort(t *testing.T) {
	_ = godotenv.Load("../.env")
	cfg, err := config.LoadLLM()
	if err != nil || !cfg.Online() {
		t.Skip("модель не настроена")
	}
	flash := "gpt://" + cfg.FolderID + "/aliceai-llm-flash"
	for i := 0; i < 3; i++ {
		code, body, d := smokePost(t, cfg, map[string]any{
			"model": flash, "max_tokens": 200, "temperature": 0,
			"messages": []map[string]any{
				{"role": "system", "content": `Извлеки условия подбора лицензий. Только JSON: {"a":"compare|select|clarify|other","n":число мест или null,"op":"new|renew|null","feat":["encryption"|"patch_management"]}`},
				{"role": "user", "content": `<query>"на самом деле компьютеров будет 50, и шифрование нужно"</query>`},
			},
			"response_format": map[string]any{"type": "json_object"},
		})
		var r smokeResp
		_ = json.Unmarshal(body, &r)
		c := ""
		if len(r.Choices) > 0 {
			c = r.Choices[0].Message.Content
		}
		t.Logf("SHORT #%d HTTP %d %v in=%d out=%d %s", i+1, code, d.Round(time.Millisecond), r.Usage.PromptTokens, r.Usage.CompletionTokens, c)
	}
}

// TestLiveGuidePrompt — промпт подбора на входе хода «сравни редакции»: время и ответ.
func TestLiveGuidePrompt(t *testing.T) {
	_ = godotenv.Load("../.env")
	cfg, err := config.LoadLLM()
	if err != nil || !cfg.Online() {
		t.Skip("модель не настроена")
	}
	in := guideModelInput{Query: "чем расширенный касперский отличается от стандартного",
		Conditions: []string{"Kaspersky", "30 ПК", "новая лицензия", "1 год"},
		Families:   []map[string]string{{"family_id": "3953", "title": "Стандартный"}, {"family_id": "3935", "title": "Расширенный"}},
		Facts: []map[string]string{{"fact_id": "kb_kes_std_encryption", "family_id": "3953", "axis": "encryption", "value": "no"},
			{"fact_id": "kb_kes_adv_encryption", "family_id": "3935", "axis": "encryption", "value": "yes"},
			{"fact_id": "kb_kes_std_patch", "family_id": "3953", "axis": "patch_management", "value": "no"},
			{"fact_id": "kb_kes_adv_patch", "family_id": "3935", "axis": "patch_management", "value": "yes"}},
		Offers: []map[string]string{{"offer_id": "4985", "family_id": "3953"}, {"offer_id": "4856", "family_id": "3935"}}}
	m := llm.New(cfg.BaseURL, cfg.IntentModel, cfg.FolderID, cfg.APIKey, 30*time.Second)
	for i := 0; i < 2; i++ {
		zero := 0.0
		started := time.Now()
		resp, err := m.Chat(context.Background(), llm.Request{Messages: []llm.Message{{Role: "system", Content: prompts.Guide().Text},
			{Role: "user", Content: in.render()}}, MaxTokens: guideMaxTokens, Temperature: &zero, ResponseFormat: llm.JSONObject})
		if err != nil {
			t.Logf("err %v", err)
			continue
		}
		t.Logf("#%d %v in=%d out=%d finish=%s %s", i+1, time.Since(started).Round(time.Millisecond), resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.FinishReason, resp.Message.Content)
	}
}
