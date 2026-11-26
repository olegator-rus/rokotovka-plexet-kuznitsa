package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
	"kuznica/api/repositories"
)

// Версии политики маржи: config_versions kind=margin. Версия 0 — исходная (вес контура поиска, в базе её нет).
// Применение и откат создают новую версию; история не стирается.

// Тексты отказов политики маржи.
const (
	MsgMarginNoStore   = "Политика маржи хранится в базе — база не подключена"
	MsgMarginWeight    = "Вес — одно из положений рычага: 0, 0,03, 0,05, 0,08, 0,12, 0,15"
	MsgMarginSame      = "Это положение уже действует"
	MsgMarginBeyond    = "Положение за упором рычага: на размеченном наборе запросы теряют качество больше бюджета"
	MsgMarginConflict  = "Политику маржи только что изменили — обновите экран"
	MsgMarginNoVersion = "Такой версии политики маржи нет"
	MsgMarginDisabled  = "Политика маржи выключена (FEATURE_MARGIN_POLICY=off)"
)

// marginPolicyTTL — сколько поиск держит прочитанную политику без похода в базу.
const marginPolicyTTL = 15 * time.Second

// marginStore — хранилище версий политики (repositories.Repos; в тестах — своё).
type marginStore interface {
	LatestMarginPolicy(ctx context.Context) (*domain.ConfigVersion, error)
	ListMarginPolicies(ctx context.Context) ([]domain.ConfigVersion, error)
	GetMarginPolicy(ctx context.Context, version int) (*domain.ConfigVersion, error)
	CreateMarginPolicy(ctx context.Context, base int, v *domain.ConfigVersion) error
}

// activeMargin — действующая политика.
type activeMargin struct {
	Version int
	Weight  float64
}

// marginState — кэш политики, хранилище в тестах и состояние симуляции на граф сервисов.
type marginState struct {
	mu       sync.Mutex
	policy   *activeMargin
	loadedAt time.Time
	store    marginStore // тесты; nil — repos
	sim      marginSimState
}

var marginStates sync.Map

func (s *Services) margin() *marginState {
	v, _ := marginStates.LoadOrStore(s, &marginState{})
	return v.(*marginState)
}

func (s *Services) marginStore() marginStore {
	if st := s.margin().store; st != nil {
		return st
	}
	if !s.repos.Enabled() {
		return nil
	}
	return s.repos
}

// marginRulesOf — вес из версии.
func marginRulesOf(v *domain.ConfigVersion) (float64, error) {
	var r domain.MarginPolicyRules
	if err := json.Unmarshal(v.Rules, &r); err != nil {
		return marginBaseWeight, fmt.Errorf("политика маржи v%d: %w", v.Version, err)
	}
	return r.Weight, nil
}

// activeMarginPolicy — действующая политика из кэша (TTL) или базы. Без базы — исходная 0.05; ошибка чтения —
// прежняя (или исходная) и ошибка.
func (s *Services) activeMarginPolicy(ctx context.Context) (activeMargin, error) {
	ms := s.margin()
	ms.mu.Lock()
	if ms.policy != nil && s.now().Sub(ms.loadedAt) < marginPolicyTTL {
		p := *ms.policy
		ms.mu.Unlock()
		return p, nil
	}
	ms.mu.Unlock()
	return s.reloadMarginPolicy(ctx)
}

func (s *Services) reloadMarginPolicy(ctx context.Context) (activeMargin, error) {
	ms := s.margin()
	base := activeMargin{Weight: marginBaseWeight}
	st := s.marginStore()
	if st == nil {
		return base, nil
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	v, err := st.LatestMarginPolicy(ctx)
	p := base
	if err == nil && v != nil {
		p.Version = v.Version
		p.Weight, err = marginRulesOf(v)
	}
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if err != nil {
		if ms.policy != nil {
			return *ms.policy, err
		}
		return base, err
	}
	ms.policy, ms.loadedAt = &p, s.now()
	return p, nil
}

// MarginPolicy — GET /api/margin/policy.
func (s *Services) MarginPolicy(ctx context.Context) (*out.MarginPolicy, error) {
	p, err := s.reloadMarginPolicy(ctx)
	if err != nil {
		return nil, err
	}
	res := &out.MarginPolicy{Version: p.Version, Weight: p.Weight, BaseWeight: marginBaseWeight, Positions: MarginPositions(),
		Guards:  out.MarginGuards{ExactQuery: true, ExactTop1: true, GroupLock: true, Compatibility: true, MaxShift: marginMaxShift},
		Epsilon: marginEpsilon, MaxShift: marginMaxShift, Synthetic: s.marginSynthetic(), Enabled: s.features.MarginPolicy,
		Versions: []out.MarginVersion{}}
	if sim := s.margin().sim.lastResult(s.marginSimKey()); sim != nil {
		w := sim.StopWeight
		res.StopWeight = &w
	}
	if st := s.marginStore(); st != nil {
		rows, err := st.ListMarginPolicies(ctx)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			w, _ := marginRulesOf(&r)
			at := r.CreatedAt
			res.Versions = append(res.Versions, out.MarginVersion{Version: r.Version, Weight: w, Source: r.Source,
				Author: r.Author, Comment: r.Comment, CreatedAt: &at})
		}
	}
	res.Versions = append(res.Versions, out.MarginVersion{Version: 0, Weight: marginBaseWeight, Source: "default",
		Author: "system", Comment: "Исходная политика: вес маржи контура поиска"})
	return res, nil
}

