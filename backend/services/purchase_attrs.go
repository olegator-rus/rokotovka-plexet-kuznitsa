package services

import (
	"fmt"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
)

// Закупка: тип покупки (новая / продление / расширение / миграция), тип организации
// (коммерческая / образовательная / государственная) и редакция — правилами, без модели. Продление — прежнее
// обязательное условие (attrs.renewal); расширение и миграция выбирают вариант семейства; тип организации —
// ограничение там, где оно задано в варианте (attributes.org_type); редакция — ответ на уточняющий вопрос.

// Поля чипов и уточнения контура доводки.
const (
	FieldPurchaseType = "purchase_type"
	FieldOrgType      = "org_type"
	FieldEdition      = "edition"
)

// Типы покупки (attributes.deal варианта и purchase.deal ответа).
const (
	DealNew       = "new"
	DealRenewal   = "renewal"
	DealExtension = "extension"
	DealMigration = "migration"
)

// Типы организации (attributes.org_type варианта и purchase.org_type ответа).
const (
	OrgCommercial = "commercial"
	OrgEducation  = "education"
	OrgGovernment = "government"
	OrgNonprofit  = "nonprofit"
)

var dealLabels = map[string]string{DealNew: "Новая лицензия", DealRenewal: "Продление", DealExtension: "Расширение",
	DealMigration: "Миграция"}

var orgLabels = map[string]string{OrgCommercial: "Коммерческая организация", OrgEducation: "Образовательное учреждение",
	OrgGovernment: "Госучреждение", OrgNonprofit: "Некоммерческая организация"}

// feedOrg — тип организации варианта фида (License.org_type: academic | government | nonprofit) → тип организации
// запроса; "" — вариант не ограничен организацией (коммерческий, для дома или тип не указан).
var feedOrg = map[string]string{"academic": OrgEducation, OrgGovernment: OrgGovernment, OrgNonprofit: OrgNonprofit}

// rejectOffer — вариант под сделку или организацию, которых запрос не называет (trace.rejected[].reason).
const rejectOffer = "offer_not_requested"

// dealStems — начало слова → тип покупки. Слова уходят в атрибут: в текст поиска не идут.
var dealStems = []struct{ stem, deal string }{
	{"расширени", DealExtension}, {"расширить", DealExtension}, {"докуп", DealExtension}, {"дозакуп", DealExtension},
	{"миграц", DealMigration}, {"мигрир", DealMigration}, {"переход", DealMigration}, {"перейти", DealMigration},
}

// orgStems — начало слова → тип организации. Слова остаются в тексте поиска: «для образования» находит
// варианты по названию, а «госкомпании» в запросах test разбирает модель — их путь не меняется.
var orgStems = []struct{ stem, org string }{
	{"образова", OrgEducation}, {"школ", OrgEducation}, {"вуз", OrgEducation}, {"учебн", OrgEducation},
	{"колледж", OrgEducation}, {"университет", OrgEducation}, {"академ", OrgEducation},
	{"госуч", OrgGovernment}, {"госком", OrgGovernment}, {"госорг", OrgGovernment}, {"государств", OrgGovernment},
	{"бюджетн", OrgGovernment}, {"муниципал", OrgGovernment},
	{"некоммерческ", OrgNonprofit}, {"нко", OrgNonprofit},
	{"коммерческ", OrgCommercial},
}

// extractPurchase — тип покупки, организация и явное требование реестра из токенов (часть extractAttrs).
func extractPurchase(tokens []string, a *queryAttrs) {
	for i, t := range tokens {
		if a.consumed[i] {
			continue
		}
		for _, d := range dealStems {
			if strings.HasPrefix(t, d.stem) {
				if a.deal == "" {
					a.deal = d.deal
				}
				a.consumed[i] = true
				break
			}
		}
		for _, o := range orgStems {
			if strings.HasPrefix(t, o.stem) && a.org == "" {
				a.org = o.org
				break
			}
		}
	}
	for _, t := range tokens {
		if strings.HasPrefix(t, "реестр") {
			a.reestrRequired = true
		}
	}
}

