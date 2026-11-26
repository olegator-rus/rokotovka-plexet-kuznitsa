package services

import (
	"testing"
	"time"

	"kuznica/api/domain"
)

// Резерв агента: разборы покупателей уходят на правила при расходе ≥ бюджет − резерв, агенту доступен весь бюджет.
// Квота на адрес считается только по вызовам с адресом и обнуляется в новые сутки.
func TestLLMGuardReserveAndAddrQuota(t *testing.T) {
	now := time.Date(2026, 5, 27, 20, 0, 0, 0, time.UTC) // 23:00 МСК
	g := newLLMGuard(llmModeOn, true, 10, func() time.Time { return now }, nil)
	g.reserve, g.perAddr = 3, 1
	ctx := t.Context()
	a, b := WithClientIP(ctx, "198.51.100.7"), WithClientIP(ctx, "203.0.113.9")

	g.charge(0.6, "198.51.100.7")
	if g.addrExceeded(a) || g.addrExceeded(b) {
		t.Fatal("0,6 из 1 ₽ — квота не выбрана")
	}
	g.charge(0.5, "198.51.100.7")
	if !g.addrExceeded(a) || g.addrExceeded(b) || g.addrExceeded(ctx) {
		t.Fatal("1,1 из 1 ₽ — квота адреса выбрана, у другого адреса и без адреса — нет")
	}
	g.add(5.8) // агент и CLI — без адреса: в квоты адресов не идут
	if g.buyerExceeded(ctx) || g.exceeded(ctx) {
		t.Fatal("6,9 ₽ < 7 ₽ (бюджет 10 − резерв 3)")
	}
	g.add(0.2)
	if !g.buyerExceeded(ctx) || g.exceeded(ctx) {
		t.Fatal("7,1 ₽: покупатели — на правилах, агенту резерв ещё доступен")
	}
	if st := (&Services{llm: g}).LLMState(ctx); st.Banner == nil || st.Budget.Exceeded {
		t.Fatalf("плашка — как только покупатели на правилах; exceeded — по всему бюджету: %+v", st)
	}
	g.add(3)
	if !g.exceeded(ctx) {
		t.Fatal("10,1 из 10 ₽ — бюджет исчерпан и для агента")
	}
	now = now.Add(2 * time.Hour) // 01:00 МСК следующих суток
	if g.addrExceeded(a) || g.buyerExceeded(ctx) {
		t.Fatal("новые сутки — квоты с нуля")
	}
}

// Разбор маршрута D сверх квоты адреса — на правилах с причиной ip_quota; другой адрес идёт в модель.
func TestParseAddrQuota(t *testing.T) {
	f := newFakeLLM(t, fakeReply{content: photoshopReply, in: 800, out: 100})
	p := newIntentEnv(t, "on", f.client("aliceai-llm-flash"), nil, nil)
	p.s.llm.perAddr = 0.0001 // первый же вызов выбирает квоту
	a := WithClientIP(t.Context(), "198.51.100.7")
	if got, _ := p.Parse(a, "аналог фотошопа на 20 пользователей", domain.Intent{}, ""); got.Source != domain.ParseLLM {
		t.Fatalf("первый разбор — модель: %+v", got)
	}
	got, _ := p.Parse(a, "чем заменить фотошоп дома", domain.Intent{}, "")
	if got.FallbackReason != fallbackIPQuota || f.calls() != 1 {
		t.Fatalf("сверх квоты адреса — правила: %+v (вызовов %d)", got, f.calls())
	}
	b := WithClientIP(t.Context(), "203.0.113.9")
	if got, _ := p.Parse(b, "чем заменить фотошоп дома", domain.Intent{}, ""); got.Source != domain.ParseLLM || f.calls() != 2 {
		t.Fatalf("другой адрес — модель: %+v", got)
	}
}

// IPv6: квота на сеть /64 (ревью-2 n2) — адреса одной /64 делят квоту, соседняя /64 — своя; IPv4 в IPv6 — как IPv4.
func TestLLMGuardIPv6Prefix(t *testing.T) {
	for addr, want := range map[string]string{
		"198.51.100.7":                     "198.51.100.7",
		"::ffff:198.51.100.7":              "198.51.100.7",
		"2001:db8:1:2:aaaa:bbbb:cccc:dddd": "2001:db8:1:2::/64",
		"2001:db8:1:2::1":                  "2001:db8:1:2::/64",
		"fe80::1%eth0":                     "fe80::/64",
		"не адрес":                         "не адрес",
	} {
		if got := quotaKey(addr); got != want {
			t.Errorf("quotaKey(%q) = %q, ожидалось %q", addr, got, want)
		}
	}
	now := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	g := newLLMGuard(llmModeOn, true, 0, func() time.Time { return now }, nil)
	g.perAddr = 1
	g.charge(0.6, "2001:db8:1:2::1")
	g.charge(0.6, "2001:db8:1:2:ffff::9")
	if !g.addrExceeded(WithClientIP(t.Context(), "2001:db8:1:2::77")) {
		t.Fatal("1,2 ₽ из одной /64 — квота выбрана для любого её адреса")
	}
	if g.addrExceeded(WithClientIP(t.Context(), "2001:db8:1:3::1")) {
		t.Fatal("соседняя /64 — своя квота")
	}
}
