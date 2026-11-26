package handlers

import (
	"testing"
	"time"
)

// Лимит частоты подсказок с адреса: max в окне, новое окно — снова можно; адреса независимы.
func TestSuggestRateWindow(t *testing.T) {
	r := &rateWindow{max: 3, window: time.Second, hits: map[string]*rateSlot{}}
	now := time.Unix(1000, 0)
	for i := 0; i < 3; i++ {
		if !r.allow("a", now) {
			t.Fatalf("запрос %d отклонён", i+1)
		}
	}
	if r.allow("a", now.Add(500*time.Millisecond)) {
		t.Fatal("четвёртый в окне принят")
	}
	if !r.allow("b", now) {
		t.Fatal("другой адрес отклонён")
	}
	if !r.allow("a", now.Add(time.Second)) {
		t.Fatal("новое окно отклонено")
	}
}
