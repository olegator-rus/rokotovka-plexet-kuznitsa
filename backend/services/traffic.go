package services

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

// Генератор mock-трафика стенда (контур автономии): модель трафика без HTTP. Фон — сессии журнала
// data/mock/events.jsonl (поиски и действия по порядку, паузы из журнала), повторяемые через API как обычный
// клиент; всплески — заранее заданные пачки неудачных запросов одного кластера (data/traffic/spikes.json).
// Исполнитель — backend/traffic_cli.go (`go run . traffic`); в таблицы агента генератор не пишет.

// Виды шагов сессии трафика: поиск и действия с товаром (типы POST /api/events контура поиска).
const (
	TrafficSearch = "search"
)

// trafficActions — действия с товаром из журнала, которые генератор повторяет (по товару текущей выдачи).
var trafficActions = []string{"click", "add_to_cart", "order", "price_request", "manager_request"}

// Виды всплесков: empty — пачка поисков одного кластера с пустой выдачей; reform — неудачный запрос и за
// <60 с переформулировка (доля переформулировок и частота нового неудачного запроса).
const (
	SpikeEmpty  = "empty"
	SpikeReform = "reform"
)

// Паузы между шагами сессии: из журнала, но не короче и не длиннее этих границ (сессия журнала может тянуться
// часами — генератор её сжимает). Переформулировка в журнале (<60 с) остаётся переформулировкой.
const (
	trafficMinGap = time.Second
	trafficMaxGap = 90 * time.Second
)

// TrafficStep — шаг сессии: поиск (Query) или действие с товаром журнала (ProductID — товар, который покупатель
// выбрал тогда; генератор повторяет действие, только если товар есть в текущей выдаче).
type TrafficStep struct {
	Kind      string        `json:"kind"`
	Query     string        `json:"query,omitempty"`
	ProductID string        `json:"product_id,omitempty"`
	Gap       time.Duration `json:"gap_ns"`
	// ClickTop — кликнуть первую позицию текущей выдачи (переформулировка всплеска нашла товар).
	ClickTop bool `json:"click_top,omitempty"`
	// Поток SearchOps на фиде (traffic_flow.go): Flow — шаг потока, действие решает модельный покупатель по
	// выдаче (FlowChoose); Expect — семейства ожидания строки; Fix — правильное название для переформулировки.
	Flow   bool     `json:"flow,omitempty"`
	Expect []string `json:"expect,omitempty"`
	Fix    string   `json:"fix,omitempty"`
}

// TrafficSession — сценарий одной сессии.
type TrafficSession struct {
	Steps []TrafficStep `json:"steps"`
	Spike string        `json:"spike,omitempty"` // id всплеска; пусто — фон
	Flow  string        `json:"flow,omitempty"`  // id строки потока SearchOps; пусто — не поток
}

// TrafficSpike — заранее заданный всплеск: кластер неудачных формулировок и (для reform) удачная переформулировка.
type TrafficSpike struct {
	ID      string   `json:"id"`
	Kind    string   `json:"kind"`
	Label   string   `json:"label"`
	Queries []string `json:"queries"`
	Fix     string   `json:"fix,omitempty"`
	Notes   string   `json:"notes,omitempty"`
}

// TrafficModel — распределение фона и описания всплесков.
type TrafficModel struct {
	Sessions []TrafficSession
	Spikes   []TrafficSpike
	Searches int // поисков в сессиях фона
}

type trafficLogRow struct {
	TS        time.Time `json:"ts"`
	SessionID string    `json:"session_id"`
	Type      string    `json:"type"`
	Query     string    `json:"query"`
	ProductID string    `json:"product_id"`
}

