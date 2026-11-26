package services

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/repositories"
)

func TestLoadFlowAndSessions(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "flow.jsonl")
	body := `{"id":"c-001","query":"astra linux","layer":"customer","kind":"customer","origin":"эталон заказчика №1","freq":3,"expect_families":["121231"]}
{"id":"s-002","query":"доставка","layer":"site","kind":"site","origin":"прогон сайта №29","freq":2,"expect_families":[]}

{"id":"y-003","query":"касперкий","layer":"synthetic","kind":"typo","origin":"опечатка","freq":1,"expect_families":["4069"],"fix":"касперский"}
`
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	rows, err := LoadFlow(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[2].Fix != "касперский" {
		t.Fatalf("строки потока: %+v", rows)
	}
	ses := FlowSessions(rows, rand.New(rand.NewPCG(1, 2)))
	if len(ses) != 6 {
		t.Fatalf("сессий %d, ожидалось 3+2+1", len(ses))
	}
	n := map[string]int{}
	for _, s := range ses {
		if len(s.Steps) != 1 || !s.Steps[0].Flow || s.Steps[0].Kind != TrafficSearch {
			t.Fatalf("сессия потока — один поиск: %+v", s)
		}
		n[s.Flow]++
	}
	if n["c-001"] != 3 || n["s-002"] != 2 || n["y-003"] != 1 {
		t.Fatalf("частоты строк не соблюдены: %v", n)
	}

	for _, bad := range []string{
		`{"id":"x","query":"","layer":"customer","freq":1}`,
		`{"id":"x","query":"q","layer":"logs","freq":1}`,
		`{"id":"x","query":"q","layer":"site","freq":0}`,
	} {
		if err := os.WriteFile(p, []byte(bad+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadFlow(p); err == nil {
			t.Fatalf("битая строка принята: %s", bad)
		}
	}
}

func TestFlowChoose(t *testing.T) {
	cards := []FlowCard{{FamilyID: "a", ProductID: "a1"}, {FamilyID: "b", ProductID: "b1", OnRequest: true},
		{FamilyID: "c", ProductID: "c1"}, {FamilyID: "d", ProductID: "d1"}, {FamilyID: "e", ProductID: "e1"}}
	cases := []struct {
		expect []string
		want   FlowChoice
	}{
		{[]string{"c"}, FlowChoice{Action: "click", ProductID: "c1", Position: 3}},
		{[]string{"b"}, FlowChoice{Action: "price_request", ProductID: "b1", Position: 2}},
		{[]string{"e"}, FlowChoice{}}, // пятая карточка — покупатель её не видит
		{nil, FlowChoice{}},           // сервисный вопрос: товар не выбирают
		{[]string{"x", "d", "a"}, FlowChoice{Action: "click", ProductID: "a1", Position: 1}},
	}
	for _, c := range cases {
		if got := FlowChoose(cards, c.expect); got != c.want {
			t.Errorf("ожидание %v: %+v, ожидалось %+v", c.expect, got, c.want)
		}
	}
}

func TestFlowReform(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	if ok, _ := FlowReform(TrafficStep{Query: "q"}, r); ok {
		t.Fatal("без правильного названия переформулировки нет")
	}
	if ok, _ := FlowReform(TrafficStep{Query: "Касперский", Fix: "касперский"}, r); ok {
		t.Fatal("то же название — не переформулировка")
	}
	yes := 0
	for range 1000 {
		ok, gap := FlowReform(TrafficStep{Query: "касперкий", Fix: "касперский"}, r)
		if ok {
			yes++
			if gap.Seconds() < 15 || gap.Seconds() > 45 {
				t.Fatalf("пауза переформулировки %s вне 15–45 с: в журнале это должна быть переформулировка (<60 с)", gap)
			}
		}
	}
	if yes < 300 || yes > 500 {
		t.Fatalf("доля переформулировок %d из 1000, ожидалось около %.0f%%", yes, flowReformShare*100)
	}
}

// Вход агента на фиде: живые и трафик — только текущей версии каталога, исторический журнал — только у источника с
// журналом; старый mock-журнал в базе остаётся, но в отбор не попадает.
func TestAgentLogScope(t *testing.T) {
	ev := func(source, catalog string) domain.SearchEvent {
		return domain.SearchEvent{Source: source, CatalogVersion: catalog}
	}
	all := []domain.SearchEvent{ev("traffic", "softline-feed"), ev("live", "softline-feed"),
		ev("traffic", "mock-2"), ev("live", "mock-1"), ev("mock_log", ""), ev("", "")}
	feed := repositories.LogScope{Catalog: "softline-feed"}
	if got := filterScope(feed, all); len(got) != 3 || got[2].CatalogVersion != "" || got[2].Source != "" {
		t.Fatalf("фид: ожидались два поиска фида и поиск без версии, получено %+v", got)
	}
	mock := repositories.LogScope{Catalog: "mock-2", Historical: true}
	got := filterScope(mock, all)
	var srcs []string
	for _, e := range got {
		srcs = append(srcs, e.Source+":"+e.CatalogVersion)
	}
	if strings.Join(srcs, ",") != "traffic:mock-2,mock_log:,:" {
		t.Fatalf("mock: %v", srcs)
	}
	if len(filterScope(repositories.Unscoped, all)) != len(all) {
		t.Fatal("Unscoped отбирает")
	}
}
