// Package tei — клиент сервиса моделей с API Text Embeddings Inference:
// POST /embed, POST /rerank, GET /health. Локально это нативный сервис на 127.0.0.1:8089, на стенде —
// контейнер TEI; Go видит одинаковый API.
//
// Сбой реализует errs.External: недоступность — 503, таймаут — 504, ответ не по контракту — 502.
package tei

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// maxResponseBody — потолок ответа: 64 вектора × 1024 числа в JSON — около 1,5 МБ.
const maxResponseBody = 16 << 20

// maxIdleConns — простаивающих соединений к сервису моделей (≈ число одновременных поисков).
const maxIdleConns = 64

// Client — клиент сервиса моделей. nil — сервис не настроен, шаг пропускается.
type Client struct {
	base string
	http *http.Client
}

// New — клиент; пустой адрес — nil.
func New(baseURL string, timeout time.Duration) *Client {
	if baseURL == "" {
		return nil
	}
	// Поиск зовёт сервис моделей из многих горутин сразу: у транспорта по умолчанию на хост держится 2
	// простаивающих соединения, остальные под нагрузкой открываются заново на каждый вызов.
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.MaxIdleConns, tr.MaxIdleConnsPerHost = maxIdleConns, maxIdleConns
	return &Client{base: strings.TrimRight(baseURL, "/"), http: &http.Client{Timeout: timeout, Transport: tr}}
}

// Enabled — клиент настроен.
func (c *Client) Enabled() bool { return c != nil }

// Error — сбой вызова.
type Error struct {
	Op     string
	Status int // 0 — ответа не было
	Body   string
	Err    error
}

func (e *Error) Error() string {
	if e.Status == 0 {
		return fmt.Sprintf("tei %s: %v", e.Op, e.Err)
	}
	return fmt.Sprintf("tei %s: HTTP %d: %s", e.Op, e.Status, e.Body)
}

func (e *Error) Unwrap() error { return e.Err }

// ExternalStatus — статус для errs.FromExternal.
func (e *Error) ExternalStatus() int {
	var ne net.Error
	switch {
	case errors.Is(e.Err, context.DeadlineExceeded), errors.As(e.Err, &ne) && ne.Timeout():
		return http.StatusGatewayTimeout
	case e.Status == 0:
		return http.StatusServiceUnavailable
	}
	return http.StatusBadGateway
}

// ErrDisabled — вызов при выключенном клиенте.
var ErrDisabled = &Error{Op: "config", Err: errors.New("адрес сервиса моделей не задан")}

// do — POST/GET с JSON. Тело ответа об ошибке в Error не попадает целиком: в нём может быть текст
// запроса покупателя.
func (c *Client) do(ctx context.Context, op, method, path string, body, out any) error {
	if c == nil {
		return ErrDisabled
	}
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("tei %s: тело: %w", op, err)
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return &Error{Op: op, Err: err}
	}
	if rd != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return &Error{Op: op, Err: err}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return &Error{Op: op, Err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &Error{Op: op, Status: resp.StatusCode, Body: http.StatusText(resp.StatusCode)}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return &Error{Op: op, Status: http.StatusBadGateway, Body: "ответ не по контракту", Err: err}
	}
	return nil
}

// Health — сервис отвечает 200.
func (c *Client) Health(ctx context.Context) error {
	return c.do(ctx, "health", http.MethodGet, "/health", nil, nil)
}

// Embed — нормированные векторы текстов в том же порядке. Число векторов и их размерность сверяются:
// ответ не по контракту — ошибка, а не тихо обрезанный индекс.
func (c *Client) Embed(ctx context.Context, texts []string, dim int) ([][]float32, error) {
	var out [][]float32
	if err := c.do(ctx, "embed", http.MethodPost, "/embed", map[string]any{"inputs": texts, "normalize": true}, &out); err != nil {
		return nil, err
	}
	if len(out) != len(texts) {
		return nil, &Error{Op: "embed", Status: http.StatusBadGateway,
			Body: fmt.Sprintf("векторов %d, текстов %d", len(out), len(texts))}
	}
	for _, v := range out {
		if len(v) != dim {
			return nil, &Error{Op: "embed", Status: http.StatusBadGateway,
				Body: fmt.Sprintf("размерность %d, ожидалась %d", len(v), dim)}
		}
	}
	return out, nil
}

// RerankHit — оценка текста реранкером; Index — номер в переданном списке.
type RerankHit struct {
	Index int     `json:"index"`
	Score float64 `json:"score"`
}

// Rerank — оценки текстов относительно запроса, по убыванию.
func (c *Client) Rerank(ctx context.Context, query string, texts []string) ([]RerankHit, error) {
	var out []RerankHit
	err := c.do(ctx, "rerank", http.MethodPost, "/rerank",
		map[string]any{"query": query, "texts": texts, "raw_scores": false}, &out)
	if err != nil {
		return nil, err
	}
	for _, h := range out {
		if h.Index < 0 || h.Index >= len(texts) {
			return nil, &Error{Op: "rerank", Status: http.StatusBadGateway, Body: "индекс вне списка"}
		}
	}
	return out, nil
}
