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
)

// Статистика совместных покупок для агента «Коммерция» (контур коммерции).
// Сырой lift на сотнях заказов непригоден, поэтому:
//   - единица счёта — аккаунт, а не заказ: повторные заказы одного клиента не раздувают пару; таблица 2×2 —
//     «аккаунт покупал A» × «аккаунт покупал B» за период, все аккаунты с заказами периода;
//   - уровень — тип товара (семейство): конкретный SKU потом подбирают правила совместимости;
//   - опора — не менее трёх разных аккаунтов, купивших пару в одном заказе;
//   - односторонний точный тест Фишера, поправка Бенджамини — Хохберга на все проверенные пары, уровень 0,99
//     (q ≤ 0,01); для сравнения в доказательствах — p по заказам и проход Бонферрони;
//   - фильтры: разные листовые категории, не заменители, связи ещё нет, штраф повсеместным товарам (покупаются
//     с большим числом якорей); направление (якорь → дополнение) определяется отдельно;
//   - повтор: та же пара на отложенном позднем периоде (последние дни заказов, как temporal holdout агента).
// Заказы берутся из events.jsonl источника (order с order_id и account_id): в журнале базы состава заказа нет.

const (
	copurchaseAlpha           = 0.01
	copurchaseMinAccounts     = 3
	copurchaseLateMinAccounts = 2
	copurchaseLateAlpha       = 0.05
	// copurchaseHubMaxAnchors — дополнение, которое покупают (≥3 аккаунта) с большим числом семейств, — повсеместное.
	copurchaseHubMaxAnchors = 6
	// copurchaseMinMargin — порог маржи дополнения и комплекта (ступень 4 лестницы).
	copurchaseMinMargin = 0.05
)

// Причины отказа паре (copurchaseNoise.Reasons).
const (
	noiseFewAccounts    = "few_accounts"
	noiseNotSignificant = "not_significant"
	noiseSubstitute     = "substitute"
	noiseSameCategory   = "same_category"
	noiseHub            = "hub"
)

var noiseLabels = map[string]string{
	noiseFewAccounts:    "меньше 3 аккаунтов",
	noiseNotSignificant: "не значимо после поправки",
	noiseSubstitute:     "заменители",
	noiseSameCategory:   "одна категория",
	noiseHub:            "повсеместный товар",
}

// orderBasket — заказ: аккаунт, время, позиции (с количеством) и семейства.
type orderBasket struct {
	ID       string
	Account  string
	TS       time.Time
	Items    map[string]int
	Families []string
}

// ordersOf — заказы из событий order с номером заказа и аккаунтом; skipped — строки заказа без них или с позицией
// не из каталога (гость без аккаунта в статистику не входит).
func ordersOf(evs []domain.LogEvent, c *commerceCatalog) ([]orderBasket, int) {
	byID := map[string]*orderBasket{}
	var ids []string
	skipped := 0
	for _, e := range evs {
		if e.Type != domain.EventOrder {
			continue
		}
		p := c.products[e.ProductID]
		if e.OrderID == "" || e.AccountID == "" || p == nil {
			skipped++
			continue
		}
		b := byID[e.OrderID]
		if b == nil {
			b = &orderBasket{ID: e.OrderID, Account: e.AccountID, TS: e.TS, Items: map[string]int{}}
			byID[e.OrderID] = b
			ids = append(ids, e.OrderID)
		}
		b.Items[p.ID] += max(e.Quantity, 1)
		if !slices.Contains(b.Families, p.FamilyID) {
			b.Families = append(b.Families, p.FamilyID)
		}
	}
	res := make([]orderBasket, 0, len(ids))
	for _, id := range ids {
		b := byID[id]
		sort.Strings(b.Families)
		res = append(res, *b)
	}
	return res, skipped
}

// lchoose — ln C(n, k).
func lchoose(n, k int) float64 {
	if k < 0 || k > n {
		return math.Inf(-1)
	}
	a, _ := math.Lgamma(float64(n + 1))
	b, _ := math.Lgamma(float64(k + 1))
	c, _ := math.Lgamma(float64(n - k + 1))
	return a - b - c
}

