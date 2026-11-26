package out

// IntentAnalytics — учёт поисковых намерений (§11.6, RESEARCH контура выдачи «Ж. Метрики»). Считается
// по событиям с intent_id за период. Конверсия — в двух определениях рядом: рост не объявляется по одной из них.
type IntentAnalytics struct {
	// Intents — намерений с исходом; Open — намерений без исхода (ввод не закончен или исход не записан).
	Intents int `json:"intents"`
	Open    int `json:"open"`
	// Outcomes — намерения по исходу: commit | suggestion | product | category | vendor | abandon.
	Outcomes []IntentOutcome `json:"outcomes"`
	// ConversionLegacy — «прежняя»: только отправленные запросы и подсказки (исход commit | suggestion); успех —
	// действие с товаром в том же намерении (клик, корзина, заказ, запрос цены).
	ConversionLegacy IntentRate `json:"conversion_legacy"`
	// ConversionUnified — «единая»: все намерения с исходом, включая отказ; успех — действие с товаром или клик по
	// товару в панели (исход product — это исход, а не пропавший поиск).
	ConversionUnified IntentRate `json:"conversion_unified"`
	// EmptyResults — доля пустых выдач: только по отправленным запросам (search_commit с известным числом результатов).
	EmptyResults IntentRate `json:"empty_results"`
	// EmptySuggestions — доля пустых подсказок: показы панели (suggest_impression) на префиксе от 2 символов без
	// единого элемента. Отдельная плитка, с пустыми выдачами не складывается.
	EmptySuggestions IntentRate `json:"empty_suggestions"`
	// Sources — откуда события намерений: live | mock_log.
	Sources map[string]int `json:"sources"`
}

// IntentOutcome — исход намерения: сколько и сколько из них с действием по товару.
type IntentOutcome struct {
	Outcome   string   `json:"outcome"`
	Label     string   `json:"label"`
	Count     int      `json:"count"`
	Share     *float64 `json:"share"`
	Converted int      `json:"converted"`
}

// IntentRate — доля: числитель, знаменатель, значение (nil — знаменатель 0, не посчитано).
type IntentRate struct {
	Num   int      `json:"num"`
	Den   int      `json:"den"`
	Value *float64 `json:"value"`
}
