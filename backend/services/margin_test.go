package services

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/pkg/errs"
	"kuznica/api/repositories"
)

// cands — позиции одной группы со скором и нормированной маржей.
func marginCands(key string, finals, pcts []float64) []marginCand {
	cs := make([]marginCand, len(finals))
	for i := range finals {
		cs[i] = marginCand{productID: string(rune('a' + i)), key: key, final: finals[i], pct: pcts[i], raw: pcts[i], known: true}
	}
	return cs
}

func TestMarginReorderBaseIsIdentity(t *testing.T) {
	in := marginInput{cands: marginCands("g", []float64{0.9, 0.89, 0.88}, []float64{0, 0.5, 1})}
	r := marginReorder(in, marginBaseWeight, marginMaxShift)
	if !sameIntOrder(r.order, []int{0, 1, 2}) || len(r.guards) != 0 {
		t.Fatalf("на весе контура поиска порядок не меняется: %+v", r)
	}
}

func TestMarginReorderMaxShiftAndGroups(t *testing.T) {
	// Пять позиций с почти равным скором, у последней самая высокая маржа: поднимается не выше чем на 2.
	finals := []float64{0.90, 0.899, 0.898, 0.897, 0.896}
	in := marginInput{cands: marginCands("g", finals, []float64{0, 0, 0, 0, 1})}
	r := marginReorder(in, 0.15, marginMaxShift)
	if !sameIntOrder(r.order, []int{0, 1, 4, 2, 3}) || !slices.Contains(r.guards, domain.MarginGuardMaxShift) {
		t.Fatalf("потолок сдвига: %+v", r)
	}
	for to, from := range r.order {
		if abs(to-from) > marginMaxShift {
			t.Fatalf("сдвиг больше 2: %v", r.order)
		}
	}
	// Разные группы: позиция второй группы не встаёт выше первой, как бы ни была велика маржа.
	cs := append(marginCands("exact", []float64{0.9}, []float64{0}), marginCands("match", []float64{0.899}, []float64{1})...)
	r = marginReorder(marginInput{cands: cs}, 0.15, marginMaxShift)
	if !sameIntOrder(r.order, []int{0, 1}) || !slices.Contains(r.guards, domain.MarginGuardGroupLock) {
		t.Fatalf("переход между группами: %+v", r)
	}
}

func TestMarginReorderExactGuards(t *testing.T) {
	cs := marginCands("g", []float64{0.9, 0.899}, []float64{0, 1})
	r := marginReorder(marginInput{cands: cs, exactQuery: true}, 0.15, marginMaxShift)
	if !sameIntOrder(r.order, []int{0, 1}) || !slices.Equal(r.guards, []string{domain.MarginGuardExactQuery}) {
		t.Fatalf("точный запрос: %+v", r)
	}
	cs = marginCands("g", []float64{0.9, 0.899, 0.898}, []float64{0, 0, 1})
	r = marginReorder(marginInput{cands: cs, protectTop1: true}, 0.15, marginMaxShift)
	if r.order[0] != 0 || !sameIntOrder(r.order, []int{0, 2, 1}) || !slices.Contains(r.guards, domain.MarginGuardExactTop1) {
		t.Fatalf("защита топ-1: %+v", r)
	}
	// Вес ниже базового снимает маржу контура поиска: позиция, стоявшая выше только за счёт маржи, опускается.
	cs = marginCands("g", []float64{0.9, 0.899}, []float64{0.5, 0})
	r = marginReorder(marginInput{cands: cs}, 0, marginMaxShift)
	if !sameIntOrder(r.order, []int{1, 0}) {
		t.Fatalf("вес 0: %+v", r)
	}
}

func TestMarginEpsilonRollsBackExpensiveBlock(t *testing.T) {
	cs := marginCands("g", []float64{0.9, 0.85, 0.5, 0.49}, []float64{0, 1, 0, 1})
	in := marginInput{cands: cs}
	order := []int{1, 0, 3, 2}
	if b := marginBlocks(order); len(b) != 2 || b[0] != [2]int{0, 2} || b[1] != [2]int{2, 4} {
		t.Fatalf("блоки: %v", b)
	}
	// Потеря — пока первый блок переставлен; откатывается самый дорогой (0.9−0.85 > 0.5−0.49).
	loss := func(o []int) float64 {
		if o[0] == 1 {
			return 0.05
		}
		return 0
	}
	got, rolled := marginApplyEpsilon(in, order, marginEpsilon, loss)
	if rolled != 1 || !sameIntOrder(got, []int{0, 1, 3, 2}) {
		t.Fatalf("откат: %v %d", got, rolled)
	}
}

