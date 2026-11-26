package services

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"time"

	"kuznica/api/domain"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
)

// Модель эффекта контура коммерции. К возвращённым
// заказам (вилка /api/effect) добавляются три непересекающихся механизма и вычеты, всё — в валовой прибыли (GP):
//
//	O₀ — заказы поиска в месяц (замер), G₀ — GP заказа = средний чек × базовая маржа;
//	1) спасённые заказы ΔO₊ (сценарий /api/effect) × G₀;
//	2) допродажа: заказы × покрытие блока × доля присоединения × инкрементальность × сумма допроданного × его маржа;
//	3) сдвиг микса от рычага маржи — только по якорной позиции (ожидаемая GP якоря при действующей политике минус
//	   при весе контура поиска, на базовой выдаче tune);
//	вычеты: потеря конверсии якоря (заказы × покрытие × доля потери, ΔO₋ × G₀) и возвраты из-за несовместимости
//	(присоединённые продажи × доля несовместимых пар × сумма × маржа).
//
// ΔO = ΔO₊ − ΔO₋, ΔG = допродажа + микс − возвраты на заказ; ΔGP = ΔO·G₀ + O₀·ΔG + ΔO·ΔG — сверяется с прямым
// (O₀+ΔO)(G₀+ΔG) − O₀G₀. Один товар учитывается либо в миксе (якорь), либо в допродаже (компаньон). У каждого входа
// источник; непосчитанное — null с причиной в missing, сумма — по посчитанным членам (complete=false).

// EffectCommerceRequest — параметры модели: допущения вилки возврата и переопределения входов (nil — по умолчанию).
type EffectCommerceRequest struct {
	Effect    EffectRequest
	Overrides map[string]*float64
}

// Входы, которые можно переопределить параметром запроса.
const (
	ecAvgCheck     = "avg_check_rub"
	ecBaseMargin   = "base_margin"
	ecCoverage     = "block_coverage"
	ecAttach       = "attach_rate"
	ecIncrement    = "incrementality"
	ecCompSum      = "companion_sum_rub"
	ecCompMargin   = "companion_margin"
	ecAnchorLoss   = "anchor_loss"
	ecReturnRate   = "return_rate"
	ecMixPerOrder  = "mix_gp_per_order"
	ecOrdersMonth  = "orders_month"
	ecRescuedOrder = "rescued_orders_month"
)

// EffectCommerceKeys — входы модели, которые принимает GET /api/effect/commerce (кроме параметров /api/effect).
var EffectCommerceKeys = []string{ecRescuedOrder, ecBaseMargin, ecCoverage, ecAttach, ecIncrement, ecCompSum, ecCompMargin, ecAnchorLoss,
	ecReturnRate, ecMixPerOrder}

// Допущения и литература.
const (
	// assumedAttachRate — доля присоединения при показе блока: событий блока на стенде нет — допущение.
	assumedAttachRate = 0.1
	// litIncrementality — на Amazon не менее 75 % кликов по рекомендациям случились бы и без них: инкрементальна
	// не больше четверти.
	litIncrementality = 0.25
	litIncrementURL   = "https://arxiv.org/abs/1510.05569"
	// litAnchorLoss — рекомендации дали −1,9 % продаж якоря на просмотр (fashion-ритейлер, эксперимент).
	litAnchorLoss       = 0.019
	litAnchorLossURL    = "https://ideas.repec.org/a/inm/orisre/v30y2019i3p819-838.html"
	effectCommerceTitle = "Модель чувствительности, не прогноз"
	effectCommerceNote  = "Сценарии на тестовых данных стенда; маржа и заказы mock синтетические. Фактический эффект — " +
		"только A/B-тестом в пилоте"
	effectCommercePilot = "Пилот 4–8 недель при 10–20 тыс. поисков в месяц различит относительное изменение конверсии " +
		"8,5–25 % и среднего чека 18–36 %; рост чека на единицы процентов не увидит (оценка оптимистичная, единица — поиск)"
	effectIdentityEps = 0.5
)

