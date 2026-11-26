package services

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"kuznica/api/domain"
)

// Детерминированные проверки справочника связей (контур коммерции): BROKEN_RELATION —
// связь на снятый или несуществующий товар, против правила лицензии вендора (CAL 2022 к серверу 2025) или против
// правила совместимости; MISSING_REQUIRED — по правилу вендора у якоря есть условно обязательный компаньон, а
// связи нет. Без покупок, без модели: агент получает находки готовыми и проверяет правку тем же кодом.

// Причины сломанной связи (relationFinding.Reason).
const (
	brokenFamilyMissing      = "family_missing"
	brokenProductMissing     = "product_missing"
	brokenProductUnavailable = "product_unavailable"
	brokenLicenseRule        = "license_rule"
	brokenIncompatible       = "incompatible"
	missingRequired          = "missing_required"
)

// relationFinding — находка проверки справочника.
type relationFinding struct {
	// Key — устойчивый ключ между запусками: broken:<id связи>, missing:<id правила>.
	Key        string `json:"key"`
	Diagnosis  string `json:"diagnosis"`
	Reason     string `json:"reason"`
	Text       string `json:"text"`
	RelationID string `json:"relation_id,omitempty"`
	// RuleIDs — правила лицензий или совместимости, на которых держится находка.
	RuleIDs       []string     `json:"rule_ids,omitempty"`
	FromFamily    string       `json:"from_family"`
	ToFamily      string       `json:"to_family,omitempty"`
	Relation      *relationDoc `json:"relation,omitempty"`
	EvidenceLevel string       `json:"evidence_level"`
	Source        string       `json:"source,omitempty"`
	SourceURL     string       `json:"source_url,omitempty"`
	// Fix — правка, которая детерминированно снимает находку (подсказка; агент может выбрать другую).
	Fix *RelationPatch `json:"fix,omitempty"`
	// Details — числа и позиции, на которых проверка споткнулась.
	Details map[string]any `json:"details,omitempty"`
}

// evidenceOf — уровень доказательности по источнику правила: документ вендора — E2, детерминированный факт
// каталога (снятая позиция) — E2, иначе E1.
func evidenceOf(source string) string {
	switch source {
	case domain.SourceVendorDocs, domain.SourceCatalogRule:
		return "E2"
	}
	return "E1"
}

// familyProducts — позиции семейства (по id).
func (c *commerceCatalog) familyProducts(f string) []*domain.Product {
	if c == nil || c.compat == nil {
		return nil
	}
	return c.compat.byFamily[f]
}

func productAvailable(p *domain.Product) bool {
	return p != nil && p.PriceMode != domain.PriceUnavailable
}

// relationTargets — позиции, на которые ведёт связь: to_product или всё семейство to_family.
func (c *commerceCatalog) relationTargets(d relationDoc) []*domain.Product {
	if d.ToProduct != nil {
		if p := c.products[*d.ToProduct]; p != nil && p.FamilyID == d.ToFamily {
			return []*domain.Product{p}
		}
		return nil
	}
	return c.familyProducts(d.ToFamily)
}

// relationAnchors — позиции, от которых идёт связь: from_product или всё семейство from_family.
func (c *commerceCatalog) relationAnchors(d relationDoc) []*domain.Product {
	if d.FromProduct != nil {
		if p := c.products[*d.FromProduct]; p != nil && p.FamilyID == d.FromFamily {
			return []*domain.Product{p}
		}
		return nil
	}
	return c.familyProducts(d.FromFamily)
}

// talksAboutCompat — связь утверждает «вместе» (а не «вместо»): по ней проверяются правила лицензий и совместимости.
func talksAboutCompat(t string) bool {
	return t != domain.RelationSubstitute && t != domain.RelationUpgrade
}

