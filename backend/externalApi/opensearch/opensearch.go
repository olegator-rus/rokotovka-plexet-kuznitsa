// Package opensearch — тонкий клиент REST API OpenSearch на net/http: ровно те вызовы, что нужны импорту
// и поиску. Тела запросов собирает вызывающий (services), клиент их только отправляет и разбирает ответ.
//
// Сбой реализует errs.External: недоступность — 503, таймаут — 504, ответ не 2xx — 502.
package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// maxResponseBody — потолок тела ответа: выдача на 50–100 документов с векторами в _source не нужна,
// но разбор маппинга и ошибок bulk укладывается с запасом.
const maxResponseBody = 32 << 20

// Client — клиент одного кластера. nil — OpenSearch не настроен.
type Client struct {
	base string
	http *http.Client
}

// New — клиент; пустой адрес — nil (поиск выключен).
func New(baseURL string, timeout time.Duration) *Client {
	if baseURL == "" {
		return nil
	}
	return &Client{base: strings.TrimRight(baseURL, "/"), http: &http.Client{Timeout: timeout}}
}

// Enabled — клиент настроен.
func (c *Client) Enabled() bool { return c != nil }

// Error — сбой вызова. Body — начало тела ответа OpenSearch (в нём нет данных покупателей: тела
// запросов в ошибку не кладутся).
type Error struct {
	Op     string
	Status int // 0 — ответа не было
	Body   string
	Err    error
}

func (e *Error) Error() string {
	if e.Status == 0 {
		return fmt.Sprintf("opensearch %s: %v", e.Op, e.Err)
	}
	return fmt.Sprintf("opensearch %s: HTTP %d: %s", e.Op, e.Status, e.Body)
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
	case e.Status == http.StatusNotFound:
		return http.StatusNotFound
	}
	return http.StatusBadGateway
}

// ErrDisabled — вызов при выключенном клиенте.
var ErrDisabled = &Error{Op: "config", Err: errors.New("OPENSEARCH_URL не задан")}

// IsNotFound — индекс или алиас не найден.
func IsNotFound(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Status == http.StatusNotFound
}

// do — один вызов. body: nil, []byte (как есть, NDJSON для bulk) или значение для json.Marshal. out —
// куда разобрать ответ (nil — не разбирать).
func (c *Client) do(ctx context.Context, op, method, path string, body, out any) error {
	if c == nil {
		return ErrDisabled
	}
	var rd io.Reader
	contentType := "application/json"
	switch b := body.(type) {
	case nil:
	case []byte:
		rd = bytes.NewReader(b)
		contentType = "application/x-ndjson"
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			return fmt.Errorf("opensearch %s: тело: %w", op, err)
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return &Error{Op: op, Err: err}
	}
	if rd != nil {
		req.Header.Set("Content-Type", contentType)
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
		return &Error{Op: op, Status: resp.StatusCode, Body: errorBody(op, raw)}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return &Error{Op: op, Status: http.StatusBadGateway, Body: "ответ не JSON", Err: err}
	}
	return nil
}

// Ping — кластер отвечает и не красный.
func (c *Client) Ping(ctx context.Context) error {
	var h struct {
		Status string `json:"status"`
	}
	if err := c.do(ctx, "health", http.MethodGet, "/_cluster/health", nil, &h); err != nil {
		return err
	}
	if h.Status == "red" {
		return &Error{Op: "health", Status: http.StatusServiceUnavailable, Body: "кластер red"}
	}
	return nil
}

// CreateIndex — новый индекс с настройками и маппингом.
func (c *Client) CreateIndex(ctx context.Context, name string, body any) error {
	return c.do(ctx, "create index", http.MethodPut, "/"+url.PathEscape(name), body, nil)
}

// DeleteIndex — удалить индекс.
func (c *Client) DeleteIndex(ctx context.Context, name string) error {
	return c.do(ctx, "delete index", http.MethodDelete, "/"+url.PathEscape(name), nil, nil)
}

// Refresh — сделать записанное видимым для поиска.
func (c *Client) Refresh(ctx context.Context, index string) error {
	return c.do(ctx, "refresh", http.MethodPost, "/"+url.PathEscape(index)+"/_refresh", nil, nil)
}

// Count — число документов в индексе или алиасе.
func (c *Client) Count(ctx context.Context, index string) (int, error) {
	var r struct {
		Count int `json:"count"`
	}
	err := c.do(ctx, "count", http.MethodGet, "/"+url.PathEscape(index)+"/_count", nil, &r)
	return r.Count, err
}

