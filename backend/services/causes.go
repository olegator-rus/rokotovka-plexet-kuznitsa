package services

import "kuznica/api/domain"

// Причины неудачного поиска с владельцами (контур доводки «Причины и владельцы»; контур фида —
// «найдено, но только по запросу»). Коды диагнозов агента остаются подпричинами внутри причины. Владелец — роль сотрудника, которая решает предложения
// этой причины (services/access.go): правки поиска — администратор поиска (ecom), ассортимент, связи и правила —
// категорийный и лицензионный менеджер (product), данные карточки — каталог (content). «Запрос не о товаре»
// ведёт справочный индекс поиска — тоже администратор поиска; шум и неразобранное — ручной разбор руководителя.

// Коды причин.
const (
	CauseNotUnderstood = "not_understood"
	CauseRankedLow     = "ranked_low"
	CauseContentGap    = "content_gap"
	CauseNotInCatalog  = "not_in_catalog"
	CauseOnRequestOnly = "on_request_only"
	CauseRelationRule  = "relation_rule"
	CauseNonProduct    = "non_product"
	CauseNoise         = "noise"
)

// Cause — причина, её подпричины (коды диагнозов) и владелец.
type Cause struct {
	Code       string   `json:"code"`
	Label      string   `json:"label"`
	Diagnoses  []string `json:"diagnoses"`
	Owner      string   `json:"owner"`
	OwnerLabel string   `json:"owner_label"`
}

// Causes — таблица в порядке показа. SUGGESTION_GAP, FAMILY_DUPLICATE, SUGGESTION_DEGRADED агент пока
// не ставит — место для них в таблице уже есть.
var Causes = []Cause{
	{Code: CauseNotUnderstood, Label: "Поиск не понял запрос",
		Diagnoses: []string{domain.DiagTypo, domain.DiagAliasGap, "SUGGESTION_GAP"}, Owner: RoleEcom},
	{Code: CauseRankedLow, Label: "Нужный товар стоит низко или задублирован",
		Diagnoses: []string{domain.DiagRankingFailure, "FAMILY_DUPLICATE", "SUGGESTION_DEGRADED"}, Owner: RoleEcom},
	{Code: CauseContentGap, Label: "В карточке не хватает данных",
		Diagnoses: []string{domain.DiagContentGap}, Owner: RoleContent},
	{Code: CauseNotInCatalog, Label: "Товара нет в каталоге",
		Diagnoses: []string{domain.DiagAssortmentGap, domain.DiagSubstitute}, Owner: RoleProduct},
	// Контур фида: на фиде две трети каталога — «по запросу». Товар находится, но купить нельзя — задача каталогу
	// (цена и наличие), поиск тут не виноват.
	{Code: CauseOnRequestOnly, Label: "Найдено, но только по запросу",
		Diagnoses: []string{domain.DiagOnRequestOnly}, Owner: RoleContent},
	{Code: CauseRelationRule, Label: "Ошибка в связи или лицензионном правиле",
		Diagnoses: []string{domain.DiagBrokenRelation, domain.DiagMissingRequired, domain.DiagConstraintConflict}, Owner: RoleProduct},
	{Code: CauseNonProduct, Label: "Запрос не о товаре",
		Diagnoses: []string{domain.DiagNonProduct}, Owner: RoleEcom},
	{Code: CauseNoise, Label: "Шум или недостаточно данных",
		Diagnoses: []string{domain.DiagNoise, domain.DiagUnknown}, Owner: RoleHead},
}

var causeByDiagnosis = func() map[string]*Cause {
	m := map[string]*Cause{}
	for i := range Causes {
		c := &Causes[i]
		c.OwnerLabel = RoleLabel(c.Owner)
		for _, d := range c.Diagnoses {
			m[d] = c
		}
	}
	return m
}()

// CauseOf — причина диагноза; неизвестный или пустой код — «Шум или недостаточно данных» (ручной разбор).
func CauseOf(diagnosis string) Cause {
	if c, ok := causeByDiagnosis[diagnosis]; ok {
		return *c
	}
	return Causes[len(Causes)-1]
}

// OwnerOf — роль, которая решает предложение с этим диагнозом.
func OwnerOf(diagnosis string) string { return CauseOf(diagnosis).Owner }

// KnownCause — код причины есть в таблице.
func KnownCause(code string) bool {
	for _, c := range Causes {
		if c.Code == code {
			return true
		}
	}
	return false
}
