package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"kuznica/api/domain"
	"kuznica/api/externalApi/llm"
	"kuznica/api/prompts"

	"go.uber.org/zap"
)

// Разбор намерения на маршруте D (ARCHITECTURE §2.3): кэш → модель → сверка со справочниками → слияние с
// тем, что извлекли правила. Модель возвращает структуру, а не запрос к движку; каждое значение
// сверяется со справочником, несуществующее уходит в dropped. Любой сбой — откат на правила, поиск не
// падает. Тексты запросов в журнал не пишутся.

// ChatModel — модель за OpenAI-совместимым API (externalApi/llm.Client или фейк в тестах).
type ChatModel interface {
	Chat(ctx context.Context, req llm.Request) (*llm.Response, error)
	Model() string
}

// Причины отката на правила (Intent.FallbackReason).
const (
	fallbackLLMOff      = "llm_off"
	fallbackNoModel     = "llm_not_configured"
	fallbackLLMError    = "llm_error"
	fallbackSchemaError = "schema_error"
	fallbackNoCatalog   = "catalog_not_loaded"
)

const (
	// intentCallTimeout — срок одного вызова модели. Разбор Flash ~2 с; дольше — пользователь ждёт зря.
	intentCallTimeout = 8 * time.Second
	// intentMaxTokens — потолок ответа: JSON разбора ~100 токенов; клиент всё равно поднимет до llm.MinMaxTokens.
	intentMaxTokens = llm.MinMaxTokens
	// intentCacheTimeout — чтение и запись кэша разборов.
	intentCacheTimeout = 500 * time.Millisecond
)

// intentService — реализация domain.IntentParser.
type intentService struct {
	s        *Services
	primary  ChatModel // nil — модели нет
	fallback ChatModel // nil — резерва нет
	prompt   prompts.Prompt

	// cache — кэш разборов; nil — без базы кэша нет.
	cache intentCacheStore

	mu   sync.Mutex
	dict *intentDict
}

// intentCacheStore — хранилище кэша разборов (repositories.Repos; в тестах — карта в памяти).
type intentCacheStore interface {
	GetIntentCache(ctx context.Context, key string, now time.Time) (*domain.IntentCache, error)
	PutIntentCache(ctx context.Context, row *domain.IntentCache) error
}

// newIntentService — парсер; primary == nil или режим off — работает на кэше и правилах.
func newIntentService(s *Services, primary, fallback ChatModel) *intentService {
	p := &intentService{
		s: s, primary: primary, fallback: fallback,
		prompt: prompts.Intent(),
	}
	if s.repos.Enabled() {
		p.cache = s.repos
	}
	return p
}

// PromptVersion — версия промпта разбора (входит в ключ кэша).
func (p *intentService) PromptVersion() string { return p.prompt.Version }

// Parse — domain.IntentParser. Ошибка не возвращается никогда: сбой модели или схемы — rulesGuess с
// Source=rules и причиной в FallbackReason; деньги, потраченные на неудачные попытки, — в LLM.
func (p *intentService) Parse(ctx context.Context, normalized string, rulesGuess domain.Intent, clientSegment string) (domain.Intent, error) {
	rules := rulesGuess
	rules.Source = domain.ParseRules
	rules.LLM = nil
	dict := p.dictionary(ctx)
	if dict == nil {
		rules.FallbackReason = fallbackNoCatalog
		return rules, nil
	}
	modelName := ""
	if p.primary != nil {
		modelName = llmModelName(p.primary.Model())
	}
	key := intentCacheKey(normalized, clientSegment, rulesGuess, dict.version, p.s.rulesVersion(), p.prompt.Version, modelName)
	if hit, ok := p.cacheGet(ctx, key); ok {
		return hit, nil
	}
	// Режим и бюджет проверяются на каждом разборе: LLM_MODE переключается без перезапуска.
	switch {
	case p.primary == nil:
		rules.FallbackReason = fallbackNoModel
		return rules, nil
	case p.s.LLMMode() != llmModeOn:
		rules.FallbackReason = fallbackLLMOff
		return rules, nil
	case p.s.guard().exceeded(ctx):
		rules.FallbackReason = fallbackBudget
		return rules, nil
	}

	parsed, info, reason := p.callModel(ctx, dict, normalized, clientSegment)
	if parsed == nil {
		rules.LLM = info
		rules.FallbackReason = reason
		return rules, nil
	}
	res := explicitOrigin(mergeIntent(rulesGuess, dict.validate(*parsed, normalized)), normalized)
	res.Source = domain.ParseLLM
	p.cachePut(key, normalized, clientSegment, dict.version, modelName, res)
	res.LLM = info
	return res, nil
}

