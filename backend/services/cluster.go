package services

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/repositories"
)

// Кластеризация неудачных запросов (ARCHITECTURE §3.1, шаги 2–3): исходы поисков из журналов
// search_events и behavior_events (живые и исторические) → неудачные запросы (пусто, без клика,
// быстрая переформулировка) → кластеры по общим непонятным словам и близости векторов BGE-M3 →
// порядок по потерянному спросу (число различных сессий с неудачным поиском: накрутка из одной сессии
// кластер наверх не выводит).

const (
	// clusterMinFailures — неудачных поисков у запроса не меньше; clusterMinFailShare — их доля.
	clusterMinFailures  = 2
	clusterMinFailShare = 0.5
	// clusterCosine — векторы двух запросов настолько близки, что это одно намерение (при общем слове);
	// clusterCosineAlone — настолько близки, что общее слово не нужно («figma» и «фигма»).
	clusterCosine      = 0.80
	clusterCosineAlone = 0.88
	// clusterMaxQueries — запросов кластера в карточке и в брифе модели.
	clusterMaxQueries = 12
)

// clusterQuery — запрос кластера с исходами.
type clusterQuery struct {
	Text           string         `json:"text"`
	Freq           int            `json:"freq"`
	Failures       int            `json:"failures"`
	Empty          int            `json:"empty"`
	Reformulated   int            `json:"reformulated"`
	NoClick        int            `json:"no_click"`
	Clicks         int            `json:"clicks"`
	Orders         int            `json:"orders"`
	ReformulatedTo []reformTarget `json:"reformulated_to,omitempty"`
	// FailSessions — различные сессии с неудачным поиском: потерянный спрос считается по ним, чтобы сотни
	// поисков из одной сессии (накрутка) не выводили запрос наверх.
	FailSessions int `json:"fail_sessions"`
	failSess     map[string]bool
}

// reformTarget — во что переформулировали запрос и нашли ли после этого товар.
type reformTarget struct {
	Text      string `json:"text"`
	Count     int    `json:"count"`
	Clicked   int    `json:"clicked"`
	ClickedOn string `json:"clicked_family,omitempty"`
}

// queryStats — исходы всех запросов периода по нормализованному тексту.
type queryStats struct {
	byText map[string]*clusterQuery
	// clickedFamily — запрос → семейство, по которому чаще всего кликали (для успешных переформулировок).
	clickedProduct map[string]map[string]int
}

// failedCluster — кластер в памяти запуска.
type failedCluster struct {
	ID       string
	Key      string
	Label    string
	Terms    []string
	Queries  []clusterQuery
	Freq     int
	Failures int
	// LostDemand — различные сессии с неудачным поиском по запросам кластера.
	LostDemand float64
}

