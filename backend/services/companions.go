package services

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Допродажа в выдаче.
// Подбор идёт после основной выдачи и вне BM25/векторов: от уверенного якоря по связям справочника, с проверкой
// правилами лицензий и совместимости (compat.go). LLM не вызывается; маржа и популярность на попадание
// обязательного не влияют — популярность только упорядочивает позиции внутри семейства и класса.

// StepCompanions — шаг трассы «Компаньоны».
const StepCompanions = "companions"

// Классы компаньона (Companion.Class).
const (
	CompanionRequired   = domain.RequirementConditional // conditional_required — строка в карточке якоря
	CompanionCompatible = "compatible"
	CompanionComplement = "complement"
	CompanionRenewal    = "renewal"
)

// Подписи классов (Companion.Label).
const (
	labelRequired   = "Проверьте необходимость"
	labelComplement = "Дополняет решение"
	labelRenewal    = "Продление и поддержка"
)

// maxCompanions — позиций в блоке «Дополните решение».
const maxCompanions = 4

// Условия, которые проверяются по атрибутам якоря (остальные — переключатели покупателя на фронте).
const condNoPreinstalledOS = "no_preinstalled_os"

// condCheckCompatRule — связь требует правила совместимости с моделью: без правила (unknown) не показывается.
const condCheckCompatRule = "check_compatibility_rule"

// CompanionsStep — шаг трассы companions: якорь, рассмотренные кандидаты, отклонённые с причиной.
type CompanionsStep struct {
	domain.StepBase
	Error      string                `json:"error,omitempty"`
	Anchor     *out.CompanionAnchor  `json:"anchor"`
	Considered []CompanionConsidered `json:"considered"`
	Rejected   []domain.RejectedHit  `json:"rejected"`
}

// CompanionConsidered — кандидат: класс, источник (relation:<id> | license_rule:<id>) и совместимость.
type CompanionConsidered struct {
	ProductID     string `json:"product_id"`
	Class         string `json:"class"`
	Source        string `json:"source"`
	Compatibility string `json:"compatibility"`
	EvidenceLevel string `json:"evidence_level,omitempty"`
	Shown         bool   `json:"shown"`
}

// Причины отказа кандидату (Rejected.Reason).
const (
	rejectIncompatible   = "incompatible"
	rejectVersion        = "version_mismatch"
	rejectEdition        = "edition_mismatch"
	rejectUnavailable    = "unavailable"
	rejectNotReviewed    = "not_approved"
	rejectCondition      = "condition_unmet"
	rejectNoCompatRule   = "unknown_compat"
	rejectMissingRel     = "missing_relation"
	rejectAnchorUnsure   = "anchor_not_confident"
	rejectClassTaken     = "class_taken"
	rejectBlockFull      = "limit"
	rejectWeakerVariant  = "weaker_variant"
	rejectCompanionError = "error"
)

// companionQuery — что из запроса нужно подбору: количество и метрика («на 50 пользователей»).
type companionQuery struct {
	quantity int
	metric   string
	tokens   []string // слова запроса после нормализации — для выбора якоря
}

// companionResult — итог подбора для одного якоря.
type companionResult struct {
	anchor   *out.CompanionAnchor
	required []out.Companion
	block    *out.CompanionBlock
	step     CompanionsStep
}

// anchorScan — сколько первых семейств выдачи смотрит выбор якоря.
const anchorScan = 5

// anchorGap — другое названное семейство со скором ниже этой доли скора якоря — не соперник: «lenovo thinkpad
// e14» (0,89) против T14 (0,36) — якорь E14; «lenovo» (0,865 / 0,864) — неоднозначно.
const anchorGap = 0.85

// anchorNamed — доля названия семейства (по idf), с которой оно считается названным в запросе: «windows server
// 2025» без «microsoft» — 0,80.
const anchorNamed = 0.75