// callModel — вызов модели: ошибка схемы — один повтор той же моделью; сбой вызова — резервная модель
// (если есть). nil — откат на правила с причиной. info — сумма расхода по всем попыткам.
func (p *intentService) callModel(ctx context.Context, dict *intentDict, normalized, segment string) (*rawIntent, *domain.LLMCallInfo, string) {
	info := &domain.LLMCallInfo{}
	reason := fallbackLLMError
	for _, model := range []ChatModel{p.primary, p.fallback} {
		if model == nil {
			continue
		}
		msgs := []llm.Message{
			{Role: "system", Content: dict.systemPrompt},
			{Role: "user", Content: intentUserMessage(normalized, segment)},
		}
		for attempt := 0; attempt < 2; attempt++ {
			content, err := p.chatOnce(ctx, model, msgs, info)
			if err != nil {
				zap.L().Warn("intent: вызов модели не удался", zap.String("model", llmModelName(model.Model())),
					zap.String("request_id", intentStrOrEmpty(llmCtxString(ctx, llmCtxRequestID))), zap.Error(err))
				reason = fallbackLLMError
				break // сбой вызова: повтор той же моделью не поможет — к резервной
			}
			raw, perr := decodeIntent(content)
			if perr == nil {
				return raw, info, ""
			}
			reason = fallbackSchemaError
			zap.L().Warn("intent: ответ модели не по схеме", zap.String("model", llmModelName(model.Model())),
				zap.Int("attempt", attempt+1), zap.Error(perr))
			msgs = append(msgs,
				llm.Message{Role: "assistant", Content: intentTruncate(content, 600)},
				llm.Message{Role: "user", Content: "Ответ не разобран (" + perr.Error() + "). Верни только один JSON-объект по схеме из системного сообщения."})
		}
		if reason == fallbackSchemaError {
			return nil, info, reason // ошибка схемы после повтора — сразу правила, резерв не зовём
		}
	}
	return nil, info, reason
}

// chatOnce — один вызов с учётом: строка llm_usage, сумма в info.
func (p *intentService) chatOnce(ctx context.Context, model ChatModel, msgs []llm.Message, info *domain.LLMCallInfo) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, intentCallTimeout)
	defer cancel()
	zero := 0.0
	req := llm.Request{Messages: msgs, MaxTokens: intentMaxTokens, Temperature: &zero, ResponseFormat: llm.JSONObject}
	if strings.HasPrefix(llmModelName(model.Model()), "qwen") {
		// Qwen3.6 в AI Studio без этого тратит весь max_tokens на рассуждения.
		req.ReasoningEffort = "none"
	}
	started := time.Now()
	resp, err := model.Chat(cctx, req)
	lat := time.Since(started)
	call := llmCall{Model: model.Model(), Latency: lat, Status: llmStatusOK}
	content := ""
	switch {
	case err != nil && (errors.Is(err, context.DeadlineExceeded) || errors.Is(cctx.Err(), context.DeadlineExceeded)):
		call.Status = llmStatusTimeout
	case err != nil:
		call.Status = llmStatusError
	default:
		call.InputTokens, call.OutputTokens, call.CachedTokens = resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.CachedTokens()
		content = resp.Message.Content
		if _, derr := decodeIntent(content); derr != nil {
			call.Status = llmStatusSchemaError
		}
	}
	cost := p.s.recordLLMUsage(ctx, call)
	info.Model = llmModelName(model.Model())
	info.InputTokens += call.InputTokens
	info.OutputTokens += call.OutputTokens
	info.CachedTokens += call.CachedTokens
	info.Ms = math.Round((info.Ms+float64(lat.Microseconds())/1000)*100) / 100
	info.CostRub = math.Round((info.CostRub+cost)*1e4) / 1e4
	info.Calls++
	return content, err
}

// intentUserMessage — запрос как данные: JSON-строка внутри тега, плюс обезличенный сегмент покупателя.
func intentUserMessage(normalized, segment string) string {
	q, _ := json.Marshal(normalized)
	msg := "<query>" + string(q) + "</query>"
	if segment != "" {
		msg += "\nСегмент покупателя: " + segment
	}
	return msg
}

// intentCacheKey — sha256 от нормализованного запроса, влияющего контекста (сегмент, разбор правил) и
// версий каталога, правил, промпта и модели.
func intentCacheKey(normalized, segment string, guess domain.Intent, catalogVersion string, rulesVersion int, promptVersion, model string) string {
	k, _ := json.Marshal(struct {
		Q, Seg, Cat, Prompt, Model string
		Rules                      int
		Guess                      any
	}{
		Q: normalized, Seg: segment, Cat: catalogVersion, Prompt: promptVersion, Model: model, Rules: rulesVersion,
		Guess: []any{guess.Kind, guess.ReferenceFamily, guess.CategoryID, guess.VendorID, guess.Origin,
			intentSortedConstraints(guess.Constraints), intentSortedConstraints(guess.Preferences)},
	})
	sum := sha256.Sum256(k)
	return hex.EncodeToString(sum[:])
}

func intentSortedConstraints(cs []domain.AttrConstraint) []domain.AttrConstraint {
	out := append([]domain.AttrConstraint{}, cs...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Field != out[j].Field {
			return out[i].Field < out[j].Field
		}
		return out[i].Value < out[j].Value
	})
	return out
}

