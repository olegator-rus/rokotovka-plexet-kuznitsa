package services

import (
	"testing"

	"kuznica/api/domain"
	"kuznica/api/externalApi/catalogsource"
	"kuznica/api/out"
)

// Количество для компаньона срабатывает только у названного семейства с правилом лицензии per_user/per_device,
// когда сам компаньон не назван; запросы контура поиска (test, tune, regression, demo) шаг не трогает.
func TestCompanionQuantityMockSets(t *testing.T) {
	const dir = "../../data/mock"
	svc, st, snap := hwEnv(t, dir, true)
	if st.commerce == nil || len(st.commerce.licenseByAnchor) == 0 {
		t.Skip("data/mock контура коммерции нет")
	}
	qs, err := catalogsource.MockJSON{Dir: dir}.LoadQueries(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"windows server 2025 standard на 50 пользователей": "windows-server-2025",
		"windows server 2025 на 50 устройств":              "windows-server-2025",
		"windows server 2022 standard на 20 пользователей": "windows-server-2022",
	}
	for _, q := range qs {
		_, attrs, ent := hwParse(svc, st, snap, q.Text)
		cq := companionQuantityFor(st.commerce, st.lex, attrs, ent)
		got := ""
		if cq != nil {
			got = cq.anchor
		}
		if got != want[q.Text] {
			t.Errorf("%s %q: якорь количества %q, ожидался %q", q.Set, q.Text, got, want[q.Text])
		}
	}
}

// Главный демо-запрос F16/F17 на data/mock: обязательное CAL 2025 × 50, CAL 2022 отвергнута правилом версии,
// блок «Дополните решение» — резервное копирование и защита сервера.
func TestCompanionQuantityMainQuery(t *testing.T) {
	const dir = "../../data/mock"
	svc, st, snap := hwEnv(t, dir, true)
	cc := st.commerce
	if cc == nil || len(cc.licenseByAnchor["windows-server-2025"]) == 0 {
		t.Skip("data/mock контура коммерции нет")
	}
	_, attrs, ent := hwParse(svc, st, snap, "windows server 2025 standard на 50 пользователей")
	cq := companionQuantityFor(cc, st.lex, attrs, ent)
	if cq == nil {
		t.Fatal("количество не отнесено к CAL")
	}
	rc := rankCtx{lx: st.lex, attrs: attrs, ent: ent}
	cq.apply(&rc)
	if rc.attrs.quantity != 0 || rc.attrs.metric != "" {
		t.Fatalf("количество осталось условием выдачи: %+v", rc.attrs)
	}

	server := &familyHit{id: "windows-server-2025", group: domain.GroupExact, tier: 2,
		heads: []indexDoc{st.lex.docs["ws25-std16"]}}
	cal25 := &familyHit{id: "windows-server-cal-2025", group: domain.GroupExact, tier: 2,
		heads: []indexDoc{st.lex.docs["wscal25-u1"]}}
	cal22 := &familyHit{id: "windows-server-cal", group: domain.GroupExact, tier: 2,
		heads: []indexDoc{st.lex.docs["wscal-u1"]}}
	cq.regroup([]*familyHit{cal25, cal22, server})
	if cal22.group != domain.GroupMatch || cal25.group != domain.GroupExact {
		t.Fatalf("группы: CAL 2022 %s, CAL 2025 %s", cal22.group, cal25.group)
	}
	if cal25.tier >= server.tier || cal22.tier >= server.tier {
		t.Fatalf("CAL выше сервера: %d %d против %d", cal25.tier, cal22.tier, server.tier)
	}

	res := cc.companionsFor(st.lex, &out.CompanionAnchor{ProductID: "ws25-std16", FamilyID: "windows-server-2025",
		Confidence: "high"}, companionQuery{quantity: attrs.quantity, metric: attrs.metric})
	if len(res.required) == 0 || res.required[0].FamilyID != "windows-server-cal-2025" ||
		res.required[0].SuggestedQuantity == nil || *res.required[0].SuggestedQuantity != 50 {
		t.Fatalf("обязательное: %+v", res.required)
	}
	if res.block == nil || len(res.block.Items) < 2 {
		t.Fatalf("блок «Дополните решение» пуст: %+v", res.block)
	}
	for _, x := range res.block.Items {
		if x.FamilyID == "windows-server-cal" {
			t.Fatalf("CAL 2022 в блоке: %+v", x)
		}
	}
}
