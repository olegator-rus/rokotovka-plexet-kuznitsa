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
	}
	if l.FolderID != "" && !strings.Contains(l.Model, "://") {
		l.Model = "gpt://" + l.FolderID + "/" + l.Model
	}
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

// summary — строка сводки без значения ключа.
func (l LLMConfig) summary() string {
	if !l.Online() {
		return fmt.Sprintf("LLM: не настроена (LLM_PROVIDER=%s, ключ %s), режим %s", l.Provider, set(l.APIKey), l.Mode)
	}
	model := strings.TrimPrefix(l.Model, "gpt://"+l.FolderID+"/") // каталог в журнал не пишем
	return fmt.Sprintf("LLM: %s, модель %s, %s, ключ %s, режим %s", l.Provider, model, l.BaseURL, set(l.APIKey), l.Mode)
}