// BulkItemError — отказ одного документа в bulk.
type BulkItemError struct {
	ID     string
	Status int
	Reason string
}

// Bulk — пакетная запись NDJSON. Ошибка — если запрос не прошёл или хоть один документ отклонён
// (первые отказы — в тексте ошибки).
func (c *Client) Bulk(ctx context.Context, ndjson []byte) error {
	var r struct {
		Errors bool `json:"errors"`
		Items  []map[string]struct {
			ID     string `json:"_id"`
			Status int    `json:"status"`
			Error  *struct {
				Type   string `json:"type"`
				Reason string `json:"reason"`
			} `json:"error"`
		} `json:"items"`
	}
	if err := c.do(ctx, "bulk", http.MethodPost, "/_bulk", ndjson, &r); err != nil {
		return err
	}
	if !r.Errors {
		return nil
	}
	var failed []string
	for _, it := range r.Items {
		for _, v := range it {
			if v.Error != nil && len(failed) < 3 {
				failed = append(failed, fmt.Sprintf("%s: %s: %s", v.ID, v.Error.Type, v.Error.Reason))
			}
		}
	}
	return &Error{Op: "bulk", Status: http.StatusBadGateway, Body: strings.Join(failed, "; ")}
}

// AliasIndices — индексы, на которые указывает алиас (пусто — алиаса нет).
func (c *Client) AliasIndices(ctx context.Context, alias string) ([]string, error) {
	var r map[string]json.RawMessage
	err := c.do(ctx, "get alias", http.MethodGet, "/_alias/"+url.PathEscape(alias), nil, &r)
	if IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(r))
	for name := range r {
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

// SwapAlias — атомарно перевести алиас на index (со всех прежних индексов).
func (c *Client) SwapAlias(ctx context.Context, alias, index string, previous []string) error {
	actions := make([]map[string]any, 0, len(previous)+1)
	for _, p := range previous {
		actions = append(actions, map[string]any{"remove": map[string]string{"index": p, "alias": alias}})
	}
	actions = append(actions, map[string]any{"add": map[string]string{"index": index, "alias": alias}})
	return c.do(ctx, "swap alias", http.MethodPost, "/_aliases", map[string]any{"actions": actions}, nil)
}

// Indices — имена индексов по шаблону (products_v*); нет ни одного — пустой список.
func (c *Client) Indices(ctx context.Context, pattern string) ([]string, error) {
	var r []struct {
		Index string `json:"index"`
	}
	err := c.do(ctx, "cat indices", http.MethodGet,
		"/_cat/indices/"+url.PathEscape(pattern)+"?format=json&h=index&expand_wildcards=open", nil, &r)
	if IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(r))
	for _, it := range r {
		out = append(out, it.Index)
	}
	sort.Strings(out)
	return out, nil
}

// Meta — _meta маппинга индекса или алиаса (первого индекса за ним).
func (c *Client) Meta(ctx context.Context, index string, out any) error {
	var r map[string]struct {
		Mappings struct {
			Meta json.RawMessage `json:"_meta"`
		} `json:"mappings"`
	}
	if err := c.do(ctx, "get mapping", http.MethodGet, "/"+url.PathEscape(index)+"/_mapping", nil, &r); err != nil {
		return err
	}
	for _, v := range r {
		if len(v.Mappings.Meta) == 0 {
			return nil
		}
		return json.Unmarshal(v.Mappings.Meta, out)
	}
	return nil
}

// Hit — найденный документ.
type Hit struct {
	ID     string          `json:"_id"`
	Score  float64         `json:"_score"`
	Source json.RawMessage `json:"_source"`
}

// SearchResult — ответ _search: попадания и время на стороне OpenSearch.
type SearchResult struct {
	TookMs int `json:"took"`
	Hits   struct {
		Hits []Hit `json:"hits"`
	} `json:"hits"`
}

// Search — _search по индексу или алиасу.
func (c *Client) Search(ctx context.Context, index string, body any) (*SearchResult, error) {
	var r SearchResult
	if err := c.do(ctx, "search", http.MethodPost, "/"+url.PathEscape(index)+"/_search", body, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// errorBody — текст отказа для лога. Отказ поиска OpenSearch цитирует запрос (в нём фраза покупателя),
// поэтому от него остаётся только тип ошибки.
func errorBody(op string, raw []byte) string {
	if op != "search" {
		return head(raw, 500)
	}
	var e struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &e) == nil && e.Error.Type != "" {
		return e.Error.Type
	}
	return "текст отказа скрыт"
}

func head(b []byte, n int) string {
	s := string(b)
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