// applyPurchaseRequest — снятые чипы контура доводки и ответ на вопрос о редакции.
func (a *queryAttrs) applyPurchaseRequest(req in.SearchRequest) {
	for _, c := range req.RemovedChips {
		switch c.Field {
		case FieldPurchaseType:
			a.deal = ""
		case FieldOrgType:
			a.org = ""
		case FieldEdition:
			a.edition = ""
		case domain.FieldOrigin:
			a.reestrRequired = false
		}
	}
	if v, ok := req.Clarify[FieldEdition]; ok {
		a.editionAnswered = true
		if v = strings.TrimSpace(v); v != "" && len(v) <= 64 && v != clarifyAny {
			a.edition = v
		}
	}
}

// purchaseChips — чипы типа покупки, организации и редакции (дописываются к chips).
func (a queryAttrs) purchaseChips(add func(field, value, label string, strict bool)) {
	if a.deal != "" {
		add(FieldPurchaseType, a.deal, dealLabels[a.deal], false)
	}
	if a.org != "" {
		add(FieldOrgType, a.org, orgLabels[a.org], false)
	}
	if a.edition != "" {
		add(FieldEdition, a.edition, "Редакция «"+a.edition+"»", false)
	}
}

// docAttr — значение атрибута позиции из строки атрибутов индекса («ключ: значение; …»).
func docAttr(d indexDoc, key string) string {
	for _, part := range strings.Split(d.Attributes, "; ") {
		if v, ok := strings.CutPrefix(part, key+": "); ok {
			return v
		}
	}
	return ""
}

// offerHidden — вариант под конкретную сделку или организацию, которых запрос не называет: в выдачу и в
// релевантность семейства он не попадает. Вариант для образования или госучреждений — только этой организации;
// расширение и миграция — только если названы; продление поштучно — если названо продление с количеством или
// сроком (без них голова семейства — готовая фасовка).
func (rc rankCtx) offerHidden(d indexDoc) (bool, string) {
	a := rc.attrs
	if org := docAttr(d, "org_type"); org != "" && org != a.org {
		return true, "вариант только для: " + strings.ToLower(orgLabels[org])
	}
	if hidden, detail := rc.feedOrgHidden(d); hidden {
		return true, detail
	}
	switch deal := docAttr(d, "deal"); deal {
	case "":
	case DealRenewal:
		if !a.renewal || a.quantity == 0 && a.duration == 0 {
			return true, "продление поштучно — когда названы количество или срок"
		}
	default:
		if deal != a.deal {
			return true, "вариант для сделки «" + strings.ToLower(dealLabels[deal]) + "»"
		}
	}
	return false, ""
}

// purchaseFit — добавка к соответствию варианта (rank.go fit): названная сделка, организация и редакция.
func (rc rankCtx) purchaseFit(d indexDoc) int {
	a := rc.attrs
	score := 0
	if a.deal != "" && docAttr(d, "deal") == a.deal {
		score += 5
	}
	if a.org != "" && docAttr(d, "org_type") == a.org {
		score += 5
	}
	if a.edition != "" && strings.EqualFold(d.LicenseEdition, a.edition) {
		score += 6
	}
	if a.edition == "" && docAttr(d, "deal") != "" && rc.upgradeTargets[d.ProductID] {
		score-- // старшая редакция не выбирается заранее: она — строкой «Старшая редакция» рядом ()
	}
	return score
}

// upgradeTargetSet — цели утверждённых связей upgrade с указанной позицией.
func upgradeTargetSet(cc *commerceCatalog) map[string]bool {
	set := map[string]bool{}
	if cc == nil {
		return set
	}
	for _, rel := range cc.relations {
		if rel.Type == domain.RelationUpgrade && rel.ToProduct != nil && rel.ReviewStatus != domain.ReviewPending &&
			rel.ReviewStatus != domain.ReviewRejected {
			set[*rel.ToProduct] = true
		}
	}
	return set
}

// purchaseDeal — тип покупки запроса для ответа и заявки.
func (a queryAttrs) purchaseDeal() string {
	switch {
	case a.renewal:
		return DealRenewal
	case a.deal != "":
		return a.deal
	}
	return ""
}

