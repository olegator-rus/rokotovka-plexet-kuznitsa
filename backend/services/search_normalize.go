package services

import (
	"strings"
	"unicode"

	"kuznica/api/domain"
)

// Минимальная нормализация запроса (шаг 1 из ARCHITECTURE §2.1): нижний регистр, ё → е, единый вид
// дефисов и кавычек, схлопнутые пробелы. Раскладка, алиасы и опечатки — следующая волна.
//
// Та же свёртка символов стоит в индексе (char_filter fold и normalizer exact в search_index.go):
// подстрока B0 сравнивается с title.exact, и обе стороны должны быть приведены одинаково.

// foldRunes — замены символов; общие с маппингом индекса (foldMappings).
var foldRunes = map[rune]rune{
	'ё': 'е', 'Ё': 'Е',
	'‐': '-', '‑': '-', '‒': '-', '–': '-', '—': '-', '―': '-', '−': '-',
	'«': '"', '»': '"', '“': '"', '”': '"', '„': '"', '‟': '"', '″': '"',
	'\'': '"', '’': '"', '‘': '"', '`': '"',
}

// foldText — свёртка символов, нижний регистр и пробелы: то же, что normalizer exact в индексе.
func foldText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for _, r := range s {
		if f, ok := foldRunes[r]; ok {
			r = f
		}
		if unicode.IsSpace(r) {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// normalizeQuery — нормализованный запрос и изменения для трассы.
func normalizeQuery(q string) (string, []domain.Change) {
	n := foldText(q)
	changes := []domain.Change{}
	if n != q {
		changes = append(changes, domain.Change{Kind: "clean", From: q, To: n})
	}
	return n, changes
}
