package services

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Строка сравнения цены за место. Сравнивается только подобное с подобным: одна
// категория (класс ПО), один вид и тип лицензии, один тип организации, один срок, одна ступень количества и одна
// метрика. Пара одна — две самые дешёвые подходящие точные карточки; общий механизм не расширяется. Формулировка
// «При этих условиях X на N% дешевле, чем Y, за узел» — без «лучше» и «самый выгодный» (ст. 5 закона «О рекламе»)
// и без дат.

// comparisonPlace — «за узел» / «за пользователя» по метрике; ступень или поштучная позиция — цена за одно место.
var comparisonPlace = map[string]string{domain.MetricDevice: "узел", domain.MetricUser: "пользователя",
	domain.MetricServer: "сервер", domain.MetricCore: "ядро"}

// comparableKey — условия, при которых цены двух позиций сравнимы. "" — позиция не сравнивается (нет ступени
// или количества: цену за место не посчитать).
func comparableKey(d indexDoc) string {
	metric := ""
	if d.LicenseMetric != nil {
		metric = *d.LicenseMetric
	}
	if comparisonPlace[metric] == "" || d.LicenseDuration == nil {
		return ""
	}
	qty := ""
	switch {
	case d.LicenseTierMin != nil && d.LicenseTierMax != nil:
		qty = fmt.Sprintf("tier:%d-%d", *d.LicenseTierMin, *d.LicenseTierMax)
	case d.LicenseQuantity != nil && *d.LicenseQuantity > 0:
		qty = fmt.Sprintf("qty:%d", *d.LicenseQuantity)
	default:
		return ""
	}
	return strings.Join([]string{docCat2(d), d.LicenseKind, d.LicenseType, d.LicenseOrgType,
		fmt.Sprint(*d.LicenseDuration), qty, metric}, "|")
}

// perPlace — цена за одно место: у ступени цена фида — уже за место, у пакета — делится на количество.
func perPlace(d indexDoc) float64 {
	if d.LicenseTierMin != nil && d.LicenseTierMax != nil {
		return *d.PriceRub
	}
	return *d.PriceRub / float64(*d.LicenseQuantity)
}

// priceComparison — одна пара точных карточек с ценой при одинаковых условиях; nil — такой пары нет.
func priceComparison(lx *searchLexicon, items []out.SearchItem) *out.PriceComparison {
	groups := map[string][]indexDoc{}
	var order []string
	seenFam := map[string]bool{}
	for _, it := range items {
		if it.MatchType != out.MatchExact || !it.MatchesQuery || itemPrice(it) == nil || seenFam[it.FamilyID] {
			continue
		}
		d, ok := lx.docs[it.ProductID]
		if !ok || !buyable(d) {
			continue
		}
		k := comparableKey(d)
		if k == "" {
			continue
		}
		seenFam[it.FamilyID] = true
		if groups[k] == nil {
			order = append(order, k)
		}
		groups[k] = append(groups[k], d)
	}
	for _, k := range order {
		ds := groups[k]
		if len(ds) < 2 {
			continue
		}
		// Редакции одной линейки (общее начало названия не короче двух слов, без объекта применения в названии) —
		// самая дешёвая пара за место.
		sort.SliceStable(ds, func(i, j int) bool { return perPlace(ds[i]) < perPlace(ds[j]) })
		a, b := -1, -1
		for i := 0; i < len(ds) && a < 0; i++ {
			for j := i + 1; j < len(ds); j++ {
				if sameLine(ds[i].FamilyName, ds[j].FamilyName) {
					a, b = i, j
					break
				}
			}
		}
		if a < 0 {
			continue
		}
		pa, pb := perPlace(ds[a]), perPlace(ds[b])
		pct := int(math.Round((pb - pa) / pb * 100))
		if pct < 1 {
			continue
		}
		na, nb := distinctNames(ds[a].FamilyName, ds[b].FamilyName)
		place := comparisonPlace[*ds[a].LicenseMetric]
		return &out.PriceComparison{
			Text:       fmt.Sprintf("При этих условиях %s на %d%% дешевле, чем %s, за %s", na, pct, nb, place),
			Conditions: comparisonConditions(ds[a]),
			Percent:    pct,
			Items: []out.ComparisonItem{
				{ProductID: ds[a].ProductID, FamilyID: ds[a].FamilyID, Name: na, Title: ds[a].Title, PerPlaceRub: pa},
				{ProductID: ds[b].ProductID, FamilyID: ds[b].FamilyID, Name: nb, Title: ds[b].Title, PerPlaceRub: pb},
			},
		}
	}
	return nil
}

// sameLine — два семейства — редакции одной линейки: общее начало названия не короче двух слов, у обоих после него
// что-то есть, и ни одно не называет объект применения («для почтовых серверов»).
func sameLine(a, b string) bool {
	if specialisedName(a) || specialisedName(b) {
		return false
	}
	wa, wb := strings.Fields(a), strings.Fields(b)
	n := 0
	for n < len(wa) && n < len(wb) && wa[n] == wb[n] {
		n++
	}
	return n >= 2 && n < len(wa) && n < len(wb)
}

// distinctNames — различающиеся части названий семейств: общее начало («Kaspersky Endpoint Security для бизнеса»)
// убирается, если после него что-то остаётся у обоих.
func distinctNames(a, b string) (string, string) {
	wa, wb := strings.Fields(a), strings.Fields(b)
	n := 0
	for n < len(wa) && n < len(wb) && wa[n] == wb[n] {
		n++
	}
	if n == 0 || n >= len(wa) || n >= len(wb) {
		return a, b
	}
	return strings.Join(wa[n:], " "), strings.Join(wb[n:], " ")
}

// comparisonConditions — условия сравнения словами: «Продление · 2 года · 100–149 устройств · для коммерческих
// организаций».
func comparisonConditions(d indexDoc) string {
	var parts []string
	if l := licenseKindLabels[d.LicenseKind]; l != "" {
		parts = append(parts, l)
	} else if l := licenseTypeLabels[d.LicenseType]; l != "" {
		parts = append(parts, l)
	}
	parts = append(parts, duration(*d.LicenseDuration))
	forms := metricForms[*d.LicenseMetric]
	if d.LicenseTierMin != nil && d.LicenseTierMax != nil {
		parts = append(parts, fmt.Sprintf("%d–%d %s", *d.LicenseTierMin, *d.LicenseTierMax, plural(*d.LicenseTierMax, forms)))
	} else {
		parts = append(parts, fmt.Sprintf("%d %s", *d.LicenseQuantity, plural(*d.LicenseQuantity, forms)))
	}
	switch d.LicenseOrgType {
	case "commercial", "":
		parts = append(parts, "для коммерческих организаций")
	default:
		if o := feedOrg[d.LicenseOrgType]; o != "" {
			parts = append(parts, "для: "+strings.ToLower(orgLabels[o]))
		}
	}
	return strings.Join(parts, " · ")
}
