package services

import (
	"context"
	"strings"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
)

// Экран подбора контура экрана: готовые поля на маленьком каталоге — сценарии 2 и 3 из SCENARIOS.md.

func screenRow(tbl *out.GuideScreenTable, key string) *out.GuideScreenRow {
	for i := range tbl.Rows {
		if tbl.Rows[i].Key == key {
			return &tbl.Rows[i]
		}
	}
	return nil
}

// Сценарий 2: сравнение на 30 ПК без требований → меньшая стоимость (Стандартный, 84 402 ₽); затем 50 ПК и
// шифрование → Расширенный по требуемой функции, 197 010 ₽, смена ступени 4 081,50 → 3 940,20 ₽.
func TestGuideScreenScenario2(t *testing.T) {
	s := guideTestServices(t, kbVerified, nil, llmModeOff)
	q := "защита для компьютеров в офисе на 30 ПК, чем расширенный отличается от стандартного"
	r1 := guideTurnReq(t, s, in.GuideRequest{Query: q, Message: q})
	sc := r1.Screen
	if sc == nil || sc.Title != "Подбор лицензии" || !strings.HasPrefix(sc.Task, "Антивирус для организации") {
		t.Fatalf("ход 1: экран и задача: %+v", sc)
	}
	if sc.Result == nil || sc.Result.Basis != out.BasisLowestTotal || sc.Result.OfferID != "4985" ||
		sc.Result.Heading != "Меньшая стоимость закупки" {
		t.Fatalf("ход 1: основание — меньшая стоимость, Стандартный: %+v", sc.Result)
	}
	if sc.Result.Reason != "Меньшая стоимость закупки: Стандартный, на 38 043 ₽ дешевле." {
		t.Fatalf("ход 1: разница словами: %q", sc.Result.Reason)
	}
	if sc.Summary == nil || sc.Summary.TotalText != "84 402 ₽" || sc.Summary.Formula != "30 × 2 813,40 ₽" || sc.Summary.Action != "add_to_spec" {
		t.Fatalf("ход 1: итоговая строка: %+v", sc.Summary)
	}
	tbl := sc.Comparison
	if tbl == nil || tbl.Title != "Различия редакций" || len(tbl.Columns) != 2 || tbl.Columns[0].Title != "Стандартный" ||
		tbl.Columns[1].Title != "Расширенный" {
		t.Fatalf("ход 1: колонки: %+v", tbl)
	}
	if len(tbl.Rows) < 3 || len(tbl.Rows) > 6 {
		t.Fatalf("ход 1: на виду 4–6 строк (меньше, если различий меньше): %d", len(tbl.Rows))
	}
	enc := screenRow(tbl, "encryption")
	if enc == nil || enc.Cells[0].Text != "Нет" || enc.Cells[1].Text != "Есть" || enc.Priority != 1 {
		t.Fatalf("ход 1: шифрование — различие: %+v", enc)
	}
	price := screenRow(tbl, "unit_price")
	if price == nil || price.Label != "За компьютер" || price.Cells[0].Text != "2 813,40 ₽" || price.Cells[1].Text != "4 081,50 ₽" {
		t.Fatalf("ход 1: цена за единицу: %+v", price)
	}
	if tbl.Rows[0].Priority > tbl.Rows[len(tbl.Rows)-1].Priority {
		t.Fatalf("ход 1: строки по приоритету: %+v", tbl.Rows)
	}
	o := findOffer(r1, "4856")
	if o.Display == nil || o.Display.Title != "Kaspersky Endpoint Security — Расширенный" ||
		o.Display.Subtitle != "Новая лицензия · 30 компьютеров · 1 год" {
		t.Fatalf("ход 1: заголовок и подзаголовок (6.1): %+v", o.Display)
	}
	if o.SaleUnit != "за компьютер" || o.UnitPrice == nil || *o.UnitPrice != 4081.5 || o.PayQuantity != 30 || o.PriceLine != "30 × 4 081,50 ₽" {
		t.Fatalf("ход 1: единица и расчёт: %+v", o)
	}
	if sc.Changed != nil {
		t.Fatalf("ход 1: «что изменилось» только после смены условий: %+v", sc.Changed)
	}

	st := r1.State
	r2 := guideTurnReq(t, s, in.GuideRequest{Message: "компьютеров будет 50, и шифрование нужно", State: &st})
	sc = r2.Screen
	if sc == nil || sc.Result == nil || sc.Result.Basis != out.BasisRequiredFeature || sc.Result.OfferID != "4857" ||
		sc.Result.Heading != "Подходит под ваши условия" || sc.Result.Source == nil || sc.Result.Source.Label != "Документация вендора" {
		t.Fatalf("ход 2: основание — требуемая функция: %+v", sc)
	}
	if sc.Changed == nil || sc.Changed.Text != "Изменено: 30 → 50 компьютеров, шифрование обязательно" {
		t.Fatalf("ход 2: что изменилось: %+v", sc.Changed)
	}
	pc := sc.Changed.PriceChange
	if pc == nil || pc.From.Text != "4 081,50 ₽" || pc.To.Text != "3 940,20 ₽" || pc.From.Tier != "25–49 компьютеров" ||
		pc.To.Tier != "50–99 компьютеров" || pc.Label != "Цена за компьютер изменилась" {
		t.Fatalf("ход 2: смена ступени: %+v", pc)
	}
	if sc.Summary == nil || sc.Summary.TotalText != "197 010 ₽" || sc.Summary.Formula != "50 × 3 940,20 ₽" {
		t.Fatalf("ход 2: итог: %+v", sc.Summary)
	}
	if len(sc.Others) != 1 || sc.Others[0].OfferID != "4986" || sc.Others[0].Reason == nil ||
		sc.Others[0].Reason.Code != "missing_feature" || sc.Others[0].Reason.Text != "Не подходит: нет шифрования данных" ||
		sc.Others[0].PriceText != "2 715,30 ₽ за компьютер" {
		t.Fatalf("ход 2: Стандартный — с причиной: %+v", sc.Others)
	}
	if sc.Comparison == nil || len(sc.Comparison.Rows) == 0 || sc.Comparison.Rows[0].Key != "encryption" || !sc.Comparison.Rows[0].Asked {
		t.Fatalf("ход 2: строка о названной функции — первой: %+v", sc.Comparison)
	}
}

