package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"kuznica/api/domain"
	"kuznica/api/externalApi/llm"
	"kuznica/api/prompts"

	"go.uber.org/zap"
)

// Тип запроса журнала: сначала правила (тот же нормализатор, атрибуты и сущности, что у поиска), неуверенные —
// пакетно модели (Flash) с кэшем в файле: повторная сборка на том же журнале не платит второй раз.

// Источник типа (DerivedQuery.TypeSource).
const (
	typeByRules   = "rules"
	typeByLLM     = "llm"
	typeByCache   = "cache"
	typeByDefault = "default"
)

const (
	classifyBatch     = 20
	classifyCallLimit = 45 * time.Second
	classifyMaxTokens = 1200
	classifyCacheFile = ".classify-cache.json"
	judgeCacheFile    = ".judge-cache.json"
	// classifyRequestPrefix — request_id вызовов классификации в llm_usage: вне стоимости запроса покупателя
	// (kind eval в /api/costs).
	classifyRequestPrefix = evalRequestPrefix + "sets-"
)

// classifyTypes — типы, которые может вернуть модель.
var classifyTypes = []string{domain.QueryExact, domain.QuerySKU, domain.QueryTypo, domain.QueryLayout, domain.QuerySlang,
	domain.QueryAliasGap, domain.QueryPurpose, domain.QueryLicense, domain.QueryAnalog, domain.QueryRenewal,
	domain.QueryAbsent, domain.QueryNonProduct, domain.QueryNoise}

// purposeStems — слова назначения: «программа для…», «чем открыть…».
var purposeStems = []string{"для", "чем", "программ", "софт", "приложени", "систем", "средств", "как "}

// ruleType — тип запроса правилами; sure=false — отдать модели. unknown — непонятные поиску слова (подсказка).
func ruleType(text string, lx *searchLexicon, rules *searchRules) (typ string, sure bool, unknown []string) {
	clean := foldText(text)
	letters := 0
	for _, r := range clean {
		if unicode.IsLetter(r) {
			letters++
		}
	}
	if letters < 2 {
		return domain.QueryNoise, true, nil
	}
	nr := normalizeFull(text, lx, rules, false)
	attrs := extractAttrs(nr.tokens)
	ent := findEntities(nr, attrs, lx, rules)
	unknown = ent.unknown
	kinds := map[string]bool{}
	for _, c := range nr.changes {
		kinds[c.Kind] = true
	}
	hasEntity := len(ent.families) > 0 || len(ent.vendors) > 0
	switch {
	case ent.nonProduct:
		return domain.QueryNonProduct, true, unknown
	case attrs.analog:
		return domain.QueryAnalog, len(unknown) == 0, unknown
	case attrs.renewal:
		return domain.QueryRenewal, true, unknown
	}
	for _, t := range strings.Fields(clean) {
		if looksLikeSKU(t) || lx.isSKU(t) {
			return domain.QuerySKU, true, unknown
		}
	}
	switch {
	case kinds[changeLayout]:
		return domain.QueryLayout, len(unknown) == 0, unknown
	case kinds[changeTypo]:
		return domain.QueryTypo, len(unknown) == 0, unknown
	case nr.correction != nil && !nr.correction.applied:
		return domain.QueryTypo, false, unknown
	}
	if len(unknown) > 0 {
		return "", false, unknown
	}
	licensed := attrs.quantity > 0 || attrs.duration > 0 || attrs.licType != "" || attrs.origin != "" || attrs.metricUnresolved
	switch {
	case licensed:
		return domain.QueryLicense, true, unknown
	case kinds[changeAlias] && hasEntity:
		return domain.QuerySlang, true, unknown
	case len(ent.named) > 0 || hasEntity && !hasPurposeWord(clean):
		return domain.QueryExact, true, unknown
	case hasPurposeWord(clean) || ent.category != "":
		return domain.QueryPurpose, true, unknown
	}
	return "", false, unknown
}

func hasPurposeWord(s string) bool {
	for _, w := range strings.Fields(s) {
		for _, p := range purposeStems {
			if strings.HasPrefix(w+" ", p) {
				return true
			}
		}
	}
	return false
}

// isSKU — слово совпадает с артикулом позиции каталога.
func (lx *searchLexicon) isSKU(t string) bool {
	if utf8.RuneCountInString(t) < 5 {
		return false
	}
	for _, d := range lx.docs {
		if d.SKU != "" && strings.EqualFold(d.SKU, t) {
			return true
		}
	}
	return false
}

// classifyItem — запрос для модели с подсказками журнала.
type classifyItem struct {
	ID                 string   `json:"id"`
	Query              string   `json:"query"`
	UnknownWords       []string `json:"unknown_words"`
	EmptyShare         float64  `json:"empty_share"`
	ClickShare         float64  `json:"click_share"`
	ReformulatedTo     string   `json:"reformulated_to,omitempty"`
	ReformulationFound bool     `json:"reformulation_found"`
}

// classifyStats — что потратила классификация.
type classifyStats struct {
	Rules, LLM, Cache, Default int
	Calls                      int
	CostRub                    float64
	Model                      string
	PromptVersion              string
}

// fileCache — кэш ответов модели в файле рядом с наборами: ключ — модель, версия промпта и вход.
type fileCache[V any] struct {
	path  string
	items map[string]V
	dirty bool
}

func loadFileCache[V any](dir, name string) *fileCache[V] {
	c := &fileCache[V]{items: map[string]V{}}
	if dir == "" {
		return c
	}
	c.path = filepath.Join(dir, name)
	if raw, err := os.ReadFile(c.path); err == nil {
		_ = json.Unmarshal(raw, &c.items)
	}
	return c
}