// LoadTrafficModel — фон из events.jsonl (каталог dataDir) и всплески из spikesPath (пусто — без всплесков).
func LoadTrafficModel(dataDir, spikesPath string) (*TrafficModel, error) {
	f, err := os.Open(filepath.Join(dataDir, "events.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("журнал событий: %w", err)
	}
	defer func() { _ = f.Close() }()
	var rows []trafficLogRow
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r trafficLogRow
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return nil, fmt.Errorf("events.jsonl:%d: %w", n, err)
		}
		rows = append(rows, r)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("журнал событий: %w", err)
	}
	m := &TrafficModel{}
	m.Sessions, m.Searches = trafficSessions(rows)
	if len(m.Sessions) == 0 {
		return nil, fmt.Errorf("в журнале нет сессий с поиском")
	}
	if spikesPath != "" {
		raw, err := os.ReadFile(spikesPath)
		if err != nil {
			return nil, fmt.Errorf("всплески: %w", err)
		}
		if err := json.Unmarshal(raw, &m.Spikes); err != nil {
			return nil, fmt.Errorf("всплески %s: %w", spikesPath, err)
		}
		for _, s := range m.Spikes {
			if err := s.validate(); err != nil {
				return nil, err
			}
		}
	}
	return m, nil
}

func (s TrafficSpike) validate() error {
	switch {
	case s.ID == "":
		return fmt.Errorf("всплеск без id")
	case s.Kind != SpikeEmpty && s.Kind != SpikeReform:
		return fmt.Errorf("всплеск %s: вид — empty или reform", s.ID)
	case len(s.Queries) == 0:
		return fmt.Errorf("всплеск %s: нет запросов", s.ID)
	case s.Kind == SpikeReform && strings.TrimSpace(s.Fix) == "":
		return fmt.Errorf("всплеск %s: у reform нужна переформулировка fix", s.ID)
	}
	return nil
}

// Spike — всплеск по id.
func (m *TrafficModel) Spike(id string) (TrafficSpike, bool) {
	for _, s := range m.Spikes {
		if s.ID == id {
			return s, true
		}
	}
	return TrafficSpike{}, false
}

// trafficSessions — сессии журнала с хотя бы одним поиском: поиски и действия контура поиска по времени. События
// панели (контур выдачи) генератор строит сам вокруг каждого поиска, из журнала их не берёт.
func trafficSessions(rows []trafficLogRow) ([]TrafficSession, int) {
	by := map[string][]trafficLogRow{}
	var order []string
	for _, r := range rows {
		if r.SessionID == "" || (r.Type != TrafficSearch && !slices.Contains(trafficActions, r.Type)) {
			continue
		}
		if r.Type == TrafficSearch && strings.TrimSpace(r.Query) == "" {
			continue
		}
		if r.Type != TrafficSearch && r.ProductID == "" {
			continue
		}
		if _, ok := by[r.SessionID]; !ok {
			order = append(order, r.SessionID)
		}
		by[r.SessionID] = append(by[r.SessionID], r)
	}
	sort.Strings(order) // порядок не зависит от порядка строк файла: одинаковый seed — одинаковый трафик
	var res []TrafficSession
	searches := 0
	for _, id := range order {
		evs := by[id]
		sort.SliceStable(evs, func(i, j int) bool { return evs[i].TS.Before(evs[j].TS) })
		if evs[0].Type != TrafficSearch {
			continue // действие без поиска (переход из каталога) — не поисковая сессия
		}
		var ses TrafficSession
		for i, e := range evs {
			st := TrafficStep{Kind: e.Type, Query: e.Query, ProductID: e.ProductID}
			if i > 0 {
				st.Gap = clampGap(e.TS.Sub(evs[i-1].TS))
			}
			if e.Type == TrafficSearch {
				searches++
			}
			ses.Steps = append(ses.Steps, st)
		}
		res = append(res, ses)
	}
	return res, searches
}

func clampGap(d time.Duration) time.Duration {
	return min(max(d, trafficMinGap), trafficMaxGap)
}