// memMarginStore — версии политики в памяти.
type memMarginStore struct {
	mu   sync.Mutex
	rows []domain.ConfigVersion
}

func (m *memMarginStore) LatestMarginPolicy(context.Context) (*domain.ConfigVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.rows) == 0 {
		return nil, nil
	}
	v := m.rows[len(m.rows)-1]
	return &v, nil
}

func (m *memMarginStore) ListMarginPolicies(context.Context) ([]domain.ConfigVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := slices.Clone(m.rows)
	slices.Reverse(out)
	return out, nil
}

func (m *memMarginStore) GetMarginPolicy(_ context.Context, version int) (*domain.ConfigVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.rows {
		if r.Version == version {
			return &r, nil
		}
	}
	return nil, nil
}

func (m *memMarginStore) CreateMarginPolicy(_ context.Context, base int, v *domain.ConfigVersion) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.rows) != base {
		return repositories.ErrMarginVersionConflict
	}
	v.Version = base + 1
	m.rows = append(m.rows, *v)
	return nil
}

// marginEnv — фикстура, флаги контура коммерции включены, политика — в памяти, набор tune — свой.
func marginEnv(t *testing.T) (*Services, *memMarginStore) {
	t.Helper()
	svc := importedWith(t, Features{Hardware: true, Companions: true, MarginPolicy: true})
	svc.source = evalSource{MockJSON: fixtureSource(), queries: []domain.EvalQuery{
		{ID: "u-1", Text: "Kaspersky Internet", Type: domain.QueryExact, Set: domain.EvalSetTune, Freq: 1,
			Judgments: map[string]int{"kis-2dev-1y": 3, "kis-5dev-1y": 3, "ksos-10dev-1y": 1}},
		{ID: "u-2", Text: "графический редактор", Type: domain.QueryPurpose, Set: domain.EvalSetTune, Freq: 1,
			Judgments: map[string]int{"photoshop-1u-1y": 3, "fotomaster-pro-perp": 2}},
		{ID: "u-3", Text: "офис", Type: domain.QuerySlang, Set: domain.EvalSetTune, Freq: 1,
			Judgments: map[string]int{"office-2021-hb": 3, "myoffice-std-perp": 3}},
		{ID: "u-4", Text: "антивирус", Type: domain.QueryPurpose, Set: domain.EvalSetTune, Freq: 1,
			Judgments: map[string]int{"kis-2dev-1y": 2, "ksos-10dev-1y": 2}},
	}}
	store := &memMarginStore{}
	svc.margin().store = store
	return svc, store
}

