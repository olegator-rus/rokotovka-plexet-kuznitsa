package catalogsource

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestGenericOrders — заказы контура коммерции: номер и аккаунт из данных, аккаунт из client_id, номер восстановлен по
// покупателю и окну, гость без сессии — без заказа; размеченные пары блока из CSV.
func TestGenericOrders(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("price.csv", "Код;Наименование;Артикул\na1;Сервер X, 2 CPU;SRV-X\na2;ИБП Y 3000;UPS-Y\na3;Кабель Z;CAB-Z\n")
	write("ev.csv", "Когда;Что;Клиент;Сессия;Товар;Заказ;Аккаунт;Кол-во\n"+
		"12.05.2026 10:00;Заказ;;s1;SRV-X;o-1;acc-1;2\n"+
		"12.05.2026 10:00;Заказ;;s1;UPS-Y;o-1;acc-1;1\n"+
		"13.05.2026 10:00;Заказ;c-2;s2;SRV-X;;;\n"+ // нет номера и аккаунта: аккаунт = клиент, номер восстановлен
		"13.05.2026 10:10;Заказ;c-2;s3;UPS-Y;;;1\n"+ // тот же клиент в окне 30 мин — тот же заказ
		"13.05.2026 12:00;Заказ;c-2;s4;CAB-Z;;;\n"+ // за окном — новый заказ
		"13.05.2026 13:00;Заказ;;s5;CAB-Z;;;\n"+ // только сессия: заказ восстановлен, аккаунта нет
		"13.05.2026 14:00;Заказ;;;CAB-Z;;;\n"+ // гость без сессии — без заказа
		"13.05.2026 14:00;Клик;;s5;CAB-Z;;;\n")
	write("pairs.csv", "Якорь;Кандидат;Класс\nSRV-X;UPS-Y;Сопутствующий\na1;a3;несовместим\na1;nope;обязательный\na1;a2;???\n")
	write("mapping.yaml", `
catalog:
  file: price.csv
  columns: {id: Код, sku: Артикул, title: Наименование}
events:
  file: ev.csv
  columns: {ts: Когда, type: Что, client_id: Клиент, session_id: Сессия, product: Товар, order_id: Заказ,
            account_id: Аккаунт, quantity: Кол-во}
companion_pairs:
  file: pairs.csv
  columns: {anchor_product: Якорь, candidate_product: Кандидат, class: Класс}
`)
	g, err := NewGeneric(filepath.Join(dir, "mapping.yaml"), nil)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := g.LoadEvents(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		order, account string
		qty            int
	}
	var got []row
	for _, e := range evs {
		if e.Type == "order" {
			got = append(got, row{e.OrderID, e.AccountID, e.Quantity})
		}
	}
	want := []row{{"o-1", "acc-1", 2}, {"o-1", "acc-1", 1}, {"rst-00001", "c-2", 0}, {"rst-00001", "c-2", 1},
		{"rst-00002", "c-2", 0}, {"rst-00003", "", 0}, {"", "", 0}}
	if len(got) != len(want) {
		t.Fatalf("заказы: %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("строка %d: %+v, ожидалось %+v", i, got[i], want[i])
		}
	}
	if !IsRestoredOrder(got[2].order) || IsRestoredOrder(got[0].order) {
		t.Fatal("пометка восстановленного номера")
	}
	prov, _ := g.Provenance(t.Context())
	status := map[string]FieldReport{}
	for _, p := range prov {
		status[p.Field] = p
	}
	if r := status[FieldOrderID]; r.Status != FieldPartial || r.FromSource != 2 || r.Restored != 4 || r.Note == "" {
		t.Fatalf("order_id: %+v", r)
	}
	if r := status[FieldAccountID]; r.FromSource != 2 || r.Restored != 3 {
		t.Fatalf("account_id: %+v", r)
	}
	pairs, err := g.LoadCompanionPairs(t.Context())
	if err != nil || len(pairs) != 2 || pairs[0].AnchorProduct != "a1" || pairs[0].CandidateProduct != "a2" ||
		pairs[0].Class != "complement" || pairs[1].Class != "incompatible" {
		t.Fatalf("пары: %+v %v", pairs, err)
	}
	files, _ := g.Files(t.Context())
	for _, f := range files {
		if f.Table == "companion_pairs" && f.Skipped != 2 {
			t.Fatalf("пропуски пар: %+v", f)
		}
	}
}

// TestGenericNoPairs — без companion_pairs в mapping — ErrNoFile (оценка пишет «нет разметки», а не ноль).
func TestGenericNoPairs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "p.csv"), []byte("t\nX\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "m.yaml"), []byte("catalog:\n  file: p.csv\n  columns: {title: t}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := NewGeneric(filepath.Join(dir, "m.yaml"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.LoadCompanionPairs(t.Context()); !errors.Is(err, ErrNoFile) {
		t.Fatalf("ожидался ErrNoFile, получено %v", err)
	}
}