// fisherGreater — односторонний точный тест Фишера: P(X ≥ k) при гипергеометрическом X (совокупность n,
// в ней rowA «успехов», выборка colB).
func fisherGreater(k, rowA, colB, n int) float64 {
	if n <= 0 || rowA <= 0 || colB <= 0 {
		return 1
	}
	den := lchoose(n, colB)
	p := 0.0
	for x := max(k, 0); x <= min(rowA, colB); x++ {
		if colB-x > n-rowA {
			continue
		}
		p += math.Exp(lchoose(rowA, x) + lchoose(n-rowA, colB-x) - den)
	}
	return min(p, 1)
}

// bhAdjust — поправка Бенджамини — Хохберга: q-значения в порядке входа.
func bhAdjust(ps []float64) []float64 {
	m := len(ps)
	idx := make([]int, m)
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(i, j int) bool { return ps[idx[i]] < ps[idx[j]] })
	q := make([]float64, m)
	prev := 1.0
	for r := m - 1; r >= 0; r-- {
		v := min(prev, ps[idx[r]]*float64(m)/float64(r+1))
		q[idx[r]], prev = min(v, 1), v
	}
	return q
}

// familyAgg — покупки семейства за период.
type familyAgg struct {
	Accounts map[string]bool
	Orders   int
}

// pairStat — пара семейств (A < B) за период.
type pairStat struct {
	A string `json:"a"`
	B string `json:"b"`
	// Accounts — разных аккаунтов, купивших пару в одном заказе (опора); JointOrders — таких заказов.
	Accounts    int `json:"accounts"`
	JointOrders int `json:"joint_orders"`
	// Таблица по аккаунтам: покупал A, покупал B, покупал оба (в любых заказах периода), всего аккаунтов.
	AccA     int `json:"accounts_a"`
	AccB     int `json:"accounts_b"`
	AccBoth  int `json:"accounts_both"`
	AccTotal int `json:"accounts_total"`
	OrdA     int `json:"orders_a"`
	OrdB     int `json:"orders_b"`
	OrdTotal int `json:"orders_total"`
	// P — односторонний Фишер по аккаунтам; Q — после поправки БХ (nil — пара не проверялась: опоры меньше 3);
	// POrders — тот же тест по заказам (для сравнения).
	P       float64  `json:"p_value"`
	Q       *float64 `json:"q_value"`
	POrders float64  `json:"p_value_orders"`
}

// copurchaseTable — покупки периода по семействам и парам.
type copurchaseTable struct {
	From, To time.Time
	Accounts int
	Orders   int
	Fam      map[string]*familyAgg
	Pairs    map[[2]string]*pairStat
	// Tested — пары с опорой ≥ 3 аккаунтов (по ним поправка), по возрастанию p.
	Tested []*pairStat
}

func pairKey(a, b string) [2]string {
	if a > b {
		a, b = b, a
	}
	return [2]string{a, b}
}

