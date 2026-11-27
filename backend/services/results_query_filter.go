package services

import (
	"fmt"
	"sort"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
)

// narrowFamilies — варианты семейств под фильтры: семейство без подходящих вариантов уходит, голова
// пересчитывается, если прежняя не прошла.
func narrowFamilies(rc rankCtx, fams []*familyHit, fs []licFilter, rejected *[]domain.RejectedHit) []*familyHit {
	res := make([]*familyHit, 0, len(fams))
	changed := false
	for _, fh := range fams {
		var heads []indexDoc
		for _, d := range fh.heads {
			if passAll(fs, d, "") {
				heads = append(heads, d)
			} else if rejected != nil {
				addRejected(rejected, d.ProductID, rejectFacet, "снята фильтром выдачи")
			}
		}
		if len(heads) == 0 {
			continue
		}
		cp := *fh
		cp.heads = heads
		if heads[0].ProductID != fh.head.ProductID {
			cp.head, cp.headOK = heads[0], fh.fits[heads[0].ProductID]
			rc.score(&cp)
			changed = true
		}
		res = append(res, &cp)
	}
	if changed {
		sortFamilies(res)
		// Дописанные по фильтру категории или производителя — всегда после найденных поиском.
		sort.SliceStable(res, func(i, j int) bool { return !isFill(res[i]) && isFill(res[j]) })
	}
	return res
}

// queryLicFilters — срок и тип лицензии, названные в тексте запроса («касперский на год», «astra бессрочная»),
// в виде фильтров выдачи. Фильтр того же поля, выбранный покупателем в панели, важнее — его поле пропускается.
func queryLicFilters(chips []out.Chip, chosen []licFilter) []licFilter {
	has := map[string]bool{}
	for _, f := range chosen {
		has[f.key] = true
	}
	var res []licFilter
	for _, c := range chips {
		switch {
		case c.Field == domain.FieldLicenseDuration && !has[facetDuration]:
			res = append(res, parseLicFilters([]in.Filter{{Key: facetDuration, Op: "eq", Value: c.Value}})...)
		case c.Field == domain.FieldLicenseType && !has[facetLicType] &&
			(c.Value == domain.LicensePerpetual || c.Value == domain.LicenseSubscription):
			res = append(res, licFilter{key: facetLicType, op: "eq", str: c.Value})
		}
	}
	return res
}

// narrowByQuery — срок и тип лицензии из запроса сужают выдачу так же, как фильтр из панели: остаются варианты
// с этим сроком, карточка показывает подходящую лицензию. Раньше эти условия только поднимали подходящее в
// сортировке, и «касперский на год» показывал в вариантах и двухлетние лицензии.
//
// Сужение не подменяет ответ: если у первого семейства выдачи (лучшее совпадение) такого срока или типа нет
// («касперский бессрочно» — у KES бессрочной нет, есть только медиапакеты), выдача остаётся прежней, а в
// relaxed — чего нет. Ничего не подошло вовсе — выдача прежняя
// (сообщение relaxed об этом уже есть, relaxedMessages). Чипы применённых строго условий помечаются strict.
func (p *resultsPlan) narrowByQuery(rc rankCtx, fams []*familyHit, chips []out.Chip, rejected *[]domain.RejectedHit) []*familyHit {
	qf := queryLicFilters(chips, p.filters)
	if len(qf) == 0 || len(fams) == 0 || p.exact {
		return fams
	}
	res := narrowFamilies(rc, fams, qf, nil)
	if len(res) == 0 {
		return fams
	}
	// Лучшее совпадение — первое семейство в порядке страницы (apply сортирует позже, здесь — копия).
	top := append([]*familyHit(nil), fams...)
	p.sortFamilies(top)
	if !hasFamily(res, top[0].id) {
		p.queryRelaxed = append(p.queryRelaxed, out.Relaxed{Field: qf[0].key,
			Message: fmt.Sprintf("У лучших совпадений нет варианта «%s», показаны все варианты", queryFilterLabel(chips))})
		return fams
	}
	narrowFamilies(rc, fams, qf, rejected) // в трассу — снятые варианты
	for i := range chips {
		if chips[i].Field == domain.FieldLicenseDuration || chips[i].Field == domain.FieldLicenseType {
			chips[i].Strict = true
		}
	}
	return res
}