// Сценарий 3: самый дешёвый CSP для сервера → серверная 70 000 ₽ единственная допустимая; рабочие места — с причиной;
// лицензия для TLS-сервера — в «При изменении условий».
func TestGuideScreenScenario3(t *testing.T) {
	s := guideTestServices(t, kbVerified, nil, llmModeOff)
	q := "самый дешёвый криптопро csp для сервера"
	r := guideTurnReq(t, s, in.GuideRequest{Query: q, Message: q})
	sc := r.Screen
	if sc == nil || sc.Result == nil || sc.Result.Basis != out.BasisOnlyValid || sc.Result.OfferID != "197958" ||
		sc.Result.Heading != "Подходящая лицензия" {
		t.Fatalf("основание — единственная допустимая: %+v", sc)
	}
	if sc.Result.Source == nil || sc.Result.Source.Label != "Правила лицензирования КриптоПро" {
		t.Fatalf("источник правила: %+v", sc.Result.Source)
	}
	if sc.Task != "КриптоПро CSP 5.0 для серверной ОС" {
		t.Fatalf("задача: %q", sc.Task)
	}
	m := findOffer(r, "197958")
	if m.Display.Title != "КриптоПро CSP 5.0" || m.Display.Subtitle != "Серверная лицензия · бессрочная" {
		t.Fatalf("заголовок серверной: %+v", m.Display)
	}
	if sc.Summary == nil || sc.Summary.TotalText != "70 000 ₽" || sc.Summary.Formula != "1 × 70 000 ₽" {
		t.Fatalf("итог: %+v", sc.Summary)
	}
	reasons := map[string]string{}
	for _, o := range sc.Others {
		if o.Reason != nil {
			reasons[o.OfferID] = o.Reason.Code + "|" + o.Reason.Text
		}
	}
	for _, id := range []string{"411620", "410137"} {
		if reasons[id] != "not_for_server_os|Не подходит для серверной ОС" {
			t.Fatalf("%s — «Не подходит для серверной ОС»: %+v", id, reasons)
		}
	}
	if reasons["385290"] != "tls_server_only|Отдельная лицензия для TLS-сервера" {
		t.Fatalf("TLS-сервер — отдельная лицензия: %+v", reasons)
	}
	if ws := findOffer(r, "411620"); ws.Display.Subtitle != "Рабочее место · 1 год" || ws.PriceText != "1 850 ₽ за рабочее место" {
		t.Fatalf("рабочее место: %+v", ws)
	}
	if sc.Comparison != nil {
		t.Fatalf("одни цены без функций — не таблица: %+v", sc.Comparison)
	}
}

