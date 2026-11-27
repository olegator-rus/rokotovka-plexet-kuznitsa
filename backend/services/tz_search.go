package services

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
)

// Поиск по позиции ТЗ (контур ТЗ, TZ-CORE §6): фронт зовёт обычный POST /api/search с source = tz, search_query
// позиции и фильтрами количества и срока. Условия уже извлечены при разборе документа, поэтому разбор запроса
// моделью выключен (маршрут D — на правилах), а количество, метрика и срок из фильтров становятся условиями
// запроса, как будто названы текстом: лицензия подбирается по диапазону («50–99 устройств»), а не точным
// совпадением фасета, и в ответе есть блок закупки для расчёта «цена × количество».

// tzSearch — контекст и запрос поиска по позиции ТЗ; иной источник — без изменений.
func tzSearch(ctx context.Context, req in.SearchRequest) (context.Context, in.SearchRequest) {
	if req.Source != domain.EventSourceTZ {
		return ctx, req
	}
	m := modesFrom(ctx)
	m.noLLM = true
	ctx = withSearchModes(ctx, m)
	var c in.TZConditions
	kept := make([]in.Filter, 0, len(req.Filters))
	for _, f := range req.Filters {
		if f.Op != "" && f.Op != "eq" {
			kept = append(kept, f)
			continue
		}
		switch f.Key {
		case facetQuantity:
			if n, ok := tzInt(f.Value); ok && n > 0 && n <= maxQuoteQuantity {
				c.Quantity = n
				continue
			}
		case facetDuration:
			if n, ok := tzInt(f.Value); ok && n > 0 && n <= maxQuoteDuration {
				c.DurationMonths = n
				continue
			}
		case facetMetric:
			switch v := strings.TrimSpace(fmt.Sprint(f.Value)); v {
			case domain.MetricDevice, domain.MetricUser, domain.MetricServer, domain.MetricCore:
				c.Metric = v
				continue
			}
		case tzFilterKind:
			if fmt.Sprint(f.Value) == tzKindService {
				c.Service = true
			}
			continue
		case tzFilterEdition:
			// Редакция позиции ТЗ («Орёл», «Стандартный»): как ответ на уточнение редакции — выбирает вариант
			// семейства (purchaseFit), а не отсекает выдачу. Не фасет страницы результатов.
			if v := strings.TrimSpace(fmt.Sprint(f.Value)); v != "" && len([]rune(v)) <= 64 {
				c.Edition = strings.ReplaceAll(strings.ReplaceAll(v, "ё", "е"), "Ё", "Е")
			}
			continue
		}
		kept = append(kept, f)
	}
	req.Filters = kept
	req.TZ = &c
	return ctx, req
}

// tzInt — целое из значения фильтра: строка «50» или число JSON.
func tzInt(v any) (int, bool) {
	switch x := v.(type) {
	case float64:
		if x == float64(int(x)) {
			return int(x), true
		}
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(x))
		return n, err == nil
	}
	return 0, false
}

// applyTZ — условия позиции ТЗ поверх разобранного из текста: названное в search_query не перебивается. Метрика —
// предпочтение, а не фильтр: у многих позиций каталога её нет (КриптоПро CSP 5.0 серверная, Astra Linux «Орёл»), и
// строгая метрика из документа убрала бы именно нужный товар.
func (a *queryAttrs) applyTZ(c *in.TZConditions) {
	if c == nil {
		return
	}
	a.tzSearch = true
	if c.Quantity > 0 && a.quantity == 0 {
		a.quantity, a.tzSoftQty = c.Quantity, true
		a.metricUnresolved = false // метрику не спрашиваем: условия позиции уже извлечены из документа
	}
	if c.Metric != "" && a.metric == "" && a.metricPref == "" {
		a.metricPref = c.Metric
	}
	if c.DurationMonths > 0 && a.duration == 0 {
		a.duration = c.DurationMonths
	}
	if c.Edition != "" && a.edition == "" {
		a.edition = c.Edition
	}
}

