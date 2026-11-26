package services

import (
	"encoding/json"
	"errors"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/pkg/errs"
)

func writeTrafficLog(t *testing.T, lines ...string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// Фон — сессии журнала с поиском: шаги по времени, паузы сжаты в [1 с, 90 с], события панели и сессии без
// поиска не берутся.
func TestTrafficSessionsFromLog(t *testing.T) {
	dir := writeTrafficLog(t,
		`{"ts":"2026-04-27T05:11:21Z","session_id":"s-b","client_id":null,"type":"click","product_id":"acad","position":1}`,
		`{"ts":"2026-04-27T05:10:50Z","session_id":"s-b","client_id":null,"type":"search","query":"акад","results_count":0}`,
		`{"ts":"2026-04-27T05:10:50.5Z","session_id":"s-b","client_id":null,"type":"search","query":"autocad","results_count":5}`,
		`{"ts":"2026-04-27T08:00:00Z","session_id":"s-b","client_id":null,"type":"add_to_cart","product_id":"acad"}`,
		`{"ts":"2026-04-27T06:00:00Z","session_id":"s-c","client_id":null,"type":"click","product_id":"x"}`,
		`{"ts":"2026-04-27T06:00:00Z","session_id":"s3-d","client_id":null,"type":"search_opened","intent_id":"it-1","payload":{}}`,
	)
	m, err := LoadTrafficModel(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Sessions) != 1 || m.Searches != 2 {
		t.Fatalf("сессии %d, поиски %d", len(m.Sessions), m.Searches)
	}
	st := m.Sessions[0].Steps
	kinds := []string{st[0].Kind + ":" + st[0].Query, st[1].Kind + ":" + st[1].Query, st[2].Kind, st[3].Kind}
	if strings.Join(kinds, ",") != "search:акад,search:autocad,click,add_to_cart" {
		t.Fatalf("шаги %v", kinds)
	}
	if st[0].Gap != 0 || st[1].Gap != trafficMinGap || st[2].Gap != 30500*time.Millisecond || st[3].Gap != trafficMaxGap {
		t.Fatalf("паузы %v %v %v %v", st[0].Gap, st[1].Gap, st[2].Gap, st[3].Gap)
	}
}

// Всплески: файл проверяется; empty — только формулировки кластера без действий; reform — неудача,
// переформулировка fix за <60 с и клик по первой позиции.
func TestTrafficSpikes(t *testing.T) {
	dir := writeTrafficLog(t, `{"ts":"2026-04-27T05:10:50Z","session_id":"s-a","type":"search","query":"kaspersky"}`)
	bad := filepath.Join(t.TempDir(), "bad.json")
	_ = os.WriteFile(bad, []byte(`[{"id":"x","kind":"reform","queries":["a"]}]`), 0o600)
	if _, err := LoadTrafficModel(dir, bad); err == nil {
		t.Fatal("reform без fix принят")
	}
	m, err := LoadTrafficModel(dir, filepath.Join("..", "..", "data", "traffic", "spikes.json"))
	if err != nil {
		t.Fatal(err)
	}
	r := rand.New(rand.NewPCG(1, 2))
	kinds := map[string]bool{}
	for _, s := range m.Spikes {
		kinds[s.Kind] = true
		for range 50 {
			ses := s.SpikeSession(r)
			if ses.Spike != s.ID || ses.Steps[0].Kind != TrafficSearch || !contains(s.Queries, ses.Steps[0].Query) {
				t.Fatalf("%s: %+v", s.ID, ses)
			}
			switch s.Kind {
			case SpikeEmpty:
				for _, st := range ses.Steps {
					if st.Kind != TrafficSearch || !contains(s.Queries, st.Query) {
						t.Fatalf("%s: в пустом всплеске шаг %+v", s.ID, st)
					}
				}
			case SpikeReform:
				if len(ses.Steps) != 3 || ses.Steps[1].Query != s.Fix || ses.Steps[1].Gap >= time.Minute ||
					!ses.Steps[2].ClickTop {
					t.Fatalf("%s: %+v", s.ID, ses.Steps)
				}
			}
		}
	}
	if !kinds[SpikeEmpty] || !kinds[SpikeReform] {
		t.Fatalf("в spikes.json нужны оба вида: %v", kinds)
	}
}

// Формулировки всплесков — новые: их нет в журнале и в наборах оценки (иначе агент исключит тексты test, а
// всплеск смешается с историей).
func TestTrafficSpikesAreNew(t *testing.T) {
	data := filepath.Join("..", "..", "data")
	m, err := LoadTrafficModel(filepath.Join(data, "mock"), filepath.Join(data, "traffic", "spikes.json"))
	if err != nil {
		t.Skip("нет data/mock:", err)
	}
	raw, err := os.ReadFile(filepath.Join(data, "mock", "queries.json"))
	if err != nil {
		t.Fatal(err)
	}
	var qs []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &qs); err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, q := range qs {
		known[foldQuery(q.Text)] = true
	}
	for _, s := range m.Sessions {
		for _, st := range s.Steps {
			if st.Kind == TrafficSearch {
				known[foldQuery(st.Query)] = true
			}
		}
	}
	for _, s := range m.Spikes {
		for _, q := range s.Queries {
			if known[foldQuery(q)] {
				t.Errorf("%s: «%s» уже есть в журнале или наборах", s.ID, q)
			}
		}
	}
}