// anchorOf — уверенный якорь выдачи (и номер его позиции) или причина, почему его нет. Якорь — первое точное
// семейство, от которого не зависит другое точное семейство выдачи: «windows server 2025 на 50 пользователей»
// поднимает CAL выше сервера (количество совпало у CAL), но якорь — сервер, CAL — его обязательный компаньон.
// high — семейство названо в запросе и других названных семейств (не связанных с якорем) нет, или точный
// маршрут A; medium — вендор назван и точное семейство одно. Категорийный, неоднозначный запрос, аналоги,
// пустая выдача — без якоря.
func anchorOf(c *commerceCatalog, lx *searchLexicon, tokens []string, fams []*familyHit, route string, analog, clarify,
	empty bool) (*out.CompanionAnchor, int, string) {
	switch {
	case len(fams) == 0 || empty:
		return nil, -1, "пустая выдача"
	case analog:
		return nil, -1, "запрос аналога"
	case clarify:
		return nil, -1, "запрос неоднозначный — нужно уточнение"
	}
	if fams[0].group != domain.GroupExact {
		return nil, -1, "категорийный запрос — товар не назван"
	}
	scan := fams
	if len(scan) > anchorScan {
		scan = scan[:anchorScan]
	}
	// Первое семейство — обязательный компаньон другого семейства выдачи (CAL поднялся выше сервера): якорь —
	// то, чему он нужен.
	// Только если оно названо в запросе (покрытие названия ≥ anchorNamed): «cal» сервер якорем не делает.
	// Названы несколько («windows server 2025 и cal 2022») — то, чьё самое редкое слово раньше в запросе.
	idx, bestCov, bestPos := 0, 0.0, len(tokens)
	for i, fh := range scan {
		if i == 0 || !c.requiredFor(fh.id, fams[0].id) || lx == nil {
			continue
		}
		cov := lx.familyCoverage(fh.id, tokens)
		if cov < anchorNamed {
			continue
		}
		pos := lx.keyWordPos(fh.id, tokens)
		if cov > bestCov+1e-9 || math.Abs(cov-bestCov) <= 1e-9 && pos < bestPos {
			idx, bestCov, bestPos = i, cov, pos
		}
	}
	top := scan[idx]
	anchor := &out.CompanionAnchor{ProductID: top.head.ProductID, FamilyID: top.id, Confidence: "high"}
	if route == domain.RouteExact {
		return anchor, idx, ""
	}
	others, sameCat := 0, 0
	for i, fh := range scan {
		if i == idx || fh.group != domain.GroupExact || c.linkedFamilies(top.id, fh.id) || fh.tier < top.tier ||
			fh.final < anchorGap*top.final {
			continue
		}
		// Другой товар той же категории (E14 и T14) — альтернатива, а не второй запрос: якорь остаётся, но средний.
		if c.familyCategory(fh.id) == c.familyCategory(top.id) {
			sameCat++
		} else {
			others++
		}
	}
	switch {
	case others > 0:
	case top.tier >= 2 && sameCat == 0:
		return anchor, idx, ""
	default:
		anchor.Confidence = "medium"
		return anchor, idx, ""
	}
	return nil, -1, "якорь не уверенный: в выдаче несколько названных товаров"
}

// requiredFor — семейство to — условно обязательный компаньон семейства from: связь с requirement
// conditional_required или правило лицензии from.
func (c *commerceCatalog) requiredFor(from, to string) bool {
	if c == nil || from == to {
		return false
	}
	for _, r := range c.relationsFrom[from] {
		if r.ToFamily == to && r.Requirement == domain.RequirementConditional {
			return true
		}
	}
	return c.ruleNamesFamily(from, to)
}

// familyCategory — категория семейства (по первой позиции).
func (c *commerceCatalog) familyCategory(fam string) string {
	if c == nil || c.compat == nil || len(c.compat.byFamily[fam]) == 0 {
		return ""
	}
	return c.compat.byFamily[fam][0].CategoryID
}

