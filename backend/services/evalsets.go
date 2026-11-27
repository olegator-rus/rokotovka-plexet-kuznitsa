package services

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/pkg/errs"
)

// Наборы оценки из журнала (ARCHITECTURE §6): у организаторов, скорее всего, не
// будет разметки — наборы tune/test/regression и оценки собираются из журнала поисков и действий.
//   1. Запросы с частотой ≥ k; неявная разметка товара по действиям после поиска: заказ 3, корзина или запрос
//      цены 2, клик 1 (клики с поправкой на позицию; минимальная поддержка). Пустые и без кликов — «проблемные».
//   2. Тип запроса — правилами нормализатора и атрибутов; неуверенные — пакетно LLM (Flash, с кэшем).
//   3. Кластеры (нормализованный текст, переформулировки, векторы) → наборы без пересечения кластеров;
//      кластеры, впервые появившиеся в отложенной неделе, — только в test.
//   4. Пул топ-10 B0–B3 без неявной оценки — LLM-судье (вслепую), в пределах лимита стоимости.
// Результат — queries.json и отчёт SETS.md.

// Значения по умолчанию сборки наборов.
const (
	setsDefaultMinFreq     = 2
	setsDefaultMinSupport  = 1
	setsDefaultJudgeRub    = 100
	setsDefaultClassifyRub = 5
	setsDefaultCalib       = 100
	// setsClickShare2 — доля поисков запроса (с поправкой на позицию), кликнувших товар, с которой клик без
	// корзины и заказа считается «подходящей альтернативой» (2), а не частичной (1).
	setsClickShare2 = 0.25
	// setsEmptyProblem — доля пустых выдач, с которой запрос — кандидат в проблемные.
	setsEmptyProblem = 0.5
	// setsPosBiasCap — предел веса клика с дальней позиции (обратная склонность к позиции √pos).
	setsPosBiasCap = 3.0
)

// Источники оценки товара в наборе (DerivedQuery.Sources).
const (
	labelImplicit = "implicit"
	labelJudge    = "judge"
	// labelImplicitJudge — неявная оценка, которую судья повысил (судья повышает, но не понижает).
	labelImplicitJudge = "implicit+judge"
)

// Проблемы запроса из журнала (DerivedQuery.Problem).
const (
	problemEmpty   = "empty"
	problemNoClick = "no_click"
)

// BuildSetsRequest — сборка наборов. Events — журнал из файла; nil — журнал из базы (search_events и
// behavior_events, без live, если не IncludeLive).
type BuildSetsRequest struct {
	Events      []domain.LogEvent
	IncludeLive bool
	// Days — только последние N дней журнала (от последнего события); 0 — весь журнал.
	Days       int
	MinFreq    int
	MinSupport int
	// ClickGrade — оценка «помогшего» клика без корзины и заказа: 2 (по умолчанию) или 1 (строгая схема).
	ClickGrade int
	// Classifier — модель классификации неуверенных типов (Flash); nil — только правила.
	Classifier     ChatModel
	ClassifyMaxRub float64
	// JudgeMaxRub — лимит стоимости судьи; 0 — судья не вызывается.
	JudgeMaxRub float64
	// JudgeCalib — сколько пар с неявной оценкой отдать судье для согласия.
	JudgeCalib int
	// CacheDir — где лежит кэш классификации (.classify-cache.json); пусто — без кэша.
	CacheDir string
	// Reference — разметка по построению (mock) для сверки; nil — сверки нет.
	Reference []domain.EvalQuery
	// Progress — строка хода работы; nil — молча.
	Progress func(string)
}

// DerivedQuery — запрос собранного набора: формат queries.json и поля происхождения.
type DerivedQuery struct {
	domain.EvalQuery
	Notes string   `json:"notes"`
	Terms []string `json:"terms"`
	// TypeSource — rules | llm | cache | default.
	TypeSource string `json:"type_source"`
	// Sources — откуда оценка товара: implicit | judge.
	Sources map[string]string `json:"judgment_sources"`
	Problem string            `json:"problem,omitempty"`
	// TargetFamily — целевое семейство exact/sku-запроса: семейство с наибольшей неявной поддержкой (поисков с
	// действием); при равенстве или без кликов — пусто, и правило «клик 2 → 3» не применяется.
	TargetFamily string `json:"target_family,omitempty"`
	// Searches, EmptyShare, ClickShare — что о запросе знает журнал.
	Searches   int     `json:"searches"`
	EmptyShare float64 `json:"empty_share"`
	ClickShare float64 `json:"click_share"`
	FirstSeen  string  `json:"first_seen"`
}

