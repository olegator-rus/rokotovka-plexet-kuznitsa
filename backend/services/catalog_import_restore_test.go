package services

import (
	"os/exec"
	"path/filepath"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/externalApi/catalogsource"
)

func TestLicenseFromTitle(t *testing.T) {
	cases := []struct {
		title         string
		typ, metric   string
		qty, duration int
	}{
		{"Kaspersky Internet Security, 2 устройства, 1 год", domain.LicenseSubscription, domain.MetricDevice, 2, 12},
		{"Microsoft Windows 11 Home, 1 устройство, бессрочная лицензия", domain.LicensePerpetual, domain.MetricDevice, 1, 0},
		{"Kaspersky Premium, 5 устройств, продление на 1 год", domain.LicenseRenewal, domain.MetricDevice, 5, 12},
		{"Microsoft SQL Server 2022 Standard, лицензия на 2 ядра", domain.LicensePerpetual, domain.MetricCore, 2, 0},
		{"Postgres Pro Standard, 1 сервер, бессрочная лицензия", domain.LicensePerpetual, domain.MetricServer, 1, 0},
		{"Microsoft 365 Business Basic, 1 пользователь, 1 год", domain.LicenseSubscription, domain.MetricUser, 1, 12},
		{"Dr.Web Security Space на 3 ПК на 2 года", domain.LicenseSubscription, domain.MetricDevice, 3, 24},
	}
	for _, c := range cases {
		l, _, _ := licenseFromText(c.title)
		got := func(p *int) int {
			if p == nil {
				return 0
			}
			return *p
		}
		metric := ""
		if l.Metric != nil {
			metric = *l.Metric
		}
		if l.Type != c.typ || metric != c.metric || got(l.Quantity) != c.qty || got(l.DurationMonths) != c.duration {
			t.Errorf("%q → %s/%s/%d/%d", c.title, l.Type, metric, got(l.Quantity), got(l.DurationMonths))
		}
	}
}

func TestFamilyKey(t *testing.T) {
	same := [][2]string{
		{"Kaspersky Plus, 1 устройство, 1 год", "Kaspersky Plus, 5 устройств, продление на 1 год"},
		{"Dr.Web Security Space, 3 ПК, 2 года", "Dr.Web Security Space на 1 ПК на 1 год"},
	}
	for _, p := range same {
		if a, b := familyKey(p[0]), familyKey(p[1]); a != b {
			t.Errorf("одно семейство: %q ≠ %q", a, b)
		}
	}
	if familyKey("Kaspersky Plus, 1 устройство, 1 год") == familyKey("Kaspersky Premium, 1 устройство, 1 год") {
		t.Error("Plus и Premium — разные семейства")
	}
	if n := familyNameFrom([]string{"Kaspersky Plus, 5 устройств, 1 год", "Kaspersky Plus, 1 устройство, 1 год"},
		familyKey("Kaspersky Plus, 1 устройство, 1 год")); n != "Kaspersky Plus" {
		t.Errorf("название семейства: %q", n)
	}
}

