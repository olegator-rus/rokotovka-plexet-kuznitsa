// Package llm — клиент OpenAI-совместимого /chat/completions (Yandex AI Studio, Alice AI LLM Flash).
// Только вызов: промптов, разбора намерения и учёта стоимости здесь нет — это services.
//
// Ключ живёт только в поле клиента и в заголовке Authorization; в ошибки, лог и тело ответа не попадает.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Message — сообщение диалога в формате OpenAI. Content у ассистента с вызовами инструментов может быть пуст.
type Message struct {
	Role       string     `json:"role"` // system | user | assistant | tool
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// ToolCall — вызов инструмента моделью; Arguments — JSON-строка.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // function
	Function FunctionCall `json:"function"`
}

// FunctionCall — имя и аргументы вызова.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool — описание инструмента для модели (function calling, JSON Schema параметров).
type Tool struct {
	Type     string       `json:"type"` // function
	Function ToolFunction `json:"function"`
}

// ToolFunction — имя, описание и схема параметров.
type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Request — тело POST /chat/completions. Полей сверх OpenAI-набора нет: Yandex отвечает 400 на
// незнакомые параметры.
type Request struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Tools       []Tool    `json:"tools,omitempty"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature *float64  `json:"temperature,omitempty"`
}

// Usage — расход токенов (для llm_usage и расчёта стоимости).
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Response — ответ модели: первый choice. Рассуждения (reasoning) в структуру не читаются намеренно.
type Response struct {
	Message      Message
	FinishReason string
	Usage        Usage
}

// Error — модель ответила не 2xx. Body — начало тела без ключа (ключ в тело не возвращается).
type Error struct {
	Status int
	Body   string
}

func (e *Error) Error() string { return fmt.Sprintf("llm: HTTP %d: %s", e.Status, e.Body) }

// ExternalStatus — статус для errs.FromExternal: перегрузка провайдера — 503, остальное — 502.
func (e *Error) ExternalStatus() int {
	if e.Status == http.StatusTooManyRequests || e.Status == http.StatusServiceUnavailable {
		return http.StatusServiceUnavailable
	}
	return http.StatusBadGateway
}

// ErrEmpty — 200 без choices: считаем сбоем провайдера.
var ErrEmpty = errors.New("llm: пустой ответ модели")

// Минимум max_tokens: запас, чтобы JSON разбора не обрезался посередине.
const MinMaxTokens = 1000

// maxResponseBody — предел тела ответа модели.
const maxResponseBody = 2 << 20

// Client — OpenAI-совместимый клиент.
type Client struct {
	baseURL string
	model   string
	project string
	apiKey  string
	http    *http.Client
}

// New — клиент; project — каталог Yandex Cloud для заголовка OpenAI-Project (пусто — без заголовка);
// timeout — срок одного вызова модели.
func New(baseURL, model, project, apiKey string, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		project: project,
		apiKey:  apiKey,
		http:    &http.Client{Timeout: timeout},
	}
}

// Model — имя модели (для учёта расхода).
func (c *Client) Model() string { return c.model }

// Chat — один вызов /chat/completions без стриминга. Model и MaxTokens (не меньше MinMaxTokens) клиент
// проставляет сам, если вызывающий не задал.
func (c *Client) Chat(ctx context.Context, req Request) (*Response, error) {
	if req.Model == "" {
		req.Model = c.model
	}
	if req.MaxTokens < MinMaxTokens {
		req.MaxTokens = MinMaxTokens
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("Authorization", "Bearer "+c.apiKey)
	if c.project != "" {
		hreq.Header.Set("OpenAI-Project", c.project)
	}
	resp, err := c.http.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &Error{Status: resp.StatusCode, Body: c.scrub(head(raw, 300))}
	}
	var parsed struct {
		Choices []struct {
			Message      Message `json:"message"`
			FinishReason string  `json:"finish_reason"`
		} `json:"choices"`
		Usage Usage `json:"usage"`
		Error *struct {
			Message string `json:"message"`
			Code    any    `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("llm: ответ не JSON: %w", err)
	}
	if parsed.Error != nil {
		return nil, &Error{Status: http.StatusBadGateway, Body: c.scrub(head([]byte(parsed.Error.Message), 300))}
	}
	if len(parsed.Choices) == 0 {
		return nil, ErrEmpty
	}
	ch := parsed.Choices[0]
	uniqueToolCallIDs(ch.Message.ToolCalls)
	return &Response{Message: ch.Message, FinishReason: ch.FinishReason, Usage: parsed.Usage}, nil
}

// uniqueToolCallIDs — Alice AI LLM даёт вызову id, равный имени инструмента: два вызова одного инструмента
// в одном ответе приходят с одинаковым id, и ответы инструментов перепутались бы. Пустой или повторный
// id заменяется на call_<номер>; модель принимает любой id, лишь бы ответ tool ссылался на него же.
func uniqueToolCallIDs(calls []ToolCall) {
	seen := make(map[string]bool, len(calls))
	for i := range calls {
		if calls[i].ID == "" || seen[calls[i].ID] {
			calls[i].ID = fmt.Sprintf("call_%d", i+1)
		}
		for seen[calls[i].ID] {
			calls[i].ID += "_"
		}
		seen[calls[i].ID] = true
	}
}

// scrub — на случай, если провайдер эхнёт заголовок: ключ из текста ошибки вырезается.
func (c *Client) scrub(s string) string {
	if c.apiKey == "" {
		return s
	}
	return strings.ReplaceAll(s, c.apiKey, "***")
}

func head(b []byte, n int) string {
	s := string(b)
	if len([]rune(s)) > n {
		return string([]rune(s)[:n]) + "…"
	}
	return s
}