// computeQueryStats — исходы поисков по тем же правилам, что аналитика (analytics.go): набор до Enter
// не считается, действие относится к поиску по request_id или к последнему поиску сессии, переформулировка —
// следующий поиск сессии с другим текстом в течение reformulateWindow без клика по первому.
func computeQueryStats(searches []domain.SearchEvent, behavior []domain.BehaviorEvent) *queryStats {
	sort.SliceStable(searches, func(i, j int) bool { return searches[i].CreatedAt.Before(searches[j].CreatedAt) })
	type outcome struct {
		ev      *domain.SearchEvent
		clicked bool
		ordered bool
		product string
	}
	bySession := map[string][]*outcome{}
	byRequest := map[string]*outcome{}
	for i := range searches {
		o := &outcome{ev: &searches[i]}
		k := sessionKey(o.ev.SessionID, o.ev.RequestID)
		bySession[k] = append(bySession[k], o)
	}
	for k, list := range bySession {
		kept := list[:0]
		for i, cur := range list {
			if i+1 < len(list) {
				next := list[i+1]
				a, b := foldQuery(cur.ev.Query), foldQuery(next.ev.Query)
				if next.ev.CreatedAt.Sub(cur.ev.CreatedAt) <= typingWindow && len(b) > len(a) && strings.HasPrefix(b, a) {
					continue
				}
			}
			kept = append(kept, cur)
			byRequest[cur.ev.RequestID] = cur
		}
		bySession[k] = kept
	}
	for i := range behavior {
		b := &behavior[i]
		var o *outcome
		if b.RequestID != nil {
			o = byRequest[*b.RequestID]
		}
		if o == nil && b.SessionID != nil {
			list := bySession[*b.SessionID]
			for j := len(list) - 1; j >= 0; j-- {
				if at := list[j].ev.CreatedAt; !at.After(b.CreatedAt) {
					if b.CreatedAt.Sub(at) <= attributionWindow {
						o = list[j]
					}
					break
				}
			}
		}
		if o == nil {
			continue
		}
		switch b.Type {
		case domain.EventClick, domain.EventAddToCart, domain.EventOrder:
			o.clicked = true
			if b.ProductID != nil && o.product == "" {
				o.product = *b.ProductID
			}
			if b.Type == domain.EventOrder {
				o.ordered = true
			}
		}
	}
	st := &queryStats{byText: map[string]*clusterQuery{}, clickedProduct: map[string]map[string]int{}}
	get := func(t string) *clusterQuery {
		q := st.byText[t]
		if q == nil {
			q = &clusterQuery{Text: t}
			st.byText[t] = q
		}
		return q
	}
	for k, list := range bySession {
		for i, o := range list {
			key := foldQuery(o.ev.Query)
			if key == "" {
				continue
			}
			q := get(key)
			q.Freq++
			if o.clicked {
				q.Clicks++
				if o.product != "" {
					if st.clickedProduct[key] == nil {
						st.clickedProduct[key] = map[string]int{}
					}
					st.clickedProduct[key][o.product]++
				}
			}
			if o.ordered {
				q.Orders++
			}
			reform := false
			if i+1 < len(list) && !o.clicked {
				next := list[i+1]
				nk := foldQuery(next.ev.Query)
				if next.ev.CreatedAt.Sub(o.ev.CreatedAt) <= reformulateWindow && nk != key && nk != "" {
					reform = true
					addReform(q, nk, next.clicked, next.product)
				}
			}
			switch {
			case o.ev.ResultsCount == 0:
				q.Empty++
			case reform:
				q.Reformulated++
			case !o.clicked:
				q.NoClick++
			}
			if o.ev.ResultsCount == 0 || reform || !o.clicked {
				if q.failSess == nil {
					q.failSess = map[string]bool{}
				}
				q.failSess[k] = true
			}
		}
	}
	for _, q := range st.byText {
		q.FailSessions = len(q.failSess)
	}
	st.finish()
	return st
}

// finish — число неудач и пять главных переформулировок каждого запроса.
func (st *queryStats) finish() {
	for _, q := range st.byText {
		q.Failures = q.Empty + q.Reformulated + q.NoClick
		sort.Slice(q.ReformulatedTo, func(i, j int) bool {
			a, b := q.ReformulatedTo[i], q.ReformulatedTo[j]
			if a.Count != b.Count {
				return a.Count > b.Count
			}
			return a.Text < b.Text
		})
		if len(q.ReformulatedTo) > 5 {
			q.ReformulatedTo = q.ReformulatedTo[:5]
		}
	}
}

// queryStatsFromSQL — те же исходы из SQL-агрегата (repositories.AgentQueryStatsBetween).
func queryStatsFromSQL(a *repositories.AgentQueryStats) *queryStats {
	st := &queryStats{byText: map[string]*clusterQuery{}, clickedProduct: map[string]map[string]int{}}
	for _, r := range a.Queries {
		st.byText[r.Query] = &clusterQuery{Text: r.Query, Freq: r.Freq, Empty: r.Empty, Reformulated: r.Reformulated,
			NoClick: r.NoClick, Clicks: r.Clicks, Orders: r.Orders, FailSessions: r.FailSessions}
	}
	for _, r := range a.Reforms {
		if q := st.byText[r.Query]; q != nil {
			q.ReformulatedTo = append(q.ReformulatedTo, reformTarget{Text: r.Target, Count: r.Count, Clicked: r.Clicked,
				ClickedOn: r.ClickedOn})
		}
	}
	for _, r := range a.Clicks {
		if st.clickedProduct[r.Query] == nil {
			st.clickedProduct[r.Query] = map[string]int{}
		}
		st.clickedProduct[r.Query][r.ProductID] = r.N
	}
	st.finish()
	return st
}