// TestForeignDataset — «чужой» набор из mock (scripts/data/make_foreign.py: XLSX + CSV cp1251 + TSV, плоский
// каталог без лицензий) через Generic и RestoreCatalog; сверка с mock — что восстановилось.
func TestForeignDataset(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 нет")
	}
	dir := t.TempDir()
	if out, err := exec.Command(py, "-I", "../../scripts/data/make_foreign.py", "--out", dir).CombinedOutput(); err != nil {
		t.Fatalf("make_foreign: %v\n%s", err, out)
	}
	g, err := catalogsource.NewGeneric(filepath.Join(dir, "mapping.yaml"), RestoreCatalog)
	if err != nil {
		t.Fatal(err)
	}
	got, err := g.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want, err := catalogsource.MockJSON{Dir: "../../data/mock"}.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Products) != len(want.Products) {
		t.Fatalf("позиций %d, ожидалось %d", len(got.Products), len(want.Products))
	}
	truth := map[string]domain.Product{}
	for _, p := range want.Products {
		truth[p.ID] = p
	}
	eq := func(a, b *int) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }
	eqs := func(a, b *string) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }
	var typ, metric, qty, dur, vendor, cat, price, reestr, origin int
	for _, p := range got.Products {
		w := truth[p.ID]
		if p.License.Type == w.License.Type {
			typ++
		}
		if eqs(p.License.Metric, w.License.Metric) {
			metric++
		}
		if eq(p.License.Quantity, w.License.Quantity) {
			qty++
		}
		if eq(p.License.DurationMonths, w.License.DurationMonths) {
			dur++
		}
		if p.VendorID != "" {
			vendor++
		}
		if p.CategoryID != "" {
			cat++
		}
		if (p.PriceRub == nil && w.PriceRub == nil) || (p.PriceRub != nil && w.PriceRub != nil && *p.PriceRub == *w.PriceRub) {
			price++
		}
		if p.InReestr == w.InReestr {
			reestr++
		}
		if p.Origin == w.Origin {
			origin++
		}
	}
	// Семейства — попарная точность и полнота против семейств mock.
	var tp, fp, fn int
	for i, a := range got.Products {
		for _, b := range got.Products[i+1:] {
			same, truthSame := a.FamilyID == b.FamilyID, truth[a.ID].FamilyID == truth[b.ID].FamilyID
			switch {
			case same && truthSame:
				tp++
			case same:
				fp++
			case truthSame:
				fn++
			}
		}
	}
	fams := map[string]bool{}
	for _, p := range got.Products {
		fams[p.FamilyID] = true
	}
	n := float64(len(got.Products))
	prec, rec := float64(tp)/float64(max(tp+fp, 1)), float64(tp)/float64(max(tp+fn, 1))
	t.Logf("тип %.0f%%, метрика %.0f%%, количество %.0f%%, срок %.0f%%; вендор %.0f%%, категория %.0f%%, цена %.0f%%, реестр %.0f%%, происхождение %.0f%%",
		100*float64(typ)/n, 100*float64(metric)/n, 100*float64(qty)/n, 100*float64(dur)/n, 100*float64(vendor)/n,
		100*float64(cat)/n, 100*float64(price)/n, 100*float64(reestr)/n, 100*float64(origin)/n)
	t.Logf("семейства: %d (в mock %d), попарно точность %.2f, полнота %.2f", len(fams), countFamilies(want), prec, rec)
	evs, err := g.LoadEvents(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	popular := 0
	for _, p := range got.Products {
		if p.Popularity > 0 {
			popular++
		}
	}
	t.Logf("связей %d (в mock %d), клиентов %d, событий %d, популярность у %d позиций", len(got.Relations), len(want.Relations),
		len(got.Clients), len(evs), popular)
	if price != len(got.Products) || reestr != len(got.Products) || vendor != len(got.Products) || origin != len(got.Products) {
		t.Error("цена, реестр, вендор и происхождение должны совпасть у всех позиций")
	}
	if float64(qty)/n < 0.9 || float64(typ)/n < 0.8 || float64(dur)/n < 0.9 || prec < 0.95 || rec < 0.5 {
		t.Error("восстановление ниже порога: количество ≥90%, тип ≥80%, срок ≥90%, семейства точность ≥0.95 и полнота ≥0.5")
	}
	if len(evs) != 5121 || len(got.Clients) != len(want.Clients) || len(got.Relations) < len(want.Relations)*9/10 {
		t.Errorf("журнал, клиенты или связи потерялись: %d событий, %d клиентов, %d связей", len(evs), len(got.Clients), len(got.Relations))
	}
}

func countFamilies(s *domain.CatalogSnapshot) int {
	m := map[string]bool{}
	for _, p := range s.Products {
		m[p.FamilyID] = true
	}
	return len(m)
}
