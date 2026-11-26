package services

import (
	"slices"
	"strings"
	"testing"

	"kuznica/api/domain"
)

// Правила по реальным запросам (data/real-queries): форумы, прайсы, сленг покупателей.

func TestSplitWordNumber(t *testing.T) {
	for in, want := range map[string][]string{
		"челов2":   {"челов", "2"},
		"кес50":    {"кес", "50"},
		"офис2021": {"офис", "2021"},
		"1с8.3":    {"1с", "8.3"},
		"р7":       {"р7"},       // одна буква — название (Р7-Офис)
		"win11pro": {"win11pro"}, // латиница — модели и артикулы
		"per740":   {"per740"},
		"х8617":    {"х8617"},
		"офис":     {"офис"},
		"2021":     {"2021"},
	} {
		if got := splitWordNumber(in); !slices.Equal(got, want) {
			t.Errorf("%q → %q, ожидалось %q", in, got, want)
		}
	}
	lx := &searchLexicon{known: map[string]bool{"кес": true}}
	got := splitKnownWordNumber([]string{"кис", "10", "челов2", "года", "кес50", "битрикс24"}, lx, nil)
	if !slices.Equal(got, []string{"кис", "10", "челов", "2", "года", "кес", "50", "битрикс24"}) {
		t.Fatalf("splitKnownWordNumber: %q", got)
	}
}

func TestDigitsWithO(t *testing.T) {
	for in, want := range map[string]string{
		"2о21": "2021", "2o19": "2019", "2021": "", "о": "", "10о": "100", "office": "", "2о": "",
	} {
		if got := digitsWithO(in); got != want {
			t.Errorf("%q → %q, ожидалось %q", in, got, want)
		}
	}
}

func TestPeopleShorthandIsUserMetric(t *testing.T) {
	for _, q := range []string{"кис 10 челов 2 года", "касперский 10 чел", "офис на 10 людей"} {
		a := extractAttrs(strings.Fields(q))
		if a.quantity != 10 || a.metric != domain.MetricUser {
			t.Errorf("%q: количество %d, метрика %q", q, a.quantity, a.metric)
		}
	}
	a := extractAttrs(splitKnownWordNumber(strings.Fields("кис 10 челов2 года"), &searchLexicon{known: map[string]bool{}}, nil))
	if a.quantity != 10 || a.metric != domain.MetricUser || a.duration != 24 {
		t.Fatalf("слитно: %d %q %d", a.quantity, a.metric, a.duration)
	}
}

func TestSupportRenewalIsNotLicenseRenewal(t *testing.T) {
	for _, q := range []string{"продление поддержки hpe", "продлить гарантию на сервер dell", "продление сервисного обслуживания"} {
		if a := extractAttrs(strings.Fields(q)); a.renewal {
			t.Errorf("%q: продление сервиса стало продлением лицензии", q)
		}
	}
	if a := extractAttrs(strings.Fields("продление касперского")); !a.renewal {
		t.Fatal("продление лицензии потеряно")
	}
}

func TestQuestionWordsAreStopWords(t *testing.T) {
	for _, w := range []string{"какую", "выбрать", "стоит", "ли", "лицензирование", "rus", "x64", "поставка", "неисключительных"} {
		if !lexStopWords[w] {
			t.Errorf("%q не стоп-слово", w)
		}
	}
	for _, w := range []string{"компам", "челов"} {
		if !lexServiceWords[w] {
			t.Errorf("%q не служебное слово: опечатку в нём исправят в название товара («компам» → «компас»)", w)
		}
	}
}