func addReform(q *clusterQuery, text string, clicked bool, product string) {
	for i := range q.ReformulatedTo {
		if q.ReformulatedTo[i].Text == text {
			q.ReformulatedTo[i].Count++
			if clicked {
				q.ReformulatedTo[i].Clicked++
				if q.ReformulatedTo[i].ClickedOn == "" {
					q.ReformulatedTo[i].ClickedOn = product
				}
			}
			return
		}
	}
	r := reformTarget{Text: text, Count: 1}
	if clicked {
		r.Clicked, r.ClickedOn = 1, product
	}
	q.ReformulatedTo = append(q.ReformulatedTo, r)
}

// failed — неудачные запросы по убыванию числа неудач.
func (st *queryStats) failed() []clusterQuery {
	var res []clusterQuery
	for _, q := range st.byText {
		if q.Failures >= clusterMinFailures && float64(q.Failures) >= clusterMinFailShare*float64(q.Freq) {
			res = append(res, *q)
		}
	}
	sort.Slice(res, func(i, j int) bool {
		if res[i].FailSessions != res[j].FailSessions {
			return res[i].FailSessions > res[j].FailSessions
		}
		if res[i].Failures != res[j].Failures {
			return res[i].Failures > res[j].Failures
		}
		return res[i].Text < res[j].Text
	})
	return res
}

// queryTerms — значимые слова запроса и среди них непонятные поиску (кандидаты в алиасы).
func queryTerms(q string, lx *searchLexicon, rules *searchRules) (significant, unknown []string) {
	for _, t := range strings.Fields(foldText(q)) {
		if lexStopWords[t] || lexServiceWords[t] || utf8.RuneCountInString(t) < 2 || allDigits(t) {
			continue
		}
		significant = append(significant, t)
		if lx != nil && !knownWord(t, lx, rules) {
			unknown = append(unknown, t)
		}
	}
	return significant, unknown
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// clusterFailed — кластеры неудачных запросов: объединяются запросы с общим непонятным словом и запросы
// с близкими векторами (порог выше без общего слова). vecs — векторы в порядке qs, nil — только по словам.
func clusterFailed(qs []clusterQuery, vecs [][]float32, lx *searchLexicon, rules *searchRules) []failedCluster {
	n := len(qs)
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			if ra < rb {
				parent[rb] = ra
			} else {
				parent[ra] = rb
			}
		}
	}
	sig := make([]map[string]bool, n)
	unk := make([][]string, n)
	byUnknown := map[string]int{}
	for i, q := range qs {
		s, u := queryTerms(q.Text, lx, rules)
		sig[i] = map[string]bool{}
		for _, w := range s {
			sig[i][w] = true
		}
		unk[i] = u
		for _, w := range u {
			if utf8.RuneCountInString(w) < 3 {
				continue
			}
			if j, ok := byUnknown[w]; ok {
				union(i, j)
			} else {
				byUnknown[w] = i
			}
		}
	}
	if len(vecs) == n {
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				c := cosine(vecs[i], vecs[j])
				if c < clusterCosine {
					continue
				}
				shared := false
				for w := range sig[i] {
					if sig[j][w] {
						shared = true
						break
					}
				}
				if shared || c >= clusterCosineAlone {
					union(i, j)
				}
			}
		}
	}
	groups := map[int][]int{}
	for i := range qs {
		groups[find(i)] = append(groups[find(i)], i)
	}
	var res []failedCluster
	for _, idx := range groups {
		c := failedCluster{}
		terms := map[string]bool{}
		sessions := map[string]bool{}
		for _, i := range idx {
			q := qs[i]
			c.Queries = append(c.Queries, q)
			c.Freq += q.Freq
			c.Failures += q.Failures
			for k := range q.failSess {
				sessions[k] = true
			}
			if q.failSess == nil {
				// Статистика без сессий (старые записи) — каждая неудача отдельной сессией.
				for n := 0; n < q.Failures; n++ {
					sessions[fmt.Sprintf("%s#%d", q.Text, n)] = true
				}
			}
			for _, w := range unk[i] {
				terms[w] = true
			}
		}
		sort.Slice(c.Queries, func(a, b int) bool {
			if c.Queries[a].Freq != c.Queries[b].Freq {
				return c.Queries[a].Freq > c.Queries[b].Freq
			}
			return c.Queries[a].Text < c.Queries[b].Text
		})
		if len(c.Queries) > clusterMaxQueries {
			c.Queries = c.Queries[:clusterMaxQueries]
		}
		for w := range terms {
			c.Terms = append(c.Terms, w)
		}
		sort.Strings(c.Terms)
		c.Label = c.Queries[0].Text
		c.Key = c.Label
		if len(c.Terms) > 0 {
			c.Key = strings.Join(c.Terms, " ")
		}
		c.LostDemand = float64(len(sessions))
		res = append(res, c)
	}
	sort.Slice(res, func(i, j int) bool {
		if res[i].LostDemand != res[j].LostDemand {
			return res[i].LostDemand > res[j].LostDemand
		}
		return res[i].Label < res[j].Label
	})
	return res
}

