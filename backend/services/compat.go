package services

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"kuznica/api/domain"
)

// Совместимость в четырёх состояниях: verified,
// conditional, incompatible, unknown. Пара без правила — unknown («не подтверждена», не равно «совместимо»).
// Функции чистые, читают только справочники снимка; их используют блок допродажи, поиск оборудования и агент
// (check_compatibility, check_license_rules, check_relation_integrity).

// Откуда взято состояние совместимости (CompatResult.Basis).
const (
	CompatBasisModelRule   = "compatibility_rule" // compatibility_rules.json: позиция или семейство × модель
	CompatBasisLicenseRule = "license_rule"       // license_rules.json: версия, редакция требуемого
	CompatBasisRelation    = "relation"           // поле compatibility связи
	CompatBasisNone        = "none"               // ничего не известно → unknown
)

// CompatResult — состояние совместимости и его основание.
type CompatResult struct {
	State string `json:"state"` // verified | conditional | incompatible | unknown
	// Condition — условие для conditional или причина incompatible («версия CAL 2022 ниже версии сервера 2025»).
	Condition string `json:"condition"`
	Basis     string `json:"basis"`   // compatibility_rule | license_rule | relation | none
	RuleID    string `json:"rule_id"` // id правила или связи; "" — без основания
	Source    string `json:"source"`
	SourceURL string `json:"source_url"`
}

// Allowed — можно показывать покупателю (incompatible — никогда).
func (r CompatResult) Allowed() bool { return r.State != domain.CompatIncompatible }

func unknownCompat() CompatResult {
	return CompatResult{State: domain.CompatUnknown, Basis: CompatBasisNone}
}

// compatIndex — то, что совместимости нужно от позиций: строковые атрибуты (версия, редакция) и позиции по
// семейству и категории. Строится вместе с commerceCatalog, неизменяем.
type compatIndex struct {
	attrs      map[string]map[string]string // product_id → атрибут → значение (строкой, в нижнем регистре ключ)
	byFamily   map[string][]*domain.Product
	byCategory map[string][]*domain.Product
	// modelRules — правила совместимости по целевой модели: "p:<product>|<model>" и "f:<family>|<model>".
	modelRules map[string]domain.CompatibilityRule
}

func newCompatIndex(c *commerceCatalog) *compatIndex {
	ix := &compatIndex{
		attrs:      make(map[string]map[string]string, len(c.products)),
		byFamily:   map[string][]*domain.Product{},
		byCategory: map[string][]*domain.Product{},
		modelRules: make(map[string]domain.CompatibilityRule, len(c.compatRules)),
	}
	for _, p := range c.products {
		ix.byFamily[p.FamilyID] = append(ix.byFamily[p.FamilyID], p)
		ix.byCategory[p.CategoryID] = append(ix.byCategory[p.CategoryID], p)
		ix.attrs[p.ID] = productAttrs(p)
	}
	for _, r := range c.compatRules {
		switch {
		case r.ProductID != "":
			ix.modelRules["p:"+r.ProductID+"|"+r.TargetModel] = r
		case r.FamilyID != "":
			ix.modelRules["f:"+r.FamilyID+"|"+r.TargetModel] = r
		}
	}
	// Порядок позиций в семействе и категории — по id: результат не зависит от порядка обхода карты.
	for _, m := range []map[string][]*domain.Product{ix.byFamily, ix.byCategory} {
		for k := range m {
			ps := m[k]
			sortProductsByID(ps)
		}
	}
	return ix
}

func sortProductsByID(ps []*domain.Product) {
	for i := 1; i < len(ps); i++ {
		for j := i; j > 0 && ps[j].ID < ps[j-1].ID; j-- {
			ps[j], ps[j-1] = ps[j-1], ps[j]
		}
	}
}

// productAttrs — строковые атрибуты позиции: attributes (числа — строкой) и редакция лицензии, если в
// attributes её нет.
func productAttrs(p *domain.Product) map[string]string {
	out := map[string]string{}
	if len(p.Attributes) > 0 {
		var raw map[string]any
		if json.Unmarshal(p.Attributes, &raw) == nil {
			for k, v := range raw {
				switch x := v.(type) {
				case string:
					out[k] = x
				case float64:
					out[k] = strconv.FormatFloat(x, 'f', -1, 64)
				case bool:
					out[k] = strconv.FormatBool(x)
				}
			}
		}
	}
	if out["edition"] == "" && p.License.Edition != "" {
		out["edition"] = p.License.Edition
	}
	return out
}

// attr — атрибут позиции; нет — "".
func (c *commerceCatalog) attr(productID, key string) string {
	if c == nil || c.compat == nil {
		return ""
	}
	return c.compat.attrs[productID][key]
}