func hasFamily(fams []*familyHit, id string) bool {
	for _, fh := range fams {
		if fh.id == id {
			return true
		}
	}
	return false
}

// queryFilterLabel — подписи чипов срока и типа: «Срок 3 года», «Бессрочная».
func queryFilterLabel(chips []out.Chip) string {
	label := ""
	for _, c := range chips {
		if c.Field == domain.FieldLicenseDuration || c.Field == domain.FieldLicenseType {
			if label != "" {
				label += ", "
			}
			label += c.Label
		}
	}
	return label
}

// queryOrg — тип организации, названный в запросе («для школы», «для вуза», «образовательная», «для
// госучреждения»), у которого в фиде бывают свои варианты лицензий; "" — не назван или таких вариантов не бывает
// (коммерческая лицензия — вариант по умолчанию).
func queryOrg(chips []out.Chip) string {
	for _, c := range chips {
		if c.Field != FieldOrgType {
			continue
		}
		for _, org := range feedOrg {
			if org == c.Value {
				return org
			}
		}
	}
	return ""
}

// orgVariant — вариант лицензии под тип организации org: License.org_type фида или атрибут org_type позиции.
func orgVariant(d indexDoc, org string) bool {
	return feedOrg[d.LicenseOrgType] == org || docAttr(d, "org_type") == org
}

// narrowByOrg — тип организации из запроса работает как срок и тип лицензии (narrowByQuery): у семейства, где есть
// варианты под эту организацию («astra linux для школы» — лицензии для образовательных учреждений), остаются только
// они, и карточка показывает такой вариант. Раньше слово только переставало скрывать эти варианты, а головой
// оставалась коммерческая лицензия.
//
// Откат мягкий и по каждому семейству отдельно: нет у семейства такого варианта — его карточка прежняя, из выдачи оно
// не уходит. Тип организации размечен в фиде не у всех («Альт Образование», «РЕД ОС для образовательных организаций»
// идут без него), и убирать такие семейства по слову «школа» нельзя. Чип помечается strict, когда условие применено
// к первому семейству страницы.
func (p *resultsPlan) narrowByOrg(rc rankCtx, fams []*familyHit, chips []out.Chip, rejected *[]domain.RejectedHit) []*familyHit {
	org := queryOrg(chips)
	if org == "" || len(fams) == 0 || p.exact {
		return fams
	}
	res := make([]*familyHit, 0, len(fams))
	applied := map[string]bool{}
	changed := false
	for _, fh := range fams {
		var heads []indexDoc
		for _, d := range fh.heads {
			if orgVariant(d, org) {
				heads = append(heads, d)
			}
		}
		if len(heads) == 0 {
			res = append(res, fh)
			continue
		}
		applied[fh.id] = true
		if len(heads) == len(fh.heads) {
			res = append(res, fh)
			continue
		}
		for _, d := range fh.heads {
			if rejected != nil && !orgVariant(d, org) {
				addRejected(rejected, d.ProductID, rejectOffer, "в запросе назван покупатель: "+strings.ToLower(orgLabels[org]))
			}
		}
		cp := *fh
		cp.heads = heads
		if heads[0].ProductID != fh.head.ProductID {
			cp.head, cp.headOK = heads[0], fh.fits[heads[0].ProductID]
			rc.score(&cp)
			changed = true
		}
		res = append(res, &cp)
	}
	if len(applied) == 0 {
		return fams
	}
	if changed {
		sortFamilies(res)
		sort.SliceStable(res, func(i, j int) bool { return !isFill(res[i]) && isFill(res[j]) })
	}
	top := append([]*familyHit(nil), res...)
	p.sortFamilies(top)
	if applied[top[0].id] {
		for i := range chips {
			if chips[i].Field == FieldOrgType {
				chips[i].Strict = true
			}
		}
	}
	return res
}