// setsLogQuery — запрос журнала с исходами.
type setsLogQuery struct {
	key       string
	text      string
	texts     map[string]int
	freq      int
	empty     int
	clicked   int // поисков с кликом
	manager   int
	first     time.Time
	last      time.Time
	products  map[string]*implicitStat
	reform    map[string]int // переформулировки в другой запрос (ключ)
	reformHit map[string]int // из них — с кликом после
}

// implicitStat — действия с товаром после поисков запроса: c1 — поисков с кликом или сильнее, c2 — с корзиной,
// запросом цены или заказом, c3 — с заказом; ipw — клики, взвешенные обратной склонностью к позиции.
type implicitStat struct {
	c1, c2, c3 int
	ipw        float64
	// sat — поисков, где после действия с товаром не было немедленной переформулировки.
	sat int
}

// setsLog — журнал, сведённый по запросам.
type setsLog struct {
	queries  map[string]*setsLogQuery
	searches int
	actions  int
	from, to time.Time
	// unattributed — действий с товаром без поиска (нет request_id).
	unattributed int
}

// actionValue — сила действия: заказ 3, корзина и запрос цены 2, клик 1.
func actionValue(t string) int {
	switch t {
	case domain.EventOrder:
		return 3
	case domain.EventAddToCart, domain.EventPriceRequest:
		return 2
	case domain.EventClick:
		return 1
	}
	return 0
}

// positionWeight — вес клика с позиции pos: обратная склонность √pos (клик ниже первого места — сильнее).
func positionWeight(pos *int) float64 {
	if pos == nil || *pos <= 1 {
		return 1
	}
	return math.Min(math.Sqrt(float64(*pos)), setsPosBiasCap)
}

// aggregateLog — поиски и действия по запросам. Действие относится к поиску по request_id (импорт логов
// проставляет его по сессии). Переформулировка — следующий поиск сессии в пределах reformulateWindow без
// действий между ними.
func aggregateLog(searches []domain.SearchEvent, behavior []domain.BehaviorEvent) *setsLog {
	lg := &setsLog{queries: map[string]*setsLogQuery{}}
	byReq := map[string]*setsLogQuery{}
	type perSearch struct {
		q    *setsLogQuery
		best map[string]int
		pos  map[string]*int
		// lastAct — время последнего действия; unsat — после действия сессия сразу искала другое (клик не помог).
		lastAct time.Time
		unsat   bool
	}
	ps := map[string]*perSearch{}
	acted := map[string]bool{}
	for i := range searches {
		e := &searches[i]
		key := foldQuery(e.Query)
		if key == "" {
			continue
		}
		q := lg.queries[key]
		if q == nil {
			q = &setsLogQuery{key: key, texts: map[string]int{}, products: map[string]*implicitStat{},
				reform: map[string]int{}, reformHit: map[string]int{}, first: e.CreatedAt}
			lg.queries[key] = q
		}
		q.freq++
		q.texts[strings.TrimSpace(e.Query)]++
		if e.ResultsCount == 0 {
			q.empty++
		}
		if e.CreatedAt.Before(q.first) {
			q.first = e.CreatedAt
		}
		if e.CreatedAt.After(q.last) {
			q.last = e.CreatedAt
		}
		if lg.from.IsZero() || e.CreatedAt.Before(lg.from) {
			lg.from = e.CreatedAt
		}
		if e.CreatedAt.After(lg.to) {
			lg.to = e.CreatedAt
		}
		lg.searches++
		byReq[e.RequestID] = q
		ps[e.RequestID] = &perSearch{q: q, best: map[string]int{}, pos: map[string]*int{}}
	}
	for _, b := range behavior {
		if b.RequestID == nil {
			if b.ProductID != nil {
				lg.unattributed++
			}
			continue
		}
		p := ps[*b.RequestID]
		if p == nil {
			continue
		}
		acted[*b.RequestID] = true
		if b.CreatedAt.After(p.lastAct) {
			p.lastAct = b.CreatedAt
		}
		if b.Type == domain.EventManagerRequest {
			p.q.manager++
			continue
		}
		v := actionValue(b.Type)
		if v == 0 || b.ProductID == nil || *b.ProductID == "" {
			continue
		}
		lg.actions++
		pid := *b.ProductID
		if v > p.best[pid] {
			p.best[pid] = v
		}
		if b.Type == domain.EventClick && p.pos[pid] == nil {
			p.pos[pid] = b.Position
		}
	}
	// Переформулировки: поиски одной сессии по времени.
	bySession := map[string][]*domain.SearchEvent{}
	for i := range searches {
		e := &searches[i]
		if e.SessionID != nil && *e.SessionID != "" {
			bySession[*e.SessionID] = append(bySession[*e.SessionID], e)
		}
	}
	for _, evs := range bySession {
		sort.SliceStable(evs, func(i, j int) bool { return evs[i].CreatedAt.Before(evs[j].CreatedAt) })
		for i := 0; i+1 < len(evs); i++ {
			a, b := evs[i], evs[i+1]
			ka, kb := foldQuery(a.Query), foldQuery(b.Query)
			if ka == "" || kb == "" || ka == kb {
				continue
			}
			if acted[a.RequestID] {
				// Клик и сразу другой поиск — клик не помог (короткий «дочитанный» интервал).
				if pa := ps[a.RequestID]; pa != nil && b.CreatedAt.Sub(pa.lastAct) <= reformulateWindow {
					pa.unsat = true
				}
				continue
			}
			if b.CreatedAt.Sub(a.CreatedAt) > reformulateWindow {
				continue
			}
			q := lg.queries[ka]
			q.reform[kb]++
			if acted[b.RequestID] {
				q.reformHit[kb]++
			}
		}
	}
	for _, p := range ps {
		clicked := false
		for pid, v := range p.best {
			st := p.q.products[pid]
			if st == nil {
				st = &implicitStat{}
				p.q.products[pid] = st
			}
			st.c1++
			if v >= 2 {
				st.c2++
			}
			if v >= 3 {
				st.c3++
			}
			st.ipw += positionWeight(p.pos[pid])
			if !p.unsat {
				st.sat++
			}
			clicked = true
		}
		if clicked {
			p.q.clicked++
		}
	}
	for _, q := range lg.queries {
		q.text = mostCommon(q.texts)
	}
	return lg
}