// buildCopurchase — таблица периода: счёт, тесты, поправка.
func buildCopurchase(bs []orderBasket) *copurchaseTable {
	t := &copurchaseTable{Fam: map[string]*familyAgg{}, Pairs: map[[2]string]*pairStat{}}
	accs := map[string]bool{}
	sameAcc := map[[2]string]map[string]bool{}
	for _, b := range bs {
		if t.From.IsZero() || b.TS.Before(t.From) {
			t.From = b.TS
		}
		if b.TS.After(t.To) {
			t.To = b.TS
		}
		accs[b.Account] = true
		t.Orders++
		for _, f := range b.Families {
			fa := t.Fam[f]
			if fa == nil {
				fa = &familyAgg{Accounts: map[string]bool{}}
				t.Fam[f] = fa
			}
			fa.Accounts[b.Account] = true
			fa.Orders++
		}
		for i := 0; i < len(b.Families); i++ {
			for j := i + 1; j < len(b.Families); j++ {
				k := pairKey(b.Families[i], b.Families[j])
				ps := t.Pairs[k]
				if ps == nil {
					ps = &pairStat{A: k[0], B: k[1]}
					t.Pairs[k] = ps
					sameAcc[k] = map[string]bool{}
				}
				ps.JointOrders++
				sameAcc[k][b.Account] = true
			}
		}
	}
	t.Accounts = len(accs)
	for k, ps := range t.Pairs {
		fa, fb := t.Fam[ps.A], t.Fam[ps.B]
		ps.Accounts = len(sameAcc[k])
		ps.AccA, ps.AccB, ps.AccTotal = len(fa.Accounts), len(fb.Accounts), t.Accounts
		for a := range fa.Accounts {
			if fb.Accounts[a] {
				ps.AccBoth++
			}
		}
		ps.OrdA, ps.OrdB, ps.OrdTotal = fa.Orders, fb.Orders, t.Orders
		ps.P = fisherGreater(ps.AccBoth, ps.AccA, ps.AccB, ps.AccTotal)
		ps.POrders = fisherGreater(ps.JointOrders, ps.OrdA, ps.OrdB, ps.OrdTotal)
		if ps.Accounts >= copurchaseMinAccounts {
			t.Tested = append(t.Tested, ps)
		}
	}
	sort.Slice(t.Tested, func(i, j int) bool {
		if t.Tested[i].P != t.Tested[j].P {
			return t.Tested[i].P < t.Tested[j].P
		}
		return t.Tested[i].A+t.Tested[i].B < t.Tested[j].A+t.Tested[j].B
	})
	ps := make([]float64, len(t.Tested))
	for i, x := range t.Tested {
		ps[i] = x.P
	}
	for i, q := range bhAdjust(ps) {
		v := q
		t.Tested[i].Q = &v
	}
	return t
}

func (t *copurchaseTable) pair(a, b string) *pairStat {
	if t == nil {
		return nil
	}
	return t.Pairs[pairKey(a, b)]
}

func (ps *pairStat) significant() bool { return ps != nil && ps.Q != nil && *ps.Q <= copurchaseAlpha }

// confidence — доля заказов from, где есть и to (доля присоединения).
func (ps *pairStat) confidence(from string) float64 {
	n := ps.OrdA
	if from == ps.B {
		n = ps.OrdB
	}
	if n == 0 {
		return 0
	}
	return float64(ps.JointOrders) / float64(n)
}

// copurchaseCandidate — пара без связи, прошедшая статистику и фильтры: гипотеза MISSING_RELATION.
type copurchaseCandidate struct {
	Key       string    `json:"key"`
	Anchor    string    `json:"anchor"`
	Companion string    `json:"companion"`
	Stat      *pairStat `json:"stats"`
	// Направление: доля присоединения в обе стороны (по заказам), каким его делает статистика и почему выбран якорь.
	Attach         float64 `json:"attach_rate"`
	AttachReverse  float64 `json:"attach_rate_reverse"`
	StatDirection  string  `json:"stat_direction"`
	DirectionBasis string  `json:"direction_basis"`
	// Оценка приоритета: покупки якоря × разрыв доли присоединения × маржа дополнения, ₽.
	AnchorOrders       int     `json:"anchor_orders"`
	Gap                float64 `json:"attach_gap"`
	CompanionMarginRub float64 `json:"companion_margin_rub"`
	Priority           float64 `json:"priority_score"`
}

// copurchaseNoise — пара, отклонённая статистикой или фильтрами.
type copurchaseNoise struct {
	A           string   `json:"a"`
	B           string   `json:"b"`
	Accounts    int      `json:"accounts"`
	JointOrders int      `json:"joint_orders"`
	P           float64  `json:"p_value"`
	Q           *float64 `json:"q_value"`
	Reasons     []string `json:"reasons"`
}

