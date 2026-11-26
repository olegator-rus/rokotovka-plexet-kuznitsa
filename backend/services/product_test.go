package services

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/externalApi/catalogsource"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
)

// productSvc — сервисы со справочниками каталога в памяти (без OpenSearch и базы): страница товара индекс не читает.
func productSvc(t *testing.T, dir string, f Features) *Services {
	t.Helper()
	svc := NewServices(nil, Options{Source: catalogsource.MockJSON{Dir: dir}, Features: f})
	svc.loadCatalogState(t.Context())
	if svc.catalog.Load() == nil {
		t.Fatalf("каталог %s не загружен", dir)
	}
	return svc
}

func productPage(t *testing.T, svc *Services, id string) *out.ProductPage {
	t.Helper()
	p, err := svc.Product(t.Context(), id)
	if err != nil {
		t.Fatalf("%s: %v", id, err)
	}
	return p
}

func attrMap(as []out.ProductAttr) map[string]out.ProductAttr {
	m := map[string]out.ProductAttr{}
	for _, a := range as {
		m[a.Key] = a
	}
	return m
}

// ПО: поля позиции, строка лицензии, варианты без самой позиции, реестр, подписи атрибутов, значение-число строкой.
func TestProductSoftware(t *testing.T) {
	svc := productSvc(t, "../testdata/catalog", Features{})
	p := productPage(t, svc, "kis-2dev-1y")
	if p.CatalogVersion != "fixture-1" || p.ProductID != "kis-2dev-1y" || p.Kind != domain.KindSoftware ||
		p.FamilyID != "kaspersky-internet-security" || p.Vendor.ID != "kaspersky" || p.Vendor.Name == "" ||
		p.Category.ID != "antivirus-home" || p.Category.Name == "" || p.Description == "" ||
		p.PriceRub == nil || *p.PriceRub != 2990 || p.PriceMode != domain.PriceFixed {
		t.Fatalf("поля: %+v", p)
	}
	if p.LicenseLine != "Подписка · 2 устройства · 1 год" {
		t.Fatalf("license_line = %q", p.LicenseLine)
	}
	if len(p.Variants) != 1 || p.Variants[0].ProductID != "kis-5dev-1y" || p.Variants[0].LicenseLine == "" {
		t.Fatalf("варианты: %+v", p.Variants)
	}
	if p.Reestr == nil || p.Reestr.Label == "" || len(p.Specs) != 0 {
		t.Fatalf("реестр %+v, specs %+v", p.Reestr, p.Specs)
	}
	am := attrMap(p.Attributes)
	if am["platform"].Label != "Платформа" || am["platform"].Value != "Windows, macOS, Android" ||
		am["edition"].Label != "Редакция" {
		t.Fatalf("атрибуты: %+v", p.Attributes)
	}
	if a := attrMap(productPage(t, svc, "ksos-10dev-1y").Attributes)["servers"]; a.Value != "1" || a.Label == "" {
		t.Fatalf("число в атрибуте — строкой: %+v", a)
	}
	// Зарубежное ПО без записи в реестре — «Не соответствует».
	if r := productPage(t, svc, "photoshop-1u-1y").Reestr; r == nil || r.Status != domain.ReestrNotMatching {
		t.Fatalf("реестр Photoshop: %+v", r)
	}
}

