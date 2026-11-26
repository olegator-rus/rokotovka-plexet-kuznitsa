// Package prompts — промпты модели, которые использует продукт. Каждый файл начинается со строки
// `<!-- version: … -->`: версия входит в ключ кэша разборов и в журнал, поэтому правка текста промпта
// без смены версии всё равно меняет ключ (к версии добавляется хэш текста).
package prompts

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"regexp"
	"strings"
)

//go:embed intent.md
var intentText string

// Prompt — текст и версия промпта.
type Prompt struct {
	// Text — текст без строки версии.
	Text string
	// Version — объявленная версия и хэш текста: intent-v1+3f2a9c1d.
	Version string
}

var versionLine = regexp.MustCompile(`^<!--\s*version:\s*([^\s]+)\s*-->\s*\n`)

// parse — версия из первой строки и хэш текста.
func parse(raw string) Prompt {
	v := "unversioned"
	if m := versionLine.FindStringSubmatch(raw); m != nil {
		v = m[1]
		raw = raw[len(m[0]):]
	}
	sum := sha256.Sum256([]byte(raw))
	return Prompt{Text: strings.TrimSpace(raw), Version: v + "+" + hex.EncodeToString(sum[:4])}
}

// Intent — промпт разбора намерения (маршрут D). Плейсхолдеры {{CATEGORIES}} и {{VENDORS}} заполняет
// services из справочников каталога.
func Intent() Prompt { return parse(intentText) }