// ModelCompat — совместимость позиции с моделью системы: правило позиции, затем правило её семейства; правила
// нет — unknown. Поле compatible_with карточки доказательством не считается (оно выводится из правил).
func (c *commerceCatalog) ModelCompat(productID, targetModel string) CompatResult {
	if c == nil || c.compat == nil || targetModel == "" {
		return unknownCompat()
	}
	r, ok := c.compat.modelRules["p:"+productID+"|"+targetModel]
	if !ok {
		if p := c.products[productID]; p != nil {
			r, ok = c.compat.modelRules["f:"+p.FamilyID+"|"+targetModel]
		}
	}
	if !ok {
		return unknownCompat()
	}
	return CompatResult{State: r.State, Condition: r.Condition, Basis: CompatBasisModelRule, RuleID: r.ID,
		Source: r.Source, SourceURL: r.SourceURL}
}

// licenseRulesFor — правила лицензий, которые относятся к якорю: семейство, позиция (anchor_product) и
// атрибуты (anchor_attrs) совпадают.
func (c *commerceCatalog) licenseRulesFor(anchorID string) []domain.LicenseRule {
	if c == nil {
		return nil
	}
	a := c.products[anchorID]
	if a == nil {
		return nil
	}
	var out []domain.LicenseRule
	for _, lr := range c.licenseByAnchor[a.FamilyID] {
		if lr.AnchorProduct != "" && lr.AnchorProduct != anchorID {
			continue
		}
		ok := true
		for k, v := range lr.AnchorAttrs {
			if !strings.EqualFold(strings.TrimSpace(c.attr(anchorID, k)), strings.TrimSpace(v)) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, lr)
		}
	}
	return out
}

// ruleCovers — правило говорит о позиции-кандидате: её семейство (requires_family) или категория
// (requires_category) и все условия requires_attrs, кроме сравнения с якорем («>=anchor.version»).
func (c *commerceCatalog) ruleCovers(lr domain.LicenseRule, candidateID string) bool {
	p := c.products[candidateID]
	if p == nil {
		return false
	}
	switch {
	case lr.RequiresFamily != "":
		if p.FamilyID != lr.RequiresFamily {
			return false
		}
	case lr.RequiresCategory != "":
		if p.CategoryID != lr.RequiresCategory {
			return false
		}
	default:
		return false
	}
	for k, want := range lr.RequiresAttrs {
		if strings.Contains(want, "anchor.") {
			continue
		}
		if !matchAlternatives(c.attr(candidateID, k), want) {
			return false
		}
	}
	return true
}

// matchAlternatives — значение совпадает с одной из альтернатив «User CAL|Device CAL» (без учёта регистра).
func matchAlternatives(have, want string) bool {
	for _, alt := range strings.Split(want, "|") {
		if strings.EqualFold(strings.TrimSpace(have), strings.TrimSpace(alt)) {
			return true
		}
	}
	return false
}

// ruleCheck — проверка кандидата, которого покрывает правило, условиями сравнения с якорем
// («version": ">=anchor.version"). Пройдено — verified по правилу вендора; не пройдено — incompatible с причиной.
func (c *commerceCatalog) ruleCheck(lr domain.LicenseRule, anchorID, candidateID string) CompatResult {
	res := CompatResult{State: domain.CompatVerified, Basis: CompatBasisLicenseRule, RuleID: lr.ID, Source: lr.Source,
		SourceURL: lr.SourceURL}
	for k, want := range lr.RequiresAttrs {
		i := strings.Index(want, "anchor.")
		if i < 0 {
			continue
		}
		op := strings.TrimSpace(want[:i])
		key := strings.TrimSpace(want[i+len("anchor."):])
		av, cv := c.attr(anchorID, key), c.attr(candidateID, k)
		if av == "" {
			continue // у якоря нет атрибута — сравнивать не с чем
		}
		if cv == "" || !compareAttr(cv, op, av) {
			res.State = domain.CompatIncompatible
			res.Condition = fmt.Sprintf("несовместимо по %s: у кандидата %s, нужно %s %s", attrWord(k), orDash(cv), op, av)
			return res
		}
	}
	return res
}

// compareAttr — сравнение значений атрибута: числа — как числа («2022» < «2025»), иначе — как строки.
func compareAttr(have, op, want string) bool {
	hf, e1 := strconv.ParseFloat(strings.TrimSpace(have), 64)
	wf, e2 := strconv.ParseFloat(strings.TrimSpace(want), 64)
	cmp := strings.Compare(strings.ToLower(have), strings.ToLower(want))
	if e1 == nil && e2 == nil {
		switch {
		case hf < wf:
			cmp = -1
		case hf > wf:
			cmp = 1
		default:
			cmp = 0
		}
	}
	switch op {
	case ">=":
		return cmp >= 0
	case ">":
		return cmp > 0
	case "<=":
		return cmp <= 0
	case "<":
		return cmp < 0
	case "", "=", "==":
		return cmp == 0
	case "!=":
		return cmp != 0
	}
	return false
}

func attrWord(k string) string {
	switch k {
	case "version":
		return "версии"
	case "edition":
		return "редакции"
	}
	return "атрибуту " + k
}