// ecScenario — множители входов по сценариям.
var ecScenarios = []struct {
	id, label, rule           string
	attach, increment, anchor float64
}{
	{"conservative", "Консервативный", "возврат — консервативный сценарий вилки; присоединение × 0,5; " +
		"инкрементальность × 0,4; потеря якоря × 1,5", 0.5, 0.4, 1.5},
	{"base", "Базовый", "входы как есть", 1, 1, 1},
	{"optimistic", "Оптимистичный", "возврат — оптимистичный сценарий вилки; присоединение × 1,5 (не больше 1); " +
		"инкрементальность — потолок литературы; потеря якоря × 0,5", 1.5, 1, 0.5},
}

// ecMeasured — замеры входов (nil — нет данных) и откуда они.
type ecMeasured struct {
	baseMargin, coverage, attach, compSum, compMargin, returnRate, mixPerOrder *float64
	refs                                                                       map[string]string
	missing                                                                    []string
	synthetic                                                                  bool
}

// EffectCommerce — GET /api/effect/commerce.
func (s *Services) EffectCommerce(ctx context.Context, req EffectCommerceRequest) (*out.EffectCommerce, error) {
	var bad []errs.FieldError
	for k, v := range req.Overrides {
		if v == nil {
			continue
		}
		switch {
		case math.IsNaN(*v) || *v < 0:
			bad = append(bad, errs.FieldError{Field: k, Code: errs.CodeOutOfRange, Message: "Значение — не меньше 0"})
		case k == ecCompSum && (*v <= 0 || *v > 1e9):
			bad = append(bad, errs.FieldError{Field: k, Code: errs.CodeOutOfRange, Message: "Сумма — больше 0 ₽"})
		case (k == ecMixPerOrder || k == ecRescuedOrder) && *v > 1e7:
			bad = append(bad, errs.FieldError{Field: k, Code: errs.CodeOutOfRange, Message: "Слишком большое значение"})
		case k != ecCompSum && k != ecMixPerOrder && k != ecRescuedOrder && *v > 1:
			bad = append(bad, errs.FieldError{Field: k, Code: errs.CodeOutOfRange, Message: "Доля — от 0 до 1"})
		}
	}
	if len(bad) > 0 {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(bad...)
	}
	eff, err := s.effect(ctx, req.Effect, nil)
	if err != nil {
		return nil, err
	}
	m := s.measureCommerceInputs(ctx, eff.From)
	res := effectCommerceFrom(eff, m, req.Overrides)
	res.Days, res.From, res.To = eff.Days, eff.From, eff.To
	return res, nil
}