// ruleNamesFamily — правило лицензии семейства anchor требует позиции семейства fam (версия не сравнивается).
func (c *commerceCatalog) ruleNamesFamily(anchor, fam string) bool {
	for _, lr := range c.licenseByAnchor[anchor] {
		if lr.RequiresFamily == fam {
			return true
		}
		if c.compat != nil && lr.RequiresCategory != "" {
			for _, p := range c.compat.byFamily[fam] {
				if p.CategoryID == lr.RequiresCategory && c.ruleCovers(lr, p.ID) {
					return true
				}
			}
		}
	}
	return false
}

// linkedFamilies — семейства связаны справочником (в любую сторону) или правилом лицензии: CAL рядом с
// сервером — не второй названный товар, а его компаньон.
func (c *commerceCatalog) linkedFamilies(a, b string) bool {
	if c == nil {
		return false
	}
	for _, r := range c.relationsFrom[a] {
		if r.ToFamily == b {
			return true
		}
	}
	for _, r := range c.relationsFrom[b] {
		if r.ToFamily == a {
			return true
		}
	}
	if c.ruleNamesFamily(a, b) || c.ruleNamesFamily(b, a) {
		return true
	}
	return false
}

// candidate — оценённый кандидат одной связи.
type candidate struct {
	p      *domain.Product
	compat CompatResult
	rule   *domain.LicenseRule
}

