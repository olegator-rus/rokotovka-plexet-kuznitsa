package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// Фейковый OpenAI-совместимый сервер: проверяем тело запроса (модель, max_tokens ≥ 1000, инструменты, нет полей
// сверх OpenAI — Yandex отвечает на них 400), заголовки ключа и каталога, разбор вызова инструмента. Сеть не нужна.
func TestChatRequestAndToolCall(t *testing.T) {
	var got map[string]any
	var auth, project string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		auth = r.Header.Get("Authorization")
		project = r.Header.Get("OpenAI-Project")
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":"",
			"reasoning":"скрытые рассуждения","tool_calls":[{"id":"c1","type":"function","function":{"name":"search_catalog","arguments":"{\"query\":\"фотошоп\"}"}}]}}],
			"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))
	}))
	defer srv.Close()

	c := New(srv.URL+"/v1/", "gpt://b1gtest/aliceai-llm-flash", "b1gtest", "sk-test", time.Second)
	resp, err := c.Chat(context.Background(), Request{
		Messages: []Message{{Role: "user", Content: "Аналог фотошопа на 20 пользователей"}},
		Tools:    []Tool{{Type: "function", Function: ToolFunction{Name: "search_catalog", Description: "d", Parameters: json.RawMessage(`{"type":"object"}`)}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer sk-test" || project != "b1gtest" {
		t.Fatalf("Authorization = %q, OpenAI-Project = %q", auth, project)
	}
	if got["model"] != "gpt://b1gtest/aliceai-llm-flash" || got["max_tokens"].(float64) < MinMaxTokens {
		t.Fatalf("тело: %v", got)
	}
	for k := range got {
		if k != "model" && k != "messages" && k != "tools" && k != "max_tokens" {
			t.Fatalf("лишнее поле %q: Yandex отвечает 400 на незнакомые параметры", k)
		}
	}
	if tools, _ := got["tools"].([]any); len(tools) != 1 {
		t.Fatalf("tools: %v", got["tools"])
	}
	if len(resp.Message.ToolCalls) != 1 || resp.Message.ToolCalls[0].Function.Name != "search_catalog" ||
		!strings.Contains(resp.Message.ToolCalls[0].Function.Arguments, "фотошоп") || resp.Usage.TotalTokens != 15 {
		t.Fatalf("ответ: %+v", resp)
	}
	if strings.Contains(resp.Message.Content, "рассужд") {
		t.Fatal("рассуждения попали в текст ответа")
	}
}

// Alice AI LLM отдаёт id вызова, равный имени инструмента: два вызова в одном ответе — одинаковые id.
// Клиент делает их уникальными, иначе ответы инструментов перепутаются.
func TestChatDuplicateToolCallIDs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"tool_calls":[
			{"id":"search_catalog","type":"function","function":{"name":"search_catalog","arguments":"{\"query\":\"фотошоп\"}"}},
			{"id":"search_catalog","type":"function","function":{"name":"search_catalog","arguments":"{\"query\":\"автокад\"}"}},
			{"id":"","type":"function","function":{"name":"get_meta","arguments":"{}"}}]}}]}`))
	}))
	defer srv.Close()
	resp, err := New(srv.URL, "m", "", "sk-test", time.Second).Chat(context.Background(), Request{Messages: []Message{{Role: "user", Content: "q"}}})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, c := range resp.Message.ToolCalls {
		if c.ID == "" || seen[c.ID] {
			t.Fatalf("id не уникальны: %+v", resp.Message.ToolCalls)
		}
		seen[c.ID] = true
	}
	if resp.Message.ToolCalls[0].ID != "search_catalog" || resp.Message.Content != "" {
		t.Fatalf("первый id меняться не должен, content null → пусто: %+v", resp.Message)
	}
}

func TestChatFailures(t *testing.T) {
	cases := map[string]struct {
		handler func(w http.ResponseWriter, r *http.Request)
		check   func(error) bool
	}{
		"401 без ключа в тексте ошибки": {
			func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(401)
				_, _ = w.Write([]byte(`{"error":{"message":"bad key sk-test"}}`))
			},
			func(err error) bool {
				var e *Error
				return errors.As(err, &e) && e.Status == 401 && !strings.Contains(err.Error(), "sk-test")
			},
		},
		"пустые choices": {
			func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"choices":[]}`)) },
			func(err error) bool { return errors.Is(err, ErrEmpty) },
		},
		"ошибка в теле 200": {
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"error":{"message":"overloaded"}}`))
			},
			func(err error) bool { var e *Error; return errors.As(err, &e) },
		},
		"мусор": {
			func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`<html>`)) },
			func(err error) bool { return err != nil },
		},
		"таймаут": {
			func(w http.ResponseWriter, r *http.Request) {
				select {
				case <-r.Context().Done():
				case <-time.After(time.Second):
				}
			},
			func(err error) bool { return err != nil },
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(c.handler))
			defer srv.Close()
			_, err := New(srv.URL, "m", "", "sk-test", 300*time.Millisecond).Chat(context.Background(), Request{Messages: []Message{{Role: "user", Content: "q"}}})
			if !c.check(err) {
				t.Fatalf("ошибка %v", err)
			}
		})
	}
}

// Живой вызов Yandex AI Studio — только вручную: LLM_LIVE_TEST=1, LLM_API_KEY и LLM_FOLDER_ID в окружении.
// В CI и обычном go test пропускается.
func TestChatLive(t *testing.T) {
	key, folder := os.Getenv("LLM_API_KEY"), os.Getenv("LLM_FOLDER_ID")
	if os.Getenv("LLM_LIVE_TEST") != "1" || key == "" || folder == "" {
		t.Skip("живой вызов: LLM_LIVE_TEST=1, LLM_API_KEY и LLM_FOLDER_ID")
	}
	base := os.Getenv("LLM_BASE_URL")
	if base == "" {
		base = "https://ai.api.cloud.yandex.net/v1"
	}
	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = "aliceai-llm-flash"
	}
	if !strings.Contains(model, "://") {
		model = "gpt://" + folder + "/" + model
	}
	c := New(base, model, folder, key, 60*time.Second)
	resp, err := c.Chat(context.Background(), Request{
		Messages: []Message{
			{Role: "system", Content: "Ты разбираешь запросы покупателей магазина ПО. Отвечай только через инструменты."},
			{Role: "user", Content: "Чем заменить фотошоп на 20 рабочих мест?"},
		},
		Tools: []Tool{{Type: "function", Function: ToolFunction{Name: "search_catalog", Description: "Поиск по каталогу ПО",
			Parameters: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`)}}},
	})
	if err != nil {
		t.Fatalf("живой вызов: %v", err)
	}
	calls := make([]string, 0, len(resp.Message.ToolCalls))
	for _, tc := range resp.Message.ToolCalls {
		calls = append(calls, tc.Function.Name+" "+tc.Function.Arguments)
	}
	t.Logf("finish=%s tool_calls=%v text=%q tokens=%d", resp.FinishReason, calls, resp.Message.Content, resp.Usage.TotalTokens)
}
