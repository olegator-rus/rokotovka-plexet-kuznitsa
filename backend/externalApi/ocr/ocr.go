// Package ocr — клиент Yandex Vision OCR (синхронный recognizeText): одна картинка страницы → слова с рамками.
// Ключ живёт только в поле клиента и в заголовке Authorization; в ошибки и лог не попадает. Распознанный текст
// в лог не пишется: в сканах ТЗ бывают персональные данные.
package ocr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Word — распознанное слово и его рамка в пикселях картинки (y растёт вниз). Line — номер строки текста, которой
// слово принадлежит по разметке распознавания (строка внутри одной колонки).
type Word struct {
	Text           string
	X0, Y0, X1, Y1 float64
	Line           int
}

// Error — сервис ответил не 2xx. Body — начало тела без ключа.
type Error struct {
	Status int
	Body   string
}

func (e *Error) Error() string { return fmt.Sprintf("ocr: HTTP %d: %s", e.Status, e.Body) }

// ExternalStatus — перегрузка сервиса — 503, остальное — 502.
func (e *Error) ExternalStatus() int {
	if e.Status == http.StatusTooManyRequests || e.Status == http.StatusServiceUnavailable {
		return http.StatusServiceUnavailable
	}
	return http.StatusBadGateway
}

const maxResponse = 16 << 20

// Client — клиент recognizeText.
type Client struct {
	url, apiKey, folder string
	http                *http.Client
}

// New — клиент; timeout — срок одного распознавания.
func New(url, apiKey, folder string, timeout time.Duration) *Client {
	return &Client{url: url, apiKey: apiKey, folder: folder, http: &http.Client{Timeout: timeout}}
}

// Recognize — слова одной картинки (mime: JPEG | PNG) на русском и английском, модель page.
func (c *Client) Recognize(ctx context.Context, image []byte, mime string) ([]Word, error) {
	body, err := json.Marshal(map[string]any{
		"mimeType": mime, "languageCodes": []string{"ru", "en"}, "model": "page",
		"content": base64.StdEncoding.EncodeToString(image),
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Api-Key "+c.apiKey)
	if c.folder != "" {
		req.Header.Set("x-folder-id", c.folder)
	}
	// Данные запроса не сохраняются у провайдера для обучения.
	req.Header.Set("x-data-logging-enabled", "false")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b := string(raw)
		if len(b) > 300 {
			b = b[:300]
		}
		return nil, &Error{Status: resp.StatusCode, Body: strings.ReplaceAll(b, c.apiKey, "***")}
	}
	return parse(raw)
}

type vertex struct {
	X json.RawMessage `json:"x"`
	Y json.RawMessage `json:"y"`
}

type box struct {
	Vertices []vertex `json:"vertices"`
}

// parse — слова из ответа: result.textAnnotation.blocks[].lines[].words[].
func parse(raw []byte) ([]Word, error) {
	var r struct {
		Result struct {
			TextAnnotation struct {
				Blocks []struct {
					Lines []struct {
						Words []struct {
							Text        string `json:"text"`
							BoundingBox box    `json:"boundingBox"`
						} `json:"words"`
					} `json:"lines"`
				} `json:"blocks"`
			} `json:"textAnnotation"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("ocr: ответ не JSON: %w", err)
	}
	var words []Word
	line := 0
	for _, b := range r.Result.TextAnnotation.Blocks {
		for _, l := range b.Lines {
			line++
			for _, w := range l.Words {
				if strings.TrimSpace(w.Text) == "" || len(w.BoundingBox.Vertices) == 0 {
					continue
				}
				wd := Word{Text: w.Text, X0: 1e18, Y0: 1e18, X1: -1, Y1: -1, Line: line}
				for _, v := range w.BoundingBox.Vertices {
					x, y := num(v.X), num(v.Y)
					wd.X0, wd.X1 = min(wd.X0, x), max(wd.X1, x)
					wd.Y0, wd.Y1 = min(wd.Y0, y), max(wd.Y1, y)
				}
				words = append(words, wd)
			}
		}
	}
	return words, nil
}

// num — координата: в ответе число строкой («"123"») или числом; нет — 0.
func num(m json.RawMessage) float64 {
	s := strings.Trim(string(m), `"`)
	f, _ := strconv.ParseFloat(s, 64)
	return f
}