// purchaseBlock — извлечённые параметры закупки (ответ поиска → спецификация → заявка КП); nil — ничего не названо.
func (a queryAttrs) purchaseBlock(query string) *out.Purchase {
	deal := a.purchaseDeal()
	if deal == "" && a.quantity == 0 && a.duration == 0 && a.org == "" && a.edition == "" {
		return nil
	}
	p := &out.Purchase{Query: query, Deal: deal, DealLabel: dealLabels[deal], Metric: a.metric, OrgType: a.org,
		OrgLabel: orgLabels[a.org], Edition: a.edition}
	if a.quantity > 0 {
		q := a.quantity
		p.Quantity = &q
	}
	if a.duration > 0 {
		m := a.duration
		p.DurationMonths = &m
	}
	return p
}

// editionClarify — вопрос о редакции для продления, расширения и миграции: у первого семейства подходящие
// варианты в нескольких редакциях, а запрос редакцию не называет. nil — спрашивать нечего.
func editionClarify(a queryAttrs, fams []*familyHit, tokens []string) *out.Clarify {
	if a.edition != "" || a.editionAnswered || a.purchaseDeal() == "" || len(fams) == 0 || fams[0].group == domain.GroupAnalog {
		return nil
	}
	var eds []string
	for _, d := range fams[0].heads {
		if fams[0].fits[d.ProductID] && d.LicenseEdition != "" && !contains(eds, d.LicenseEdition) {
			eds = append(eds, d.LicenseEdition)
		}
	}
	if len(eds) < 2 {
		return nil
	}
	q := strings.Join(tokens, " ")
	for _, e := range eds {
		if strings.Contains(q, foldText(e)) {
			return nil
		}
	}
	opts := make([]out.ClarifyOption, 0, len(eds)+1)
	for _, e := range eds {
		opts = append(opts, out.ClarifyOption{Value: e, Label: e})
	}
	opts = append(opts, out.ClarifyOption{Value: clarifyAny, Label: "Не знаю"})
	question := fmt.Sprintf("Какая редакция у вас сейчас: %s?", strings.Join(eds, " или "))
	if len(eds) > 2 {
		question = "Какая редакция у вас сейчас?"
	}
	return &out.Clarify{Attribute: FieldEdition, Question: question, Options: opts}
}

// withoutHiddenOffers — список кандидатов без скрытых вариантов, ранги пересчитаны подряд. Скрытых нет —
// тот же список.
func withoutHiddenOffers(lx *searchLexicon, attrs queryAttrs, hits []domain.RankedHit) []domain.RankedHit {
	rc := rankCtx{lx: lx, attrs: attrs}
	var res []domain.RankedHit
	dropped := false
	for _, h := range hits {
		if d, ok := lx.docs[h.ProductID]; ok {
			if hidden, _ := rc.offerHiddenInFusion(d); hidden {
				dropped = true
				continue
			}
		}
		h.Rank = len(res) + 1
		res = append(res, h)
	}
	if !dropped {
		return hits
	}
	return res
}

// isOffer — вариант под конкретную сделку или организацию (attributes.deal, attributes.org_type).
func isOffer(d indexDoc) bool { return docAttr(d, "deal") != "" || docAttr(d, "org_type") != "" }

// feedOrgHidden — вариант фида для академических, государственных или некоммерческих организаций (License.org_type),
// а тип организации в запросе не назван: головой и вариантом карточки он не становится.
func (rc rankCtx) feedOrgHidden(d indexDoc) (bool, string) {
	if org := feedOrg[d.LicenseOrgType]; org != "" && org != rc.attrs.org {
		return true, "вариант только для: " + strings.ToLower(orgLabels[org])
	}
	return false, ""
}

// offerHiddenInFusion — вариант не идёт в слияние и не поднимает семейство. Вариант фида под тип организации здесь
// не скрывается: семейство релевантно по названию, какой бы из его вариантов ни совпал лучше («Microsoft Office Word
// 2019 (для академических организаций)» поднимает Word 2019), а показываются только подходящие варианты.
func (rc rankCtx) offerHiddenInFusion(d indexDoc) (bool, string) {
	if hidden, _ := rc.feedOrgHidden(d); hidden {
		saved := d.LicenseOrgType
		d.LicenseOrgType = ""
		h, detail := rc.offerHidden(d)
		d.LicenseOrgType = saved
		return h, detail
	}
	return rc.offerHidden(d)
}
