package services

import (
	"regexp"
	"strings"
)

// ФИО в тексте ТЗ (контур ТЗ, TZ-CORE §9): контактные лица заказчика. llm.MaskPII находит телефоны, почту, ИНН и
// т. п., но не имена — их маскирует этот файл до отправки текста в модель. Признак имени — отчество
// (-ович, -евич, -ич, -овна, -евна, -ична, -инична) или инициалы рядом с фамилией.

// tzPIIName — метка вместо ФИО.
const tzPIIName = "[фио]"

const (
	cyrUpper  = `[А-ЯЁ]`
	cyrWord   = `[А-ЯЁ][а-яё]+(?:-[А-ЯЁ][а-яё]+)?`
	patronym  = `[А-ЯЁ][а-яё]+(?:ович|евич|ьич|ич|овна|евна|ична|инична)`
	notLetter = `(^|[^\p{L}])`
	endLetter = `($|[^\p{L}])`
)

var tzNameRules = []*regexp.Regexp{
	// Фамилия Имя Отчество / Имя Отчество Фамилия.
	regexp.MustCompile(notLetter + `(` + cyrWord + `\s+` + cyrWord + `\s+` + patronym + `)` + endLetter),
	regexp.MustCompile(notLetter + `(` + cyrWord + `\s+` + patronym + `\s+` + cyrWord + `)` + endLetter),
	// Имя Отчество.
	regexp.MustCompile(notLetter + `(` + cyrWord + `\s+` + patronym + `)` + endLetter),
	// Фамилия И. О. / И. О. Фамилия.
	regexp.MustCompile(notLetter + `(` + cyrWord + `\s+` + cyrUpper + `\.\s?` + cyrUpper + `\.)`),
	regexp.MustCompile(notLetter + `(` + cyrUpper + `\.\s?` + cyrUpper + `\.\s?` + cyrWord + `)` + endLetter),
}

// tzMaskNames — текст с ФИО, заменёнными меткой, и число замен.
func tzMaskNames(s string) (string, int) {
	n := 0
	for _, re := range tzNameRules {
		for {
			m := re.FindStringSubmatchIndex(s)
			if m == nil {
				break
			}
			// Группа 2 — само имя; группа 1 — граница слева.
			s = s[:m[4]] + tzPIIName + s[m[5]:]
			n++
		}
	}
	return s, n
}

// tzMaskText — текст документа для модели: ФИО и остальные ПДн (llm.MaskPII) — метками.
func tzMaskText(s string) string {
	s, _ = tzMaskNames(s)
	s, _ = maskPII(s)
	return s
}

// tzHasPII — в строке есть метка ПДн или сами ПДн: такую строку в позицию не кладём.
func tzHasPII(s string) bool {
	if strings.Contains(s, "[") && (strings.Contains(s, tzPIIName) || strings.Contains(s, "[телефон]") ||
		strings.Contains(s, "[email]") || strings.Contains(s, "[инн]") || strings.Contains(s, "[снилс]") ||
		strings.Contains(s, "[паспорт]") || strings.Contains(s, "[карта]") || strings.Contains(s, "[огрн]") ||
		strings.Contains(s, "[кпп]")) {
		return true
	}
	if _, n := maskPII(s); n > 0 {
		return true
	}
	_, n := tzMaskNames(s)
	return n > 0
}
