package services

import (
	"encoding/json"
	"strings"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/externalApi/catalogsource"
	"kuznica/api/in"
	"kuznica/api/out"
)

// Шаблон по триггерам на data/mock: каждый запрос p2 с expect.template_id получает этот шаблон, запросы контура поиска
// и остальные запросы p2 — ни одного (комплект не навязывается обычному поиску).
func TestMatchTemplateMockSets(t *testing.T) {
	const dir = "../../data/mock"
	snap, err := catalogsource.MockJSON{Dir: dir}.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.SolutionTemplates) == 0 {
		t.Skip("data/mock контура коммерции нет")
	}
	qs, err := catalogsource.MockJSON{Dir: dir}.LoadQueries(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	hits, want := 0, 0
	for _, q := range qs {
		var e struct {
			TemplateID string `json:"template_id"`
		}
		if len(q.Expect) > 0 {
			_ = json.Unmarshal(q.Expect, &e)
		}
		got := ""
		if m := matchTemplate(snap.SolutionTemplates, q.Text); m != nil {
			got = m.tmpl.ID
		}
		if e.TemplateID != "" {
			want++
		}
		if got != e.TemplateID {
			t.Errorf("%s %s %q: шаблон %q, ожидался %q", q.Set, q.ID, q.Text, got, e.TemplateID)
		} else if got != "" {
			hits++
		}
	}
	if want == 0 || hits != want {
		t.Fatalf("шаблон распознан у %d из %d запросов с expect.template_id", hits, want)
	}
}

func TestSolutionSeats(t *testing.T) {
	for _, c := range []struct {
		q          string
		lic, hw, n int
		src        string
	}{
		{"рабочее место для 20 сотрудников", 0, 0, 20, "query"},
		{"оснастить офис на 10 человек ноутбуками", 0, 0, 10, "query"},
		{"филиал на 15 рабочих мест", 0, 0, 15, "query"},
		{"сервер под 1с на 50 пользователей", 50, 0, 50, "query"},
		{"рабочее место бухгалтера", 0, 0, 1, "default"},
		{"удаленный офис", 0, 7, 7, "query"},
	} {
		if n, src := solutionSeats(c.q, c.lic, c.hw); n != c.n || src != c.src {
			t.Errorf("%q: %d (%s), ожидалось %d (%s)", c.q, n, src, c.n, c.src)
		}
	}
}

// Фасовка на N мест — дешевле всего внутри семейства, редакции и метрики; ноутбук — N штук.
func TestFitPackMock(t *testing.T) {
	_, st, _ := hwEnv(t, "../../data/mock", true)
	lx := st.lex
	if _, ok := lx.docs["kes-std10"]; !ok {
		t.Skip("data/mock контура коммерции нет")
	}
	all := func(indexDoc) bool { return true }
	for _, c := range []struct {
		head, strategy string
		seats          int
		want           string
		qty            int
	}{
		{"kes-std25", "per_seat", 20, "kes-std10", 2},    // 2 × 10 узлов дешевле 25 узлов
		{"1ccl-5", "per_user", 50, "1ccl-50", 1},         // ровно 50 мест — одна позиция
		{"wscal25-u1", "per_user", 50, "wscal25-u5", 10}, // пакет на 5 дешевле 50 штук по 1, Device CAL не берётся
		{"m365b-bs", "per_seat", 20, "m365b-bs", 20},     // поштучно
		{"nb-mbd16-i5-16-512", "per_seat", 20, "nb-mbd16-i5-16-512", 20},
		{"ws25-std16", "per_core", 50, "ws25-std16", 1},
	} {
		d, q, _ := fitPack(lx, lx.docs[c.head], c.strategy, c.seats, all)
		if d.ProductID != c.want || q != c.qty {
			t.Errorf("%s × %d мест (%s): %s × %d, ожидалось %s × %d", c.head, c.seats, c.strategy, d.ProductID, q, c.want, c.qty)
		}
	}
}