// copurchaseScan — итог get_copurchase_stats.
type copurchaseScan struct {
	SeenFrom     time.Time `json:"seen_from"`
	SeenTo       time.Time `json:"seen_to"`
	LateFrom     time.Time `json:"late_from"`
	LateTo       time.Time `json:"late_to"`
	Orders       int       `json:"orders"`
	Accounts     int       `json:"accounts"`
	LateOrders   int       `json:"late_orders"`
	LateAccounts int       `json:"late_accounts"`
	Skipped      int       `json:"order_lines_skipped"`
	Tests        int       `json:"tests"`
	Alpha        float64   `json:"alpha"`
	Correction   string    `json:"correction"`
	Significant  int       `json:"significant"`
	// Linked — значимые пары, связь которых уже есть в справочнике.
	Linked     []map[string]any      `json:"linked"`
	Candidates []copurchaseCandidate `json:"candidates"`
	// Noise — отклонённые пары с двумя и более совместными заказами (≤ 30), NoiseCounts — все по причинам,
	// Singles — пары из одного совместного заказа.
	Noise       []copurchaseNoise `json:"noise"`
	NoiseCounts map[string]int    `json:"noise_counts"`
	Singles     int               `json:"single_order_pairs"`
	seen, late  *copurchaseTable
	baskets     []orderBasket
}

// familyIsSystem — у семейства есть позиции с каноническими моделями (сервер, ноутбук): система-якорь.
func (c *commerceCatalog) familyIsSystem(f string) bool {
	return slices.ContainsFunc(c.familyProducts(f), func(p *domain.Product) bool { return p.ModelID != "" })
}

func (c *commerceCatalog) familyKind(f string) string {
	for _, p := range c.familyProducts(f) {
		return p.Kind
	}
	return ""
}

// familyMedianPrice — медиана известных цен доступных позиций семейства (0 — цен нет).
func (c *commerceCatalog) familyMedianPrice(f string) float64 {
	var ps []float64
	for _, p := range c.familyProducts(f) {
		if productAvailable(p) && p.PriceRub != nil {
			ps = append(ps, *p.PriceRub)
		}
	}
	if len(ps) == 0 {
		return 0
	}
	sort.Float64s(ps)
	return ps[len(ps)/2]
}

// skuMarginRub — маржа позиции в рублях (цена × доля); цены нет — 0.
func skuMarginRub(p *domain.Product) float64 {
	if p == nil || p.PriceRub == nil {
		return 0
	}
	return *p.PriceRub * p.Margin
}

// directionOf — якорь пары: система с моделью; иначе оборудование к ПО; иначе дороже; иначе по доле присоединения.
func (c *commerceCatalog) directionOf(ps *pairStat) (anchor, companion, basis string) {
	a, b := ps.A, ps.B
	sa, sb := c.familyIsSystem(a), c.familyIsSystem(b)
	switch {
	case sa != sb:
		if sb {
			a, b = b, a
		}
		return a, b, "якорь — система (сервер, ноутбук), дополнение покупают к ней"
	}
	ka, kb := c.familyKind(a), c.familyKind(b)
	if ka != kb && (ka == "hardware" || kb == "hardware") {
		if kb == "hardware" {
			a, b = b, a
		}
		return a, b, "якорь — оборудование, к нему покупают ПО"
	}
	pa, pb := c.familyMedianPrice(a), c.familyMedianPrice(b)
	if pa != pb {
		if pb > pa {
			a, b = b, a
		}
		return a, b, "якорь — более дорогая позиция пары"
	}
	if ps.confidence(b) > ps.confidence(a) {
		a, b = b, a
	}
	return a, b, "по доле присоединения"
}

// hasRelation — между семействами есть связь типа из types (пусто — любого), в любую сторону.
func hasRelation(docs []relationDoc, a, b string, types ...string) *relationDoc {
	for i, d := range docs {
		if (d.FromFamily == a && d.ToFamily == b) || (d.FromFamily == b && d.ToFamily == a) {
			if len(types) == 0 || slices.Contains(types, d.Type) {
				return &docs[i]
			}
		}
	}
	return nil
}