// checkRelation — проверки одной связи; nil — связь цела.
func (c *commerceCatalog) checkRelation(d relationDoc, all []relationDoc) *relationFinding {
	f := &relationFinding{Key: "broken:" + d.ID, Diagnosis: domain.DiagBrokenRelation, RelationID: d.ID,
		FromFamily: d.FromFamily, ToFamily: d.ToFamily, Relation: &d, EvidenceLevel: "E2", Source: domain.SourceCatalogRule}
	remove := &RelationPatch{Remove: []string{d.ID}}
	for _, end := range []struct{ fam, what string }{{d.FromFamily, "источника"}, {d.ToFamily, "цели"}} {
		if len(c.familyProducts(end.fam)) == 0 {
			f.Reason, f.Fix = brokenFamilyMissing, remove
			f.Text = fmt.Sprintf("Связь %s ведёт в никуда: семейства %s (%s) нет в каталоге", d.ID, end.fam, end.what)
			return f
		}
	}
	for _, end := range []struct {
		id   *string
		fam  string
		what string
	}{{d.FromProduct, d.FromFamily, "источник"}, {d.ToProduct, d.ToFamily, "цель"}} {
		if end.id == nil {
			continue
		}
		p := c.products[*end.id]
		upd := d
		if end.what == "цель" {
			upd.ToProduct = nil
		} else {
			upd.FromProduct = nil
		}
		alts := c.availableIn(end.fam, *end.id)
		f.Details = map[string]any{"product_id": *end.id, "alternatives": alts}
		switch {
		case p == nil || p.FamilyID != end.fam:
			f.Reason, f.Fix = brokenProductMissing, &RelationPatch{Update: []relationDoc{upd}}
			f.Text = fmt.Sprintf("Связь %s: %s — позиция %s, её нет в семействе %s", d.ID, end.what, *end.id, end.fam)
			return f
		case !productAvailable(p):
			f.Reason, f.Fix = brokenProductUnavailable, &RelationPatch{Update: []relationDoc{upd}}
			f.Text = fmt.Sprintf("Связь %s: %s — позиция %s «%s» снята с продажи (price_mode unavailable)", d.ID, end.what, p.ID, p.Title)
			return f
		}
	}
	if !talksAboutCompat(d.Type) {
		return nil
	}
	targets := c.relationTargets(d)
	if !slices.ContainsFunc(targets, productAvailable) {
		f.Reason, f.Fix = brokenProductUnavailable, remove
		f.Text = fmt.Sprintf("Связь %s: все позиции семейства %s сняты с продажи", d.ID, d.ToFamily)
		return f
	}
	if lf := c.licenseBreak(d, targets, all); lf != nil {
		return lf
	}
	return c.compatBreak(d, targets)
}

// availableIn — доступные позиции семейства, кроме except.
func (c *commerceCatalog) availableIn(fam, except string) []string {
	var ids []string
	for _, p := range c.familyProducts(fam) {
		if p.ID != except && productAvailable(p) {
			ids = append(ids, p.ID)
		}
	}
	return nonNil(ids)
}