// measureCommerceInputs — замеры входов: базовая маржа заказов, покрытие и несовместимость блока (последний прогон
// лестницы p2), присоединение (события стенда), сумма и маржа компаньонов (размеченные пары и каталог), сдвиг микса
// (симуляция политики маржи на базовой выдаче tune).
func (s *Services) measureCommerceInputs(ctx context.Context, since time.Time) ecMeasured {
	m := ecMeasured{refs: map[string]string{}, synthetic: s.marginSynthetic()}
	cs := s.catalog.Load()
	var docs map[string]indexDoc
	if cs != nil && cs.lex != nil {
		docs = cs.lex.docs
	}
	if s.repos.Enabled() && docs != nil {
		if ordered, err := s.repos.OrderedProductsSince(ctx, since); err == nil {
			num, den, n := 0.0, 0.0, 0
			for _, pc := range ordered {
				d, ok := docs[pc.ProductID]
				if !ok || d.PriceRub == nil || *d.PriceRub <= 0 || d.Margin <= 0 {
					continue
				}
				num += *d.PriceRub * d.Margin * float64(pc.N)
				den += *d.PriceRub * float64(pc.N)
				n += pc.N
			}
			if den > 0 {
				m.baseMargin = evalPtr(evalRound4(num / den))
				m.refs[ecBaseMargin] = fmt.Sprintf("маржа позиций в %d заказах периода, взвешенная по цене (каталог)", n)
			}
		}
		if rows, err := s.repos.CommerceEventsSince(ctx, since); err == nil {
			var imp, add int
			for _, r := range rows {
				switch r.Type {
				case "companion_impression":
					imp = r.Sessions
				case "companion_add":
					add = r.Sessions
				}
			}
			if imp > 0 {
				m.attach = evalPtr(evalRound4(float64(add) / float64(imp)))
				m.refs[ecAttach] = fmt.Sprintf("события стенда: сессии с добавлением из блока %d / сессии с показом блока %d", add, imp)
			}
		}
	}
	if m.baseMargin == nil {
		m.missing = append(m.missing, "базовая маржа: нет заказов с известной маржой за период")
	}
	if ev, err := s.CommerceEvaluation(ctx, domain.EvalSetP2); err == nil && len(ev.Ladder) > 0 {
		var cfg *out.CommerceConfig
		for i := range ev.Ladder {
			if c := &ev.Ladder[i]; (c.Config == "C4" || c.Config == "C2") && c.Metrics.Coverage != nil {
				cfg = c
			}
		}
		if cfg != nil {
			m.coverage = cfg.Metrics.Coverage
			m.refs[ecCoverage] = fmt.Sprintf("прогон лестницы %s (%s): доля якорей размеченных пар с непустым блоком — "+
				"офлайн-прокси доли заказов с блоком (на трафике меряется показами)", ev.RunID, cfg.Config)
			var sc CompanionScore
			if raw, err := json.Marshal(cfg.Block); err == nil && json.Unmarshal(raw, &sc) == nil {
				if sc.ViolationRate != nil {
					m.returnRate = sc.ViolationRate
				} else if sc.ShownPairs > 0 {
					m.returnRate = evalPtr(0.0)
				}
				if m.returnRate != nil {
					m.refs[ecReturnRate] = fmt.Sprintf("прогон лестницы %s (%s): несовместимых пар %d среди размеченных показанных %d — "+
						"допущение: каждая несовместимая продажа возвращается", ev.RunID, cfg.Config, sc.CompatViolations, sc.LabeledShown)
				}
			}
		}
	}
	if m.coverage == nil {
		m.missing = append(m.missing, "покрытие блока: нет прогона коммерческой лестницы — make eval SET=p2")
	}
	if docs != nil {
		seen := map[string]bool{}
		num, den, sum, n := 0.0, 0.0, 0.0, 0
		for _, p := range s.companionPairs(ctx) {
			if !expectsBlock[p.Class] || seen[p.CandidateProduct] {
				continue
			}
			seen[p.CandidateProduct] = true
			d, ok := docs[p.CandidateProduct]
			if !ok || d.PriceRub == nil || *d.PriceRub <= 0 {
				continue
			}
			sum += *d.PriceRub
			n++
			if d.Margin > 0 {
				num += *d.PriceRub * d.Margin
				den += *d.PriceRub
			}
		}
		if n > 0 {
			m.compSum = evalPtr(math.Round(sum / float64(n)))
			m.refs[ecCompSum] = fmt.Sprintf("средняя цена %d компаньонов из размеченных пар (обязательные, совместимые, дополнения; каталог)", n)
		}
		if den > 0 {
			m.compMargin = evalPtr(evalRound4(num / den))
			m.refs[ecCompMargin] = "маржа тех же компаньонов, взвешенная по цене (каталог)"
		}
	}
	if m.compSum == nil {
		m.missing = append(m.missing, "сумма допроданного: нет размеченных пар с ценой (companion_pairs.json)")
	}
	if v, ref, why := s.measureMix(ctx, docs); v != nil {
		m.mixPerOrder, m.refs[ecMixPerOrder] = v, ref
	} else {
		m.missing = append(m.missing, why)
	}
	return m
}

