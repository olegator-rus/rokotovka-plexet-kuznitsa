package catalogsource

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
)

func TestParsePrice(t *testing.T) {
	cases := map[string]struct {
		v     float64
		onReq bool
		ok    bool
	}{
		"12 990,00 ₽": {12990, false, true}, "12990.5": {12990.5, false, true}, "1.234.567": {1234567, false, true},
		"$1,299.00": {1299, false, true}, "1 299 руб.": {1299, false, true}, "по запросу": {0, true, true},
		"0": {0, true, true}, "": {0, false, false}, "—": {0, false, false}, "12,99": {12.99, false, true},
	}
	for in, want := range cases {
		v, onReq, ok := parsePrice(in)
		if v != want.v || onReq != want.onReq || ok != want.ok {
			t.Errorf("%q → %v %v %v, ожидалось %+v", in, v, onReq, ok, want)
		}
	}
}

func TestParseValues(t *testing.T) {
	for in, want := range map[string]bool{"да": true, "Нет": false, "TRUE": true, "0": false, "+": true} {
		if v, ok := parseBool(in); !ok || v != want {
			t.Errorf("bool %q → %v %v", in, v, ok)
		}
	}
	tp, err := newTimeParser(nil, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range []string{"12.05.2026 15:03:11", "2026-05-12T12:03:11Z", "2026-05-12 15:03:11", "1778587391"} {
		ts, ok := tp.parse(in)
		if !ok || ts.Format("2006-01-02T15:04") != "2026-05-12T12:03" {
			t.Errorf("дата %q → %v %v", in, ts, ok)
		}
	}
	if d, ok := parseDurationMonths("2 года"); !ok || d == nil || *d != 24 {
		t.Errorf("срок 2 года → %v", d)
	}
	if d, ok := parseDurationMonths("бессрочно"); !ok || d != nil {
		t.Error("бессрочно — nil")
	}
	if s := Slug("МойОфис Стандартный 2.0"); s != "moyofis-standartnyy-2-0" {
		t.Errorf("slug: %q", s)
	}
}

// TestGenericSmall — CSV в cp1251 с «;», вложенный JSON, путь категорий, справочник значений.
func TestGenericSmall(t *testing.T) {
	dir := t.TempDir()
	csvText := "Код;Наименование;Бренд;Раздел;Цена;Лиц.\r\n" +
		"a1;Kaspersky Plus, 3 устройства, 1 год;Лаборатория Касперского;Безопасность > Антивирусы;2 990,00 руб.;подписка\r\n" +
		"a2;МойОфис Стандартный, 25 пользователей;МойОфис;Офис;по запросу;бессрочная\r\n" +
		"a3;Без названия;;;;\r\n" +
		";;;;;\r\n"
	raw, err := charmap.Windows1251.NewEncoder().Bytes([]byte(csvText))
	if err != nil {
		t.Fatal(err)
	}
	write := func(name string, b []byte) {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("price.csv", raw)
	write("vendors.json", []byte(`{"data":{"items":[{"name":"Лаборатория Касперского","meta":{"country":"Россия"}},{"name":"МойОфис","meta":{"country":"РФ"}}]}}`))
	write("ev.jsonl", []byte(`{"when":"12.05.2026 15:03","act":"Поиск","q":"касперский"}`+"\n"+`{"when":"12.05.2026 15:04","act":"Клик","sku":"a1","pos":"1"}`+"\n"))
	write("mapping.yaml", []byte(`
version: small-1
catalog:
  file: price.csv
  columns: {id: Код, title: Наименование, vendor: Бренд, category: Раздел, price_rub: Цена, license_type: Лиц.}
vendors:
  file: vendors.json
  json_path: data.items
  columns: {name: name, origin: meta.country}
events:
  file: ev.jsonl
  columns: {ts: when, type: act, query: q, product: sku, position: pos}
`))
	g, err := NewGeneric(filepath.Join(dir, "mapping.yaml"), nil)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := g.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if snap.Version != "small-1" || len(snap.Products) != 3 || len(snap.Vendors) != 2 || len(snap.Categories) != 3 {
		t.Fatalf("снимок: %d позиций, %d вендоров, %d категорий", len(snap.Products), len(snap.Vendors), len(snap.Categories))
	}
	p := snap.Products[0]
	if p.PriceRub == nil || *p.PriceRub != 2990 || p.License.Type != "subscription" || p.Origin != "ru" ||
		p.VendorID != "laboratoriya-kasperskogo" || p.CategoryID != "bezopasnost-antivirusy" {
		t.Fatalf("позиция: %+v", p)
	}
	if !snap.Products[1].PriceOnRequest || snap.Products[1].License.Type != "perpetual" {
		t.Fatalf("цена по запросу: %+v", snap.Products[1])
	}
	if c := snap.Categories; c[1].ParentID == nil || *c[1].ParentID != c[0].ID {
		t.Fatalf("путь категорий: %+v", c)
	}
	evs, err := g.LoadEvents(t.Context())
	if err != nil || len(evs) != 2 || evs[1].ProductID != "a1" || evs[1].Position == nil || evs[0].SessionID == "" {
		t.Fatalf("события: %+v %v", evs, err)
	}
	files, _ := g.Files(t.Context())
	if !strings.Contains(files[1].Detected, "cp1251") || !strings.Contains(files[1].Detected, ";") {
		t.Fatalf("кодировка и разделитель: %+v", files)
	}
	if _, err := g.LoadQueries(t.Context()); err != ErrNoFile {
		t.Fatalf("без eval_queries — ErrNoFile, получено %v", err)
	}
}

func TestMappingRejectsTypos(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "m.yaml")
	if err := os.WriteFile(path, []byte("catalog:\n  file: x.csv\n  columns: {titel: Название}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadMapping(path)
	if err == nil || !strings.Contains(err.Error(), `неизвестное поле "titel"`) || !strings.Contains(err.Error(), `нужна колонка "title"`) {
		t.Fatalf("ошибка: %v", err)
	}
}

// TestMappingExample — пример в data/ разбирается: в нём нет опечаток в полях.
func TestMappingExample(t *testing.T) {
	if _, err := LoadMapping("../../../data/mapping.example.yaml"); err != nil {
		t.Fatal(err)
	}
}