// orderWindow — граница позднего периода по последнему заказу: [lateFrom, lateTo).
func (s *Services) orderWindow(bs []orderBasket) (time.Time, time.Time) {
	var last time.Time
	for _, b := range bs {
		if b.TS.After(last) {
			last = b.TS
		}
	}
	h := HoldoutOptions{}
	if s.agent != nil {
		h = s.agent.opts.Holdout
	}
	return s.holdoutRangeAt(h, last, last)
}

// loadOrders — заказы из источника (events.jsonl); источника логов нет — пусто.
func (s *Services) loadOrders(ctx context.Context, c *commerceCatalog) ([]orderBasket, int, error) {
	l, ok := s.source.(logLoader)
	if !ok {
		return nil, 0, nil
	}
	evs, err := l.LoadEvents(ctx)
	if err != nil {
		if isNoFile(err) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	bs, skipped := ordersOf(evs, c)
	return bs, skipped, nil
}

// scanCopurchase — статистика видимого периода, повтор на позднем и разбор пар: кандидаты, связанные, шум.
func scanCopurchase(c *commerceCatalog, docs []relationDoc, bs []orderBasket, lateFrom, lateTo time.Time) *copurchaseScan {
	var seenB, lateB []orderBasket
	for _, b := range bs {
		switch {
		case b.TS.Before(lateFrom):
			seenB = append(seenB, b)
		case b.TS.Before(lateTo):
			lateB = append(lateB, b)
		}
	}
	seen, late := buildCopurchase(seenB), buildCopurchase(lateB)
	res := &copurchaseScan{SeenFrom: seen.From, SeenTo: lateFrom, LateFrom: lateFrom, LateTo: lateTo, Orders: seen.Orders,
		Accounts: seen.Accounts, LateOrders: late.Orders, LateAccounts: late.Accounts, Tests: len(seen.Tested),
		Alpha: copurchaseAlpha, Correction: "Бенджамини — Хохберг", Linked: []map[string]any{},
		Candidates: []copurchaseCandidate{}, Noise: []copurchaseNoise{}, NoiseCounts: map[string]int{}, seen: seen, late: late,
		baskets: seenB}
	// Сколько семейств покупают с дополнением (опора ≥ 3 аккаунта) — для штрафа повсеместным.
	partners := map[string]int{}
	for _, ps := range seen.Tested {
		partners[ps.A]++
		partners[ps.B]++
	}
	keys := make([][2]string, 0, len(seen.Pairs))
	for k := range seen.Pairs {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i][0]+">"+keys[i][1] < keys[j][0]+">"+keys[j][1] })
	for _, k := range keys {
		ps := seen.Pairs[k]
		if ps.significant() {
			res.Significant++
		}
		var reasons []string
		if ps.Accounts < copurchaseMinAccounts {
			reasons = append(reasons, noiseFewAccounts)
		} else if !ps.significant() {
			reasons = append(reasons, noiseNotSignificant)
		}
		if hasRelation(docs, ps.A, ps.B, domain.RelationSubstitute) != nil {
			reasons = append(reasons, noiseSubstitute)
		}
		if ca, cb := c.familyCategory(ps.A), c.familyCategory(ps.B); ca != "" && ca == cb {
			reasons = append(reasons, noiseSameCategory)
		}
		anchor, comp, basis := c.directionOf(ps)
		if partners[comp] > copurchaseHubMaxAnchors {
			reasons = append(reasons, noiseHub)
		}
		if len(reasons) == 0 {
			if d := hasRelation(docs, ps.A, ps.B); d != nil {
				res.Linked = append(res.Linked, map[string]any{"a": ps.A, "b": ps.B, "relation_id": d.ID, "type": d.Type,
					"accounts": ps.Accounts, "q_value": ps.Q})
				continue
			}
			cand := copurchaseCandidate{Key: "copurchase:" + anchor + ">" + comp, Anchor: anchor, Companion: comp, Stat: ps,
				Attach: ps.confidence(anchor), AttachReverse: ps.confidence(comp), DirectionBasis: basis}
			cand.StatDirection = anchor + " → " + comp
			if cand.AttachReverse > cand.Attach {
				cand.StatDirection = comp + " → " + anchor
			}
			cand.AnchorOrders = seen.Fam[anchor].Orders
			cand.Gap = 1 - cand.Attach
			var sum float64
			n := 0
			for _, p := range c.familyProducts(comp) {
				if productAvailable(p) && p.PriceRub != nil {
					sum += skuMarginRub(p)
					n++
				}
			}
			if n > 0 {
				cand.CompanionMarginRub = math.Round(sum / float64(n))
			}
			cand.Priority = math.Round(float64(cand.AnchorOrders)*cand.Gap*cand.CompanionMarginRub*100) / 100
			res.Candidates = append(res.Candidates, cand)
			continue
		}
		for _, r := range reasons {
			res.NoiseCounts[r]++
		}
		if ps.JointOrders < 2 {
			res.Singles++
			continue
		}
		res.Noise = append(res.Noise, copurchaseNoise{A: ps.A, B: ps.B, Accounts: ps.Accounts, JointOrders: ps.JointOrders,
			P: ps.P, Q: ps.Q, Reasons: reasons})
	}
	sort.SliceStable(res.Candidates, func(i, j int) bool {
		if res.Candidates[i].Priority != res.Candidates[j].Priority {
			return res.Candidates[i].Priority > res.Candidates[j].Priority
		}
		return res.Candidates[i].Stat.P < res.Candidates[j].Stat.P
	})
	sort.SliceStable(res.Noise, func(i, j int) bool {
		if res.Noise[i].JointOrders != res.Noise[j].JointOrders {
			return res.Noise[i].JointOrders > res.Noise[j].JointOrders
		}
		return res.Noise[i].P < res.Noise[j].P
	})
	if len(res.Noise) > 30 {
		res.Noise = res.Noise[:30]
	}
	return res
}

