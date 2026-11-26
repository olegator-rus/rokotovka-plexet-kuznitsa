package services

import (
	"strings"
	"testing"
	"time"
)

// P8 (QA r2 №1): шаг «Неудачные запросы» запуска на одном запросе говорит, какую часть журнала видит агент и что
// последняя неделя отложена для проверки, а не «в журнале за 30 дней».
func TestAgentWindowPhrase(t *testing.T) {
	if got := agentWindowPhrase(holdoutInfo{}, 30); got != "в журнале за 30 дней" {
		t.Fatalf("без разреза: %q", got)
	}
	hf, ht := "2026-05-20", "2026-05-26"
	h := holdoutInfo{Enabled: true, HoldoutFrom: &hf, HoldoutTo: &ht, ExcludeTest: true, TestTexts: 144,
		SeenFrom: time.Date(2026, 4, 19, 21, 0, 0, 0, time.UTC), SeenTo: time.Date(2026, 5, 19, 21, 0, 0, 0, time.UTC)}
	got := agentWindowPhrase(h, 30)
	for _, want := range []string{"которую видит агент", "с 20.04 по 19.05", "неделя 20.05–26.05 отложена для проверки", "test"} {
		if !strings.Contains(got, want) {
			t.Fatalf("нет %q в %q", want, got)
		}
	}
	// Окно после отложенной недели (всплеск, ночной разбор).
	h.SeenFrom, h.SeenTo, h.TestTexts = time.Date(2026, 5, 26, 21, 0, 0, 0, time.UTC), time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC), 0
	if got := agentWindowPhrase(h, 30); !strings.Contains(got, "(с 27.05;") || strings.Contains(got, "test") {
		t.Fatalf("окно после недели: %q", got)
	}
}