func TestParseSpikePlanAndOffsets(t *testing.T) {
	plan, err := ParseSpikePlan("b@40m, a@20m", 10*time.Minute, 20)
	if err != nil || len(plan) != 2 || plan[0].ID != "a" || plan[0].At != 20*time.Minute || plan[1].Sessions != 20 {
		t.Fatalf("%+v %v", plan, err)
	}
	for _, bad := range []string{"a", "a@x", "a@-5m"} {
		if _, err := ParseSpikePlan(bad, time.Minute, 1); err == nil {
			t.Fatalf("%q принят", bad)
		}
	}
	offs := SpikeOffsets(rand.New(rand.NewPCG(3, 4)), plan[0])
	if len(offs) != 20 || offs[0] < 0 || offs[19] > plan[0].Len {
		t.Fatalf("смещения %v", offs)
	}
	for i := 1; i < len(offs); i++ {
		if offs[i] < offs[i-1] {
			t.Fatalf("не по возрастанию: %v", offs)
		}
	}
}

func TestPanelPrefixes(t *testing.T) {
	for q, want := range map[string]string{"ок": "", "word": "wor", "касперский": "кас|каспе", "ms office 2024": "ms |ms offi"} {
		if got := strings.Join(PanelPrefixes(q), "|"); got != want {
			t.Errorf("%q: %q, ждали %q", q, got, want)
		}
	}
}

// source=traffic принимают поиск и события; иное — 422. Для агента traffic — живой журнал (не исторический).
func TestTrafficSource(t *testing.T) {
	svc := NewServices(nil, Options{})
	var e *errs.Error
	err := svc.RecordEvent(t.Context(), in.EventRequest{Type: "click", ProductID: "p", Source: "bot"})
	if !errors.As(err, &e) || e.Type != errs.ErrTypeValidation {
		t.Fatalf("чужой источник: %v", err)
	}
	err = svc.RecordEvent(t.Context(), in.EventRequest{Type: "click", ProductID: "p", Source: "traffic"})
	if !errors.As(err, &e) || e.Type != errs.ErrTypeUnavailable {
		t.Fatalf("traffic без базы — 503: %v", err)
	}
	if _, _, err := validateSearch(&in.SearchRequest{Query: "word", Source: "Traffic"}); err != nil {
		t.Fatalf("поиск traffic: %v", err)
	}
	if _, _, err := validateSearch(&in.SearchRequest{Query: "word", Source: "mock_log"}); err == nil {
		t.Fatal("поиск mock_log принят")
	}
	for src, live := range map[string]bool{"": true, "live": true, "traffic": true, "mock_log": false, "compare": false} {
		if domain.IsLiveSource(src) != live {
			t.Errorf("IsLiveSource(%q) = %v", src, !live)
		}
	}
}
