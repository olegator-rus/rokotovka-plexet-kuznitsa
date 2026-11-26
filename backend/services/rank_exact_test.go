package services

import (
	"testing"

	"kuznica/api/domain"
)

// Точные запросы первыми (хвосты контура доводки, regression «fqc10572»): артикул без дефиса и в нижнем регистре
// восстанавливается по каталогу — дальше его находит точное поле sku и маршрут A. Слово без цифр и чужой код — нет.
func TestRestoreSKUs(t *testing.T) {
	snap, err := fixtureSource().Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	lx := newSearchLexicon(snap)
	nr := normalizeFull("t5d03511", lx, nil, false)
	nr.restoreSKUs(lx)
	if nr.normalized() != "t5d-03511" || len(nr.changes) == 0 || nr.changes[len(nr.changes)-1].Kind != changeSKU {
		t.Fatalf("артикул без дефиса: %q, изменения %+v", nr.normalized(), nr.changes)
	}
	if r, _ := routeSignals(nr, extractAttrs(nr.tokens), findEntities(nr, extractAttrs(nr.tokens), lx, nil)); r != domain.RouteExact {
		t.Fatalf("восстановленный артикул — маршрут A, а не %s", r)
	}
	for _, q := range []string{"windows server", "t5d0351", "zz9-99999"} {
		nr := normalizeFull(q, lx, nil, false)
		before := nr.normalized()
		nr.restoreSKUs(lx)
		if nr.normalized() != before {
			t.Errorf("%q: артикула нет в каталоге, а запрос изменён: %q", q, nr.normalized())
		}
	}
}

// «windows server 2025»: названы два семейства, сервер — целиком (имя вендора можно не называть), у CAL в
// названии есть «cal», которого в запросе нет: сервер на ступень выше. «клиентские лицензии windows server 2025»
// называет CAL словом из её названий — ступени равны, порядок прежний. «windows server cal 2025» — сервер не выше CAL.
// Каталог — data/mock (в фикстуре тестов слишком мало семейств, чтобы CAL считалась названной).
func TestFullNamedFamilyTier(t *testing.T) {
	lx, rules, _ := mockLexicon(t)
	const server, cal = "windows-server-2025", "windows-server-cal-2025"
	tiers := func(q string) (int, int) {
		nr := normalizeFull(q, lx, rules, false)
		attrs := extractAttrs(nr.tokens)
		rc := rankCtx{lx: lx, attrs: attrs, ent: findEntities(nr, attrs, lx, rules)}
		if !contains(rc.ent.named, cal) {
			t.Fatalf("%q: названы %v", q, rc.ent.named)
		}
		s, _ := rc.tierOf(server)
		c, _ := rc.tierOf(cal)
		return s, c
	}
	if s, c := tiers("windows server 2025"); s <= c {
		t.Errorf("windows server 2025: ступень сервера %d, CAL %d — сервер должен быть выше", s, c)
	}
	if s, c := tiers("клиентские лицензии windows server 2025"); s != c {
		t.Errorf("клиентские лицензии …: ступени %d и %d — CAL названа, ступени равны", s, c)
	}
	if s, c := tiers("windows server cal 2025"); s > c {
		t.Errorf("windows server cal 2025: ступени %d и %d — CAL названа, сервер не выше", s, c)
	}
}

// «dr.web для бизнеса»: вендор назван, словами названа подкатегория «для бизнеса» — Enterprise Security Suite
// выше Security Space внутри ступени. Без вендора («защита от утечек информации») правило не действует.
func TestNamedCategoryWithVendor(t *testing.T) {
	lx, rules, _ := mockLexicon(t)
	ctx := func(q string) rankCtx {
		nr := normalizeFull(q, lx, rules, false)
		attrs := extractAttrs(nr.tokens)
		return rankCtx{lx: lx, attrs: attrs, ent: findEntities(nr, attrs, lx, rules)}
	}
	rc := ctx("dr.web для бизнеса")
	if !rc.namedCategory("drweb-enterprise-security-suite") || rc.namedCategory("drweb-security-space") {
		t.Fatalf("dr.web для бизнеса: категория %q (по словам %v), вендоры %v", rc.ent.category, rc.ent.categoryByWords, rc.ent.vendors)
	}
	rc = ctx("защита от утечек информации")
	for _, f := range lx.familyOrder {
		if rc.namedCategory(f) {
			t.Fatalf("без вендора подкатегория не поднимает семейства: %s", f)
		}
	}
}