// mostCommon — самый частый вариант написания (при равенстве — первый по алфавиту).
func mostCommon(m map[string]int) string {
	best, n := "", -1
	for t, c := range m {
		if c > n || c == n && t < best {
			best, n = t, c
		}
	}
	return best
}

// implicitGrade — неявная оценка товара: заказ 3, корзина/запрос цены 2; клик — clickGrade, если после него
// не искали сразу другое (клик «помог»), иначе 1; при clickGrade 1 частый клик (≥ setsClickShare2 поисков с
// поправкой на позицию, не меньше двух) — 2. ok=false — поддержки не хватает, оценки нет.
func implicitGrade(st *implicitStat, searches, minSupport, clickGrade int) (int, bool) {
	switch {
	case st.c3 >= minSupport:
		return 3, true
	case st.c2 >= minSupport:
		return 2, true
	case st.c1 >= minSupport:
		if st.sat >= minSupport && clickGrade >= 2 {
			return 2, true
		}
		if st.c1 >= 2 && searches > 0 && st.ipw/float64(searches) >= setsClickShare2 {
			return 2, true
		}
		return 1, true
	}
	return 0, false
}

// topReform — самая частая переформулировка запроса и нашёлся ли после неё товар.
func (q *setsLogQuery) topReform() (string, bool) {
	t := mostCommon(q.reform)
	return t, t != "" && q.reformHit[t] > 0
}

// setsJournal — журнал для сборки: из файла (как импорт логов) или из базы.
func (s *Services) setsJournal(ctx context.Context, req BuildSetsRequest) ([]domain.SearchEvent, []domain.BehaviorEvent, error) {
	var searches []domain.SearchEvent
	var behavior []domain.BehaviorEvent
	if req.Events != nil {
		searches, behavior, _ = logRows(req.Events)
	} else {
		if !s.repos.Enabled() {
			return nil, nil, errs.NewUnavailableError("Журнал из базы недоступен: сервис без DATABASE_URL — передайте файл событий")
		}
		since := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		var err error
		if searches, err = s.repos.SearchEventsSince(ctx, since); err != nil {
			return nil, nil, err
		}
		if behavior, err = s.repos.BehaviorEventsSince(ctx, since); err != nil {
			return nil, nil, err
		}
		keep := func(src string) bool {
			return src != domain.EventSourceCompare && (req.IncludeLive || !domain.IsLiveSource(src))
		}
		searches = slices.DeleteFunc(searches, func(e domain.SearchEvent) bool {
			return !keep(e.Source) || strings.HasPrefix(e.RequestID, evalRequestPrefix)
		})
		behavior = slices.DeleteFunc(behavior, func(e domain.BehaviorEvent) bool { return !keep(e.Source) })
	}
	if req.Days > 0 && len(searches) > 0 {
		last := searches[0].CreatedAt
		for _, e := range searches {
			if e.CreatedAt.After(last) {
				last = e.CreatedAt
			}
		}
		since := last.Add(-time.Duration(req.Days) * 24 * time.Hour)
		searches = filterSince(searches, since)
		behavior = filterBehaviorSince(behavior, since)
	}
	if len(searches) == 0 {
		return nil, nil, errs.NewValidationError("В журнале нет поисков — собирать наборы не из чего")
	}
	return searches, behavior, nil
}

// derivedID — id запроса набора: стабилен для текста.
func derivedID(n int) string { return fmt.Sprintf("d-%04d", n) }
