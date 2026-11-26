package services

import "testing"

func TestLicenseLine(t *testing.T) {
	ptr := func(v int) *int { return &v }
	str := func(v string) *string { return &v }
	cases := []struct {
		doc  indexDoc
		want string
	}{
		{indexDoc{LicenseType: "subscription", LicenseMetric: str("device"), LicenseQuantity: ptr(10), LicenseDuration: ptr(12)}, "Подписка · 10 устройств · 1 год"},
		{indexDoc{LicenseType: "subscription", LicenseMetric: str("user"), LicenseQuantity: ptr(2), LicenseDuration: ptr(24)}, "Подписка · 2 пользователя · 2 года"},
		{indexDoc{LicenseType: "perpetual", LicenseMetric: str("user"), LicenseQuantity: ptr(21)}, "Бессрочная · 21 пользователь"},
		{indexDoc{LicenseType: "perpetual", LicenseMetric: str("org")}, "Бессрочная · на организацию"},
		{indexDoc{LicenseType: "renewal", LicenseMetric: str("core"), LicenseQuantity: ptr(16), LicenseDuration: ptr(6)}, "Продление · 16 ядер · 6 месяцев"},
		{indexDoc{LicenseType: "cloud", LicenseDuration: ptr(60)}, "Облачная · 5 лет"},
		{indexDoc{}, ""},
	}
	for _, c := range cases {
		if got := licenseLine(c.doc); got != c.want {
			t.Fatalf("%+v → %q, ожидалось %q", c.doc, got, c.want)
		}
	}
}

func TestPlural(t *testing.T) {
	forms := [3]string{"устройство", "устройства", "устройств"}
	for n, want := range map[int]string{1: "устройство", 2: "устройства", 5: "устройств", 11: "устройств", 14: "устройств",
		21: "устройство", 22: "устройства", 25: "устройств", 111: "устройств", 101: "устройство"} {
		if got := plural(n, forms); got != want {
			t.Fatalf("%d → %s, ожидалось %s", n, got, want)
		}
	}
}