// Варианты: по цене, без цены — в конце, затем по названию (на data/mock, где у семейств много позиций).
func TestProductVariantsOrder(t *testing.T) {
	svc := productSvc(t, "../../data/mock", Features{})
	cs := svc.catalog.Load()
	checked := 0
	for fam, pids := range cs.lex.families {
		if len(pids) < 3 {
			continue
		}
		p := productPage(t, svc, pids[0])
		if len(p.Variants) != len(pids)-1 {
			t.Fatalf("%s: вариантов %d, позиций %d", fam, len(p.Variants), len(pids))
		}
		for _, v := range p.Variants {
			if v.ProductID == p.ProductID {
				t.Fatalf("%s: позиция среди своих вариантов", fam)
			}
		}
		if !sort.SliceIsSorted(p.Variants, func(i, j int) bool {
			a, b := p.Variants[i], p.Variants[j]
			pa, pb := a.PriceRub != nil && !a.PriceOnRequest, b.PriceRub != nil && !b.PriceOnRequest
			if pa != pb {
				return pa
			}
			if pa && *a.PriceRub != *b.PriceRub {
				return *a.PriceRub < *b.PriceRub
			}
			return a.Title < b.Title
		}) {
			t.Fatalf("%s: порядок вариантов %+v", fam, p.Variants)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("в data/mock нет семейств из 3+ позиций")
	}
}

// Оборудование: specs с подписью и единицей, совместимость из правил, реестра нет.
func TestProductHardware(t *testing.T) {
	svc := productSvc(t, "../testdata/catalog", Features{})
	p := productPage(t, svc, "ram-32g-rdimm")
	if p.Kind != domain.KindHardware || p.Reestr != nil {
		t.Fatalf("вид %q, реестр %+v", p.Kind, p.Reestr)
	}
	if len(p.Specs) != 1 || p.Specs[0] != (out.ProductSpec{Key: "ram_gb", Label: "Память", Value: 32, Unit: "ГБ"}) {
		t.Fatalf("specs: %+v", p.Specs)
	}
	if len(p.Compatibility) != 1 || p.Compatibility[0].TargetModel != "dell-poweredge-r740" ||
		p.Compatibility[0].State != "verified" || p.Compatibility[0].SourceURL == "" {
		t.Fatalf("совместимость: %+v", p.Compatibility)
	}
	srv := productPage(t, svc, "dell-r740-base")
	if len(srv.Specs) != 3 || srv.PriceMode != domain.PriceFrom || len(srv.Compatibility) != 0 {
		t.Fatalf("сервер: specs %+v, price_mode %q, compat %+v", srv.Specs, srv.PriceMode, srv.Compatibility)
	}
}

// Аналоги — связи substitute от семейства, с пояснением связи.
func TestProductAnalogs(t *testing.T) {
	svc := productSvc(t, "../testdata/catalog", Features{})
	p := productPage(t, svc, "photoshop-1u-1y")
	if len(p.Analogs) != 1 || p.Analogs[0].FamilyID != "fotomaster" || p.Analogs[0].ProductID != "fotomaster-pro-perp" ||
		p.Analogs[0].Note != "отечественный фоторедактор" || p.Analogs[0].Vendor == "" {
		t.Fatalf("аналоги: %+v", p.Analogs)
	}
	if a := productPage(t, svc, "kis-2dev-1y").Analogs; len(a) != 0 {
		t.Fatalf("у Kaspersky аналогов в фикстуре нет: %+v", a)
	}
}

// «С этим покупают» — подбор GET /api/companions; флаг выключен — пустые массивы.
func TestProductCompanions(t *testing.T) {
	on := productSvc(t, "../testdata/catalog", Features{Companions: true, Solutions: true})
	p := productPage(t, on, "ws2025-std-16core")
	want, err := on.ProductCompanions(t.Context(), "ws2025-std-16core", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Companions.Required)+len(p.Companions.Items) == 0 ||
		len(p.Companions.Required) != len(want.Required) || len(p.Companions.Items) != len(want.Items) {
		t.Fatalf("компаньоны: %+v, /api/companions: %+v", p.Companions, want)
	}
	off := productPage(t, productSvc(t, "../testdata/catalog", Features{}), "ws2025-std-16core")
	if off.Companions.Required == nil || off.Companions.Items == nil ||
		len(off.Companions.Required)+len(off.Companions.Items) != 0 {
		t.Fatalf("флаг выключен: %+v", off.Companions)
	}
}

// Массивы — всегда массивы; в JSON нет маржи и популярности.
func TestProductJSONShape(t *testing.T) {
	svc := productSvc(t, "../testdata/catalog", Features{Companions: true, Solutions: true})
	for _, id := range []string{"kis-2dev-1y", "dell-r740-base", "ram-32g-rdimm", "ws2025-cal-user", "fotomaster-pro-perp"} {
		raw, err := json.Marshal(productPage(t, svc, id))
		if err != nil {
			t.Fatal(err)
		}
		s := string(raw)
		for _, bad := range []string{`"margin"`, `"popularity"`, `"attributes":null`, `"specs":null`,
			`"variants":null`, `"compatibility":null`, `"analogs":null`, `"required":null`, `"items":null`} {
			if strings.Contains(s, bad) {
				t.Fatalf("%s: в ответе %s: %s", id, bad, s)
			}
		}
		if strings.Contains(s, `"upgrade"`) {
			t.Fatalf("%s: поле upgrade из /api/companions попало в ответ: %s", id, s)
		}
	}
}

// Ошибки: неизвестная позиция — 404, id длиннее 128 — ошибка валидации поля id, каталог не загружен — 503.
func TestProductErrors(t *testing.T) {
	svc := productSvc(t, "../testdata/catalog", Features{})
	var e *errs.Error
	if _, err := svc.Product(t.Context(), "net-takogo"); !errors.As(err, &e) || e.HTTPStatus() != 404 {
		t.Fatalf("неизвестный id: %v", err)
	}
	_, err := svc.Product(t.Context(), strings.Repeat("x", 129))
	if !errors.As(err, &e) || e.Type != errs.ErrTypeValidation || len(e.Details) != 1 || e.Details[0].Field != "id" {
		t.Fatalf("id > 128: %v", err)
	}
	if _, err := NewServices(nil, Options{}).Product(t.Context(), "kis-2dev-1y"); !errors.As(err, &e) ||
		e.HTTPStatus() != 503 {
		t.Fatalf("без каталога: %v", err)
	}
}

// data/mock: у каждого ключа attributes есть подпись (attribute_definitions категории или словарь сервиса).
func TestProductMockAttrLabels(t *testing.T) {
	svc := productSvc(t, "../../data/mock", Features{})
	cc := svc.catalog.Load().commerce
	for id, p := range cc.products {
		var raw map[string]any
		if len(p.Attributes) > 0 {
			if err := json.Unmarshal(p.Attributes, &raw); err != nil {
				t.Fatalf("%s: %v", id, err)
			}
		}
		got := attrMap(productPageAttrs(cc, p))
		for k := range raw {
			if got[k].Label == "" {
				t.Errorf("%s: у ключа %q нет подписи", id, k)
			}
		}
		for k, v := range p.Specs {
			found := false
			for _, s := range productSpecs(cc, p) {
				found = found || (s.Key == k && s.Value == v && s.Label != "")
			}
			if !found {
				t.Errorf("%s: характеристика %q без подписи", id, k)
			}
		}
	}
}