// lateRepeat — повтор пары на позднем периоде: аккаунтов с парой в одном заказе ≥ 2 и p ≤ 0,05.
func (sc *copurchaseScan) lateRepeat(a, b string) (bool, map[string]any) {
	info := map[string]any{"from": sc.LateFrom, "to": sc.LateTo, "orders": sc.LateOrders, "accounts_total": sc.LateAccounts,
		"min_accounts": copurchaseLateMinAccounts, "alpha": copurchaseLateAlpha}
	ps := sc.late.pair(a, b)
	if sc.LateOrders == 0 {
		info["repeated"], info["why"] = false, "в позднем периоде нет заказов"
		return false, info
	}
	if ps == nil {
		info["accounts"], info["repeated"], info["why"] = 0, false, "в позднем периоде пару не покупали вместе"
		return false, info
	}
	info["accounts"], info["joint_orders"], info["p_value"] = ps.Accounts, ps.JointOrders, ps.P
	ok := ps.Accounts >= copurchaseLateMinAccounts && ps.P <= copurchaseLateAlpha
	info["repeated"] = ok
	if !ok {
		info["why"] = fmt.Sprintf("в позднем периоде аккаунтов %d, p = %s", ps.Accounts, fmtP(ps.P))
	}
	return ok, info
}

// structureCheck — ступень 1: позиции есть и доступны; пара не объяснена общим якорем («соседи» — дополнения к
// одной системе) и не производная от значимой пары с системой-якорем.
func (sc *copurchaseScan) structureCheck(c *commerceCatalog, docs []relationDoc, anchor, comp string) (bool, string) {
	for _, f := range []string{anchor, comp} {
		if !slices.ContainsFunc(c.familyProducts(f), productAvailable) {
			return false, fmt.Sprintf("у семейства %s нет доступных позиций", f)
		}
	}
	// Производная: одна из пары — дополнение к системе S, а пара (S, другая) сама значима.
	for _, d := range docs {
		if !talksAboutCompat(d.Type) || !c.familyIsSystem(d.FromFamily) {
			continue
		}
		for _, x := range [][2]string{{anchor, comp}, {comp, anchor}} {
			if d.ToFamily == x[0] && d.FromFamily != x[1] {
				if ps := sc.seen.pair(d.FromFamily, x[1]); ps.significant() {
					return false, fmt.Sprintf("производная пара: %s покупают к системе %s, а пара %s × %s значима сама — "+
						"сворачивается к паре с системой-якорем", x[0], d.FromFamily, d.FromFamily, x[1])
				}
			}
		}
	}
	// Соседи: обе — дополнения к одному семейству-якорю.
	for _, d := range docs {
		if !talksAboutCompat(d.Type) || d.ToFamily != anchor || d.FromFamily == comp {
			continue
		}
		for _, e := range docs {
			if talksAboutCompat(e.Type) && e.FromFamily == d.FromFamily && e.ToFamily == comp {
				return false, fmt.Sprintf("обе — дополнения к %s (связи %s и %s): пара объяснена общим якорем, это соседи, "+
					"а не новая связь", d.FromFamily, d.ID, e.ID)
			}
		}
	}
	// Соседи без связей: обе — комплектующие одной модели системы.
	if !c.familyIsSystem(anchor) && !c.familyIsSystem(comp) {
		models := map[string]bool{}
		for _, p := range c.familyProducts(anchor) {
			for _, m := range p.CompatibleWith {
				models[m] = true
			}
		}
		for _, p := range c.familyProducts(comp) {
			for _, m := range p.CompatibleWith {
				if models[m] {
					return false, fmt.Sprintf("обе — комплектующие модели %s: пара объяснена общей системой-якорем", m)
				}
			}
		}
	}
	return true, ""
}