// cacheGet — разбор из кэша: Source=cache, стоимость 0.
func (p *intentService) cacheGet(ctx context.Context, key string) (domain.Intent, bool) {
	if p.cache == nil {
		return domain.Intent{}, false
	}
	cctx, cancel := context.WithTimeout(ctx, intentCacheTimeout)
	defer cancel()
	row, err := p.cache.GetIntentCache(cctx, key, p.s.now())
	if err != nil {
		zap.L().Warn("intent: кэш разборов не прочитан", zap.Error(err))
		return domain.Intent{}, false
	}
	if row == nil {
		return domain.Intent{}, false
	}
	var in domain.Intent
	if err := json.Unmarshal(row.Intent, &in); err != nil {
		zap.L().Warn("intent: строка кэша не разобрана", zap.Error(err))
		return domain.Intent{}, false
	}
	in.Source = domain.ParseCache
	in.LLM = nil
	in.FallbackReason = ""
	return in, true
}

// cachePut — успешный разбор модели в кэш (в фоне).
func (p *intentService) cachePut(key, normalized, segment, catalogVersion, model string, in domain.Intent) {
	if p.cache == nil {
		return
	}
	in.LLM = nil
	raw, err := domain.ToJSONB(in)
	if err != nil {
		return
	}
	row := &domain.IntentCache{
		Key: key, Normalized: normalized, Segment: segment, CatalogVersion: catalogVersion,
		RulesVersion: p.s.rulesVersion(), PromptVersion: p.prompt.Version, Model: model, Intent: raw,
		CreatedAt: p.s.now(),
	}
	p.s.writes.Add(1)
	go func() {
		defer p.s.writes.Done()
		ctx, cancel := context.WithTimeout(context.Background(), llmUsageSaveTimeout)
		defer cancel()
		if err := p.cache.PutIntentCache(ctx, row); err != nil {
			zap.L().Warn("intent: разбор не положен в кэш", zap.Error(err))
		}
	}()
}

// dictionary — справочники для промпта и сверки; перестраиваются при смене версии каталога.
func (p *intentService) dictionary(ctx context.Context) *intentDict {
	want := ""
	if st := p.s.catalog.Load(); st != nil {
		want = st.version
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dict != nil && (want == "" || p.dict.version == want) {
		return p.dict
	}
	snap := p.s.catalogSnapshot(ctx)
	if snap == nil {
		return p.dict
	}
	p.dict = newIntentDict(snap, p.prompt.Text)
	return p.dict
}

// catalogSnapshot — каталог из базы, иначе из источника; nil — нет ни того, ни другого.
func (s *Services) catalogSnapshot(ctx context.Context) *domain.CatalogSnapshot {
	if s.repos.Enabled() {
		snap, err := s.repos.LoadCatalog(ctx)
		if err == nil && snap != nil {
			return snap
		}
		if err != nil {
			zap.L().Warn("intent: каталог из базы не прочитан", zap.Error(err))
		}
	}
	if s.source != nil {
		snap, err := s.source.Load(ctx)
		if err == nil {
			return snap
		}
		zap.L().Warn("intent: каталог из источника не прочитан", zap.Error(err))
	}
	return nil
}

// rulesVersion — текущая версия правил (1, пока справочники не загружены).
func (s *Services) rulesVersion() int {
	if st := s.catalog.Load(); st != nil && st.rulesVersion > 0 {
		return st.rulesVersion
	}
	return 1
}

// IntentTrace — шаг intent, trace.llm и стоимость разбора для трассы поиска. ms — время шага целиком
// (кэш + модель + сверка).
func IntentTrace(in domain.Intent, ms float64) (domain.IntentStep, *domain.TraceLLM, float64) {
	step := domain.IntentStep{
		StepBase: domain.StepBase{Name: domain.StepIntent, Ms: ms, Reason: in.FallbackReason},
		Source:   in.Source,
		Fields: map[string]any{
			"kind": in.Kind, "reference_family": in.ReferenceFamily, "category_id": in.CategoryID,
			"vendor_id": in.VendorID, "origin": in.Origin,
			"constraints": intentNonNilConstraints(in.Constraints), "preferences": intentNonNilConstraints(in.Preferences),
			"unresolved": intentNonNilStrings(in.Unresolved),
		},
		Dropped: append([]domain.DroppedField{}, in.Dropped...),
	}
	if in.LLM == nil {
		return step, nil, 0
	}
	l := in.LLM
	return step, &domain.TraceLLM{Model: l.Model, InputTokens: l.InputTokens, OutputTokens: l.OutputTokens,
		CachedTokens: l.CachedTokens, Ms: l.Ms, CostRub: l.CostRub, Calls: l.Calls}, l.CostRub
}

func intentNonNilConstraints(cs []domain.AttrConstraint) []domain.AttrConstraint {
	if cs == nil {
		return []domain.AttrConstraint{}
	}
	return cs
}

func intentNonNilStrings(ss []string) []string {
	if ss == nil {
		return []string{}
	}
	return ss
}

func intentStrOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func intentTruncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// intentSchemaErr — ответ модели не по схеме.
func intentSchemaErr(format string, args ...any) error {
	return fmt.Errorf("схема: "+format, args...)
}
