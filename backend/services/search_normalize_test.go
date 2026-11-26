package services

import "testing"

func TestNormalizeQuery(t *testing.T) {
	for in, want := range map[string]string{
		"  Касперский  «Ёлка» — 10 ПК ": `касперский "елка" - 10 пк`,
		"Office\t2021\n":     "office 2021",
		"1С:Предприятие 8.3": "1с:предприятие 8.3",
		"‘Word’ – “Excel”":   `"word" - "excel"`,
	} {
		got, changes := normalizeQuery(in)
		if got != want {
			t.Fatalf("%q → %q, ожидалось %q", in, got, want)
		}
		if len(changes) != 1 || changes[0].Kind != "clean" || changes[0].From != in || changes[0].To != want {
			t.Fatalf("%q: изменения %+v", in, changes)
		}
	}
	got, changes := normalizeQuery("windows 11")
	if got != "windows 11" || changes == nil || len(changes) != 0 {
		t.Fatalf("без изменений — пустой (не nil) список: %q %+v", got, changes)
	}
}
