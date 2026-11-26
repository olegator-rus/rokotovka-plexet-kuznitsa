package domain

// Агент «Коммерция» на статистике покупок (контур коммерции): пара семейств,
// которую устойчиво покупают вместе, а связи в справочнике нет (MISSING_RELATION, предложение relation_patch с
// source co_purchase, E4), и якорь с двумя и более значимыми дополнениями разных типов — кандидат в комплект
// (BUNDLE_OPPORTUNITY, действие bundle: принятие — «взять в работу», версия справочника не создаётся).
const (
	DiagMissingRelation   = "MISSING_RELATION"
	DiagBundleOpportunity = "BUNDLE_OPPORTUNITY"
	ActionBundle          = "bundle"
)
