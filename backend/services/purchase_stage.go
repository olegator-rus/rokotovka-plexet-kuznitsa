package services

import (
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
)

// Шаг «Закупка» контура доводки: после выдачи, вне BM25 и векторов, 0 вызовов модели. Параметры
// закупки, вопрос о редакции, «Вы искали», реестр, ответ справки, поставка отсутствующего товара. Основная
// выдача меняется только у сервисного запроса (товаров нет) и при явном требовании реестра.

// StepProcurement — имя шага в трассе.
const StepProcurement = "procurement"

// procurementTrace — шаг трассы.
type procurementTrace struct {
	domain.StepBase
	Deal           string `json:"deal,omitempty"`
	OrgType        string `json:"org_type,omitempty"`
	Edition        string `json:"edition,omitempty"`
	Clarify        string `json:"clarify,omitempty"`
	Original       string `json:"original,omitempty"`
	ReestrRequired bool   `json:"reestr_required,omitempty"`
	ReestrMoved    int    `json:"reestr_moved,omitempty"`
	Help           string `json:"help,omitempty"`
	HelpMode       string `json:"help_mode,omitempty"`
	Supply         bool   `json:"supply,omitempty"`
}

// procurementStage — шаг закупки над готовым ответом.
func (s *Services) procurementStage(lx *searchLexicon, query string, tokens []string, attrs queryAttrs,
	ent queryEntities, fams []*familyHit, resp *out.SearchResponse) procurementTrace {
	t := time.Now()
	st := procurementTrace{StepBase: domain.StepBase{Name: StepProcurement}, Deal: attrs.purchaseDeal(),
		OrgType: attrs.org, Edition: attrs.edition, ReestrRequired: attrs.reestrRequired}
	resp.Purchase = attrs.purchaseBlock(query)
	if resp.Purchase != nil {
		matchingPriceFrom(resp.Items)
	}
	if resp.Clarify == nil {
		if c := editionClarify(attrs, fams, tokens); c != nil {
			resp.Clarify = c
			st.Clarify = c.Attribute
		}
	}
	if attrs.analog && len(ent.reference) > 0 {
		if o := s.analogOriginal(lx, ent.reference); o != nil {
			resp.AnalogOriginal = o
			st.Original = o.FamilyID
		}
	}
	s.applyReestr(lx, resp, attrs.reestrRequired)
	st.ReestrMoved = len(resp.ReestrUnconfirmed)
	if resp.Empty != nil && resp.Empty.Supply {
		name := strings.TrimPrefix(strings.TrimSuffix(resp.Empty.Message, "» нет в каталоге"), "Товара «")
		absentAnalogNotes(resp, name)
		st.Supply = true
	}
	if h := s.helpFor(tokens, attrs, ent); h != nil {
		resp.Help = h
		st.Help, st.HelpMode = h.ID, h.Mode
		if h.Mode == HelpModeAnswer {
			applyHelpAnswer(resp)
			if resp.Empty != nil {
				resp.Empty.Supply = false
			}
			st.Supply = false
		}
	}
	st.Ms = msSince(t)
	return st
}

// metricRelaxedMessage — пояснение, что метрика запроса снята: у названного продукта лицензии на другую единицу.
func metricRelaxedMessage(asked string, qty int, fams []*familyHit) *out.Relaxed {
	if asked == "" || len(fams) == 0 || fams[0].head.LicenseMetric == nil {
		return nil
	}
	got := *fams[0].head.LicenseMetric
	af, gf := metricForms[asked], metricForms[got]
	if af[0] == "" || gf[0] == "" {
		return nil
	}
	msg := "Лицензий на " + af[1] + " нет — показаны лицензии на " + gf[1]
	if qty > 0 {
		msg += ": " + quantityLabel(qty, asked) + " — это " + quantityLabel(qty, got) + ", если у каждого одно"
	}
	return &out.Relaxed{Field: domain.FieldLicenseMetric, Message: msg}
}

// matchingPriceFrom — цена «от» по вариантам, подходящим под условия запроса (тип покупки, количество, срок):
// у «продлить … на 2 года» — от продления на 2 года, а не от самой дешёвой подписки семейства. Подходящих нет —
// прежняя цена «от» по всем вариантам.
func matchingPriceFrom(items []out.SearchItem) {
	for i := range items {
		it := &items[i]
		var best *float64
		consider := func(v out.SearchItem) {
			if v.MatchesQuery && v.PriceRub != nil && !v.PriceOnRequest && (best == nil || *v.PriceRub < *best) {
				x := *v.PriceRub
				best = &x
			}
		}
		consider(*it)
		for _, v := range it.Variants {
			consider(v)
		}
		if best != nil && len(it.Variants) > 0 {
			it.PriceFromRub = best
		}
	}
}

// Пределы параметров закупки в заявке КП.
const (
	maxQuoteQuery    = 300
	maxQuoteDuration = 120
)

var quoteDeals = map[string]bool{"": true, DealNew: true, DealRenewal: true, DealExtension: true, DealMigration: true}
var quoteOrgs = map[string]bool{"": true, OrgCommercial: true, OrgEducation: true, OrgGovernment: true}

// quotePurchase — проверенные параметры закупки позиции заявки КП (исходный запрос и извлечённое поиском).
func quotePurchase(field string, p *in.QuotePurchase) (*domain.QuotePurchase, []errs.FieldError) {
	if p == nil {
		return nil, nil
	}
	var bad []errs.FieldError
	f := field + ".purchase"
	q := strings.TrimSpace(p.Query)
	switch {
	case len([]rune(q)) > maxQuoteQuery:
		bad = append(bad, errs.FieldError{Field: f + ".query", Code: errs.CodeOutOfRange, Message: "Запрос длиннее 300 символов"})
	case !quoteDeals[p.Deal]:
		bad = append(bad, errs.FieldError{Field: f + ".deal", Code: errs.CodeInvalidFormat, Message: "Тип покупки — new, renewal, extension или migration"})
	case !quoteOrgs[p.OrgType]:
		bad = append(bad, errs.FieldError{Field: f + ".org_type", Code: errs.CodeInvalidFormat, Message: "Тип организации — commercial, education или government"})
	case p.Quantity != nil && (*p.Quantity < 1 || *p.Quantity > maxQuoteQuantity):
		bad = append(bad, errs.FieldError{Field: f + ".quantity", Code: errs.CodeOutOfRange, Message: "Количество от 1 до 100000"})
	case p.DurationMonths != nil && (*p.DurationMonths < 1 || *p.DurationMonths > maxQuoteDuration):
		bad = append(bad, errs.FieldError{Field: f + ".duration_months", Code: errs.CodeOutOfRange, Message: "Срок от 1 до 120 месяцев"})
	case len(p.Metric) > 16 || len([]rune(p.Edition)) > 64:
		bad = append(bad, errs.FieldError{Field: f, Code: errs.CodeOutOfRange, Message: "Слишком длинное значение"})
	}
	if len(bad) > 0 {
		return nil, bad
	}
	return &domain.QuotePurchase{Query: q, Deal: p.Deal, Quantity: p.Quantity, Metric: p.Metric,
		DurationMonths: p.DurationMonths, OrgType: p.OrgType, Edition: strings.TrimSpace(p.Edition)}, nil
}