func cosine(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / math.Sqrt(na*nb)
}

// embedTexts — векторы текстов пачками; эмбеддер не настроен или упал — nil (кластеры только по словам).
func (s *Services) embedTexts(ctx context.Context, texts []string) [][]float32 {
	if !s.embedder.Enabled() || len(texts) == 0 {
		return nil
	}
	var res [][]float32
	for i := 0; i < len(texts); i += 64 {
		j := min(i+64, len(texts))
		v, err := s.embedder.Embed(ctx, texts[i:j], embeddingDim)
		if err != nil {
			return nil
		}
		res = append(res, v...)
	}
	return res
}

// currentOutcome — что отвечает действующий поиск (B4) на запрос: без журнала, как покупателю.
type currentOutcome struct {
	Query  string       `json:"query"`
	Route  string       `json:"route"`
	Total  int          `json:"total"`
	Empty  bool         `json:"empty"`
	Top    []currentTop `json:"top"`
	Solved bool         `json:"solved"`
}

type currentTop struct {
	FamilyID string `json:"family_id"`
	Title    string `json:"title"`
	Group    string `json:"group"`
}

// currentSearch — запрос через действующий поиск B4 (или со снимком правил), без журнала.
func (s *Services) currentSearch(ctx context.Context, q string, rules *domain.RulesSnapshot) (currentOutcome, error) {
	resp, err := s.SearchWith(ctx, agentRequestID(), in.SearchRequest{Query: q, Config: domain.ConfigB4, Limit: 10, SessionID: "agent"},
		SearchOptions{Rules: rules, NoLog: true})
	if err != nil {
		return currentOutcome{Query: q}, err
	}
	o := currentOutcome{Query: q, Total: resp.Total, Empty: resp.Empty != nil, Top: []currentTop{}}
	if resp.Trace != nil {
		o.Route = resp.Trace.Route
	}
	for i, it := range resp.Items {
		if i >= 3 {
			break
		}
		o.Top = append(o.Top, currentTop{FamilyID: it.FamilyID, Title: it.Title, Group: it.Group})
	}
	// «Решено» — честная точная выдача: первый результат в группе exact и нет «нет в каталоге».
	o.Solved = !o.Empty && len(o.Top) > 0 && o.Top[0].Group == "exact"
	return o, nil
}
