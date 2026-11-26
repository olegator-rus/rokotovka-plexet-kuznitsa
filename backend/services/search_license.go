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
	case d.LicenseQuantity != nil && metricForms[metric][0] != "":
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
