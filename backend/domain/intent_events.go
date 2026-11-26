package domain

// События поискового намерения контура выдачи. Все — с intent_id.
const (
	EventSearchOpened          = "search_opened"
	EventSuggestImpression     = "suggest_impression"
	EventSuggestionSelect      = "suggestion_select"
	EventInstantProductClick   = "instant_product_click"
	EventCategorySelect        = "category_select"
	EventVendorSelect          = "vendor_select"
	EventRefinementApply       = "refinement_apply"
	EventRefinementRemove      = "refinement_remove"
	EventSearchCommit          = "search_commit"
	EventSearchAbandon         = "search_abandon"
	EventVariantExpand         = "variant_expand"
	EventFilterApply           = "filter_apply"
	EventFilterRemove          = "filter_remove"
	EventFiltersReset          = "filters_reset"
	EventSortChange            = "sort_change"
	EventGroupChange           = "group_change"
	EventRecsImpression        = "recs_impression"
	EventRecsClick             = "recs_click"
	EventRecsAdd               = "recs_add"
	EventPersonalizationToggle = "personalization_toggle"
	EventHistoryClear          = "history_clear"
)

// IntentEventTypes — типы контура выдачи: принимаются POST /api/events только с intent_id.
var IntentEventTypes = []string{
	EventSearchOpened, EventSuggestImpression, EventSuggestionSelect, EventInstantProductClick, EventCategorySelect,
	EventVendorSelect, EventRefinementApply, EventRefinementRemove, EventSearchCommit, EventSearchAbandon,
	EventVariantExpand, EventFilterApply, EventFilterRemove, EventFiltersReset, EventSortChange, EventGroupChange,
	EventRecsImpression, EventRecsClick, EventRecsAdd, EventPersonalizationToggle, EventHistoryClear,
}

// Исходы намерения: первое событие-исход определяет исход всего намерения.
const (
	IntentCommit     = "commit"
	IntentSuggestion = "suggestion"
	IntentProduct    = "product"
	IntentCategory   = "category"
	IntentVendor     = "vendor"
	IntentAbandon    = "abandon"
)

// IntentOutcomes — исходы по порядку показа.
var IntentOutcomes = []string{IntentCommit, IntentSuggestion, IntentProduct, IntentCategory, IntentVendor, IntentAbandon}

// IntentOutcomeEvents — событие → исход намерения. У search_commit исход уточняет payload.outcome (фронт шлёт его
// на любом исходе, кроме отказа: commit | suggestion | product | category | vendor).
var IntentOutcomeEvents = map[string]string{
	EventSearchCommit: IntentCommit, EventSuggestionSelect: IntentSuggestion, EventInstantProductClick: IntentProduct,
	EventCategorySelect: IntentCategory, EventVendorSelect: IntentVendor, EventSearchAbandon: IntentAbandon,
}

// intentRequiredFields — обязательные поля событий контура выдачи (product_id — поле события, остальное — payload).
// Минимум, без которого не посчитать исход: остальные поля «События» необязательны.
var intentRequiredFields = map[string][]string{
	EventInstantProductClick: {"product_id"},
	EventRecsClick:           {"product_id"},
	EventRecsAdd:             {"product_id"},
}

// Типы контура выдачи дописываются к допустимым и обязательным полям (аддитивно к контурам поиска и коммерции).
func init() {
	BehaviorEventTypes = append(BehaviorEventTypes, IntentEventTypes...)
	for t, f := range intentRequiredFields {
		EventRequiredFields[t] = f
	}
}
