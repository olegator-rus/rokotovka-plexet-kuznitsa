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
	// Categories — вес по категории позиции поверх Weight; nil — нет.
	Categories map[string]float64
}

// weights — веса для перестановки.
func (p activeMargin) weights() marginWeights {
	return marginWeights{global: p.Weight, cats: p.Categories}
}

// catsKey — отпечаток весов по категориям (ключ кэша симуляции: её числа зависят от действующих весов).
func (p activeMargin) catsKey() string {
	if len(p.Categories) == 0 {
		return ""
	}
	ks := make([]string, 0, len(p.Categories))
	for k, w := range p.Categories {
		ks = append(ks, fmt.Sprintf("%s=%.4f", k, w))
	}
	slices.Sort(ks)
	return strings.Join(ks, ",")
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
	p, err := marginPolicyOf(v)
	return p.Weight, err
}

// marginPolicyOf — политика из версии (вес и веса по категориям).
func marginPolicyOf(v *domain.ConfigVersion) (activeMargin, error) {
	var r domain.MarginPolicyRules
	if err := json.Unmarshal(v.Rules, &r); err != nil {
		return activeMargin{Version: v.Version, Weight: marginBaseWeight}, fmt.Errorf("политика маржи v%d: %w", v.Version, err)
	}
	p := activeMargin{Version: v.Version, Weight: r.Weight}
	if len(r.Categories) > 0 {
		p.Categories = r.Categories
	}
	return p, nil
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
		p, err = marginPolicyOf(v)
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

// categoryName — название категории каталога; нет — id.
func (s *Services) categoryName(id string) string {
	if st := s.catalog.Load(); st != nil && st.lex != nil {
		if n := st.lex.categoryName[id]; n != "" {
			return n
		}
	}
	return id
}

// knownCategory — категория есть в каталоге (у позиций или в справочнике).
func (s *Services) knownCategory(id string) bool {
	st := s.catalog.Load()
	if st == nil || st.lex == nil {
		return false
	}
	if _, ok := st.lex.categoryName[id]; ok {
		return true
	}
	for _, d := range st.lex.docs {
		if d.CategoryID == id {
			return true
		}
	}
	return false
}

// MarginPolicy — GET /api/margin/policy.
func (s *Services) MarginPolicy(ctx context.Context) (*out.MarginPolicy, error) {
	p, err := s.reloadMarginPolicy(ctx)
	if err != nil {
		return nil, err
	}
	res := &out.MarginPolicy{Version: p.Version, Weight: p.Weight, Categories: map[string]float64{},
		CategoryNames: map[string]string{}, BaseWeight: marginBaseWeight, Positions: MarginPositions(),
		Guards:  out.MarginGuards{ExactQuery: true, ExactTop1: true, GroupLock: true, Compatibility: true, MaxShift: marginMaxShift},
		Epsilon: marginEpsilon, MaxShift: marginMaxShift, Synthetic: s.marginSynthetic(), Enabled: s.features.MarginPolicy,
		Versions: []out.MarginVersion{}}
	for k, w := range p.Categories {
		res.Categories[k], res.CategoryNames[k] = w, s.categoryName(k)
	}
	if sim := s.margin().sim.lastResult(s.marginResultKey(s.marginSimKey(), p)); sim != nil {
		w := sim.StopWeight
		res.StopWeight = &w
	}
	if st := s.marginStore(); st != nil {
		rows, err := st.ListMarginPolicies(ctx)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			rp, _ := marginPolicyOf(&r)
			at := r.CreatedAt
			res.Versions = append(res.Versions, out.MarginVersion{Version: r.Version, Weight: rp.Weight, Categories: rp.Categories,
				ProposalID: r.ProposalID, Source: r.Source, Author: r.Author, Comment: r.Comment, CreatedAt: &at})
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

// Отказы применения весов по категориям и предложения агента.
const (
	MsgMarginNoChange       = "Укажите вес или веса по категориям"
	MsgMarginCategory       = "Такой категории нет в каталоге"
	MsgMarginProposal       = "Это не предложение по марже"
	MsgMarginProposalChange = "Настройка отличается от предложения агента — примените её без предложения или откройте предложение заново"
)

// ApplyMarginPolicy — POST /api/margin/policy: новое положение рычага (общий вес и/или веса по категориям) новой
// версией. Положение за упором (по симуляции на tune) не применяется. С proposal_id — применяется предложение
// агента MARGIN_OPPORTUNITY: настройка должна совпасть с предложенной, предложение становится принятым.
func (s *Services) ApplyMarginPolicy(ctx context.Context, req in.MarginPolicyRequest) (*out.MarginApplied, error) {
	if req.Weight == nil && req.Categories == nil {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(errs.FieldError{
			Field: "weight", Code: errs.CodeInvalidFormat, Message: MsgMarginNoChange})
	}
	if req.Weight != nil && !validMarginWeight(*req.Weight) {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(errs.FieldError{
			Field: "weight", Code: errs.CodeInvalidFormat, Message: MsgMarginWeight})
	}
	for k, w := range req.Categories {
		if !validMarginWeight(w) {
			return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(errs.FieldError{
				Field: "categories." + k, Code: errs.CodeInvalidFormat, Message: MsgMarginWeight})
		}
		if !s.knownCategory(k) {
			return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(errs.FieldError{
				Field: "categories." + k, Code: errs.CodeInvalidFormat, Message: MsgMarginCategory})
		}
	}
	if s.marginStore() == nil {
		return nil, errs.NewUnavailableError(MsgMarginNoStore)
	}
	cur, err := s.reloadMarginPolicy(ctx)
	if err != nil {
		return nil, err
	}
	next := activeMargin{Weight: cur.Weight, Categories: cur.Categories}
	if req.Weight != nil {
		next.Weight = *req.Weight
	}
	if req.Categories != nil {
		next.Categories = req.Categories
	}
	next.Categories = normalizeMarginCats(next.Categories, next.Weight)
	var prop *domain.Proposal
	if id := strings.TrimSpace(req.ProposalID); id != "" {
		if prop, err = s.marginProposalFor(ctx, id, cur, next); err != nil {
			return nil, err
		}
	}
	comment := marginPolicyComment(s, next)
	if c := strings.TrimSpace(req.Comment); c != "" {
		comment += ": " + truncateRunes(c, 300)
	}
	source := "human"
	if prop != nil {
		source = "agent"
	}
	return s.setMarginPolicy(ctx, next, source, comment, nil, prop)
}

// normalizeMarginCats — веса по категориям без совпадающих с общим (они ничего не меняют); пусто — nil.
func normalizeMarginCats(cats map[string]float64, global float64) map[string]float64 {
	var res map[string]float64
	for k, w := range cats {
		if sameWeight(w, global) {
			continue
		}
		if res == nil {
			res = map[string]float64{}
		}
		res[k] = w
	}
	return res
}

// marginPolicyComment — подпись версии: «Рычаг маржи: вес 0,05; Антивирусы для бизнеса 0,08».
func marginPolicyComment(s *Services, p activeMargin) string {
	c := fmt.Sprintf("Рычаг маржи: вес %s", marginWeightLabel(p.Weight))
	ks := make([]string, 0, len(p.Categories))
	for k := range p.Categories {
		ks = append(ks, k)
	}
	slices.Sort(ks)
	for _, k := range ks {
		c += fmt.Sprintf("; %s %s", s.categoryName(k), marginWeightLabel(p.Categories[k]))
	}
	return c
}

func sameMarginCats(a, b map[string]float64) bool {
	if len(a) != len(b) {
		return false
	}
	for k, w := range a {
		if v, ok := b[k]; !ok || !sameWeight(v, w) {
			return false
		}
	}
	return true
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
	p := activeMargin{Weight: marginBaseWeight}
	if to > 0 {
		v, err := st.GetMarginPolicy(ctx, to)
		if err != nil {
			return nil, err
		}
		if v == nil {
			return nil, errs.NewNotFoundError(MsgMarginNoVersion)
		}
		if p, err = marginPolicyOf(v); err != nil {
			return nil, err
		}
	}
	comment := fmt.Sprintf("Откат к версии %d (%s)", to, strings.TrimPrefix(marginPolicyComment(s, p), "Рычаг маржи: "))
	if c := strings.TrimSpace(req.Comment); c != "" {
		comment += ": " + truncateRunes(c, 300)
	}
	return s.setMarginPolicy(ctx, p, "rollback", comment, &to, nil)
}

// setMarginPolicy — новая версия политики поверх действующей; prop — принимаемое предложение агента.
func (s *Services) setMarginPolicy(ctx context.Context, next activeMargin, source, comment string, rolledTo *int,
	prop *domain.Proposal) (*out.MarginApplied, error) {
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
	if rolledTo == nil && sameWeight(cur.Weight, next.Weight) && sameMarginCats(cur.Categories, next.Categories) {
		return nil, errs.NewConflictError(MsgMarginSame)
	}
	// Упор рычага: положение, при котором размеченные запросы теряют больше ε, не применяется. Откат к прежней
	// версии разрешён всегда — это возврат к уже действовавшей выдаче.
	if rolledTo == nil {
		if err := s.checkMarginStops(ctx, cur, next); err != nil {
			return nil, err
		}
	}
	rules, _ := json.Marshal(domain.MarginPolicyRules{Weight: next.Weight, Categories: next.Categories})
	diff, _ := json.Marshal(map[string]any{"weight_before": cur.Weight, "weight_after": next.Weight,
		"categories_before": nonNilCats(cur.Categories), "categories_after": nonNilCats(next.Categories)})
	author := "human"
	v := &domain.ConfigVersion{Kind: domain.ConfigKindMargin, Source: source, Author: author, Comment: comment,
		Rules: rules, Diff: diff}
	if prop != nil {
		pid := prop.ID.String()
		v.ProposalID, v.Author = &pid, "agent+human"
	}
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
	ms.policy, ms.loadedAt = &activeMargin{Version: v.Version, Weight: next.Weight, Categories: next.Categories}, s.now()
	ms.mu.Unlock()
	res := &out.MarginApplied{Version: v.Version, Weight: next.Weight, VersionBefore: cur.Version, WeightBefore: cur.Weight,
		RolledBackTo: rolledTo, Categories: nonNilCats(next.Categories), CategoriesBefore: nonNilCats(cur.Categories)}
	if prop != nil {
		s.acceptMarginProposal(ctx, prop, v.Version)
		res.ProposalID = v.ProposalID
	}
	return res, nil
}

// checkMarginStops — новое положение не за упором: общий вес — по симуляции на tune (как раньше); изменённый
// вес категории — по симуляции этой категории (остальные позиции — по действующей политике).
func (s *Services) checkMarginStops(ctx context.Context, cur, next activeMargin) error {
	if !sameWeight(next.Weight, cur.Weight) && !sameWeight(next.Weight, marginBaseWeight) {
		sim, err := s.SimulateMargin(ctx, in.MarginSimulateRequest{})
		if err != nil {
			return err
		}
		for _, it := range sim.Items {
			if sameWeight(it.Weight, next.Weight) && !it.Available {
				return errs.NewValidationError(MsgMarginBeyond)
			}
		}
	}
	for k, w := range next.Categories {
		if old, ok := cur.Categories[k]; ok && sameWeight(old, w) || sameWeight(w, marginBaseWeight) {
			continue
		}
		sim, err := s.SimulateMargin(ctx, in.MarginSimulateRequest{Category: k, Positions: []float64{w}})
		if err != nil {
			return err
		}
		for _, it := range sim.Items {
			if sameWeight(it.Weight, w) && !it.Available {
				return errs.NewValidationError(MsgMarginBeyond + " (" + s.categoryName(k) + ")")
			}
		}
	}
	return nil
}

func nonNilCats(m map[string]float64) map[string]float64 {
	if m == nil {
		return map[string]float64{}
	}
	return m
}

func sameWeight(a, b float64) bool { return a-b < 1e-9 && b-a < 1e-9 }

// marginWeightLabel — вес по-русски: 0,08.
func marginWeightLabel(w float64) string {
	return strings.Replace(fmt.Sprintf("%.2f", w), ".", ",", 1)
}
