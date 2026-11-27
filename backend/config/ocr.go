package config

import (
	"strings"
)

// OCRConfig — распознавание сканов ТЗ (контур ТЗ, TZ-CORE §4): Yandex Vision OCR. Ключ — только здесь и в клиенте
// externalApi/ocr; в лог не попадает.
type OCRConfig struct {
	// URL — OCR_URL, синхронный recognizeText; off — распознавания нет (скан — причина ocr).
	URL string
	// APIKey — OCR_API_KEY: API-ключ сервисного аккаунта с ролью ai.vision.user; пусто — ключ модели (LLM_API_KEY).
	APIKey string
	// FolderID — OCR_FOLDER_ID; пусто — LLM_FOLDER_ID.
	FolderID string
}

const defaultOCRURL = "https://ocr.api.cloud.yandex.net/ocr/v1/recognizeText"

// Online — распознавание настроено.
func (o OCRConfig) Online() bool { return o.URL != "" && o.APIKey != "" }

// LoadOCR — настройки распознавания; ключ и каталог по умолчанию — те же, что у модели.
func LoadOCR(l LLMConfig) OCRConfig {
	o := OCRConfig{
		URL:      strings.TrimSpace(env("OCR_URL", defaultOCRURL)),
		APIKey:   env("OCR_API_KEY", l.APIKey),
		FolderID: env("OCR_FOLDER_ID", l.FolderID),
	}
	if strings.EqualFold(o.URL, "off") {
		o.URL = ""
	}
	return o
}