// licenseBreak — связь ведёт к позициям, о которых говорит правило лицензии якоря (категория, редакция), но ни
// одна не проходит условие сравнения с якорем (версия CAL ниже версии сервера).
func (c *commerceCatalog) licenseBreak(d relationDoc, targets []*domain.Product, all []relationDoc) *relationFinding {
	var rules []string
	var conds []string
	var src, srcURL string
	for _, a := range c.relationAnchors(d) {
		for _, lr := range c.licenseRulesFor(a.ID) {
			covered, passed := 0, 0
			var cond string
			for _, t := range targets {
				if !c.ruleCovers(lr, t.ID) {
					continue
				}
				covered++
				if r := c.ruleCheck(lr, a.ID, t.ID); r.State != domain.CompatIncompatible {
					passed++
				} else if cond == "" {
					cond = r.Condition
				}
			}
			if covered > 0 && passed == 0 && !slices.Contains(rules, lr.ID) {
				rules = append(rules, lr.ID)
				conds = append(conds, cond)
				if src == "" {
					src, srcURL = lr.Source, lr.SourceURL
				}
			}
		}
	}
	if len(rules) == 0 {
		return nil
	}
	sort.Strings(rules)
	f := &relationFinding{Key: "broken:" + d.ID, Diagnosis: domain.DiagBrokenRelation, Reason: brokenLicenseRule,
		RelationID: d.ID, RuleIDs: rules, FromFamily: d.FromFamily, ToFamily: d.ToFamily, Relation: &d,
		EvidenceLevel: evidenceOf(src), Source: src, SourceURL: srcURL,
		Details: map[string]any{"conditions": conds, "target_versions": c.familyAttrValues(d.ToFamily, "version"),
			"anchor_versions": c.familyAttrValues(d.FromFamily, "version")}}
	f.Text = fmt.Sprintf("Связь %s: %s → %s нарушает правило вендора %s (%s)", d.ID, d.FromFamily, d.ToFamily,
		strings.Join(rules, ", "), firstNonEmpty(conds...))
	// Правильная связь уже есть (к семейству, которое правило проходит) — убрать сломанную; иначе — перевести цель.
	if ok := c.satisfyingFamilies(d.FromFamily, rules); len(ok) > 0 {
		for _, o := range all {
			if o.ID != d.ID && o.FromFamily == d.FromFamily && slices.Contains(ok, o.ToFamily) && talksAboutCompat(o.Type) {
				f.Fix = &RelationPatch{Remove: []string{d.ID}}
				f.Details["correct_relation"] = o.ID
				return f
			}
		}
		upd := d
		upd.ToFamily, upd.ToProduct = ok[0], nil
		f.Fix = &RelationPatch{Update: []relationDoc{upd}}
	} else {
		f.Fix = &RelationPatch{Remove: []string{d.ID}}
	}
	return f
}

// compatBreak — связь системы с компонентом, а правило совместимости говорит «несовместимо» про все позиции цели,
// о которых есть правило (и ни одна не совместима).
func (c *commerceCatalog) compatBreak(d relationDoc, targets []*domain.Product) *relationFinding {
	var rules []string
	for _, a := range c.relationAnchors(d) {
		if a.ModelID == "" {
			continue
		}
		bad, good := 0, 0
		var ids []string
		for _, t := range targets {
			r := c.ModelCompat(t.ID, a.ModelID)
			switch r.State {
			case domain.CompatIncompatible:
				bad++
				ids = append(ids, r.RuleID)
			case domain.CompatVerified, domain.CompatConditional:
				good++
			}
		}
		if bad > 0 && good == 0 {
			for _, id := range ids {
				if !slices.Contains(rules, id) {
					rules = append(rules, id)
				}
			}
		}
	}
	if len(rules) == 0 {
		return nil
	}
	sort.Strings(rules)
	return &relationFinding{Key: "broken:" + d.ID, Diagnosis: domain.DiagBrokenRelation, Reason: brokenIncompatible,
		RelationID: d.ID, RuleIDs: rules, FromFamily: d.FromFamily, ToFamily: d.ToFamily, Relation: &d, EvidenceLevel: "E2",
		Source: domain.SourceVendorDocs, Fix: &RelationPatch{Remove: []string{d.ID}},
		Text: fmt.Sprintf("Связь %s: %s → %s, а правила совместимости %s говорят «несовместимо»", d.ID, d.FromFamily,
			d.ToFamily, strings.Join(rules, ", "))}
}

// familyAttrValues — различные значения атрибута у позиций семейства (по возрастанию).
func (c *commerceCatalog) familyAttrValues(fam, key string) []string {
	var vs []string
	for _, p := range c.familyProducts(fam) {
		if v := c.attr(p.ID, key); v != "" && !slices.Contains(vs, v) {
			vs = append(vs, v)
		}
	}
	sort.Strings(vs)
	return nonNil(vs)
}