// companionsFor — подбор к якорю: обязательные (строка в карточке) и блок «Дополните решение».
func (c *commerceCatalog) companionsFor(lx *searchLexicon, anchor *out.CompanionAnchor, q companionQuery) companionResult {
	res := companionResult{anchor: anchor, required: []out.Companion{},
		step: CompanionsStep{StepBase: domain.StepBase{Name: StepCompanions}, Anchor: anchor,
			Considered: []CompanionConsidered{}, Rejected: []domain.RejectedHit{}}}
	if c == nil || c.compat == nil || anchor == nil {
		return res
	}
	a := c.products[anchor.ProductID]
	if a == nil {
		return res
	}
	rejected := map[string]bool{}
	reject := func(pid, reason, details string) {
		if rejected[pid] {
			return
		}
		rejected[pid] = true
		res.step.Rejected = append(res.step.Rejected, domain.RejectedHit{ProductID: pid, Reason: reason, Detail: details})
	}
	considered := map[string]int{}
	consider := func(pid, class, source string, cr CompatResult, ev string) {
		if _, ok := considered[pid]; ok {
			return
		}
		considered[pid] = len(res.step.Considered)
		res.step.Considered = append(res.step.Considered, CompanionConsidered{ProductID: pid, Class: class, Source: source,
			Compatibility: cr.State, EvidenceLevel: ev})
	}

	var required, block []out.Companion
	coveredRules := map[string]bool{}
	for i := range c.relationsFrom[a.FamilyID] {
		rel := &c.relationsFrom[a.FamilyID][i]
		if rel.Type == domain.RelationSubstitute || rel.Type == domain.RelationUpgrade {
			continue // аналоги и старшая редакция — рядом с выдачей, не в блоке
		}
		if rel.Type == domain.RelationCompatibility && (rel.Requirement == "" || rel.Requirement == domain.RequirementOptional) {
			continue // «CAL подходит к серверу» (optional) — факт совместимости, а не предложение докупить сервер
		}
		if rel.FromProduct != nil && *rel.FromProduct != a.ID {
			continue
		}
		cls := relationClass(rel, a)
		src := "relation:" + relID(rel)
		var pool []*domain.Product
		if rel.ToProduct != nil {
			if p := c.products[*rel.ToProduct]; p != nil {
				pool = []*domain.Product{p}
			}
		} else if rel.ToFamily != a.FamilyID {
			pool = c.compat.byFamily[rel.ToFamily]
		}
		if rel.ReviewStatus == domain.ReviewPending || rel.ReviewStatus == domain.ReviewRejected {
			for _, p := range pool {
				consider(p.ID, cls, src, unknownCompat(), rel.EvidenceLevel)
				reject(p.ID, rejectNotReviewed, "связь "+relID(rel)+" не утверждена ("+rel.ReviewStatus+")")
			}
			continue
		}
		if ok, why := conditionsHold(c, a.ID, rel.Conditions); !ok {
			for _, p := range pool {
				consider(p.ID, cls, src, unknownCompat(), rel.EvidenceLevel)
				reject(p.ID, rejectCondition, why)
			}
			continue
		}
		var ok []candidate
		for _, p := range pool {
			if p.ID == a.ID {
				continue
			}
			cr := c.PairCompat(a.ID, p.ID, rel)
			consider(p.ID, cls, src, cr, rel.EvidenceLevel)
			switch {
			case !cr.Allowed():
				reject(p.ID, incompatibleCode(cr), cr.Condition+basisNote(cr))
				continue
			case p.PriceMode == domain.PriceUnavailable:
				reject(p.ID, rejectUnavailable, "нет в наличии")
				continue
			case cr.State == domain.CompatUnknown && contains(rel.Conditions, condCheckCompatRule):
				reject(p.ID, rejectNoCompatRule, "нет правила совместимости с моделью якоря")
				continue
			}
			_, rule, _ := c.LicenseCompat(a.ID, p.ID)
			ok = append(ok, candidate{p: p, compat: cr, rule: rule})
		}
		if len(ok) == 0 {
			continue
		}
		strategy := rel.QuantityStrategy
		best := pickCandidate(ok, strategy, q)
		for _, x := range ok {
			if x.p.ID != best.p.ID {
				reject(x.p.ID, rejectWeakerVariant, "из семейства показана одна позиция: "+best.p.ID)
			}
		}
		if best.rule != nil {
			coveredRules[best.rule.ID] = true
			if strategy == "" || strategy == "none" {
				strategy = best.rule.QuantityStrategy
			}
		}
		comp := c.companion(lx, a, best, rel, cls, strategy, q)
		if cls == CompanionRequired {
			required = append(required, comp)
		} else {
			block = append(block, comp)
		}
	}
	// Правило вендора есть, связи в справочнике нет (MISSING_REQUIRED): блок компаньона не показывает — это
	// находка агента, а не догадка поиска. В трассе — с причиной.
	for _, lr := range c.licenseRulesFor(a.ID) {
		if coveredRules[lr.ID] {
			continue
		}
		var pool []*domain.Product
		if lr.RequiresFamily != "" {
			pool = c.compat.byFamily[lr.RequiresFamily]
		} else {
			pool = c.compat.byCategory[lr.RequiresCategory]
		}
		for _, p := range pool {
			if p.ID == a.ID || !c.ruleCovers(lr, p.ID) {
				continue
			}
			if _, seen := considered[p.ID]; seen {
				continue
			}
			cr := c.ruleCheck(lr, a.ID, p.ID)
			consider(p.ID, CompanionRequired, "license_rule:"+lr.ID, cr, "")
			if !cr.Allowed() {
				reject(p.ID, incompatibleCode(cr), cr.Condition+" (правило "+lr.ID+")")
				continue
			}
			reject(p.ID, rejectMissingRel, "правило "+lr.ID+" требует, связи в справочнике нет")
		}
	}

	res.required = dedupeByFamily(required, reject)
	res.block = c.assembleBlock(anchor, block, reject)
	shown := map[string]bool{}
	for _, x := range res.required {
		shown[x.ProductID] = true
	}
	if res.block != nil {
		for _, x := range res.block.Items {
			shown[x.ProductID] = true
		}
	}
	for i := range res.step.Considered {
		res.step.Considered[i].Shown = shown[res.step.Considered[i].ProductID]
	}
	kept := res.step.Rejected[:0]
	for _, r := range res.step.Rejected {
		if !shown[r.ProductID] {
			kept = append(kept, r)
		}
	}
	res.step.Rejected = kept
	return res
}