// skuCompat — совместимость позиции дополнения с якорем.
type skuCompat struct {
	ProductID string   `json:"product_id"`
	Title     string   `json:"title"`
	State     string   `json:"state"`
	Condition string   `json:"condition"`
	Basis     string   `json:"basis"`
	RuleID    string   `json:"rule_id,omitempty"`
	PriceRub  *float64 `json:"price_rub"`
	Margin    float64  `json:"margin"`
	PriceMode string   `json:"price_mode"`
}

// attrCompatRules — совместимость по атрибутам, когда правила вендора нет: категория дополнения, ключ, правило
// «значение дополнения ≥ значения якоря» (мощность ИБП не ниже мощности блока питания сервера).
var attrCompatRules = []struct {
	category, key, label, unit string
}{
	{"ups", "power_w", "мощность ИБП не ниже мощности блока питания сервера", "Вт"},
}

// pairSKUCompat — ступень 2: каждая доступная позиция дополнения против всех доступных позиций якоря. Правило
// модели или лицензии; нет правила — правило по атрибутам; нет и его — unknown («не проверено» ≠ «совместимо»).
func (c *commerceCatalog) pairSKUCompat(anchor, comp string) []skuCompat {
	var anchors []*domain.Product
	for _, p := range c.familyProducts(anchor) {
		if productAvailable(p) {
			anchors = append(anchors, p)
		}
	}
	var res []skuCompat
	for _, p := range c.familyProducts(comp) {
		if !productAvailable(p) {
			continue
		}
		sc := skuCompat{ProductID: p.ID, Title: p.Title, PriceRub: p.PriceRub, Margin: p.Margin, PriceMode: p.PriceMode,
			State: domain.CompatUnknown, Basis: CompatBasisNone}
		worst, best := "", ""
		for _, a := range anchors {
			var r CompatResult
			if a.ModelID != "" {
				r = c.ModelCompat(p.ID, a.ModelID)
			} else {
				r = c.PairCompat(a.ID, p.ID, nil)
			}
			if r.State == domain.CompatUnknown {
				for _, ar := range attrCompatRules {
					if p.CategoryID != ar.category {
						continue
					}
					need, have := a.Specs[ar.key], p.Specs[ar.key]
					if need <= 0 || have <= 0 {
						continue
					}
					r = CompatResult{State: domain.CompatConditional, Basis: "attributes",
						Condition: fmt.Sprintf("%s: %.0f %s ≥ %.0f %s", ar.label, have, ar.unit, need, ar.unit)}
					if have < need {
						r.State = domain.CompatIncompatible
						r.Condition = fmt.Sprintf("%s: %.0f %s < %.0f %s у %s", ar.label, have, ar.unit, need, ar.unit, a.ID)
					}
				}
			}
			if r.State == domain.CompatIncompatible && worst == "" {
				worst = r.State
				sc.Condition, sc.Basis, sc.RuleID = r.Condition, r.Basis, r.RuleID
			}
			if worst == "" && compatRank(r.State) < compatRank(best) {
				best = r.State
				sc.Condition, sc.Basis, sc.RuleID = r.Condition, r.Basis, r.RuleID
			}
		}
		sc.State = firstNonEmpty(worst, best, domain.CompatUnknown)
		res = append(res, sc)
	}
	return res
}