// Старшая редакция: связь upgrade от позиции — карточка с 1–2 строками «что добавится»; к самой старшей — нет.
func TestUpgradeForMock(t *testing.T) {
	_, st, _ := hwEnv(t, "../../data/mock", true)
	cc := st.commerce
	if cc == nil || cc.products["ws25-std16"] == nil {
		t.Skip("data/mock контура коммерции нет")
	}
	up, rel, _ := upgradeFor(cc, st.lex, "ws25-std16")
	if up == nil || up.ProductID != "ws25-dc16" || rel == nil || len(up.UpgradeAdds) == 0 || len(up.UpgradeAdds) > 2 ||
		up.Label != labelUpgrade || up.Class != domain.RelationUpgrade || up.ForProductID != "ws25-std16" {
		t.Fatalf("ws25-std16 → Datacenter: %+v", up)
	}
	if up, _, reason := upgradeFor(cc, st.lex, "ws25-dc16"); up != nil || reason != upgradeNoRel {
		t.Fatalf("к Datacenter старшей редакции нет: %+v %s", up, reason)
	}
	if up, _, _ := upgradeFor(cc, st.lex, "kes-std25"); up != nil {
		t.Fatalf("связь rel-0076 уточнена до kes-std10 — к kes-std25 карточки нет: %+v", up)
	}
}

// Поиск на фикстуре: шаблон «сервер под ключ» — решение из двух слотов, выдача та же, что без флага; флаг выключен —
// solution и upgrade null, шагов нет.
func TestSolutionSearchFixture(t *testing.T) {
	on := importedWith(t, Features{Hardware: true, Companions: true, Solutions: true})
	off := importedWith(t, Features{Hardware: true, Companions: true})
	const q = "сервер под ключ на 3 сервера"
	a, err := on.Search(t.Context(), "r", in.SearchRequest{Query: q, Config: "B3"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := off.Search(t.Context(), "r", in.SearchRequest{Query: q, Config: "B3"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Solution == nil || a.Solution.TemplateID != "tpl-server" || len(a.Solution.Slots) != 2 {
		t.Fatalf("решение по шаблону: %+v", a.Solution)
	}
	for _, sl := range a.Solution.Slots {
		if sl.Item == nil || !sl.Active || sl.Quantity != 1 {
			t.Errorf("слот %s: %+v", sl.ID, sl)
		}
	}
	if b.Solution != nil || b.Upgrade != nil {
		t.Fatalf("флаг выключен — решения нет: %+v", b.Solution)
	}
	if ids(a) != ids(b) {
		t.Fatalf("выдача от шаблона изменилась:\n%s\n%s", ids(a), ids(b))
	}
	chip := false
	for _, c := range a.Chips {
		chip = chip || c.Field == FieldSolution
	}
	if !chip {
		t.Fatalf("нет чипа «Комплексная закупка»: %+v", a.Chips)
	}
	steps := map[string]bool{}
	for _, s := range a.Trace.Steps {
		raw, _ := json.Marshal(s)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		steps[m["name"].(string)] = true
	}
	if !steps[StepSolution] || !steps[StepUpgrade] {
		t.Fatalf("шаги solution и upgrade: %v", steps)
	}
	// Снятый чип выключает шаблон.
	c, err := on.Search(t.Context(), "r", in.SearchRequest{Query: q, Config: "B3",
		RemovedChips: []in.RemovedChip{{Field: FieldSolution, Value: "tpl-server"}}})
	if err != nil {
		t.Fatal(err)
	}
	if c.Solution != nil {
		t.Fatalf("чип снят — решения нет: %+v", c.Solution)
	}
}

func ids(r *out.SearchResponse) string {
	var xs []string
	for _, it := range r.Items {
		xs = append(xs, it.ProductID)
	}
	return strings.Join(xs, ",")
}

// GET /api/companions: компаньоны к позиции спецификации; неизвестная позиция — 404, без id — 422.
func TestProductCompanionsFixture(t *testing.T) {
	svc := importedWith(t, Features{Companions: true, Solutions: true})
	r, err := svc.ProductCompanions(t.Context(), "ws2025-std-16core", 0)
	if err != nil {
		t.Fatal(err)
	}
	if r.ProductID != "ws2025-std-16core" || r.Required == nil || r.Items == nil ||
		len(r.Required)+len(r.Items) == 0 {
		t.Fatalf("к серверной ОС есть обязательные или дополнения: %+v", r)
	}
	if _, err := svc.ProductCompanions(t.Context(), "no-such", 0); err == nil || !strings.Contains(err.Error(), "Позиция") {
		t.Fatalf("неизвестная позиция — 404: %v", err)
	}
	if _, err := svc.ProductCompanions(t.Context(), "", 0); err == nil {
		t.Fatal("без product_id — 422")
	}
	off := importedWith(t, Features{})
	r, err = off.ProductCompanions(t.Context(), "ws2025-std-16core", 0)
	if err != nil || len(r.Required)+len(r.Items) != 0 || r.Upgrade != nil {
		t.Fatalf("флаг выключен — пусто: %+v %v", r, err)
	}
}
