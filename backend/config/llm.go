package config

import (
	"fmt"
	"strings"
)

// Провайдеры модели (LLM_PROVIDER).
const (
	// LLMOpenAICompatible — OpenAI-совместимый /chat/completions (Yandex AI Studio).
	LLMOpenAICompatible = "openai-compatible"
	// LLMNone — модели нет: сложные запросы разбираются правилами.
	LLMNone = "none"
)

// Режимы LLM_MODE: on — сложные запросы (маршрут D) идут в модель; off — только правила.
const (
	LLMModeOn  = "on"
	LLMModeOff = "off"
)

// Значения по умолчанию: Yandex AI Studio, Alice AI LLM Flash («Стек»).
const (
	defaultLLMBaseURL = "https://ai.api.cloud.yandex.net/v1"
	defaultLLMModel   = "aliceai-llm-flash"
	defaultJudgeModel = "qwen3.6-35b-a3b"
)

// LLMConfig — настройки модели. Ключ — только здесь и в клиенте externalApi/llm; в лог не попадает.
type LLMConfig struct {
	// Provider — openai-compatible | none. Пусто: есть ключ — openai-compatible, нет — none.
	Provider string
	BaseURL  string
	// Model — имя модели для API. У Yandex — полный URI gpt://<каталог>/<модель>: короткое имя из
	// LLM_MODEL дополняется каталогом из LLM_FOLDER_ID.
	Model string
	// FolderID — каталог Yandex Cloud (LLM_FOLDER_ID): часть URI модели и заголовок OpenAI-Project.
	FolderID string
	// APIKey — LLM_API_KEY (API-ключ сервисного аккаунта с ролью ai.languageModels.user).
	APIKey string
	// Mode — LLM_MODE: on | off. Не задан — on, если модель настроена, иначе off.
	Mode string
	// IntentModel — модель разбора намерения (LLM_INTENT_MODEL, по умолчанию aliceai-llm-flash), полный URI
	// как у Model. LLM_MODEL остаётся моделью по умолчанию для остального (агент).
	IntentModel string
	// FallbackModel — резервная модель разбора (LLM_FALLBACK_MODEL, например qwen3.6-35b-a3b); пусто — нет.
	FallbackModel string
	// JudgeModel — модель LLM-судьи оценки (LLM_JUDGE_MODEL, по умолчанию qwen3.6-35b-a3b): другого семейства,
	// чем разбор и агент; пусто — судьи нет.
	JudgeModel string
	// ConsultantReplyModel — модель второго вызова консультанта (CONSULTANT_REPLY_MODEL, контур консультанта); пусто — та же,
	// что у подбора (модель разбора).
	ConsultantReplyModel string
}

// Online — модель настроена: провайдер openai-compatible и ключ задан.
func (l LLMConfig) Online() bool { return l.Provider == LLMOpenAICompatible && l.APIKey != "" }

// Enabled — модель настроена и включена режимом LLM_MODE.
func (l LLMConfig) Enabled() bool { return l.Online() && l.Mode == LLMModeOn }

// LoadLLM — настройки модели из окружения.
func LoadLLM() (LLMConfig, error) {
	l := LLMConfig{
		Provider: strings.ToLower(env("LLM_PROVIDER", "")),
		BaseURL:  strings.TrimRight(env("LLM_BASE_URL", defaultLLMBaseURL), "/"),
		Model:    env("LLM_MODEL", defaultLLMModel),
		FolderID: env("LLM_FOLDER_ID", ""),
		APIKey:   env("LLM_API_KEY", ""),
		Mode:     strings.ToLower(env("LLM_MODE", "")),

		IntentModel:   env("LLM_INTENT_MODEL", defaultLLMModel),
		FallbackModel: env("LLM_FALLBACK_MODEL", ""),
		JudgeModel:    env("LLM_JUDGE_MODEL", defaultJudgeModel),

		ConsultantReplyModel: env("CONSULTANT_REPLY_MODEL", ""),
	}
	l.Model = l.modelURI(l.Model)
	l.IntentModel = l.modelURI(l.IntentModel)
	l.FallbackModel = l.modelURI(l.FallbackModel)
	l.JudgeModel = l.modelURI(l.JudgeModel)
	l.ConsultantReplyModel = l.modelURI(l.ConsultantReplyModel)
	switch l.Provider {
	case "":
		l.Provider = LLMNone
		if l.APIKey != "" {
			l.Provider = LLMOpenAICompatible
		}
	case LLMOpenAICompatible, LLMNone:
	default:
		return LLMConfig{}, fmt.Errorf("LLM_PROVIDER=%q: ожидается openai-compatible или none", l.Provider)
	}
	if !httpURL(l.BaseURL) {
		return LLMConfig{}, fmt.Errorf("LLM_BASE_URL=%q: ожидается http(s)-адрес", l.BaseURL)
	}
	// Yandex без каталога ответит 4xx на каждый вызов — лучше упасть при старте с понятной причиной.
	if l.Online() && strings.Contains(l.BaseURL, ".cloud.yandex.net") && !strings.HasPrefix(l.Model, "gpt://") {
		return LLMConfig{}, fmt.Errorf("LLM_FOLDER_ID пуст: для модели %q в Yandex AI Studio нужен каталог Yandex Cloud", l.Model)
	}
	switch l.Mode {
	case "":
		l.Mode = LLMModeOff
		if l.Online() {
			l.Mode = LLMModeOn
		}
	case LLMModeOff:
	case LLMModeOn:
		if !l.Online() {
			return LLMConfig{}, fmt.Errorf("LLM_MODE=on, но модель не настроена (LLM_PROVIDER=%s, ключ %s)", l.Provider, set(l.APIKey))
		}
	default:
		return LLMConfig{}, fmt.Errorf("LLM_MODE=%q: ожидается on или off", l.Mode)
	}
	return l, nil
}

// modelURI — короткое имя модели дополняется каталогом до gpt://<каталог>/<модель>; пусто — пусто.
func (l LLMConfig) modelURI(m string) string {
	if m != "" && l.FolderID != "" && !strings.Contains(m, "://") {
		return "gpt://" + l.FolderID + "/" + m
	}
	return m
}

// ShortModel — имя модели без каталога: для журнала, трассы и llm_usage (каталог наружу не уходит).
func (l LLMConfig) ShortModel(m string) string {
	return strings.TrimPrefix(m, "gpt://"+l.FolderID+"/")
}

// summary — строка сводки без значения ключа.
func (l LLMConfig) summary() string {
	if !l.Online() {
		return fmt.Sprintf("LLM: не настроена (LLM_PROVIDER=%s, ключ %s), режим %s", l.Provider, set(l.APIKey), l.Mode)
	}
	fallback := l.ShortModel(l.FallbackModel) // каталог в журнал не пишем
	if fallback == "" {
		fallback = "нет"
	}
	return fmt.Sprintf("LLM: %s, модель %s, разбор %s, резерв %s, %s, ключ %s, режим %s", l.Provider,
		l.ShortModel(l.Model), l.ShortModel(l.IntentModel), fallback, l.BaseURL, set(l.APIKey), l.Mode)
}
