package repositories

import (
	"testing"
	"time"

	"kuznica/api/domain"
)

// Контур коммерции на живой базе (KUZNICA_TEST_DATABASE_URL, своя схема): цепочки версий по kind независимы, прежний
// уникальный индекс по version снимается миграцией, справочники контура коммерции переживают запись и чтение каталога.

func TestConfigVersionKinds(t *testing.T) {
	r := testSchemaRepos(t)
	ctx := t.Context()
	// База до контура коммерции: уникальный индекс по одному version. Повторная миграция его снимает.
	if err := r.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_config_versions_version ON config_versions (version)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := r.MigrateDB(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.SaveImportRules(ctx, "c1", rulesJSON(t, "c1", "a"), "import"); err != nil {
		t.Fatal(err)
	}
	for _, v := range []domain.ConfigVersion{
		{Kind: domain.ConfigKindRelations, Version: 1, Source: "import", Rules: domain.JSONB(`{}`)},
		{Kind: domain.ConfigKindRelations, Version: 2, Source: "agent", Rules: domain.JSONB(`{}`)},
		{Kind: domain.ConfigKindMargin, Version: 7, Source: "human", Rules: domain.JSONB(`{}`)},
	} {
		if err := r.db.Create(&v).Error; err != nil {
			t.Fatalf("версия %s/%d: %v", v.Kind, v.Version, err)
		}
	}
	if err := r.db.Create(&domain.ConfigVersion{Kind: domain.ConfigKindRelations, Version: 2, Rules: domain.JSONB(`{}`)}).Error; err == nil {
		t.Fatal("повтор (kind, version) принят")
	}
	if v, err := r.RulesVersion(ctx); err != nil || v != 1 {
		t.Fatalf("версия правил %d, %v", v, err)
	}
	if rows, err := r.ListConfigVersions(ctx); err != nil || len(rows) != 1 || rows[0].Kind != domain.ConfigKindRules {
		t.Fatalf("список версий правил: %+v, %v", rows, err)
	}
	if v, err := r.GetConfigVersion(ctx, 7); err != nil || v != nil {
		t.Fatalf("версия маржи видна как версия правил: %+v, %v", v, err)
	}
	if snap, err := r.LatestRules(ctx); err != nil || snap == nil || snap.Version != 1 {
		t.Fatalf("последние правила: %+v, %v", snap, err)
	}
	// Новая версия правил агентом — поверх версии правил, а не связей.
	ch := &RulesChange{Base: 1, Version: &domain.ConfigVersion{Version: 2, Source: "agent", Rules: rulesJSON(t, "c1", "b")}}
	if err := r.ApplyRulesChange(ctx, ch); err != nil {
		t.Fatal(err)
	}
	if ch.Version.Kind != domain.ConfigKindRules {
		t.Fatalf("пустой kind не стал rules: %q", ch.Version.Kind)
	}
}

func TestCatalogP2RoundTrip(t *testing.T) {
	r := testSchemaRepos(t)
	ctx := t.Context()
	cal := "ws-cal"
	snap := &domain.CatalogSnapshot{Version: "p2-test", Synthetic: []string{"margin", "orders"},
		Products: []domain.Product{
			{ID: "ws", FamilyID: "ws", Title: "Windows Server", Kind: domain.KindSoftware, PriceMode: domain.PriceFixed},
			{ID: "ram", FamilyID: "ram", Title: "RAM 32G", Kind: domain.KindHardware, MPN: "SNS-R740-32G",
				Specs: map[string]float64{"ram_gb": 32}, CompatibleWith: []string{"r740"}, PriceMode: domain.PriceQuoteRequired},
		},
		Relations: []domain.Relation{{RelID: "rel-1", FromFamily: "ws", ToFamily: "ws-cal", ToProduct: &cal, Type: domain.RelationUpgrade,
			Requirement: domain.RequirementConditional, Compatibility: domain.CompatConditional, Conditions: []string{"x"},
			UpgradeAdds: []string{"Hyper-V"}, EvidenceLevel: "E2"}},
		LicenseRules: []domain.LicenseRule{{ID: "lr-1", AnchorFamily: "ws", AnchorAttrs: map[string]string{"edition": "Standard"},
			Class: domain.RequirementConditional, Conditions: []string{"access_license_required"}}},
		CompatibilityRules: []domain.CompatibilityRule{{ID: "cr-1", ProductID: "ram", TargetModel: "r740", State: domain.CompatVerified, RuleVersion: 1}},
		AttributeDefinitions: []domain.AttributeDefinition{{CategoryID: "mem", Attrs: []domain.AttributeSpec{
			{Key: "ram_gb", Label: "Память", Unit: "ГБ", Type: "number", Facet: true, UnitAliases: []string{"гб"}}}}},
	}
	for range 2 { // повторный импорт идемпотентен
		if err := r.ReplaceCatalog(ctx, snap, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	got, err := r.LoadCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]domain.Product{}
	for _, p := range got.Products {
		byID[p.ID] = p
	}
	ram := byID["ram"]
	if ram.Kind != domain.KindHardware || ram.MPN != "SNS-R740-32G" || ram.Specs["ram_gb"] != 32 || len(ram.CompatibleWith) != 1 ||
		ram.PriceMode != domain.PriceQuoteRequired || byID["ws"].Kind != domain.KindSoftware {
		t.Fatalf("позиции: %+v", got.Products)
	}
	rel := got.Relations[0]
	if len(got.Relations) != 1 || rel.RelID != "rel-1" || rel.ToProduct == nil || *rel.ToProduct != cal || rel.Type != domain.RelationUpgrade ||
		len(rel.Conditions) != 1 || len(rel.UpgradeAdds) != 1 || rel.EvidenceLevel != "E2" {
		t.Fatalf("связь: %+v", got.Relations)
	}
	if len(got.LicenseRules) != 1 || got.LicenseRules[0].AnchorAttrs["edition"] != "Standard" ||
		len(got.CompatibilityRules) != 1 || len(got.AttributeDefinitions) != 1 || got.AttributeDefinitions[0].Attrs[0].Key != "ram_gb" ||
		len(got.Synthetic) != 2 {
		t.Fatalf("справочники: %+v %+v %+v %v", got.LicenseRules, got.CompatibilityRules, got.AttributeDefinitions,
			got.Synthetic)
	}
	q := &domain.QuoteRequest{Items: []domain.QuoteItem{{ProductID: "ram", Quantity: 2, Origin: "search"}}, Status: domain.QuoteNew,
		Positions: 1, OnRequest: 1}
	if err := r.db.Create(q).Error; err != nil {
		t.Fatalf("запрос КП: %v", err)
	}
}