// ruleAnchors — позиции семейства, к которым относится правило.
func (c *commerceCatalog) ruleAnchors(lr domain.LicenseRule) []*domain.Product {
	var as []*domain.Product
	for _, p := range c.familyProducts(lr.AnchorFamily) {
		if slices.ContainsFunc(c.licenseRulesFor(p.ID), func(x domain.LicenseRule) bool { return x.ID == lr.ID }) {
			as = append(as, p)
		}
	}
	return as
}

// ruleSatisfiers — доступные позиции, которые правило покрывает и которые проходят его для всех якорей.
func (c *commerceCatalog) ruleSatisfiers(lr domain.LicenseRule) []*domain.Product {
	anchors := c.ruleAnchors(lr)
	if len(anchors) == 0 {
		return nil
	}
	var pool []*domain.Product
	switch {
	case lr.RequiresFamily != "":
		pool = c.familyProducts(lr.RequiresFamily)
	case lr.RequiresCategory != "" && c.compat != nil:
		pool = c.compat.byCategory[lr.RequiresCategory]
	}
	var res []*domain.Product
	for _, p := range pool {
		if !productAvailable(p) || !c.ruleCovers(lr, p.ID) {
			continue
		}
		ok := true
		for _, a := range anchors {
			if c.ruleCheck(lr, a.ID, p.ID).State == domain.CompatIncompatible {
				ok = false
				break
			}
		}
		if ok {
			res = append(res, p)
		}
	}
	return res
}

// satisfyingFamilies — семейства, которые проходят все правила rules якоря anchorFam; ближайшая версия к якорю — первой.
func (c *commerceCatalog) satisfyingFamilies(anchorFam string, rules []string) []string {
	var fams []string
	for i, id := range rules {
		var lr *domain.LicenseRule
		for j := range c.licenseByAnchor[anchorFam] {
			if c.licenseByAnchor[anchorFam][j].ID == id {
				lr = &c.licenseByAnchor[anchorFam][j]
			}
		}
		if lr == nil {
			continue
		}
		var these []string
		for _, p := range c.ruleSatisfiers(*lr) {
			if !slices.Contains(these, p.FamilyID) {
				these = append(these, p.FamilyID)
			}
		}
		if i == 0 {
			fams = these
		} else {
			fams = slices.DeleteFunc(fams, func(f string) bool { return !slices.Contains(these, f) })
		}
	}
	av := c.familyAttrValues(anchorFam, "version")
	sort.SliceStable(fams, func(i, j int) bool {
		vi, vj := c.familyAttrValues(fams[i], "version"), c.familyAttrValues(fams[j], "version")
		ei, ej := len(av) > 0 && slices.Equal(vi, av), len(av) > 0 && slices.Equal(vj, av)
		if ei != ej {
			return ei
		}
		return fams[i] < fams[j]
	})
	return fams
}

