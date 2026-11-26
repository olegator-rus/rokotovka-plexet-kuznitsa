package out

// Страница результатов контура выдачи (флаг FEATURE_RESULTS_V3). Поля ответа
// поиска с omitempty: флаг выключен — ответ как в контуре коммерции.

// Refinement — объявлен в suggest.go (общий тип бейджей панели и чипов-уточнений над выдачей).

// ResultCounts — заголовок «N продуктов · M вариантов»: семейства и позиции выдачи до обрезки по limit.
type ResultCounts struct {
	Families int `json:"families"`
	Variants int `json:"variants"`
}

// ResultSort — применённая сортировка и пояснение к ней.
type ResultSort struct {
	Value string `json:"value"`
	Note  string `json:"note"`
}

// ItemAttrs — атрибуты карточки для строки атрибутов: одинаковый набор у всех карточек категории,
// отсутствующее значение — null/"" (фронт показывает прочерк).
type ItemAttrs struct {
	Edition        string `json:"edition"`
	LicenseType    string `json:"license_type"`
	LicenseMetric  string `json:"license_metric"`
	Quantity       *int   `json:"license_quantity"`
	DurationMonths *int   `json:"duration_months"`
	// Delivery — electronic | oem | physical | "" (нет данных).
	Delivery string `json:"delivery"`
	InReestr bool   `json:"in_reestr"`
}
