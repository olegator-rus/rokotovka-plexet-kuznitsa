package services

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"strings"
	"time"
)

// Тестовый поток SearchOps на фиде заказчика: логов поиска у заказчика нет, поэтому
// журнал агента наполняется настоящими поисками через API по потоку из трёх слоёв с пометкой происхождения —
// запросы заказчика, прогоны живого сайта, синтетика из каталога (data/softline/searchops_flow.jsonl, собирает
// scripts/data/searchops_flow.py). Частота строки — модельная: столько сессий с этим запросом проигрывает
// `go run . traffic --flow`. Поведение покупателя тоже модельное, но исход решает настоящая выдача: покупатель
// выбирает товар, только если семейство из ожидания строки стоит в первых карточках; «по запросу» — запрос цены
// (не клик: купить нельзя); не нашёл — иногда переформулирует запрос правильным названием.

// Слои потока.
const (
	FlowLayerCustomer  = "customer"
	FlowLayerSite      = "site"
	FlowLayerSynthetic = "synthetic"
)

// FlowLayerLabels — подписи слоёв в интерфейсе.
var FlowLayerLabels = map[string]string{
	FlowLayerCustomer:  "запросы заказчика",
	FlowLayerSite:      "прогоны сайта",
	FlowLayerSynthetic: "синтетика из каталога",
}

// FlowKindLabels — подписи видов строк потока.
var FlowKindLabels = map[string]string{
	"customer": "эталон заказчика", "tz": "пример из ТЗ", "site": "прогон сайта", "typo": "опечатка",
	"translit": "транслит", "layout": "раскладка", "jargon": "жаргон", "declension": "склонение",
	"on_request": "по запросу", "absent": "нет в каталоге", "service": "сервисный вопрос",
}

// flowTopCards — покупатель смотрит первые четыре карточки (требование «минимум четыре ответа»).
const flowTopCards = 4

// flowReformShare — доля неудачных сессий с правильным названием (fix), в которых покупатель переформулирует запрос.
const flowReformShare = 0.4

// FlowRow — строка потока.
type FlowRow struct {
	ID     string `json:"id"`
	Query  string `json:"query"`
	Layer  string `json:"layer"`
	Kind   string `json:"kind"`
	Origin string `json:"origin"`
	// Freq — модельная частота: сессий с этим запросом за один проход потока.
	Freq int `json:"freq"`
	// ExpectFamilies — семейства фида, которые покупатель счёл бы ответом (по построению); пусто — сервисный вопрос
	// или товара в каталоге нет. Поиск и агент ожидание не видят — только генератор трафика.
	ExpectFamilies []string `json:"expect_families"`
	// Fix — правильное название, которым покупатель переформулирует неудачный запрос; пусто — не переформулирует.
	Fix string `json:"fix,omitempty"`
}

// LoadFlow — строки потока из JSONL; пустой запрос, неизвестный слой или частота < 1 — ошибка с номером строки.
func LoadFlow(path string) ([]FlowRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("поток SearchOps: %w", err)
	}
	defer func() { _ = f.Close() }()
	var rows []FlowRow
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r FlowRow
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, n, err)
		}
		switch {
		case strings.TrimSpace(r.Query) == "":
			return nil, fmt.Errorf("%s:%d: пустой запрос", path, n)
		case FlowLayerLabels[r.Layer] == "":
			return nil, fmt.Errorf("%s:%d: слой %q — customer, site или synthetic", path, n, r.Layer)
		case r.Freq < 1:
			return nil, fmt.Errorf("%s:%d: модельная частота меньше 1", path, n)
		}
		rows = append(rows, r)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("поток SearchOps: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("поток SearchOps %s пуст", path)
	}
	return rows, nil
}

// FlowSessions — сессии одного прохода потока: Freq сессий на строку, в случайном порядке (seed).
func FlowSessions(rows []FlowRow, r *rand.Rand) []TrafficSession {
	var res []TrafficSession
	for _, row := range rows {
		for range row.Freq {
			res = append(res, TrafficSession{Flow: row.ID, Steps: []TrafficStep{{Kind: TrafficSearch, Query: row.Query,
				Expect: row.ExpectFamilies, Fix: row.Fix, Flow: true}}})
		}
	}
	r.Shuffle(len(res), func(i, j int) { res[i], res[j] = res[j], res[i] })
	return res
}

// FlowCard — карточка выдачи, которую видит модельный покупатель.
type FlowCard struct {
	FamilyID   string
	ProductID  string
	OnRequest  bool
	VariantIDs []string
}

// FlowChoice — что делает модельный покупатель после поиска: Action — click (купить можно), price_request (нашёл,
// но только «по запросу») или "" (не нашёл); Position — позиция карточки с 1.
type FlowChoice struct {
	Action    string
	ProductID string
	Position  int
}

// FlowChoose — решение покупателя по выдаче: первая из первых flowTopCards карточек, чьё семейство в ожидании.
func FlowChoose(cards []FlowCard, expect []string) FlowChoice {
	if len(expect) == 0 {
		return FlowChoice{}
	}
	for i, c := range cards {
		if i >= flowTopCards {
			break
		}
		if !slices.Contains(expect, c.FamilyID) {
			continue
		}
		if c.OnRequest {
			return FlowChoice{Action: "price_request", ProductID: c.ProductID, Position: i + 1}
		}
		return FlowChoice{Action: "click", ProductID: c.ProductID, Position: i + 1}
	}
	return FlowChoice{}
}

// FlowReform — переформулирует ли покупатель неудачный запрос и через сколько.
func FlowReform(st TrafficStep, r *rand.Rand) (bool, time.Duration) {
	if st.Fix == "" || strings.EqualFold(strings.TrimSpace(st.Fix), strings.TrimSpace(st.Query)) {
		return false, 0
	}
	if r.Float64() >= flowReformShare {
		return false, 0
	}
	return true, time.Duration(15+r.IntN(31)) * time.Second
}