// checkLicenseRule — MISSING_REQUIRED: условно обязательный компаньон по правилу вендора есть в каталоге, а связи
// от якоря ни к одной подходящей позиции нет. nil — связь есть или правило не к чему применить.
func (c *commerceCatalog) checkLicenseRule(lr domain.LicenseRule, docs []relationDoc) (*relationFinding, map[string]any) {
	info := map[string]any{"rule_id": lr.ID, "anchor_family": lr.AnchorFamily, "class": lr.Class}
	if lr.Class != "" && lr.Class != domain.RequirementConditional {
		info["status"] = "not_required"
		return nil, info
	}
	if len(c.ruleAnchors(lr)) == 0 {
		info["status"] = "no_anchor"
		return nil, info
	}
	sat := c.ruleSatisfiers(lr)
	if len(sat) == 0 {
		info["status"] = "no_candidates"
		return nil, info
	}
	satIDs := map[string]bool{}
	var fams []string
	for _, p := range sat {
		satIDs[p.ID] = true
		if !slices.Contains(fams, p.FamilyID) {
			fams = append(fams, p.FamilyID)
		}
	}
	info["satisfying_families"] = fams
	for _, d := range docs {
		if d.FromFamily != lr.AnchorFamily || !talksAboutCompat(d.Type) || !slices.Contains(fams, d.ToFamily) {
			continue
		}
		if d.FromProduct != nil && lr.AnchorProduct != "" && *d.FromProduct != lr.AnchorProduct {
			continue
		}
		if d.ToProduct != nil && !satIDs[*d.ToProduct] {
			continue
		}
		info["status"], info["relation_id"] = "linked", d.ID
		return nil, info
	}
	info["status"] = "missing"
	best := c.satisfyingFamilies(lr.AnchorFamily, []string{lr.ID})
	target := fams[0]
	if len(best) > 0 {
		target = best[0]
	}
	qs := lr.QuantityStrategy
	if qs == "" {
		qs = "one"
	}
	add := relationDoc{FromFamily: lr.AnchorFamily, ToFamily: target, Type: domain.RelationComplement,
		Note: shortText(lr.Reason, 160), Requirement: domain.RequirementConditional, Compatibility: domain.CompatVerified,
		Conditions: append([]string{}, lr.Conditions...), QuantityStrategy: qs, Source: firstNonEmpty(lr.Source, domain.SourceVendorDocs),
		SourceURL: lr.SourceURL, EvidenceLevel: evidenceOf(lr.Source), ReviewStatus: domain.ReviewPending}
	if lr.AnchorProduct != "" {
		ap := lr.AnchorProduct
		add.FromProduct = &ap
	}
	f := &relationFinding{Key: "missing:" + lr.ID, Diagnosis: domain.DiagMissingRequired, Reason: missingRequired,
		RuleIDs: []string{lr.ID}, FromFamily: lr.AnchorFamily, ToFamily: target, EvidenceLevel: evidenceOf(lr.Source),
		Source: lr.Source, SourceURL: lr.SourceURL, Fix: &RelationPatch{Add: []relationDoc{add}},
		Details: map[string]any{"satisfying_families": fams, "conditions": lr.Conditions, "quantity_strategy": qs,
			"reason": lr.Reason}}
	f.Text = fmt.Sprintf("Правило %s: к %s условно обязательна позиция из %s, но связи в справочнике нет", lr.ID,
		lr.AnchorFamily, strings.Join(fams, ", "))
	return f, info
}

// relationScan — итог проверки справочника: находки и сводка для ленты.
type relationScan struct {
	Findings  []relationFinding `json:"findings"`
	Relations int               `json:"relations"`
	Rules     int               `json:"rules"`
	// RulesStatus — по каждому правилу: linked | missing | no_candidates | no_anchor | not_required.
	RulesStatus []map[string]any `json:"rules_status"`
	// NeedsReview — связей без проверки дольше relationStaleDays (информация, не находка).
	NeedsReview int `json:"needs_review"`
}

func (r *relationScan) keys() map[string]bool {
	m := map[string]bool{}
	for _, f := range r.Findings {
		m[f.Key] = true
	}
	return m
}

// scanRelations — все проверки справочника docs поверх справочников base (позиции, правила): сначала сломанные
// связи, затем недостающие обязательные. Порядок находок устойчив (по ключу внутри вида).
func scanRelations(base *commerceCatalog, docs []relationDoc) *relationScan {
	c := base.withRelations(relationsOfDocs(docs))
	res := &relationScan{Relations: len(docs), Rules: len(c.licenseRules), Findings: []relationFinding{}, RulesStatus: []map[string]any{}}
	var broken, missing []relationFinding
	for _, d := range docs {
		if f := c.checkRelation(d, docs); f != nil {
			broken = append(broken, *f)
		}
	}
	for _, lr := range c.licenseRules {
		f, info := c.checkLicenseRule(lr, docs)
		res.RulesStatus = append(res.RulesStatus, info)
		if f != nil {
			missing = append(missing, *f)
		}
	}
	sort.Slice(broken, func(i, j int) bool { return broken[i].Key < broken[j].Key })
	sort.Slice(missing, func(i, j int) bool { return missing[i].Key < missing[j].Key })
	res.Findings = append(append(res.Findings, broken...), missing...)
	return res
}
