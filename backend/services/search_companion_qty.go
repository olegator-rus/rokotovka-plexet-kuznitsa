package services

import (
	"context"

	"kuznica/api/domain"
)

// Количество для компаньона. «windows server 2025 standard на 50
// пользователей»: сервер лицензируется по ядрам, а «на 50 пользователей» — это число лицензий доступа (CAL), которые
// правило вендора требует к серверу. Если названное семейство — якорь правила лицензии с количеством по
// пользователям/устройствам, а сами лицензии доступа в запросе не названы, количество не обязательное условие к
// серверу, а количество компаньона: сервер остаётся точным товаром первым, CAL 2025 × N — в строке «Проверьте
// необходимость» (companions.go), CAL другой версии, которую правило отвергает, в группу exact не входит.
// Правила нет — поведение контура поиска («касперский на 10 пк», «1с на 5 пользователей» — без изменений). Флаг
// FEATURE_COMPANIONS выключен — шаг не действует.

// Сигналы позиции в трассе.
const (
	// signalCompanionQty — количество из запроса отнесено к компаньону якоря, сервер — точный товар.
	signalCompanionQty = "quantity_for_companion"
	// signalRuleVersionMismatch — правило лицензии названного якоря отвергает позицию (CAL 2022 к серверу 2025).
	signalRuleVersionMismatch = "anchor_rule_mismatch"
)

// companionQtyStrategies — стратегии количества, при которых «на N пользователей/устройств» относится к компаньону.
var companionQtyStrategies = setOf("per_user", "per_device", "per_seat")

// companionQty — решение: количество запроса — у компаньонов семейства anchor.
type companionQty struct {
	cc        *commerceCatalog
	anchor    string // семейство-якорь, названное в запросе
	anchorPID string // позиция якоря для сравнения версий правила
	rules     []domain.LicenseRule
	quantity  int
	metric    string
}

// companionQuantity — количество запроса отнести к компаньону названного семейства; nil — нет (поведение контура поиска).
func (s *Services) companionQuantity(ctx context.Context, lx *searchLexicon, attrs queryAttrs, ent queryEntities) *companionQty {
	if s == nil || lx == nil || !s.featuresFor(ctx).Companions {
		return nil
	}
	return companionQuantityFor(s.commerceFor(ctx), lx, attrs, ent)
}

func companionQuantityFor(cc *commerceCatalog, lx *searchLexicon, attrs queryAttrs, ent queryEntities) *companionQty {
	if cc == nil || cc.compat == nil || attrs.quantity <= 0 || attrs.renewal || attrs.analog {
		return nil
	}
	if m := attrs.metric; m != "" && m != domain.MetricUser && m != domain.MetricDevice {
		return nil // «16 ядер» — количество самого сервера
	}
	queryWords := map[string]bool{}
	for _, t := range ent.entityTokens {
		for _, w := range wordsOf(t) {
			queryWords[w] = true
		}
	}
	named := append([]string{}, ent.named...)
	if ent.parsedFamily != "" && !contains(named, ent.parsedFamily) {
		named = append(named, ent.parsedFamily)
	}
	var best *companionQty
	bestCov := 0.0
	for _, fam := range named {
		rules, ok := cc.quantityRules(lx, fam, queryWords)
		if !ok || len(rules) == 0 {
			continue
		}
		cov := lx.familyCoverage(fam, ent.entityTokens)
		if best != nil && cov <= bestCov {
			continue
		}
		pid := ""
		if ps := lx.families[fam]; len(ps) > 0 {
			pid = ps[0]
		}
		best, bestCov = &companionQty{cc: cc, anchor: fam, anchorPID: pid, rules: rules, quantity: attrs.quantity,
			metric: attrs.metric}, cov
	}
	return best
}