// tzQuantity — поиск по ТЗ: у позиции каталога число мест не указано (КриптоПро CSP 5.0 серверная, Astra Linux
// «Орёл» на рабочую станцию) — она продаётся поштучно, нужное количество задаётся при заказе. В живом поиске такое
// количество строгое (quantityOK), здесь — нет: иначе условие из документа убирает именно названный товар.
func (a queryAttrs) tzQuantity(d indexDoc) bool {
	return a.tzSoftQty && a.quantity > 0 && d.LicenseQuantity == nil
}

// tzFilterKind — ключ фильтра вида позиции ТЗ: {"key":"kind","op":"eq","value":"service"} у работ и услуг.
const tzFilterKind = "kind"

// tzServiceResult — поиск по позиции-работе (монтаж, настройка): в каталоге только ПО и оборудование, поэтому
// ни один товар не точное совпадение — ступень exact становится related, группа exact — match. По правилу
// TZ-CORE §6 фронт ставит такой позиции «Нет в каталоге», а не «Похожие» из случайной стойки.
func tzServiceResult(req in.SearchRequest, resp *out.SearchResponse) {
	if resp == nil || strings.ToLower(strings.TrimSpace(req.Source)) != domain.EventSourceTZ {
		return
	}
	service := false
	for _, f := range req.Filters {
		if f.Key == tzFilterKind && fmt.Sprint(f.Value) == tzKindService {
			service = true
		}
	}
	if !service {
		return
	}
	var demote func(items []out.SearchItem)
	demote = func(items []out.SearchItem) {
		for i := range items {
			if items[i].MatchType == out.MatchExact {
				items[i].MatchType = out.MatchRelated
			}
			if items[i].Group == domain.GroupExact {
				items[i].Group = domain.GroupMatch
			}
			demote(items[i].Variants)
		}
	}
	demote(resp.Items)
	if c := resp.Counts; c != nil && c.ByMatchType != nil {
		if n := c.ByMatchType[out.MatchExact]; n > 0 {
			c.ByMatchType[out.MatchRelated] += n
			delete(c.ByMatchType, out.MatchExact)
		}
	}
	if resp.Target != nil && (resp.Target.Type == "sku" || resp.Target.Type == "family") {
		resp.Target = nil // вести на «точный» товар для работ некуда
	}
}

// tzHeadLess — поиск по ТЗ, варианты семейства с равным соответствием условиям и одинаковым названием: сначала
// предложение без ограничения по размеру организации (акция «для малого и среднего бизнеса» — вариантом, а не
// головой: размер заказчика в ТЗ обычно не назван), затем дешевле. Без этого голова — первый по порядку фида
// из одинаковых по названию вариантов (Astra Linux «Орёл»: привилегированная поддержка 14 550 ₽ вместо 6 150 ₽).
func (a queryAttrs) tzHeadLess(x, y indexDoc) (less, ok bool) {
	if !a.tzSearch || !strings.EqualFold(strings.TrimSpace(x.Title), strings.TrimSpace(y.Title)) {
		return false, false // разные названия — порядок решает релевантность текста
	}
	if sx, sy := tzSegmentOffer(x), tzSegmentOffer(y); sx != sy {
		return sy, true
	}
	px, py := x.PriceRub, y.PriceRub
	switch {
	case px != nil && py != nil && *px != *py:
		return *px < *py, true
	case (px == nil) != (py == nil):
		return px != nil, true
	}
	return false, false
}

// tzSegmentOffer — предложение только для части организаций по размеру («Малый и средний бизнес», «для МСП»).
func tzSegmentOffer(d indexDoc) bool {
	v := strings.ToLower(attrTextValue(d.Attributes, "Размер организации"))
	return strings.Contains(v, "мал") || strings.Contains(strings.ToLower(d.Title), "для мсп")
}
