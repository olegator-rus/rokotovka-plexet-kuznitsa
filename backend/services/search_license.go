package services

import (
	"fmt"
	"strings"

	"kuznica/api/domain"
)

// Строка лицензии для карточки выдачи: «Подписка · 10 устройств · 1 год».

var licenseTypeLabels = map[string]string{
	domain.LicenseSubscription: "Подписка",
	domain.LicensePerpetual:    "Бессрочная",
	domain.LicenseRenewal:      "Продление",
	domain.LicenseUpgrade:      "Обновление",
	domain.LicenseOEM:          "OEM",
	domain.LicenseCloud:        "Облачная",
}

// licenseKindLabels — вид позиции фида, если это не базовая лицензия: подпись вместо типа («Продление», а не «Подписка»).
var licenseKindLabels = map[string]string{
	domain.LicenseKindRenewal:   "Продление",
	domain.LicenseKindUpgrade:   "Обновление",
	domain.LicenseKindMigration: "Переход с другого продукта",
	domain.LicenseKindSupport:   "Техподдержка",
	domain.LicenseKindModule:    "Модуль",
	domain.LicenseKindCourse:    "Курс",
}

// metricForms — формы слова для 1, 2–4, 5+.
var metricForms = map[string][3]string{
	domain.MetricDevice: {"устройство", "устройства", "устройств"},
	domain.MetricUser:   {"пользователь", "пользователя", "пользователей"},
	domain.MetricServer: {"сервер", "сервера", "серверов"},
	domain.MetricCore:   {"ядро", "ядра", "ядер"},
}

// licenseLine — тип, количество с метрикой, срок; неизвестные части пропускаются.
func licenseLine(d indexDoc) string {
	var parts []string
	if l := licenseKindLabels[d.LicenseKind]; l != "" {
		parts = append(parts, l)
	} else if l := licenseTypeLabels[d.LicenseType]; l != "" {
		parts = append(parts, l)
	} else if d.LicenseType != "" {
		parts = append(parts, d.LicenseType)
	}
	metric := ""
	if d.LicenseMetric != nil {
		metric = *d.LicenseMetric
	}
	switch {
	case metric == domain.MetricOrg:
		parts = append(parts, "на организацию")
	case d.LicenseQuantity != nil && *d.LicenseQuantity > 0 && metricForms[metric][0] != "": // «0 серверов» не пишем
		parts = append(parts, fmt.Sprintf("%d %s", *d.LicenseQuantity, plural(*d.LicenseQuantity, metricForms[metric])))
	}
	if d.LicenseDuration != nil && *d.LicenseDuration > 0 {
		parts = append(parts, duration(*d.LicenseDuration))
	}
	return strings.Join(parts, " · ")
}

// duration — срок: годы, если делится на 12, иначе месяцы.
func duration(months int) string {
	if months%12 == 0 {
		y := months / 12
		return fmt.Sprintf("%d %s", y, plural(y, [3]string{"год", "года", "лет"}))
	}
	return fmt.Sprintf("%d %s", months, plural(months, [3]string{"месяц", "месяца", "месяцев"}))
}

// plural — русская форма слова для числа n.
func plural(n int, forms [3]string) string {
	n %= 100
	if n < 0 {
		n = -n
	}
	switch {
	case n >= 11 && n <= 14:
		return forms[2]
	case n%10 == 1:
		return forms[0]
	case n%10 >= 2 && n%10 <= 4:
		return forms[1]
	}
	return forms[2]
}

// denseFilters — обязательные условия векторного списка. Вектор в индексе один на семейство (у позиции-представителя,
// catalog_import.go embedRank), поэтому условие на вариант лицензии — «продление» — сюда не входит: с ним в списке
// оставались только семейства, чей представитель случайно оказался продлением (несколько десятков на весь каталог),
// и последняя из двух таких позиций получала в слиянии второе место — «продлить антивирус» открывался не антивирусом.
// Условия уровня семейства (вендор, категория намерения, отечественное, характеристики оборудования) — те же, что у
// текстового поиска.
func denseFilters(attrs queryAttrs, ent queryEntities, hw *hwPlan) []any {
	attrs.renewal = false
	return append(strictFilters(attrs, ent), hw.filters()...)
}

// renewalDense — векторный список запроса с продлением: семейство, найденное по смыслу, представляет его
// позиция-продление — та, которую выше всех поставил текстовый поиск (ранги двух списков складываются у одной
// позиции: смысл найден у семейства, а не у случайного его варианта), иначе сам представитель, если он продление,
// иначе первое продление семейства. Семейство без продлений из списка уходит; ранги пересчитаны подряд, скор —
// скор представителя.
func renewalDense(lx *searchLexicon, dense, bm25 []domain.RankedHit) []domain.RankedHit {
	found := map[string]string{} // семейство → лучшая позиция текстового списка
	for _, h := range bm25 {
		if d, ok := lx.docs[h.ProductID]; ok && d.LicenseType == domain.LicenseRenewal && found[d.FamilyID] == "" {
			found[d.FamilyID] = d.ProductID
		}
	}
	res := make([]domain.RankedHit, 0, len(dense))
	seen := map[string]bool{}
	for _, h := range dense {
		d, ok := lx.docs[h.ProductID]
		if !ok {
			continue
		}
		pid := found[d.FamilyID]
		if pid == "" && d.LicenseType == domain.LicenseRenewal {
			pid = d.ProductID
		}
		if pid == "" {
			for _, v := range lx.families[d.FamilyID] {
				if lx.docs[v].LicenseType == domain.LicenseRenewal {
					pid = v
					break
				}
			}
		}
		if pid == "" || seen[pid] {
			continue
		}
		seen[pid] = true
		res = append(res, domain.RankedHit{ProductID: pid, Rank: len(res) + 1, Score: h.Score})
	}
	return res
}