// incompatibleCode — код причины для фронта: несовпадение версии или редакции по правилу лицензии, иначе
// incompatible.
func incompatibleCode(cr CompatResult) string {
	if cr.Basis == CompatBasisLicenseRule {
		switch {
		case strings.Contains(cr.Condition, "по версии"):
			return rejectVersion
		case strings.Contains(cr.Condition, "по редакции"):
			return rejectEdition
		}
	}
	return rejectIncompatible
}

func relID(r *domain.Relation) string {
	if r.RelID != "" {
		return r.RelID
	}
	return r.FromFamily + "→" + r.ToFamily
}

func basisNote(cr CompatResult) string {
	if cr.RuleID == "" {
		return ""
	}
	return " (" + cr.Basis + " " + cr.RuleID + ")"
}

// relationClass — класс компаньона по связи: обязательность сильнее типа; у системы с моделью (сервер)
// комплектующие — «совместимое».
func relationClass(r *domain.Relation, anchor *domain.Product) string {
	switch {
	case r.Requirement == domain.RequirementConditional:
		return CompanionRequired
	case r.Type == domain.RelationRenewal:
		return CompanionRenewal
	case anchor.ModelID != "" && (r.Type == domain.RelationCompatibility || contains(r.Conditions, condCheckCompatRule)):
		return CompanionCompatible
	}
	return CompanionComplement
}

// conditionsHold — условия связи, которые можно проверить по якорю. Остальные (есть ли действующие лицензии,
// нужен ли RDS) — вопрос покупателю: связь показывается, условия уходят в conditions.
func conditionsHold(c *commerceCatalog, anchorID string, conds []string) (bool, string) {
	for _, cond := range conds {
		if cond == condNoPreinstalledOS {
			if os := strings.ToLower(c.attr(anchorID, "os")); os != "" && !strings.Contains(os, "без") {
				return false, "условие no_preinstalled_os не выполнено: у якоря ОС " + c.attr(anchorID, "os")
			}
		}
	}
	return true, ""
}

// wantMetric — метрика позиции-компаньона: из запроса («на 50 устройств» → Device CAL), иначе по стратегии.
func wantMetric(strategy string, q companionQuery) string {
	switch strategy {
	case "per_user", "per_device", "per_seat":
		if q.metric == "user" || q.metric == "device" {
			return q.metric
		}
		if strategy == "per_device" {
			return "device"
		}
		return "user"
	}
	return ""
}

// pickCandidate — одна позиция из семейства: совместимость (verified > conditional > unknown), метрика
// запроса, количество ровно как в запросе, иначе поштучная, затем популярность и id.
func pickCandidate(xs []candidate, strategy string, q companionQuery) candidate {
	metric := wantMetric(strategy, q)
	score := func(x candidate) []float64 {
		cs := map[string]float64{domain.CompatVerified: 2, domain.CompatConditional: 1}[x.compat.State]
		m, qty := 0.0, 0.0
		if metric != "" && x.p.License.Metric != nil && *x.p.License.Metric == metric {
			m = 1
		}
		if x.p.License.Quantity != nil {
			switch {
			case q.quantity > 0 && *x.p.License.Quantity == q.quantity:
				qty = 2
			case *x.p.License.Quantity == 1:
				qty = 1
			}
		}
		return []float64{cs, m, qty, x.p.Popularity}
	}
	sort.SliceStable(xs, func(i, j int) bool {
		a, b := score(xs[i]), score(xs[j])
		for k := range a {
			if a[k] != b[k] {
				return a[k] > b[k]
			}
		}
		return xs[i].p.ID < xs[j].p.ID
	})
	return xs[0]
}

