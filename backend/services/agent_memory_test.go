package services

import (
	"testing"

	"kuznica/api/domain"

	"github.com/google/uuid"
)

func TestPhoneticKey(t *testing.T) {
	same := [][2]string{{"ноушн", "notion"}, {"фигма", "figma"}, {"касперский", "kaspersky"}, {"джира", "jira"},
		{"чатгпт", "chatgpt"}}
	for _, p := range same {
		if a, b := phoneticKey(p[0]), phoneticKey(p[1]); a != b {
			t.Errorf("%q → %q, %q → %q: ждали один ключ", p[0], a, p[1], b)
		}
	}
	diff := [][2]string{{"ноушн", "нод32"}, {"notion", "битрикс24"}, {"ноушн", "мойофис"}, {"фигма", "фотошоп"}}
	for _, p := range diff {
		if termMatch(p[0], p[1]) != "" {
			t.Errorf("%q и %q не должны совпасть", p[0], p[1])
		}
	}
	// Короткие ключи не сравниваются: «кис» и «ксос» — разные продукты.
	if m := termMatch("кис", "ксос"); m != "" {
		t.Errorf("кис/ксос совпали: %s", m)
	}
	if m := termMatch("ygjhn", "нпорт"); m != "" && m != "layout" {
		t.Errorf("раскладка: %s", m)
	}
	if m := termMatch("abuvf", "фигма"); m != "layout" {
		t.Errorf("фигма в латинской раскладке: %q", m)
	}
	if m := termsMatch([]string{"чат", "гпт"}, []string{"chatgpt"}); m == "" {
		t.Errorf("чат гпт = chatgpt: %q", m)
	}
}

// Ждущее предложение по «notion» закрывает кластер «ноушн» (дубль из запуска 01a2101c); вектором — близкий термин.
func TestMemoryForTranslitAndVector(t *testing.T) {
	notion := domain.Proposal{Status: domain.ProposalPending, ClusterKey: "notion", ClusterLabel: "notion",
		Evidence: toJSON(map[string]any{"terms": []string{"notion"}})}
	notion.ID = uuid.New()
	nod := domain.Proposal{Status: domain.ProposalPending, ClusterKey: "нод32", ClusterLabel: "нод32",
		Evidence: toJSON(map[string]any{"terms": []string{"нод32"}})}
	nod.ID = uuid.New()
	rc := &runCtx{proposals: []domain.Proposal{notion, nod}}
	hits := rc.memoryFor(t.Context(), "ноушн", []string{"ноушн"})
	if len(hits) != 1 || hits[0].ProposalID != notion.ID.String() || hits[0].Match != "phonetic" {
		t.Fatalf("ноушн: %+v", hits)
	}
	if hits := rc.memoryFor(t.Context(), "мойофис", []string{"мойофис"}); len(hits) != 0 {
		t.Fatalf("мойофис: %+v", hits)
	}
	// Вектор: близко (косинус ≥ 0,85) — совпадение, дальше — нет.
	rc.memVecs = map[string][]float32{"notion": {1, 0}, "нод32": {0, 1}, "блокнот": {0.9, 0.1}, "ноутбук": {0.6, 0.8}}
	if hits := rc.memoryFor(t.Context(), "блокнот", []string{"блокнот"}); len(hits) != 1 || hits[0].Match != "vector" {
		t.Fatalf("вектор: %+v", hits)
	}
	if hits := rc.memoryFor(t.Context(), "ноутбук", []string{"ноутбук"}); len(hits) != 0 {
		t.Fatalf("далёкий вектор: %+v", hits)
	}
}
