package services

import (
	"slices"
	"testing"
)

func TestSuggestDictRelated(t *testing.T) {
	e := func(text, source, cat string, weight float64) dictEntry {
		return dictEntry{text: text, norm: foldText(text), source: source, kind: suggestQuery,
			categories: []string{cat}, weight: weight, hits: 3}
	}
	d := &suggestDict{entries: []dictEntry{
		e("контроль устройств", "catalog_template", "av", 0.5),
		e("антивирус", "successful_query", "av", 0.5),
		e("касперский", "successful_query", "av", 0.4),
		e("касперский на 10 пк", "successful_query", "av", 0.3),
		e("защита рабочих станций", "catalog_template", "av", 0.1),
		e("касперского на 10 пк", "successful_query", "av", 0.2),
		e("фш", "dictionary", "av", 0.9),
		e("DEPO", "catalog_template", "av", 0.9),
		e("офис", "successful_query", "office", 0.5),
	}}
	got := d.related("av", map[string]bool{"антивирус": true}, []string{}, 5)
	// Сам запрос и его продолжения не повторяются, чужая категория не берётся, шаблоны — после успешных.
	// Словарные формы и однословные шаблоны не показываются, разные окончания — не два запроса.
	if want := []string{"касперский", "контроль устройств", "защита рабочих станций"}; !slices.Equal(got, want) {
		t.Fatalf("related = %q, ждали %q", got, want)
	}
	if got := (*suggestDict)(nil).related("av", nil, []string{"x"}, 5); !slices.Equal(got, []string{"x"}) {
		t.Fatalf("без словаря — прежний список: %q", got)
	}
}