func (c *fileCache[V]) key(model, version, input string) string {
	sum := sha256.Sum256([]byte(model + "\x00" + version + "\x00" + foldQuery(input)))
	return hex.EncodeToString(sum[:8])
}

func (c *fileCache[V]) save() error {
	if c.path == "" || !c.dirty {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(c.items, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, raw, 0o644)
}

// classifyQueries — типы запросов: правила, затем кэш и модель для неуверенных, остальное — по умолчанию
// (exact, если есть кликнутые товары, иначе absent).
func (s *Services) classifyQueries(ctx context.Context, qs []*DerivedQuery, items map[string]classifyItem, ruleTypes map[string]string,
	model ChatModel, maxRub float64, cacheDir string) classifyStats {
	var st classifyStats
	prompt := prompts.EvalSetsClassify()
	st.PromptVersion = prompt.Version
	cache := loadFileCache[string](cacheDir, classifyCacheFile)
	var pending []*DerivedQuery
	for _, q := range qs {
		if t := ruleTypes[q.ID]; t != "" {
			q.Type, q.TypeSource = t, typeByRules
			st.Rules++
			continue
		}
		if model != nil {
			st.Model = llmModelName(model.Model())
			if t, ok := cache.items[cache.key(st.Model, prompt.Version, q.Text)]; ok {
				q.Type, q.TypeSource = t, typeByCache
				st.Cache++
				continue
			}
		}
		pending = append(pending, q)
	}
	for b := 0; model != nil && b*classifyBatch < len(pending); b++ {
		if ctx.Err() != nil || maxRub > 0 && st.CostRub >= maxRub {
			break
		}
		batch := pending[b*classifyBatch : min(len(pending), (b+1)*classifyBatch)]
		var in []classifyItem
		for _, q := range batch {
			in = append(in, items[q.ID])
		}
		rid := fmt.Sprintf("%s%03d", classifyRequestPrefix, b+1)
		got, cost, err := s.classifyBatch(WithRequestID(ctx, rid), model, prompt.Text, in)
		st.Calls++
		st.CostRub += cost
		if err != nil {
			zap.L().Warn("наборы: пачка классификации не разобрана", zap.Int("batch", b+1), zap.Error(err))
		}
		for _, q := range batch {
			if t, ok := got[q.ID]; ok {
				q.Type, q.TypeSource = t, typeByLLM
				cache.items[cache.key(st.Model, prompt.Version, q.Text)] = t
				cache.dirty = true
				st.LLM++
			}
		}
	}
	for _, q := range qs {
		if q.Type == "" {
			q.Type, q.TypeSource = domain.QueryExact, typeByDefault
			if q.ClickShare == 0 {
				q.Type = domain.QueryAbsent
			}
			st.Default++
		}
	}
	if err := cache.save(); err != nil {
		zap.L().Warn("наборы: кэш классификации не записан", zap.Error(err))
	}
	st.CostRub = round4(st.CostRub)
	return st
}

// classifyBatch — один вызов модели на пачку запросов; ответ — id → тип (неизвестные типы отброшены).
func (s *Services) classifyBatch(ctx context.Context, model ChatModel, system string, items []classifyItem) (map[string]string, float64, error) {
	raw, err := json.Marshal(map[string]any{"queries": items})
	if err != nil {
		return nil, 0, err
	}
	zero := 0.0
	cctx, cancel := context.WithTimeout(ctx, classifyCallLimit)
	defer cancel()
	t := time.Now()
	resp, err := model.Chat(cctx, llm.Request{Messages: []llm.Message{{Role: "system", Content: system},
		{Role: "user", Content: "<queries>" + string(raw) + "</queries>"}}, MaxTokens: classifyMaxTokens, Temperature: &zero,
		ResponseFormat: llm.JSONObject})
	call := llmCall{Model: model.Model(), Latency: time.Since(t), Status: llmStatusOK}
	if resp != nil {
		call.InputTokens, call.OutputTokens, call.CachedTokens = resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.CachedTokens()
	}
	var got map[string]string
	if err == nil {
		got, err = parseClassify(resp.Message.Content)
		if err != nil {
			call.Status = llmStatusSchemaError
		}
	} else {
		call.Status = llmStatusError
		if errors.Is(err, context.DeadlineExceeded) {
			call.Status = llmStatusTimeout
		}
	}
	return got, s.recordLLMUsage(ctx, call), err
}

// parseClassify — {"types": [{"id", "type"}]} → id → тип.
func parseClassify(content string) (map[string]string, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(strings.TrimPrefix(content, "```json"), "```")
	content = strings.TrimSpace(strings.TrimSuffix(content, "```"))
	var body struct {
		Types []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"types"`
	}
	if err := json.Unmarshal([]byte(content), &body); err != nil {
		return nil, fmt.Errorf("классификация: ответ не JSON: %w", err)
	}
	res := map[string]string{}
	for _, t := range body.Types {
		typ := strings.ToLower(strings.TrimSpace(t.Type))
		if slices.Contains(classifyTypes, typ) {
			res[t.ID] = typ
		}
	}
	return res, nil
}

// gradeCache — кэш оценок судьи (пара «текст запроса, товар»).
type gradeCache struct {
	*fileCache[int]
	grades map[string]int
}

func loadGradeCache(dir string) gradeCache {
	c := loadFileCache[int](dir, judgeCacheFile)
	return gradeCache{fileCache: c, grades: c.items}
}
