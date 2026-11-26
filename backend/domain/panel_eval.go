package domain

// Оценка панели поиска контура выдачи: набор префиксов data/mock/prefixes.json и прогон
// `make eval SET=panel` по прежним подсказкам (контур коммерции) и панели контура выдачи.

// EvalSetPanel — набор префиксов панели (не из queries.json).
const EvalSetPanel = "panel"

// Типы префиксов (PanelPrefix.Type).
const (
	PrefixBroad        = "broad"
	PrefixFamily       = "family"
	PrefixSKU          = "sku"
	PrefixLayout       = "layout"
	PrefixTypo         = "typo"
	PrefixNoResults    = "no_results"
	PrefixLicenseDupes = "license_dupes"
)

// PrefixTypes — допустимые типы по порядку показа.
var PrefixTypes = []string{PrefixBroad, PrefixFamily, PrefixSKU, PrefixLayout, PrefixTypo, PrefixNoResults, PrefixLicenseDupes}

// PanelPrefix — строка prefixes.json: что набрано и чего человек ждёт. Source — откуда цель: log_late (успешный
// поиск позднего периода журнала), log_empty (пустая выдача позднего периода), catalog (артикул, семейство).
type PanelPrefix struct {
	ID            string  `json:"id"`
	Prefix        string  `json:"prefix"`
	TargetQuery   string  `json:"target_query"`
	TargetFamily  *string `json:"target_family"`
	TargetProduct *string `json:"target_product"`
	Type          string  `json:"type"`
	Set           string  `json:"set"`
	Source        string  `json:"source,omitempty"`
	Freq          int     `json:"freq,omitempty"`
}