// LicenseCompat — проверка пары «якорь → кандидат» правилами лицензий якоря. covered=false — ни одно правило
// о кандидате не говорит (состояние unknown). Если правил несколько, побеждает пройденное, из пройденных — самое
// точное (семейство и больше условий на атрибуты: правило RDS CAL точнее общего правила CAL).
func (c *commerceCatalog) LicenseCompat(anchorID, candidateID string) (CompatResult, *domain.LicenseRule, bool) {
	var best, failed *CompatResult
	var bestRule, failedRule *domain.LicenseRule
	bestSpec := -1
	for _, lr := range c.licenseRulesFor(anchorID) {
		if !c.ruleCovers(lr, candidateID) {
			continue
		}
		res := c.ruleCheck(lr, anchorID, candidateID)
		rule := lr
		if res.State == domain.CompatIncompatible {
			if failed == nil {
				failed, failedRule = &res, &rule
			}
			continue
		}
		if sp := ruleSpecificity(lr); sp > bestSpec {
			best, bestRule, bestSpec = &res, &rule, sp
		}
	}
	switch {
	case best != nil:
		return *best, bestRule, true
	case failed != nil:
		return *failed, failedRule, true
	}
	return unknownCompat(), nil, false
}

// ruleSpecificity — насколько точно правило называет требуемое: семейство и условия на атрибуты кандидата.
func ruleSpecificity(lr domain.LicenseRule) int {
	n := 0
	if lr.RequiresFamily != "" {
		n += 10
	}
	for _, v := range lr.RequiresAttrs {
		if !strings.Contains(v, "anchor.") {
			n++
		}
	}
	return n
}

// PairCompat — совместимость кандидата с якорем (позиция к позиции) — общая функция для блока допродажи,
// поиска «X для Y» и агента. Порядок оснований: правило совместимости с моделью якоря (если у якоря есть
// model_id) → правило лицензии якоря (версия, редакция) → поле compatibility связи rel (может быть nil) →
// unknown. Несовместимость по любому основанию сильнее «совместимо» из связи: связь, которая противоречит
// правилу вендора, — сломанная (BROKEN_RELATION).
func (c *commerceCatalog) PairCompat(anchorID, candidateID string, rel *domain.Relation) CompatResult {
	if c == nil {
		return unknownCompat()
	}
	if a := c.products[anchorID]; a != nil && a.ModelID != "" {
		r := c.ModelCompat(candidateID, a.ModelID)
		// Комплектующее к системе с моделью — только по правилу совместимости: связь семейства («модули из
		// списка совместимости») про конкретный модуль ничего не доказывает.
		if p := c.products[candidateID]; r.Basis != CompatBasisNone || p != nil && p.Kind == domain.KindHardware {
			return r
		}
	}
	if r, _, ok := c.LicenseCompat(anchorID, candidateID); ok {
		return r
	}
	if rel != nil && rel.Compatibility != "" {
		res := CompatResult{State: rel.Compatibility, Basis: CompatBasisRelation, RuleID: rel.RelID, Source: rel.Source,
			SourceURL: rel.SourceURL}
		if res.State == domain.CompatUnknown {
			res.Basis = CompatBasisNone
		}
		return res
	}
	return unknownCompat()
}

// CheckCompatibility — совместимость позиции с целью для агента и трека оборудования: target — id модели
// системы («dell-poweredge-r740») или id позиции-якоря. Каталог не загружен — unknown.
func (s *Services) CheckCompatibility(productID, target string) CompatResult {
	cs := s.catalog.Load()
	if cs == nil || cs.commerce == nil {
		return unknownCompat()
	}
	c := cs.commerce
	if a := c.products[target]; a != nil {
		return c.PairCompat(target, productID, c.relationBetween(target, productID))
	}
	return c.ModelCompat(productID, target)
}

// relationBetween — связь от позиции-якоря к позиции-кандидату (уточнённая до позиции — раньше семейной);
// нет — nil. Связи substitute и upgrade о совместимости не говорят и не учитываются.
func (c *commerceCatalog) relationBetween(anchorID, candidateID string) *domain.Relation {
	a, p := c.products[anchorID], c.products[candidateID]
	if a == nil || p == nil {
		return nil
	}
	var famLevel *domain.Relation
	for i := range c.relationsFrom[a.FamilyID] {
		r := &c.relationsFrom[a.FamilyID][i]
		if r.ToFamily != p.FamilyID || r.Type == domain.RelationSubstitute || r.Type == domain.RelationUpgrade {
			continue
		}
		if r.FromProduct != nil && *r.FromProduct != anchorID || r.ToProduct != nil && *r.ToProduct != candidateID {
			continue
		}
		if r.FromProduct != nil || r.ToProduct != nil {
			return r
		}
		if famLevel == nil {
			famLevel = r
		}
	}
	return famLevel
}