// PickBackground — случайная сессия фона.
func (m *TrafficModel) PickBackground(r *rand.Rand) TrafficSession {
	return m.Sessions[r.IntN(len(m.Sessions))]
}

// SpikeSession — сценарий одной сессии всплеска. empty: одна-две формулировки кластера подряд (вторая — через
// 10–40 с, как попытка уточнить), без клика. reform: неудачная формулировка, через 15–45 с — переформулировка
// fix и клик по первой позиции.
func (s TrafficSpike) SpikeSession(r *rand.Rand) TrafficSession {
	q := s.Queries[r.IntN(len(s.Queries))]
	ses := TrafficSession{Spike: s.ID, Steps: []TrafficStep{{Kind: TrafficSearch, Query: q}}}
	switch s.Kind {
	case SpikeEmpty:
		if len(s.Queries) > 1 && r.IntN(3) == 0 {
			q2 := s.Queries[r.IntN(len(s.Queries))]
			if q2 != q {
				ses.Steps = append(ses.Steps, TrafficStep{Kind: TrafficSearch, Query: q2,
					Gap: time.Duration(10+r.IntN(31)) * time.Second})
			}
		}
	case SpikeReform:
		ses.Steps = append(ses.Steps,
			TrafficStep{Kind: TrafficSearch, Query: s.Fix, Gap: time.Duration(15+r.IntN(31)) * time.Second},
			TrafficStep{Kind: "click", ClickTop: true, Gap: time.Duration(3+r.IntN(8)) * time.Second})
	}
	return ses
}

// SpikePlan — всплеск в расписании генератора: вид, смещение от старта, длительность и число сессий.
type SpikePlan struct {
	ID       string
	At       time.Duration
	Len      time.Duration
	Sessions int
}

// ParseSpikePlan — расписание вида "spike-id@20m,other@3h" (длительность и число сессий — общие).
func ParseSpikePlan(s string, length time.Duration, sessions int) ([]SpikePlan, error) {
	var res []SpikePlan
	for part := range strings.SplitSeq(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, at, ok := strings.Cut(part, "@")
		if !ok {
			return nil, fmt.Errorf("всплеск %q: ожидается id@смещение, например vpn-empty@20m", part)
		}
		d, err := time.ParseDuration(strings.TrimSpace(at))
		if err != nil || d < 0 {
			return nil, fmt.Errorf("всплеск %q: смещение — длительность Go (20m, 3h)", part)
		}
		res = append(res, SpikePlan{ID: strings.TrimSpace(id), At: d, Len: length, Sessions: sessions})
	}
	sort.SliceStable(res, func(i, j int) bool { return res[i].At < res[j].At })
	return res, nil
}

// SpikeOffsets — моменты старта сессий всплеска от его начала: равномерно по длительности с дрожанием ±25 %
// шага; по возрастанию.
func SpikeOffsets(r *rand.Rand, p SpikePlan) []time.Duration {
	if p.Sessions <= 0 {
		return nil
	}
	step := p.Len / time.Duration(p.Sessions)
	res := make([]time.Duration, p.Sessions)
	for i := range res {
		jitter := time.Duration(0)
		if step > 0 {
			jitter = time.Duration(r.Int64N(int64(step)/2+1)) - step/4
		}
		res[i] = max(0, time.Duration(i)*step+step/2+jitter)
	}
	slices.Sort(res)
	return res
}

// PanelPrefixes — префиксы, которые «набирает» покупатель перед поиском через панель: 3 символа и примерно
// половина запроса (если она длиннее). Считаются по рунам; короче 3 символов — без панели.
func PanelPrefixes(q string) []string {
	rs := []rune(strings.TrimSpace(q))
	if len(rs) < 3 {
		return nil
	}
	res := []string{string(rs[:3])}
	if half := (len(rs) + 1) / 2; half > 3 && half < len(rs) {
		res = append(res, strings.TrimRight(string(rs[:half]), " "))
	}
	return res
}