// suggestedQuantity — количество из запроса: «на 50 пользователей» и позиция на 1 пользователя → 50, на 5 → 10.
// Без числа в запросе — null (поле пустое с подсказкой); на сервер и «одна штука» — 1.
func suggestedQuantity(strategy string, p *domain.Product, q companionQuery) *int {
	switch strategy {
	case "per_user", "per_device", "per_seat":
		if q.quantity <= 0 {
			return nil
		}
		per := 1
		if p.License.Quantity != nil && *p.License.Quantity > 0 {
			per = *p.License.Quantity
		}
		n := int(math.Ceil(float64(q.quantity) / float64(per)))
		return &n
	case "per_server", "one":
		n := 1
		return &n
	}
	return nil
}

// companion — карточка компаньона для ответа.
func (c *commerceCatalog) companion(lx *searchLexicon, anchor *domain.Product, x candidate, rel *domain.Relation, cls,
	strategy string, q companionQuery) out.Companion {
	p := x.p
	d := lx.docs[p.ID]
	comp := out.Companion{ProductID: p.ID, FamilyID: p.FamilyID, Title: p.Title, Vendor: d.Vendor, Category: d.Category,
		LicenseLine: licenseLine(d), PriceRub: p.PriceRub, PriceMode: p.PriceMode, Class: cls,
		Compatibility: x.compat.State, Reason: rel.Note, Conditions: append([]string{}, rel.Conditions...),
		EvidenceLevel: rel.EvidenceLevel, Source: rel.Source, SourceURL: rel.SourceURL,
		SuggestedQuantity: suggestedQuantity(strategy, p, q), UpgradeAdds: []string{}}
	if comp.PriceMode == "" {
		comp.PriceMode = domain.PriceFixed
		if p.PriceOnRequest || p.PriceRub == nil {
			comp.PriceMode = domain.PriceQuoteRequired
		}
	}
	if d.ProductID == "" {
		comp.LicenseLine = ""
	}
	if r := x.rule; r != nil {
		// Правило вендора — основание сильнее связи: причина, источник и условия — из него.
		comp.Reason, comp.Source, comp.SourceURL = r.Reason, r.Source, r.SourceURL
		for _, cond := range r.Conditions {
			if !contains(comp.Conditions, cond) {
				comp.Conditions = append(comp.Conditions, cond)
			}
		}
	}
	if comp.Reason == "" {
		comp.Reason = rel.Note
	}
	switch cls {
	case CompanionRequired:
		comp.Label = labelRequired
	case CompanionCompatible:
		name := anchor.FamilyName
		if name == "" {
			name = anchor.Title
		}
		comp.Label = "Совместимо с " + name
	case CompanionRenewal:
		comp.Label = labelRenewal
	default:
		comp.Label = labelComplement
	}
	return comp
}

// dedupeByFamily — по одной позиции на семейство (первая — по порядку связей).
func dedupeByFamily(xs []out.Companion, reject func(pid, reason, details string)) []out.Companion {
	out := []out.Companion{}
	seen := map[string]bool{}
	for _, x := range xs {
		if seen[x.FamilyID] {
			reject(x.ProductID, rejectClassTaken, "из семейства "+x.FamilyID+" уже показана позиция")
			continue
		}
		seen[x.FamilyID] = true
		out = append(out, x)
	}
	return out
}

// classOrder — порядок классов в блоке: совместимое, дополнения, продление.
var classOrder = map[string]int{CompanionCompatible: 0, CompanionComplement: 1, CompanionRenewal: 2}