// Пакет на N мест: стоимость всей задачи, места в запасе, цена пакета не делится.
func TestGuideScreenPackage(t *testing.T) {
	s := guideTestServices(t, kbVerified, nil, llmModeOff)
	g := s.guide()
	p := guideProduct("9001", "9000", "Dr.Web Desktop Security Suite", "drweb", "Dr.Web Комплексная защита на 1 год на 25 ПК", 40000, "base", gi(12), nil, nil)
	g.fixedCatalog.products[p.ID] = p
	g.fixedCatalog = newGuideCatalog("test-feed", g.fixedCatalog.products)
	size := 25
	g.kb.kb.marks[p.ID] = guideOfferMark{OfferID: p.ID, SaleUnit: "package", PackageSize: &size}
	tt := &guideTurn{g: g, cat: g.fixedCatalog, kb: g.kb.kb, st: &domain.GuideState{Requirements: domain.GuideRequirements{
		Objects: []domain.GuideObject{{Kind: "workstation", Count: 20}}}}}
	o := tt.offerOut(tt.cat.view(p, tt.kb), 20, "workstation", out.OfferMatch, "")
	tt.decorateOffer(&o)
	if o.SaleUnit != "пакет на 25 компьютеров" || o.SaleUnitKind != "package" || o.PayQuantity != 1 || *o.UnitPrice != 40000 {
		t.Fatalf("пакет: %+v", o)
	}
	if o.Package == nil || o.Package.Spare != 5 || o.Package.Text != "Пакет на 25 компьютеров: 5 мест в запасе" {
		t.Fatalf("запас: %+v", o.Package)
	}
	if o.Display.Subtitle != "Новая лицензия · пакет на 25 компьютеров · 1 год" || o.PriceLine != "1 × 40 000 ₽" {
		t.Fatalf("подзаголовок и расчёт: %+v %q", o.Display, o.PriceLine)
	}
}

// Признак «раскрыть подбор сразу» (UI §2).
func TestGuideRouteOpenNow(t *testing.T) {
	s := &Services{}
	cases := map[string]string{
		"подбери антивирь":                           "ask",
		"что взять для офиса":                        "ask",
		"чем расширенный отличается от стандартного": "compare",
		"криптопро для сервера":                      "use_limit",
		"антивирус":                                  "",
		"криптопро csp 5.0":                          "",
		"KL4867RAQFS":                                "",
		"продлить касперского на 100 пк на 2 года":   "",
	}
	for q, want := range cases {
		r := s.RouteGuide(context.Background(), q)
		if r.OpenReason != want || r.OpenNow != (want != "") || (r.OpenNow && r.Mode != routeGuide) {
			t.Errorf("%q: %+v, ждали %q", q, r, want)
		}
	}
}

// Цитата источника — без дат.
func TestGuideScreenCleanFragment(t *testing.T) {
	got := cleanFragment("Шифрование доступно (© ООО «Вендор», 2026) — от 03.08.2025 по данным 2025 года")
	if strings.ContainsAny(got, "0123456789") || !strings.Contains(got, "Шифрование доступно") {
		t.Fatalf("даты в цитате: %q", got)
	}
}
