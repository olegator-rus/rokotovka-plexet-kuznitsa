package catalogsource

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kuznica/api/domain"
)

// TestGenericFeedFields — поля контура фида: цена только у позиции в наличии с ценой > 0, прежняя
// цена, страницы, вид и ступень лицензии, атрибуты «attrs.*», путь категорий, вендор с URL, необязательный
// файл URL семейств, артикул одной позиции, совпавший с id другой.
func TestGenericFeedFields(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("products.jsonl", strings.Join([]string{
		`{"id":"1","sku":"2","title":"Astra Linux SE, 12 мес.","vendor":"astralinux","category":"20","family_id":"500","family_name":"Astra Linux Special Edition","price":5000,"old_price":6000,"available":true,"offer_url":"https://s/astralinux/x-1/","license_kind":"base","license_org_type":"commercial","attrs":{"Версия":"Орел","Срок действия":"12 мес."}}`,
		`{"id":"2","sku":"","title":"Astra Linux SE, техподдержка","vendor":"astralinux","category":"20","family_id":"500","family_name":"Astra Linux Special Edition","price":1200,"old_price":0,"available":false,"license_kind":"Подписка","license_tier_min":100,"license_tier_max":149}`,
		`{"id":"3","sku":"","title":"Windows Server 2025","vendor":"microsoft","category":"20","family_id":"600","family_name":"Windows Server","price":0,"old_price":0,"available":true}`,
	}, "\n"))
	write("vendors.jsonl", `{"id":"astralinux","name":"Группа Астра","url":"https://s/astralinux/","slug":"astralinux"}`+"\n"+
		`{"id":"microsoft","name":"Microsoft","url":"https://s/microsoft/","slug":"microsoft"}`)
	write("categories.jsonl", `{"id":"10","name":"Операционные системы","parent_id":""}`+"\n"+
		`{"id":"20","name":"Российские ОС","parent_id":"10"}`)
	mapping := `version: feed-test
catalog:
  file: products.jsonl
  columns: {id: id, sku: sku, title: title, vendor: vendor, category: category, family_id: family_id,
    family_name: family_name, price_rub: price, old_price_rub: old_price, available: available, offer_url: offer_url,
    license_kind: license_kind, license_org_type: license_org_type, license_tier_min: license_tier_min,
    license_tier_max: license_tier_max}
  attributes: ["attrs.*"]
vendors:
  file: vendors.jsonl
  columns: {id: id, name: name, url: url, slug: slug}
categories:
  file: categories.jsonl
  columns: {id: id, name: name, parent_id: parent_id}
family_urls:
  file: family_urls.jsonl
  columns: {family_id: group_id, url: family_url}
`
	write("mapping.yaml", mapping)
	g, err := NewGeneric(filepath.Join(dir, "mapping.yaml"), nil)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := g.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Products) != 3 {
		t.Fatalf("позиций %d, ожидалось 3 (артикул «2» у первой не делает id «2» повтором)", len(snap.Products))
	}
	by := map[string]domain.Product{}
	for _, p := range snap.Products {
		by[p.ID] = p
	}
	p1, p2, p3 := by["1"], by["2"], by["3"]
	if p1.PriceRub == nil || *p1.PriceRub != 5000 || p1.PriceOnRequest || p1.FeedPriceRub != nil {
		t.Errorf("в наличии с ценой — цена: %+v", p1)
	}
	if p1.OldPriceRub == nil || *p1.OldPriceRub != 6000 || p1.Available == nil || !*p1.Available {
		t.Errorf("прежняя цена и наличие: %+v", p1)
	}
	if p2.PriceRub != nil || !p2.PriceOnRequest || p2.FeedPriceRub == nil || *p2.FeedPriceRub != 1200 || p2.OldPriceRub != nil {
		t.Errorf("не в наличии — по запросу, цена фида отдельно: %+v", p2)
	}
	if p3.PriceRub != nil || !p3.PriceOnRequest || p3.FeedPriceRub != nil {
		t.Errorf("цена 0 — по запросу: %+v", p3)
	}
	if p1.OfferURL != "https://s/astralinux/x-1/" || p1.License.Kind != domain.LicenseKindBase || p1.License.OrgType != "commercial" {
		t.Errorf("страница и лицензия: %+v", p1)
	}
	if p2.License.Kind != domain.LicenseKindOther || p2.License.TierMin == nil || *p2.License.TierMin != 100 ||
		p2.License.TierMax == nil || *p2.License.TierMax != 149 {
		t.Errorf("незнакомый вид — other, ступень 100–149: %+v", p2.License)
	}
	if !strings.Contains(string(p1.Attributes), `"Версия":"Орел"`) {
		t.Errorf("атрибуты attrs.*: %s", p1.Attributes)
	}
	if strings.Join(p1.CategoryPathIDs, ">") != "10>20" {
		t.Errorf("путь категорий: %v", p1.CategoryPathIDs)
	}
	v := map[string]domain.Vendor{}
	for _, x := range snap.Vendors {
		v[x.ID] = x
	}
	if v["astralinux"].URL != "https://s/astralinux/" || v["astralinux"].Slug != "astralinux" {
		t.Errorf("вендор: %+v", v["astralinux"])
	}
	warned := false
	for _, w := range snap.Warnings {
		warned = warned || strings.Contains(w, "family_urls")
	}
	if !warned || p1.FamilyURL != "" {
		t.Errorf("нет файла URL семейств — предупреждение и пустой URL: %v", snap.Warnings)
	}

	// Файл URL семейств появился — URL у всех позиций семейства.
	write("family_urls.jsonl", `{"group_id":"500","family_url":"https://s/astralinux/astra-linux-special-edition/"}`)
	snap, err = g.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range snap.Products {
		want := ""
		if p.FamilyID == "500" {
			want = "https://s/astralinux/astra-linux-special-edition/"
		}
		if p.FamilyURL != want {
			t.Errorf("%s: URL семейства %q, ожидалось %q", p.ID, p.FamilyURL, want)
		}
	}
}
