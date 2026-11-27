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

func TestMergeDeclensions(t *testing.T) {
	byText := map[string]*logQuery{
		"продление касперский":  {text: "продление касперский", searches: 4, succ: 4},
		"продление касперского": {text: "продление касперского", searches: 3, succ: 3},
		"касперский на 10 пк":   {text: "касперский на 10 пк", searches: 2, succ: 1},
		"касперский":            {text: "касперский", searches: 5, succ: 2},
		"винда":                 {text: "винда", searches: 2, succ: 2},
		"винду":                 {text: "винду", searches: 2, succ: 1},
	}
	order := []string{"продление касперского", "касперский", "продление касперский", "касперский на 10 пк", "винда", "винду"}
	got := mergeDeclensions(order, byText)
	want := []string{"продление касперский", "касперский", "касперский на 10 пк", "винда", "винду"}
	if !slices.Equal(got, want) {
		t.Fatalf("порядок: %v, want %v", got, want)
	}
	if q := byText["продление касперский"]; q.searches != 7 || q.succ != 7 {
		t.Fatalf("счётчики не сложены: %+v", q)
	}
	if _, ok := byText["продление касперского"]; ok {
		t.Fatal("склонение осталось отдельной строкой")
	}
	if q := byText["касперский"]; q.searches != 5 {
		t.Fatalf("продолжение фразы склеено с ней: %+v", q)
	}
}
