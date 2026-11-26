package services

import (
	"slices"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/out"
)

func TestGuideEvalNumbersAndPrice(t *testing.T) {
	got := guideNumbersIn("Подходит «Расширенный»: 3 940,20 ₽ за компьютер, 197 010 ₽ на 50.")
	for _, want := range []string{"3940.2", "197010", "50"} {
		if !slices.Contains(got, want) {
			t.Fatalf("числа %v без %s", got, want)
		}
	}
	price := 3940.2
	doc := indexDoc{ProductID: "o1", FamilyID: "f", PriceRub: &price}
	total := 197010.0
	ok := out.GuideOffer{OfferID: "o1", FamilyID: "f", PriceState: out.PricePriced, PricePerUnit: &price, Total: &total, Quantity: 50}
	if !guidePriceOK(ok, doc) {
		t.Fatal("цена и сумма каталога — верно")
	}
	wrong := 3900.0
	bad := ok
	bad.PricePerUnit = &wrong
	if guidePriceOK(bad, doc) {
		t.Fatal("цена не из каталога — ошибка")
	}
	over := 51 * price
	bad = ok
	bad.Total = &over
	if guidePriceOK(bad, doc) {
		t.Fatal("сумма на 51 единицу при 50 — ошибка")
	}
	onReq := out.GuideOffer{OfferID: "o2", PriceState: out.PricePriced, PricePerUnit: &price}
	if guidePriceOK(onReq, indexDoc{ProductID: "o2"}) {
		t.Fatal("цена у позиции «по запросу» — выдумана")
	}
}

func TestGuideEvalLicense(t *testing.T) {
	kb := &guideEvalKB{scope: map[string]string{"ws": "workstation", "srv": "server"}, pack: map[string]int{"pk": 30}}
	server := domain.GuideRequirements{Operation: "new", Objects: []domain.GuideObject{{Kind: "server", Count: 1}}}
	if v := guideLicenseViolation(out.GuideOffer{OfferID: "ws"}, indexDoc{FamilyID: "22268"}, server, kb); v == "" {
		t.Fatal("рабочее место CSP для сервера — нарушение")
	}
	if v := guideLicenseViolation(out.GuideOffer{OfferID: "srv"}, indexDoc{FamilyID: "22268"}, server, kb); v != "" {
		t.Fatal("серверная лицензия — без нарушения: " + v)
	}
	if v := guideLicenseViolation(out.GuideOffer{OfferID: "r"}, indexDoc{FamilyID: "x", LicenseKind: domain.LicenseKindRenewal}, server, kb); v == "" {
		t.Fatal("продление при новой лицензии — нарушение")
	}
	price, two := 100.0, 200.0
	ws := domain.GuideRequirements{Operation: "new", Objects: []domain.GuideObject{{Kind: "workstation", Count: 30}}}
	if v := guideLicenseViolation(out.GuideOffer{OfferID: "pk", Total: &price}, indexDoc{FamilyID: "d", PriceRub: &price}, ws, kb); v != "" {
		t.Fatal("комплект на 30 мест на 30 ПК — без нарушения: " + v)
	}
	if v := guideLicenseViolation(out.GuideOffer{OfferID: "pk", Total: &two}, indexDoc{FamilyID: "d", PriceRub: &price}, ws, kb); v == "" {
		t.Fatal("два комплекта на 30 ПК — нарушение")
	}
	if c, _ := guideRemovedClass("offer:1:cryptopro_workstation_not_server"); c != "license" {
		t.Fatal(c)
	}
	if c, _ := guideRemovedClass("claim:kb_x"); c != "claim" {
		t.Fatal(c)
	}
}
