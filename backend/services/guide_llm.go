package services

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"kuznica/api/externalApi/llm"
	"kuznica/api/prompts"
)

// Вызов модели в ходе подбора: один короткий JSON (CHECKS.md). Семафор — 5 одновременных вызовов на процесс;
// бюджет и квота адреса — общие с разбором намерения (llmguard.go); расход — строкой llm_usage.

const (
	guideLLMSlots     = 5
	guideCallTimeout  = 4500 * time.Millisecond
	guideMaxTokens    = 400
	guideLeadMaxWords = 60
)

// Причины ответа без модели (meta.fallback).
const (
	guideFbNoModel = "llm_not_configured"
	guideFbOff     = "llm_off"
	guideFbBudget  = "budget_exceeded"
	guideFbIPQuota = "ip_quota"
	guideFbBusy    = "llm_busy"
	guideFbError   = "llm_error"
	guideFbTimeout = "llm_timeout"
	guideFbSchema  = "schema_error"
	guideFbNoNeed  = "not_needed"
	guideFbTurn    = "turn_timeout"
)

// guideSem — семафор вызовов модели подбора.
var guideSem = make(chan struct{}, guideLLMSlots)

// guideReply — ответ модели.
type guideReply struct {
	A      string         `json:"a"`
	Lead   string         `json:"lead"`
	Claims []string       `json:"claims"`
	Fam    []string       `json:"fam"`
	Off    []string       `json:"off"`
	Set    map[string]any `json:"set"`
}

// guideCall — расход одного вызова.
type guideCall struct {
	in, out int
	cost    float64
	ms      float64
}

// model — модель подбора: модель разбора намерения (Flash); nil — нет.
func (g *guideService) model() ChatModel {
	if g.modelOverride != nil {
		return g.modelOverride
	}
	if p, ok := g.s.intent.(*intentService); ok && p != nil {
		return p.primary
	}
	return nil
}

// modelBlocked — почему модель сейчас не вызывается ("" — можно).
func (g *guideService) modelBlocked(ctx context.Context) string {
	switch {
	case g.model() == nil:
		return guideFbNoModel
	case g.s.LLMMode() != llmModeOn || llmOffBy(ctx):
		return guideFbOff
	case g.s.guard().buyerExceeded(ctx):
		return guideFbBudget
	case g.s.guard().addrExceeded(ctx):
		return guideFbIPQuota
	}
	return ""
}

// callModel — один вызов: семафор, срок, учёт; ошибка схемы — без повтора (ход и так ограничен по времени).
func (g *guideService) callModel(ctx context.Context, user string) (*guideReply, guideCall, string) {
	var call guideCall
	m := g.model()
	select {
	case guideSem <- struct{}{}:
	case <-ctx.Done():
		return nil, call, guideFbBusy
	case <-time.After(1500 * time.Millisecond):
		return nil, call, guideFbBusy
	}
	defer func() { <-guideSem }()
	cctx, cancel := context.WithTimeout(ctx, guideCallTimeout)
	defer cancel()
	zero := 0.0
	req := llm.Request{
		Messages:  []llm.Message{{Role: "system", Content: prompts.Guide().Text}, {Role: "user", Content: user}},
		MaxTokens: guideMaxTokens, Temperature: &zero, ResponseFormat: llm.JSONObject,
	}
	started := time.Now()
	resp, err := m.Chat(cctx, req)
	lat := time.Since(started)
	rec := llmCall{Model: m.Model(), Latency: lat, Status: llmStatusOK}
	var reply *guideReply
	reason := ""
	switch {
	case err != nil && (errors.Is(err, context.DeadlineExceeded) || errors.Is(cctx.Err(), context.DeadlineExceeded)):
		rec.Status, reason = llmStatusTimeout, guideFbTimeout
	case err != nil:
		rec.Status, reason = llmStatusError, guideFbError
	default:
		rec.InputTokens, rec.OutputTokens, rec.CachedTokens = resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.CachedTokens()
		r, perr := decodeGuideReply(resp.Message.Content)
		if perr != nil {
			rec.Status, reason = llmStatusSchemaError, guideFbSchema
		} else {
			reply = r
		}
	}
	call.cost = g.s.recordLLMUsage(ctx, rec)
	call.in, call.out = rec.InputTokens, rec.OutputTokens
	call.ms = float64(lat.Microseconds()) / 1000
	return reply, call, reason
}

// decodeGuideReply — JSON-объект ответа; лишний текст вокруг объекта отрезается.
func decodeGuideReply(content string) (*guideReply, error) {
	c := strings.TrimSpace(content)
	if i := strings.Index(c, "{"); i > 0 {
		c = c[i:]
	}
	if j := strings.LastIndex(c, "}"); j >= 0 && j < len(c)-1 {
		c = c[:j+1]
	}
	var r guideReply
	if err := json.Unmarshal([]byte(c), &r); err != nil {
		return nil, err
	}
	switch r.A {
	case "comparison", "selection", "clarification", "no_data", "out_of_scope":
	default:
		return nil, errors.New("неизвестный тип ответа")
	}
	return &r, nil
}

// guideModelInput — данные хода для модели: только идентификаторы, короткие названия и значения фактов. Текст
// каталога — как данные в тегах.
type guideModelInput struct {
	Query      string
	Conditions []string
	Families   []map[string]string
	Facts      []map[string]string
	Offers     []map[string]string
	Changes    []string
}

func (in guideModelInput) render() string {
	q, _ := json.Marshal(in.Query)
	b := &strings.Builder{}
	b.WriteString("<query>" + string(q) + "</query>\n")
	write := func(tag string, v any) {
		raw, _ := json.Marshal(v)
		b.WriteString("<" + tag + ">" + string(raw) + "</" + tag + ">\n")
	}
	write("conditions", in.Conditions)
	write("families", in.Families)
	write("facts", in.Facts)
	write("offers", in.Offers)
	write("changes", in.Changes)
	return b.String()
}
