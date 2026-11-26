package services

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"kuznica/api/domain"
)

// Враждебные запросы: эмодзи, длинные цепочки слов и иероглифов не должны разворачиваться в запрос движка
// больше потолка OpenSearch (1024 предложения) — иначе 502 вместо ответа.
func TestEngineText(t *testing.T) {
	cases := map[string]string{
		"касперский на 10 пк":         "касперский на 10 пк",
		"windows 🪟 11":                "windows 11",
		"антивирус 🛡️":                "антивирус",
		"\u200bофис\ufeff":            "офис",
		"👨\u200d👩\u200d👧\u200d👦":      "",
		"1️⃣0️⃣ лицензий":             "1 0 лицензий",
		"№123":                        "123",
		"<script>alert(1)</script>":   "<script>alert(1)</script>",
		"kaspersky endpoint security": "kaspersky endpoint security",
	}
	for in, want := range cases {
		if got := engineText(in); got != want {
			t.Errorf("engineText(%q) = %q, want %q", in, got, want)
		}
	}
	if got := engineText(strings.Repeat("😀", 299)); got != "" {
		t.Errorf("эмодзи: %q", got)
	}
	if got := strings.Fields(engineText(strings.Repeat("1 ", 150))); len(got) != engineMaxUnits {
		t.Errorf("цифры: %d слов", len(got))
	}
	if got := engineText(strings.Repeat("微软办公", 75)); utf8.RuneCountInString(got) != engineMaxUnits {
		t.Errorf("иероглифы: %d знаков", utf8.RuneCountInString(got))
	}
	if got := engineText("微软 " + strings.Repeat("слово ", 40)); len(strings.Fields(got)) != 1+engineMaxUnits-2 {
		t.Errorf("смешанный: %q", got)
	}
}

// S1: в ответе витрины у точного запроса остаётся шаг маржи без чисел — только защита; иначе шага нет.
func TestStripMarginStepKeepsExactGuard(t *testing.T) {
	exact := &domain.MarginStep{StepBase: domain.StepBase{Name: domain.StepMargin}, Weight: 0.05, PolicyVersion: 3,
		Shifts: []domain.MarginShift{}, Guards: []string{domain.MarginGuardExactQuery, domain.MarginNoteUnknown}}
	other := &domain.MarginStep{StepBase: domain.StepBase{Name: domain.StepMargin}, Weight: 0.05,
		Shifts: []domain.MarginShift{{ProductID: "a", From: 2, To: 1, Margin: 0.9}}, Guards: []string{}}
	tr := &domain.Trace{Steps: []any{exact}}
	stripMarginStep(tr)
	raw, _ := json.Marshal(tr.Steps)
	if string(raw) != `[{"name":"margin","ms":0,"guards":["exact_query"]}]` {
		t.Fatalf("точный запрос: %s", raw)
	}
	tr = &domain.Trace{Steps: []any{other}}
	stripMarginStep(tr)
	if len(tr.Steps) != 0 {
		t.Fatalf("сдвиги маржи ушли на витрину: %+v", tr.Steps)
	}
}

// Сохранённая трасса: ПДн в строках любых шагов маскируются, числа остаются числами.
func TestMaskJSONStrings(t *testing.T) {
	raw := domain.JSONB(`{"steps":[{"name":"normalize","ms":0.3,"changes":[{"from":"мой номер ИНН 7707083893","to":"x"}]}],"total_ms":12.5}`)
	got, err := maskJSONStrings(raw)
	if err != nil || strings.Contains(string(got), "7707083893") || !strings.Contains(string(got), `"total_ms":12.5`) ||
		!json.Valid(got) {
		t.Fatalf("трасса: %s %v", got, err)
	}
	clean := domain.JSONB(`{"a":"касперский на 10 пк"}`)
	if got, _ := maskJSONStrings(clean); string(got) != string(clean) {
		t.Fatalf("без ПДн байты изменились: %s", got)
	}
}