// marginSynthetic — маржа в данных синтетическая (catalog.synthetic).
func (s *Services) marginSynthetic() bool {
	st := s.catalog.Load()
	return st != nil && st.commerce != nil && slices.Contains(st.commerce.synthetic, "margin")
}

// ApplyMarginPolicy — POST /api/margin/policy: новое положение рычага новой версией. Положение за упором
// (по симуляции на tune) не применяется.
func (s *Services) ApplyMarginPolicy(ctx context.Context, req in.MarginPolicyRequest) (*out.MarginApplied, error) {
	if req.Weight == nil || !validMarginWeight(*req.Weight) {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(errs.FieldError{
			Field: "weight", Code: errs.CodeInvalidFormat, Message: MsgMarginWeight})
	}
	w := *req.Weight
	comment := fmt.Sprintf("Рычаг маржи: вес %s", marginWeightLabel(w))
	if c := strings.TrimSpace(req.Comment); c != "" {
		comment += ": " + truncateRunes(c, 300)
	}
	return s.setMarginWeight(ctx, w, "human", comment, nil)
}

// RollbackMarginPolicy — POST /api/margin/policy/rollback: новая версия = копия выбранной (0 — исходная).
func (s *Services) RollbackMarginPolicy(ctx context.Context, req in.MarginRollbackRequest) (*out.MarginApplied, error) {
	if req.ToVersion == nil || *req.ToVersion < 0 {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(errs.FieldError{
			Field: "to_version", Code: errs.CodeInvalidFormat, Message: "Укажите номер версии"})
	}
	to := *req.ToVersion
	st := s.marginStore()
	if st == nil {
		return nil, errs.NewUnavailableError(MsgMarginNoStore)
	}
	w := marginBaseWeight
	if to > 0 {
		v, err := st.GetMarginPolicy(ctx, to)
		if err != nil {
			return nil, err
		}
		if v == nil {
			return nil, errs.NewNotFoundError(MsgMarginNoVersion)
		}
		if w, err = marginRulesOf(v); err != nil {
			return nil, err
		}
	}
	comment := fmt.Sprintf("Откат к версии %d (вес %s)", to, marginWeightLabel(w))
	if c := strings.TrimSpace(req.Comment); c != "" {
		comment += ": " + truncateRunes(c, 300)
	}
	return s.setMarginWeight(ctx, w, "rollback", comment, &to)
}

// setMarginWeight — новая версия политики поверх действующей.
func (s *Services) setMarginWeight(ctx context.Context, w float64, source, comment string, rolledTo *int) (*out.MarginApplied, error) {
	st := s.marginStore()
	if st == nil {
		return nil, errs.NewUnavailableError(MsgMarginNoStore)
	}
	cur, err := s.reloadMarginPolicy(ctx)
	if err != nil {
		return nil, err
	}
	if rolledTo != nil && *rolledTo == cur.Version {
		return nil, errs.NewConflictError("Эта версия политики уже действует")
	}
	if rolledTo == nil && sameWeight(cur.Weight, w) {
		return nil, errs.NewConflictError(MsgMarginSame)
	}
	// Упор рычага: положение, при котором размеченные запросы теряют больше ε, не применяется. Откат к прежней
	// версии разрешён всегда — это возврат к уже действовавшей выдаче.
	if rolledTo == nil && !sameWeight(w, marginBaseWeight) {
		sim, err := s.SimulateMargin(ctx, in.MarginSimulateRequest{})
		if err != nil {
			return nil, err
		}
		for _, it := range sim.Items {
			if sameWeight(it.Weight, w) && !it.Available {
				return nil, errs.NewValidationError(MsgMarginBeyond)
			}
		}
	}
	rules, _ := json.Marshal(domain.MarginPolicyRules{Weight: w})
	diff, _ := json.Marshal(map[string]float64{"weight_before": cur.Weight, "weight_after": w})
	v := &domain.ConfigVersion{Kind: domain.ConfigKindMargin, Source: source, Author: "human", Comment: comment,
		Rules: rules, Diff: diff}
	if st := s.catalog.Load(); st != nil {
		v.CatalogVersion = st.version
	}
	if err := st.CreateMarginPolicy(ctx, cur.Version, v); err != nil {
		if errors.Is(err, repositories.ErrMarginVersionConflict) {
			return nil, errs.NewConflictError(MsgMarginConflict)
		}
		return nil, err
	}
	ms := s.margin()
	ms.mu.Lock()
	ms.policy, ms.loadedAt = &activeMargin{Version: v.Version, Weight: w}, s.now()
	ms.mu.Unlock()
	return &out.MarginApplied{Version: v.Version, Weight: w, VersionBefore: cur.Version, WeightBefore: cur.Weight,
		RolledBackTo: rolledTo}, nil
}

func sameWeight(a, b float64) bool { return a-b < 1e-9 && b-a < 1e-9 }

// marginWeightLabel — вес по-русски: 0,08.
func marginWeightLabel(w float64) string {
	return strings.Replace(fmt.Sprintf("%.2f", w), ".", ",", 1)
}
