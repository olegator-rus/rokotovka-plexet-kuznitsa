package services

import (
	"context"
	"math"
	"strings"
	"time"

	"kuznica/api/domain"

	"go.uber.org/zap"
)

// Учёт вызовов модели (llm_usage, ARCHITECTURE §4 и §7): каждый вызов — строка с токенами, задержкой,
// версией прайса и посчитанной стоимостью в рублях. Стоимость считает сервер по прайсу ниже.

// PricingVersion — версия прайса Yandex AI Studio, по которому считается calculated_cost.
const PricingVersion = "yandex-2026-05-26"

// PricingDate — дата, на которую взят прайс (для экрана «Экономика»).
const PricingDate = "2026-05-26"

// LLMProviderYandex — провайдер в llm_usage.provider.
const LLMProviderYandex = "yandex-ai-studio"

// ModelPrice — цена модели, ₽ за 1 млн токенов.
type ModelPrice struct {
	Model  string  `json:"model"`
	Input  float64 `json:"input_rub_per_mtok"`
	Output float64 `json:"output_rub_per_mtok"`
}

// modelPrices — прайс на PricingDate (ARCHITECTURE §7). Токены из кэша провайдера считаются по цене
// входа: скидку за них прайс не обещает, оценка консервативная.
var modelPrices = []ModelPrice{
	{Model: "aliceai-llm-flash", Input: 100, Output: 200},
	{Model: "qwen3.6-35b-a3b", Input: 200, Output: 300},
	{Model: "aliceai-llm", Input: 500, Output: 1200},
}

// Статусы вызова (llm_usage.status).
const (
	llmStatusOK          = "ok"
	llmStatusError       = "error"
	llmStatusTimeout     = "timeout"
	llmStatusSchemaError = "schema_error"
)

// llmModelName — имя модели без схемы и каталога: gpt://<каталог>/aliceai-llm-flash/latest →
// aliceai-llm-flash. Каталог не уходит ни в трассу, ни в журнал.
func llmModelName(m string) string {
	if i := strings.Index(m, "://"); i >= 0 {
		m = m[i+3:]
		if j := strings.Index(m, "/"); j >= 0 {
			m = m[j+1:]
		}
	}
	for _, suffix := range []string{"/latest", "/rc", "/deprecated"} {
		m = strings.TrimSuffix(m, suffix)
	}
	return m
}

// priceOf — цена модели; ok=false — модели нет в прайсе.
func priceOf(model string) (ModelPrice, bool) {
	m := llmModelName(model)
	for _, p := range modelPrices {
		if p.Model == m {
			return p, true
		}
	}
	return ModelPrice{}, false
}

// llmCost — стоимость вызова в рублях, округлённая до 1/10000 ₽ (точность колонки numeric(12,4)).
func llmCost(model string, input, output int) float64 {
	p, ok := priceOf(model)
	if !ok {
		return 0
	}
	return math.Round((float64(input)*p.Input+float64(output)*p.Output)/1e6*1e4) / 1e4
}

// llmCall — один вызов модели для учёта.
type llmCall struct {
	Model        string
	InputTokens  int
	OutputTokens int
	CachedTokens int
	Latency      time.Duration
	Status       string
}

// Контекст вызова: к какому поиску или запуску агента относится расход.
type llmCtxKey int

const (
	llmCtxRequestID llmCtxKey = iota
	llmCtxAgentRunID
)

// WithRequestID — привязать вызовы модели в ctx к поиску request_id (llm_usage.request_id). Поиск
// оборачивает контекст перед IntentParser.Parse.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, llmCtxRequestID, requestID)
}

// WithAgentRunID — привязать вызовы модели к запуску агента (llm_usage.agent_run_id).
func WithAgentRunID(ctx context.Context, runID string) context.Context {
	return context.WithValue(ctx, llmCtxAgentRunID, runID)
}

func llmCtxString(ctx context.Context, k llmCtxKey) *string {
	if v, ok := ctx.Value(k).(string); ok && v != "" {
		return &v
	}
	return nil
}

// recordLLMUsage — строка llm_usage в фоне (поиск не ждёт записи); возвращает стоимость вызова.
func (s *Services) recordLLMUsage(ctx context.Context, c llmCall) float64 {
	cost := llmCost(c.Model, c.InputTokens, c.OutputTokens)
	if _, ok := priceOf(c.Model); !ok {
		zap.L().Warn("llm: модели нет в прайсе — стоимость 0", zap.String("model", llmModelName(c.Model)),
			zap.String("pricing_version", PricingVersion))
	}
	if !s.repos.Enabled() {
		return cost
	}
	row := &domain.LLMUsage{
		RequestID: llmCtxString(ctx, llmCtxRequestID), AgentRunID: llmCtxString(ctx, llmCtxAgentRunID),
		Provider: LLMProviderYandex, Model: llmModelName(c.Model),
		InputTokens: c.InputTokens, OutputTokens: c.OutputTokens, CachedTokens: c.CachedTokens,
		LatencyMs: int(math.Round(float64(c.Latency.Microseconds()) / 1000)), PricingVersion: PricingVersion,
		CalculatedCost: cost, Status: c.Status,
	}
	s.writes.Add(1)
	go func() {
		defer s.writes.Done()
		wctx, cancel := context.WithTimeout(context.Background(), llmUsageSaveTimeout)
		defer cancel()
		if err := s.repos.SaveLLMUsage(wctx, row); err != nil {
			zap.L().Warn("llm: вызов не записан в llm_usage", zap.Error(err))
		}
	}()
	return cost
}

// llmUsageSaveTimeout — срок фоновой записи строки учёта.
const llmUsageSaveTimeout = 3 * time.Second
