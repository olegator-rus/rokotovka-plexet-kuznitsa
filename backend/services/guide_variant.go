package services

import (
	"fmt"
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Семейство с вариантами внутри (Astra Linux SE 121231: «Орёл», «Воронеж», «Смоленск», D3). Уровень защищённости
// система сама не выбирает: вопрос обязателен, без ответа результат предварительный и показаны все уровни.
// Рабочие станции и серверы — отдельные строки; смена числа одной строки не трогает другую.

// guideVariantOrder — порядок уровней (как в конфигураторе магазина).
var guideVariantOrder = []string{"Орел", "Воронеж", "Смоленск"}

// variantFamily — семейство кандидатов, у офферов которого в базе знаний размечены варианты ("" — нет).
func (t *guideTurn) variantFamily(fams []string) string {
	for _, f := range fams {
		for _, p := range t.cat.byFamily[f] {
			if m, ok := t.kb.marks[p.ID]; ok && m.Variant != "" {
				return f
			}
		}
	}
	return ""
}

// variantLabel — «Орёл» для экрана.
func variantLabel(v string) string {
	if guideNorm(v) == "орел" {
		return "Орёл"
	}
	return v
}

// variantOffer — лучший оффер варианта для строки объекта: полная версия, x86-64, техподдержка «Стандартная», не
// только для МСП; правила кода и базы знаний — validate_license.
func (t *guideTurn) variantOffer(fid, variant string, line domain.GuideObject) (*guideOfferView, []guideViolation) {
	r := t.st.Requirements
	r.Objects = []domain.GuideObject{line}
	r.Variant = variant
	var best *guideOfferView
	var lastBad []guideViolation
	for _, pol := range []guideTermPolicy{termDefault, termAny} {
		var ok []guideOfferView
		for _, p := range t.cat.byFamily[fid] {
			m := t.kb.marks[p.ID]
			if guideNorm(m.Variant) != guideNorm(variant) || (m.Object != "" && m.Object != line.Kind) {
				continue
			}
			if (m.LicenseType != "" && m.LicenseType != "Полная версия") || (m.Arch != "" && m.Arch != "x86-64") ||
				(m.SupportLevel != "" && m.SupportLevel != "Стандартная") || m.OrgSize == "Малый и средний бизнес" {
				continue
			}
			v := t.cat.view(p, t.kb)
			if !v.priced() {
				continue
			}
			if bad := validateLicense(v, r, t.kb, pol); len(bad) > 0 {
				lastBad = bad
				continue
			}
			ok = append(ok, v)
		}
		if len(ok) > 0 {
			sortOffers(ok, max(line.Count, 1), "")
			best = &ok[0]
			break
		}
		if t.st.Origin["term_months"] == originUser {
			break
		}
	}
	return best, lastBad
}

// variantTurn — ход по семейству с вариантами.
func (t *guideTurn) variantTurn(fid string) *out.GuideResponse {
	t2 := time.Now()
	st := t.st
	r := st.Requirements
	lines := []domain.GuideObject{}
	for _, o := range r.Objects {
		if o.Count > 0 {
			lines = append(lines, o)
		}
	}
	if len(lines) == 0 {
		lines = []domain.GuideObject{{Kind: "workstation", Count: 1}}
	}
	summary := []string{}
	for _, l := range lines {
		summary = append(summary, countLine(l))
	}
	unknown := containsStr(st.Unresolved, "variant")
	// Уровень не назван: вопрос и сразу все три уровня с ценой на условия (CHAT.md §5: «показать все три и спросить»).
	var ask *out.GuideClarify
	askLead := ""
	if r.Variant == "" && !unknown {
		chips := []out.GuideChip{}
		for _, v := range guideVariantOrder {
			chips = append(chips, out.GuideChip{Label: "«" + variantLabel(v) + "»", Key: "variant", Value: v})
		}
		chips = append(chips, out.GuideChip{Label: "не знаю", Key: "variant", Value: "unknown"})
		st.Pending = "variant"
		st.Questions++
		t.prelim = true
		t.meta.Fallback = guideFbNoNeed
		t.step("Вопрос", "", "уровень защищённости", t2)
		if len(lines) > 1 {
			askLead = "Собрал отдельные строки: " + strings.Join(summary, " и ") + "."
		}
		ask = &out.GuideClarify{Key: "variant", Question: "Какой уровень защищённости указан в ваших требованиях?",
			Chips: chips, Required: true}
	}
	variants := guideVariantOrder
	if r.Variant != "" {
		variants = []string{r.Variant}
	}
	res := &out.GuideResponse{Claims: []out.GuideClaim{}, Changes: t.changes}
	matched := 0
	grid := map[string]map[string]guideVariantCell{} // вариант → объект → оффер
	for _, vr := range variants {
		grid[vr] = map[string]guideVariantCell{}
		for _, l := range lines {
			v, bad := t.variantOffer(fid, vr, l)
			if v == nil {
				t.gap("no_matching_offer")
				for _, b := range bad {
					if b.Status == kbDraft {
						t.prelim = true
					}
				}
				continue
			}
			matched++
			grid[vr][l.Kind] = guideVariantCell{v, l.Count}
		}
	}
	t.step("Офферы вариантов", toolGetFamilyOffers, fmt.Sprintf("вариантов: %d, строк: %d", len(variants), len(lines)), t2)
	t.step("Правила лицензирования", toolValidateLicense, fmt.Sprintf("подходят: %d", matched), t2)
	if r.Variant != "" {
		res.Type = out.GuideSelection
		for _, l := range lines {
			if c, ok := grid[r.Variant][l.Kind]; ok {
				o := t.offerOut(*c.v, c.qty, l.Kind, out.OfferMatch, objectLine(l.Kind))
				res.Offers = append(res.Offers, o)
			}
		}
		if len(lines) > 1 {
			res.Lead = "Рабочие станции и серверы — отдельные строки."
		} else {
			res.Lead = "Подходит «" + variantLabel(r.Variant) + "»."
		}
	} else if ask != nil {
		res.Type, res.Clarification, res.Lead = out.GuideClarification, ask, askLead
		if askLead == "" {
			res.Lead = "У «" + suggestDisplay(t.cat.famName[fid]) + "» три уровня защищённости."
		}
		for _, vr := range variants {
			if c, ok := grid[vr][lines[0].Kind]; ok {
				res.Offers = append(res.Offers, t.offerOut(*c.v, c.qty, lines[0].Kind, out.OfferMatch, "«"+variantLabel(vr)+"», "+objectLine(lines[0].Kind)))
			}
		}
		res.Next = []string{"change_conditions", "request_quote"}
		return res
	} else {
		// «Не знаю»: все уровни, результат предварительный.
		t.prelim = true
		res.Type = out.GuideComparison
		tbl := &out.GuideComparisonTbl{}
		for _, vr := range variants {
			tbl.Columns = append(tbl.Columns, out.GuideColumn{FamilyID: fid, Title: "«" + variantLabel(vr) + "»", FamilyURL: t.cat.famURL[fid]})
		}
		for _, l := range lines {
			price := out.GuideRow{Key: "price:" + l.Kind, Label: "Цена за " + unitNoun(l.Kind) + ", " + termRowLabel(grid, variants, l.Kind), Kind: "commercial"}
			total := out.GuideRow{Key: "total:" + l.Kind, Label: "Итого на " + countLine(l), Kind: "commercial"}
			for _, vr := range variants {
				c, ok := grid[vr][l.Kind]
				if !ok || c.v.p.PriceRub == nil {
					price.Cells = append(price.Cells, out.GuideCell{Kind: out.CellUnknown})
					total.Cells = append(total.Cells, out.GuideCell{Kind: out.CellValue, Value: "по запросу"})
					continue
				}
				price.Cells = append(price.Cells, out.GuideCell{Kind: out.CellValue, Value: *c.v.p.PriceRub})
				total.Cells = append(total.Cells, out.GuideCell{Kind: out.CellValue, Value: guideRound2(*c.v.p.PriceRub * float64(c.v.units(c.qty)))})
			}
			tbl.Rows = append(tbl.Rows, price, total)
		}
		res.Comparison = tbl
		for _, vr := range variants {
			if c, ok := grid[vr][lines[0].Kind]; ok {
				res.Offers = append(res.Offers, t.offerOut(*c.v, c.qty, lines[0].Kind, out.OfferMatch, "«"+variantLabel(vr)+"», "+objectLine(lines[0].Kind)))
			}
		}
		res.Lead = "Показываю все три уровня."
		if rule := t.kb.ruleByID("astra_orel_scope"); rule.show && rule.Message != "" {
			res.Lead = "Показываю все три уровня. " + strings.TrimRight(rule.Message, ".") + "."
		}
		res.Notice = "Уровень защищённости определяют требования к вашей системе — результат предварительный"
	}
	if matched == 0 {
		res.Type = out.GuideNoData
		res.Notice = "Под эти условия подходящих позиций нет — измените условия или запросите КП"
	}
	res.Next = t.nextSteps(res)
	return res
}

// countLine — «30 рабочих станций», «3 сервера».
func countLine(l domain.GuideObject) string {
	if l.Kind == "workstation" {
		return fmt.Sprintf("%d %s", l.Count, guidePlural(l.Count, "рабочая станция", "рабочие станции", "рабочих станций"))
	}
	return countLabel(l.Kind, l.Count)
}

func objectLine(kind string) string {
	if kind == "server" {
		return "строка «серверы»"
	}
	return "строка «рабочие станции»"
}

func unitNoun(kind string) string {
	if kind == "server" {
		return "сервер"
	}
	return "рабочую станцию"
}

// guideVariantCell — оффер варианта для строки объекта.
type guideVariantCell struct {
	v   *guideOfferView
	qty int
}

// termRowLabel — срок позиций строки: «1 год» или «бессрочная» (у всех уровней одинаковый — иначе «срок разный»).
func termRowLabel(grid map[string]map[string]guideVariantCell, variants []string, kind string) string {
	label := ""
	for _, vr := range variants {
		if c, ok := grid[vr][kind]; ok {
			l := c.v.termLabel()
			if label != "" && label != l {
				return "срок разный"
			}
			label = l
		}
	}
	return label
}
