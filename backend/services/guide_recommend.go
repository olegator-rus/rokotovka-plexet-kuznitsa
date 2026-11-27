package services

import (
	"fmt"
	"sort"
	"strings"

	"kuznica/api/out"
)

// Рекомендация среди подходящих (контур подбора, после живой проверки): одна позиция с причиной из каталога или фактов,
// сравнение вендоров по приведённой единице и вопрос, который реально разводит варианты.

// recommendBlock — к ответу selection с несколькими подходящими: рекомендация первой, таблица между вендорами,
// фраза по делу, вопрос «Что ещё важно?» по функциям, которыми подходящие различаются.
func (t *guideTurn) recommendBlock(res *out.GuideResponse, data *guideData) {
	r := t.st.Requirements
	var idx []int
	vendors := map[string]bool{}
	for i, o := range res.Offers {
		if o.Role == out.OfferMatch && o.PriceState == out.PricePriced {
			idx = append(idx, i)
			vendors[t.cat.famVend[o.FamilyID]] = true
		}
	}
	if len(idx) < 2 {
		return
	}
	pick, basis, reason := -1, "", ""
	// 1) требуемая функция по факту; 2) названная редакция — rankNamed уже поставил их первыми.
	first := res.Offers[idx[0]]
	for _, axis := range r.RequiredFeatures {
		if f := t.kb.factYesAnyVariant(first.FamilyID, axis); f != nil {
			pick, basis = idx[0], "feature"
			label := f.AxisLabel
			if label == "" {
				label = axisLabel(axis)
			}
			reason = "Есть «" + guideLowerFirst(label) + "» по данным вендора"
		}
	}
	if pick < 0 && guideNameScore(t.cat.famName[first.FamilyID], guideProductTokens(t.query+" "+t.text)) >
		guideNameScore(t.cat.famName[res.Offers[idx[1]].FamilyID], guideProductTokens(t.query+" "+t.text)) {
		pick, basis, reason = idx[0], "named", "Редакция, которую вы назвали"
	}
	if pick < 0 {
		// 3) дешевле за место в год среди подходящих (цена за место — у пакета цена пакета / мест).
		best := -1.0
		for _, i := range idx {
			o := res.Offers[i]
			p := t.cat.products[o.OfferID]
			if p == nil || o.PricePerUnit == nil {
				continue
			}
			v := t.cat.view(p, t.kb)
			if v.term == 0 {
				continue // бессрочную в цену за год не пересчитываем
			}
			y := *o.PricePerUnit * 12 / float64(v.term)
			if best < 0 || y < best {
				best, pick = y, i
			}
		}
		if pick >= 0 {
			noun := "место"
			if data.kind == "workstation" {
				noun = "компьютер"
			}
			basis, reason = "price_per_seat", fmt.Sprintf("Дешевле всех за %s в год среди подходящих: %s ₽", noun, rubRU(guideRound2(best)))
		}
	}
	if pick < 0 {
		return
	}
	rec := res.Offers[pick]
	res.Recommendation = &out.GuideRecommendation{OfferID: rec.OfferID, Reason: reason, Basis: basis}
	if pick != 0 {
		res.Offers = append([]out.GuideOffer{rec}, append(append([]out.GuideOffer{}, res.Offers[:pick]...), res.Offers[pick+1:]...)...)
	}
	short := t.shortOf(data, rec.FamilyID)
	switch basis {
	case "price_per_seat":
		res.Lead = "Для " + t.countPhrase(data) + " дешевле всего за место — «" + short + "»."
	case "feature":
		res.Lead = "Рекомендую «" + short + "»: требуемая функция есть по данным вендора."
	default:
		res.Lead = "Рекомендую «" + short + "» — редакцию, которую вы назвали."
	}
	if len(vendors) >= 2 {
		res.Lead += " Состав функций у продуктов различается."
		t.crossTable(res, data)
	}
	t.dividingQuestion(res, data)
}

// countPhrase — «20 ПК», «5 серверов» для фразы (число — из условий).
func (t *guideTurn) countPhrase(data *guideData) string {
	return countLabel(orDefault(data.kind, "workstation"), data.qty)
}

// crossTable — таблица между вендорами по подходящим карточкам: единица, назначение и платформы из фактов, цена за
// место в год (только приведённая единица), итог.
func (t *guideTurn) crossTable(res *out.GuideResponse, data *guideData) {
	var fams []*guideFamData
	for _, o := range res.Offers {
		if o.Role != out.OfferMatch {
			continue
		}
		for _, fd := range data.fams {
			if fd.fid == o.FamilyID && len(fd.matched) > 0 {
				// Колонка — та же позиция, что в карточке.
				cp := *fd
				for _, v := range fd.matched {
					if v.p.ID == o.OfferID {
						cp.matched = []guideOfferView{v}
					}
				}
				fams = append(fams, &cp)
			}
		}
	}
	if len(fams) < 2 {
		return
	}
	prevCross := t.cross
	t.cross = true
	sub := &guideData{fams: fams, qty: data.qty, kind: data.kind}
	tbl := t.comparison(sub)
	t.cross = prevCross
	res.Comparison = tbl.Comparison
}

// dividingQuestion — «Что ещё важно?»: функции, которыми подходящие различаются по проверенным фактам (есть у одних,
// нет у других). Ответ — чип feature; без ответа подбор не блокируется.
func (t *guideTurn) dividingQuestion(res *out.GuideResponse, data *guideData) {
	if res.Clarification != nil || t.st.Questions >= 2 || len(t.st.Requirements.RequiredFeatures) > 0 {
		return
	}
	type ax struct {
		axis, label string
		yes, no     int
	}
	m := map[string]*ax{}
	for _, o := range res.Offers {
		if o.Role != out.OfferMatch {
			continue
		}
		for _, f := range t.kb.byFamily[o.FamilyID] {
			if !f.verified() {
				continue
			}
			k, _ := f.factValue()
			if k != "yes" && k != "no" {
				continue
			}
			a := m[f.Axis]
			if a == nil {
				a = &ax{axis: f.Axis, label: f.AxisLabel}
				if a.label == "" {
					a.label = axisLabel(f.Axis)
				}
				m[f.Axis] = a
			}
			if k == "yes" {
				a.yes++
			} else {
				a.no++
			}
		}
	}
	var axes []*ax
	for _, a := range m {
		if a.yes > 0 && a.no > 0 {
			axes = append(axes, a)
		}
	}
	if len(axes) == 0 {
		return
	}
	sort.Slice(axes, func(i, j int) bool {
		if axes[i].yes+axes[i].no != axes[j].yes+axes[j].no {
			return axes[i].yes+axes[i].no > axes[j].yes+axes[j].no
		}
		return axes[i].axis < axes[j].axis
	})
	chips := []out.GuideChip{}
	for _, a := range axes {
		if len(chips) >= 3 {
			break
		}
		chips = append(chips, out.GuideChip{Label: "нужно: " + strings.ToLower(a.label), Key: "feature", Value: a.axis})
	}
	chips = append(chips, out.GuideChip{Label: "ничего из этого", Key: "feature", Value: ""})
	t.st.Pending, t.st.Questions = "feature", t.st.Questions+1
	res.Clarification = &out.GuideClarify{Key: "feature", Question: "Что ещё важно? Варианты различаются этим", Chips: chips}
}