// assembleBlock — «Дополните решение»: по одной позиции на класс и категорию, не больше maxCompanions; пусто — nil.
// Внутри класса — порядок связей справочника (бизнес-слой), затем популярность.
func (c *commerceCatalog) assembleBlock(anchor *out.CompanionAnchor, xs []out.Companion,
	reject func(pid, reason, details string)) *out.CompanionBlock {
	sort.SliceStable(xs, func(i, j int) bool { return classOrder[xs[i].Class] < classOrder[xs[j].Class] })
	items := []out.Companion{}
	seen := map[string]bool{}
	for _, x := range xs {
		key := x.Class + "|" + c.categoryOf(x.ProductID)
		switch {
		case seen[key] || seen["f:"+x.FamilyID]:
			reject(x.ProductID, rejectClassTaken, "в блоке уже есть позиция того же класса и типа")
			continue
		case len(items) == maxCompanions:
			reject(x.ProductID, rejectBlockFull, fmt.Sprintf("в блоке не больше %d позиций", maxCompanions))
			continue
		}
		seen[key], seen["f:"+x.FamilyID] = true, true
		items = append(items, x)
	}
	if len(items) == 0 {
		return nil
	}
	return &out.CompanionBlock{Anchor: *anchor, Items: items}
}

func (c *commerceCatalog) categoryOf(pid string) string {
	if p := c.products[pid]; p != nil {
		return p.CategoryID
	}
	return ""
}

// companionsStep — шаг поиска: якорь из выдачи, подбор, раскладка в ответ. Сбой не ломает поиск — шаг с error.
func (s *Services) companionsStep(cc *commerceCatalog, lx *searchLexicon, fams []*familyHit, route string, q companionQuery,
	analog bool, resp *out.SearchResponse) (step CompanionsStep) {
	t := time.Now()
	step = CompanionsStep{StepBase: domain.StepBase{Name: StepCompanions}, Considered: []CompanionConsidered{},
		Rejected: []domain.RejectedHit{}}
	defer func() {
		if r := recover(); r != nil {
			step = CompanionsStep{StepBase: domain.StepBase{Name: StepCompanions, Reason: rejectCompanionError},
				Error: fmt.Sprint(r), Considered: []CompanionConsidered{}, Rejected: []domain.RejectedHit{}}
			resp.Companions = nil
			for i := range resp.Items {
				resp.Items[i].Required = []out.Companion{}
			}
		}
		step.Ms = msSince(t)
	}()
	if cc == nil || lx == nil || len(resp.Items) == 0 {
		step.Skipped, step.Reason = true, "пустая выдача"
		return step
	}
	anchor, idx, why := anchorOf(cc, lx, q.tokens, fams, route, analog, resp.Clarify != nil, resp.Empty != nil)
	if anchor == nil {
		step.Skipped, step.Reason = true, why
		if len(fams) > 0 && why != "" {
			step.Rejected = append(step.Rejected, domain.RejectedHit{ProductID: fams[0].head.ProductID,
				Reason: rejectAnchorUnsure, Detail: why})
		}
		return step
	}
	anchor.ProductID = cc.familyAnchorProduct(anchor.ProductID)
	r := cc.companionsFor(lx, anchor, q)
	if idx < len(resp.Items) {
		resp.Items[idx].Required = r.required
	}
	resp.Companions = r.block
	return r.step
}

// familyAnchorProduct — якорь внутри семейства: если первая позиция — условно обязательный компаньон другой
// позиции того же семейства (SQL CAL к SQL Server Standard «Server + CAL»), якорь — та позиция.
func (c *commerceCatalog) familyAnchorProduct(pid string) string {
	p := c.products[pid]
	if p == nil {
		return pid
	}
	for _, r := range c.relationsFrom[p.FamilyID] {
		if r.ToFamily == p.FamilyID && r.Requirement == domain.RequirementConditional && r.ToProduct != nil &&
			*r.ToProduct == pid && r.FromProduct != nil && c.products[*r.FromProduct] != nil {
			return *r.FromProduct
		}
	}
	return pid
}

// keyWordPos — позиция в запросе самого редкого (по idf) слова названия семейства; нет — len(tokens).
func (lx *searchLexicon) keyWordPos(fid string, tokens []string) int {
	pos, best := len(tokens), -1.0
	for i, t := range tokens {
		for _, w := range wordsOf(t) {
			if lx.familyWords[fid][w] && lx.idf[w] > best {
				pos, best = i, lx.idf[w]
			}
		}
	}
	return pos
}