// quantityRules — правила лицензии семейства fam с количеством по пользователям/устройствам, у которых
// компаньон — другое семейство, не названное в запросе своими словами («cal», «клиентские», «rds»). ok = false —
// компаньон назван или правило внутри семейства (SQL Server + CAL): количество остаётся условием выдачи.
func (c *commerceCatalog) quantityRules(lx *searchLexicon, fam string, queryWords map[string]bool) ([]domain.LicenseRule, bool) {
	var out []domain.LicenseRule
	for _, lr := range c.licenseByAnchor[fam] {
		if !companionQtyStrategies[lr.QuantityStrategy] {
			continue
		}
		fams := c.ruleFamilies(lr)
		if len(fams) == 0 {
			continue
		}
		anchorStems := lx.familyStems(fam)
		for _, cf := range fams {
			if cf == fam {
				return nil, false
			}
			for w := range lx.familyWords[cf] {
				if queryWords[w] && !lx.familyWords[fam][w] {
					return nil, false // компаньон назван: «cal 2025 на 25 пользователей», «1с сервер и клиентские лицензии»
				}
			}
			cs := lx.familyStems(cf)
			for w := range queryWords {
				if st := cqStem(w); st != "" && cs[st] && !anchorStems[st] {
					return nil, false // назван описанием: «удаленный рабочий стол» — RDS CAL, не сервер
				}
			}
		}
		out = append(out, lr)
	}
	return out, true
}

// ruleFamilies — семейства позиций, которые покрывает правило (без сравнения с якорем).
func (c *commerceCatalog) ruleFamilies(lr domain.LicenseRule) []string {
	var pool []*domain.Product
	if lr.RequiresFamily != "" {
		pool = c.compat.byFamily[lr.RequiresFamily]
	} else {
		pool = c.compat.byCategory[lr.RequiresCategory]
	}
	var out []string
	for _, p := range pool {
		if c.ruleCovers(lr, p.ID) && !contains(out, p.FamilyID) {
			out = append(out, p.FamilyID)
		}
	}
	return out
}

// apply — количество и метрика запроса перестают быть условиями выдачи (они у компаньона).
func (q *companionQty) apply(rc *rankCtx) {
	if q == nil {
		return
	}
	rc.attrs.quantity, rc.attrs.metric, rc.attrs.metricUnresolved = 0, "", false
}

// regroup — после групп: якорь помечается сигналом; семейство, все позиции которого правило якоря отвергает
// (CAL 2022 к серверу 2025), уходит из exact в match; компаньоны якоря не выше якоря.
func (q *companionQty) regroup(fams []*familyHit) {
	if q == nil {
		return
	}
	anchorTier := -1
	for _, fh := range fams {
		if fh.id == q.anchor {
			anchorTier = fh.tier
			fh.signals = append(fh.signals, signalCompanionQty)
		}
	}
	for _, fh := range fams {
		if fh.id == q.anchor {
			continue
		}
		covered, allowed := q.check(fh)
		if !covered {
			continue
		}
		if !allowed && fh.group == domain.GroupExact {
			fh.group = domain.GroupMatch
			fh.signals = append(fh.signals, signalRuleVersionMismatch)
		}
		if anchorTier >= 0 && fh.tier >= anchorTier {
			fh.tier = anchorTier - 1 // CAL с теми же словами названия не выше сервера, к которому она нужна
		}
	}
}

// check — правила якоря покрывают позиции семейства (covered) и пропускают хотя бы одну (allowed).
func (q *companionQty) check(fh *familyHit) (covered, allowed bool) {
	for _, d := range fh.heads {
		for _, lr := range q.rules {
			if !q.cc.ruleCovers(lr, d.ProductID) {
				continue
			}
			covered = true
			if q.anchorPID == "" || q.cc.ruleCheck(lr, q.anchorPID, d.ProductID).Allowed() {
				return true, true
			}
		}
	}
	return covered, false
}

// stemPrefix — длина основы слова для сравнения со словами названий и описаний («удаленный» ~ «удалённому»).
const stemPrefix = 5

// cqStem — основа слова: первые stemPrefix букв; короче 4 букв и служебные — "".
func cqStem(w string) string {
	r := []rune(foldText(w))
	if len(r) < 4 || lexStopWords[string(r)] {
		return ""
	}
	if len(r) > stemPrefix {
		r = r[:stemPrefix]
	}
	return string(r)
}

// familyStems — основы слов названий и описаний позиций семейства.
func (lx *searchLexicon) familyStems(fam string) map[string]bool {
	out := map[string]bool{}
	for _, pid := range lx.families[fam] {
		d := lx.docs[pid]
		for _, w := range wordsOf(d.Title + " " + d.FamilyName + " " + d.Description) {
			if st := cqStem(w); st != "" {
				out[st] = true
			}
		}
	}
	return out
}