// measureMix — сдвиг микса: изменение ожидаемой GP якоря на заказ при действующей политике маржи против веса
// контура поиска, на базовой выдаче tune. Якорь — позиция первой десятки с вероятностью ∝ 1/log2(i+2) (допущение — та же
// нормировка позиций, что у индекса показанной маржи); позиции без цены или маржи не входят.
func (s *Services) measureMix(ctx context.Context, docs map[string]indexDoc) (*float64, string, string) {
	if docs == nil {
		return nil, "", "сдвиг микса: каталог не загружен"
	}
	pol, err := s.activeMarginPolicy(ctx)
	if err != nil {
		return nil, "", "сдвиг микса: политика маржи не прочитана"
	}
	ref := fmt.Sprintf("политика маржи v%d (вес %s, категорий со своим весом %d) против веса контура поиска на базовой выдаче tune",
		pol.Version, marginWeightLabel(pol.Weight), len(pol.Categories))
	if pol.weights().isBase() {
		return evalPtr(0.0), ref + ": политика равна контуру поиска — сдвига нет", ""
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	capture, _, err := s.marginCaptureFor(cctx, domain.EvalSetTune, s.marginSimKeyFor(domain.EvalSetTune))
	if err != nil || capture == nil {
		return nil, "", "сдвиг микса: базовая выдача tune ещё считается — повторите через полминуты"
	}
	v, n := marginMixPerOrder(capture, pol.weights(), docs)
	if n == 0 {
		return nil, "", "сдвиг микса: в выдаче tune нет позиций с ценой и маржой"
	}
	return evalPtr(math.Round(v*100) / 100), fmt.Sprintf("%s: %d запросов; якорь — первая десятка с весом позиции 1/log2(i+2) (допущение)", ref, n), ""
}

// marginMixPerOrder — среднее по запросам изменение ожидаемой GP якоря (₽ на заказ) при весах mw (без ε, как в
// живом поиске) против порядка контура поиска.
func marginMixPerOrder(c *marginCapture, mw marginWeights, docs map[string]indexDoc) (float64, int) {
	expected := func(q marginSimQuery, order []int) (float64, bool) {
		num, den := 0.0, 0.0
		for i, idx := range order {
			if i >= evalTopK {
				break
			}
			d, ok := docs[q.in.cands[idx].productID]
			if !ok || d.PriceRub == nil || *d.PriceRub <= 0 || d.Margin <= 0 {
				continue
			}
			w := 1 / math.Log2(float64(i)+2)
			num += w * *d.PriceRub * d.Margin
			den += w
		}
		if den == 0 {
			return 0, false
		}
		return num / den, true
	}
	sum, n := 0.0, 0
	for _, q := range c.queries {
		before := identityOrder(len(q.in.cands))
		after := marginReorderW(q.in, mw, marginMaxShift).order
		eb, ok1 := expected(q, before)
		ea, ok2 := expected(q, after)
		if ok1 && ok2 {
			sum += ea - eb
			n++
		}
	}
	if n == 0 {
		return 0, 0
	}
	return sum / float64(n), n
}

// effectCommerceFrom — модель из вилки возврата, замеров и переопределений (чистая функция).
func effectCommerceFrom(eff *out.Effect, m ecMeasured, over map[string]*float64) *out.EffectCommerce {
	res := &out.EffectCommerce{Title: effectCommerceTitle, Note: effectCommerceNote, Pilot: effectCommercePilot,
		Identity: "ΔGP = ΔO·G₀ + O₀·ΔG + ΔO·ΔG",
		Formula: []string{
			"G₀ = средний чек × базовая маржа; O₀ — заказы поиска в месяц",
			"спасённые = ΔO₊ × G₀ (ΔO₊ — заказы сценария вилки возврата /api/effect)",
			"допродажа = O₁ × покрытие блока × доля присоединения × инкрементальность × сумма допроданного × его маржа",
			"сдвиг микса = O₁ × изменение ожидаемой GP якоря на заказ (только якорная позиция)",
			"потеря якоря = −ΔO₋ × G₀, ΔO₋ = (O₀ + ΔO₊) × покрытие × доля потери",
			"возвраты = −O₁ × покрытие × доля присоединения × доля несовместимых × сумма × маржа",
			"ΔO = ΔO₊ − ΔO₋; O₁ = O₀ + ΔO; ΔG = (допродажа + микс + возвраты) / O₁",
		},
		Inputs: []out.EffectInput{}, Scenarios: []out.EffectCommerceScenario{}, SyntheticKeys: []string{},
		Missing: append([]string{}, m.missing...), Warnings: []string{}}
	add := func(key, label, unit, source, ref, url, hint string, measured *float64, def *float64, sc map[string]float64) *float64 {
		in := out.EffectInput{Key: key, Label: label, Unit: unit, Source: source, SourceRef: ref, SourceURL: url, Hint: hint,
			Scenarios: sc}
		in.Default = measured
		if measured == nil && def != nil {
			in.Default = def
			if source == "measured" || source == "synthetic" {
				in.Source = "assumed"
			}
		}
		in.Value = in.Default
		if v, ok := over[key]; ok && v != nil {
			in.Value, in.Source, in.SourceRef = evalPtr(*v), "request", "задано в запросе"
		}
		if in.Source == "synthetic" {
			res.SyntheticKeys = append(res.SyntheticKeys, key)
		}
		res.Inputs = append(res.Inputs, in)
		return in.Value
	}
	scen := func(f func(i int) float64) map[string]float64 {
		mm := map[string]float64{}
		for i, sc := range ecScenarios {
			mm[sc.id] = f(i)
		}
		return mm
	}

	o0 := eff.Measured.OrdersMonth
	orders := add(ecOrdersMonth, "Заказы поиска в месяц (O₀)", "заказов", "synthetic",
		"журнал событий: заказы после поиска за период, пересчёт на 30 дней (заказы mock синтетические)", "",
		"База модели. На данных заказчика — его заказы", evalPtr(o0), nil, nil)
	rescued := map[string]float64{}
	for _, sc := range eff.Scenarios {
		rescued[sc.ID] = sc.OrdersMonth
	}
	var rescuedBase *float64
	if v, ok := rescued["base"]; ok {
		rescuedBase = evalPtr(v)
	}
	add(ecRescuedOrder, "Спасённые заказы в месяц (ΔO₊)", "заказов", "measured",
		"вилка /api/effect: реплей отложенной недели (доля спасённых) × допущения вилки", "",
		"Параметры вилки (session_factor, order_prob, rescue_factor) передаются так же, как в /api/effect", rescuedBase, nil,
		nilIfEmpty(rescued))
	var avgDef *float64
	for _, a := range eff.Assumptions {
		if a.Key == ecAvgCheck {
			avgDef = evalPtr(a.Value)
			src := a.Source // measured | assumed | request, как в /api/effect
			res.Inputs = append(res.Inputs, out.EffectInput{Key: ecAvgCheck, Label: "Средний чек", Unit: "₽", Value: avgDef,
				Default: evalPtr(a.Default), Source: src, SourceRef: "как в /api/effect: средняя цена позиций в заказах (каталог)",
				Hint: a.Hint})
		}
	}
	baseMargin := add(ecBaseMargin, "Базовая маржа заказа", "доля", "synthetic", m.refs[ecBaseMargin], "",
		"G₀ = средний чек × базовая маржа", m.baseMargin, nil, nil)
	coverage := add(ecCoverage, "Покрытие блока «Дополните решение»", "доля", "measured", m.refs[ecCoverage], "",
		"Доля заказов, у якоря которых блок непуст; офлайн-замер на размеченных парах", m.coverage, nil, nil)
	attachRef, attachSrc := m.refs[ecAttach], "measured"
	if m.attach == nil {
		attachRef, attachSrc = "событий блока на стенде нет — допущение до пилота", "assumed"
	}
	attach := add(ecAttach, "Доля присоединения", "доля", attachSrc, attachRef, "",
		"Доля показов блока, после которых компаньон добавлен; меряется в пилоте", m.attach, evalPtr(assumedAttachRate),
		scen(func(i int) float64 { return ecScenarios[i].attach }))
	incr := add(ecIncrement, "Коэффициент инкрементальности", "доля", "literature",
		"не менее 75 % кликов по рекомендациям случились бы и без них — инкрементальна не больше четверти", litIncrementURL,
		"Доля присоединённых продаж, которых без блока не было бы", nil, evalPtr(litIncrementality),
		scen(func(i int) float64 { return ecScenarios[i].increment }))
	compSum := add(ecCompSum, "Сумма допроданного", "₽", "measured", m.refs[ecCompSum], "",
		"Средняя цена позиции из блока", m.compSum, nil, nil)
	compMargin := add(ecCompMargin, "Маржа допроданного", "доля", "synthetic", m.refs[ecCompMargin], "",
		"Маржа позиций блока", m.compMargin, nil, nil)
	mix := add(ecMixPerOrder, "Сдвиг микса: изменение GP якоря на заказ", "₽", "synthetic", m.refs[ecMixPerOrder], "",
		"Только якорная позиция; при весе маржи контура поиска — 0", m.mixPerOrder, nil, nil)
	anchor := add(ecAnchorLoss, "Потеря конверсии якоря", "доля", "literature",
		"эксперимент fashion-ритейлера: рекомендации дали −1,9 % продаж якоря на просмотр при +11 % суммарных", litAnchorLossURL,
		"Вычет: доля заказов якоря, потерянных из-за показа блока", nil, evalPtr(litAnchorLoss),
		scen(func(i int) float64 { return ecScenarios[i].anchor }))
	retRef := m.refs[ecReturnRate]
	returns := add(ecReturnRate, "Доля несовместимых пар (возвраты)", "доля", "measured", retRef, "",
		"Вычет: несовместимый компаньон возвращается, его GP теряется", m.returnRate, nil, nil)
	res.Synthetic = len(res.SyntheticKeys) > 0 || m.synthetic

	if eff.Measured.Searches == 0 || orders == nil || o0 <= 0 {
		res.Missing = append(res.Missing, "нет заказов поиска за период — модели не на чем стоять")
		return res
	}
	if avgDef == nil || baseMargin == nil {
		res.Missing = append(res.Missing, "нет G₀: средний чек или базовая маржа не посчитаны")
		return res
	}
	if len(rescued) == 0 {
		res.Missing = append(res.Missing, eff.Missing...)
	}
	g0 := *avgDef * *baseMargin
	for i, sc := range ecScenarios {
		var dOplus *float64
		if v, ok := rescued[sc.id]; ok {
			dOplus = evalPtr(v)
		}
		if v, ok := over[ecRescuedOrder]; ok && v != nil {
			dOplus = evalPtr(*v)
		}
		row := out.EffectCommerceScenario{ID: sc.id, Label: sc.label, Rule: sc.rule, Complete: true}
		dOp := 0.0
		if dOplus != nil {
			dOp = *dOplus
		} else {
			row.Complete = false
		}
		var dOminus *float64
		if coverage != nil && anchor != nil {
			dOminus = evalPtr((o0 + dOp) * *coverage * *anchor * ecScenarios[i].anchor)
		} else {
			row.Complete = false
		}
		dO := dOp - ecDeref(dOminus)
		o1 := o0 + dO
		var upsellPO, returnsPO *float64
		if coverage != nil && attach != nil && incr != nil && compSum != nil && compMargin != nil {
			at := math.Min(1, *attach*sc.attach)
			upsellPO = evalPtr(*coverage * at * math.Min(1, *incr*sc.increment) * *compSum * *compMargin)
			if returns != nil {
				returnsPO = evalPtr(-*coverage * at * *returns * *compSum * *compMargin)
			}
		}
		if upsellPO == nil || returnsPO == nil || mix == nil {
			row.Complete = false
		}
		dG := ecDeref(upsellPO) + ecDeref(mix) + ecDeref(returnsPO)
		gp := func(v *float64, mul float64) *float64 {
			if v == nil {
				return nil
			}
			return evalPtr(math.Round(*v * mul))
		}
		row.Terms = []out.EffectTerm{
			{Key: "rescued", Label: "Возврат поиска: спасённые заказы", Kind: "mechanism", GPRub: gp(dOplus, g0),
				Orders: roundPtr1(dOplus), Formula: "ΔO₊ × G₀"},
			{Key: "upsell", Label: "Допродажа", Kind: "mechanism", GPRub: gp(upsellPO, o1),
				Formula: "O₁ × покрытие × присоединение × инкрементальность × сумма × маржа"},
			{Key: "mix", Label: "Сдвиг микса (рычаг маржи, якорь)", Kind: "mechanism", GPRub: gp(mix, o1),
				Formula: "O₁ × ΔGP якоря на заказ"},
			{Key: "anchor_loss", Label: "Потеря конверсии якоря", Kind: "deduction", GPRub: gp(dOminus, -g0),
				Orders: negPtr(roundPtr1(dOminus)), Formula: "−(O₀ + ΔO₊) × покрытие × доля потери × G₀"},
			{Key: "returns", Label: "Возвраты из-за несовместимости", Kind: "deduction", GPRub: gp(returnsPO, o1),
				Formula: "−O₁ × покрытие × присоединение × доля несовместимых × сумма × маржа"},
		}
		terms := 0.0
		for _, t := range row.Terms {
			terms += ecDeref(t.GPRub)
		}
		row.DeltaGPRub = math.Round(terms)
		direct := o1*(g0+dG) - o0*g0
		chk := &out.EffectIdentity{O0: round2(o0), G0: round2(g0), DeltaO: round2(dO), DeltaG: round2(dG),
			DOxG0: math.Round(dO * g0), O0xDG: math.Round(o0 * dG), DOxDG: math.Round(dO * dG), Direct: math.Round(direct),
			Terms: math.Round(terms), Epsilon: effectIdentityEps}
		chk.Sum = math.Round(dO*g0 + o0*dG + dO*dG)
		// Члены округлены по отдельности: допуск — по рублю на член.
		chk.OK = math.Abs(direct-(dO*g0+o0*dG+dO*dG)) < effectIdentityEps && math.Abs(terms-direct) <= float64(len(row.Terms))*effectIdentityEps+1e-9
		row.Check = chk
		if o0*g0 > 0 {
			row.Uplift = evalPtr(evalRound4(terms / (o0 * g0)))
		}
		res.Scenarios = append(res.Scenarios, row)
	}
	if slices.ContainsFunc(res.Scenarios, func(sc out.EffectCommerceScenario) bool { return !sc.Complete }) {
		res.Warnings = append(res.Warnings, "Не все члены посчитаны — сумма по посчитанным, причины в missing")
	}
	return res
}

func ecDeref(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

func roundPtr1(v *float64) *float64 {
	if v == nil {
		return nil
	}
	return evalPtr(math.Round(*v*10) / 10)
}

func negPtr(v *float64) *float64 {
	if v == nil {
		return nil
	}
	return evalPtr(-*v)
}

func nilIfEmpty(m map[string]float64) map[string]float64 {
	if len(m) == 0 {
		return nil
	}
	return m
}