// compatRank — чем меньше, тем надёжнее; пусто — хуже всего.
func compatRank(s string) int {
	switch s {
	case domain.CompatVerified:
		return 0
	case domain.CompatConditional:
		return 1
	case domain.CompatUnknown:
		return 2
	}
	return 3
}

// compatSummary — итог ступени 2: позиции, которые можно предлагать, и совместимость связи.
func compatSummary(items []skuCompat) (passing []skuCompat, state, condition string) {
	for _, it := range items {
		if it.State != domain.CompatIncompatible {
			passing = append(passing, it)
		}
	}
	if len(passing) == 0 {
		return nil, domain.CompatIncompatible, ""
	}
	state = passing[0].State
	var conds []string
	for _, it := range passing {
		if compatRank(it.State) > compatRank(state) {
			state = it.State
		}
		if it.Condition != "" && !slices.Contains(conds, it.Condition) && len(conds) < 2 {
			conds = append(conds, it.Condition)
		}
	}
	return passing, state, strings.Join(conds, "; ")
}

// licenseCheck — ступень 3: правило вендора якоря, которое касается дополнения и не проходится ни одной позицией.
func (c *commerceCatalog) licenseCheck(anchor string, passing []skuCompat) (bool, string, []string) {
	var rules []string
	for _, a := range c.familyProducts(anchor) {
		if !productAvailable(a) {
			continue
		}
		for _, lr := range c.licenseRulesFor(a.ID) {
			covered, ok := 0, 0
			for _, it := range passing {
				if !c.ruleCovers(lr, it.ProductID) {
					continue
				}
				covered++
				if c.ruleCheck(lr, a.ID, it.ProductID).State != domain.CompatIncompatible {
					ok++
				}
			}
			if covered > 0 && !slices.Contains(rules, lr.ID) {
				rules = append(rules, lr.ID)
				if ok == 0 {
					return false, fmt.Sprintf("правило вендора %s: ни одна позиция дополнения его не проходит", lr.ID), rules
				}
			}
		}
	}
	if len(rules) == 0 {
		return true, "правил вендора для пары нет", nil
	}
	return true, "правила вендора пройдены: " + strings.Join(rules, ", "), rules
}

// meanMargin — средняя маржа (доля и рубли) позиций с ценой.
func meanMargin(items []skuCompat) (float64, float64) {
	var share, rub float64
	n := 0
	for _, it := range items {
		if it.PriceRub == nil {
			continue
		}
		share += it.Margin
		rub += *it.PriceRub * it.Margin
		n++
	}
	if n == 0 {
		return 0, 0
	}
	return share / float64(n), math.Round(rub / float64(n))
}

// fmtP — p-значение по-русски: «4,5e-04» или «0,013».
func fmtP(p float64) string {
	s := fmt.Sprintf("%.2g", p)
	if p < 0.001 {
		s = fmt.Sprintf("%.1e", p)
	}
	return strings.ReplaceAll(s, ".", ",")
}