func liveIDs(t *testing.T, svc *Services, q string, opts SearchOptions) []string {
	t.Helper()
	resp, err := svc.SearchWith(t.Context(), "r", in.SearchRequest{Query: q, Config: domain.ConfigLive}, opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range resp.Trace.Steps {
		if _, ok := st.(*domain.MarginStep); ok {
			t.Fatalf("шаг маржи попал в ответ витрины")
		}
	}
	ids := []string{}
	for _, it := range resp.Items {
		ids = append(ids, it.ProductID)
	}
	return ids
}

// Вес 0.05 — выдача контура поиска (как с выключенным флагом); положения рычага не трогают точный запрос.
func TestMarginLiveBaseIsPackage1(t *testing.T) {
	svc, _ := marginEnv(t)
	off := importedWith(t, Features{})
	for _, q := range []string{"касперский", "фотошоп", "графический редактор", "офис", "антивирус", "Kaspersky Internet"} {
		want := liveIDs(t, off, q, SearchOptions{})
		if got := liveIDs(t, svc, q, SearchOptions{}); !slices.Equal(got, want) {
			t.Fatalf("%q: политика по умолчанию меняет выдачу: %v vs %v", q, got, want)
		}
		if got := liveIDs(t, svc, q, MarginC3(SearchOptions{}, marginBaseWeight)); !slices.Equal(got, want) {
			t.Fatalf("%q: вес 0.05 меняет выдачу: %v vs %v", q, got, want)
		}
		for _, w := range marginPositions {
			got := liveIDs(t, svc, q, MarginC3(SearchOptions{}, w))
			if len(got) != len(want) || len(want) > 0 && q == "Kaspersky Internet" && got[0] != want[0] {
				t.Fatalf("%q вес %.2f: %v vs %v", q, w, got, want)
			}
		}
	}
}

func TestMarginPolicyApplyAndRollback(t *testing.T) {
	svc, store := marginEnv(t)
	p, err := svc.MarginPolicy(t.Context())
	if err != nil || p.Version != 0 || p.Weight != marginBaseWeight || len(p.Positions) != 6 {
		t.Fatalf("исходная политика: %+v %v", p, err)
	}
	bad := 0.07
	if _, err := svc.ApplyMarginPolicy(t.Context(), in.MarginPolicyRequest{Weight: &bad}); !isErrType(err, errs.ErrTypeValidation) {
		t.Fatalf("вес не из положений: %v", err)
	}
	sim, err := svc.SimulateMargin(t.Context(), in.MarginSimulateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range sim.Items {
		if it.ExactTop1Changed != 0 {
			t.Fatalf("точный запрос сменил топ-1 при весе %.2f", it.Weight)
		}
		if it.Available && it.ShareOverEps != 0 {
			t.Fatalf("доступное положение выводит запросы за ε: %+v", it)
		}
	}
	w := sim.StopWeight
	if sameWeight(w, marginBaseWeight) {
		w = sim.MinWeight
	}
	if sameWeight(w, marginBaseWeight) {
		t.Skip("на фикстуре все положения кроме базового за упором")
	}
	before := liveIDs(t, svc, "графический редактор", SearchOptions{})
	res, err := svc.ApplyMarginPolicy(t.Context(), in.MarginPolicyRequest{Weight: &w, Comment: "тест"})
	if err != nil || res.Version != 1 || res.VersionBefore != 0 || len(store.rows) != 1 || store.rows[0].Kind != domain.ConfigKindMargin {
		t.Fatalf("применение: %+v %v", res, err)
	}
	var r domain.MarginPolicyRules
	_ = json.Unmarshal(store.rows[0].Rules, &r)
	if r.Weight != w {
		t.Fatalf("вес в версии: %+v", r)
	}
	if _, err := svc.ApplyMarginPolicy(t.Context(), in.MarginPolicyRequest{Weight: &w}); !isErrType(err, errs.ErrTypeConflict) {
		t.Fatalf("то же положение: %v", err)
	}
	zero := 0
	back, err := svc.RollbackMarginPolicy(t.Context(), in.MarginRollbackRequest{ToVersion: &zero})
	if err != nil || back.Version != 2 || back.Weight != marginBaseWeight {
		t.Fatalf("откат: %+v %v", back, err)
	}
	if got := liveIDs(t, svc, "графический редактор", SearchOptions{}); !slices.Equal(got, before) {
		t.Fatalf("откат не вернул выдачу: %v vs %v", got, before)
	}
	missing := 9
	if _, err := svc.RollbackMarginPolicy(t.Context(), in.MarginRollbackRequest{ToVersion: &missing}); !isErrType(err, errs.ErrTypeNotFound) {
		t.Fatalf("нет версии: %v", err)
	}
}

func TestMarginPolicyWithoutStore(t *testing.T) {
	svc := importedWith(t, Features{MarginPolicy: true})
	w := 0.08
	if _, err := svc.ApplyMarginPolicy(t.Context(), in.MarginPolicyRequest{Weight: &w}); !isErrType(err, errs.ErrTypeUnavailable) {
		t.Fatalf("без базы: %v", err)
	}
}

func isErrType(err error, typ errs.Type) bool {
	var e *errs.Error
	return errors.As(err, &e) && e.Type == typ
}
