package services

import (
	"fmt"
	"sort"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Ранжирование B3 (ARCHITECTURE §2.5): допустимость (обязательные условия) → порог релевантности → группы
// exact/match/analog → бизнес-слой внутри группы. Бизнес-слой не поднимает товар из нижней группы в верхнюю.

// Настройки ранжирования. Стартовые значения подобраны на tune-наборе.
const (
	// b3RRFK — константа RRF для B3: меньше, чем 60 в B2, — верх списков весит заметно больше хвоста.
	b3RRFK = 20
	// relThreshold — порог релевантности семейства (RRF, нормированный на лучший в выдаче, 0–1).
	relThreshold = 0.30
	// denseOnlyMin — кандидат, найденный только векторами, нужен со скором kNN не ниже этого.
	denseOnlyMin = 0.745
	// absentDenseMin — аналог для товара, которого нет в каталоге, — только очень близкий по смыслу.
	absentDenseMin = 0.76
	// abbrevDenseMin — то же для запроса из непонятного сокращения («кис»): найти можно только по смыслу.
	abbrevDenseMin = 0.70
	// analogBaseRel — релевантность аналога из справочника, если поиск его не нашёл сам.
	analogBaseRel = 0.5
	// licenseFitWeight — доля релевантности, которую даёт соответствие количества лицензии.
	licenseFitWeight = 0.5
	// licenseTypeMissFactor, licenseDurationMissFactor — доля соответствия лицензии, если у семейства нет
	// запрошенного типа (бессрочная / подписка) или срока.
	licenseTypeMissFactor     = 0.5
	licenseDurationMissFactor = 0.75
	// maxRejected — сколько отказов писать в трассу.
	maxRejected = 60
)

// Веса бизнес-слоя: final = 0.80·relevance + 0.10·popularity + 0.05·margin + 0.05·context.
var businessWeights = map[string]float64{"relevance": 0.80, "popularity": 0.10, "margin": 0.05, "context": 0.05}

// Причины отказа (trace.rejected[].reason).
const (
	rejectMetric     = "license_metric_mismatch"
	rejectType       = "license_type_mismatch"
	rejectOrigin     = "origin_mismatch"
	rejectVendor     = "vendor_mismatch"
	rejectThreshold  = "below_threshold"
	rejectReference  = "reference_product"
	rejectDenseScore = "dense_score_too_low"
	rejectQuantity   = "license_quantity_mismatch"
	rejectCategory   = "category_mismatch"
	// rejectAbsentDense — товара нет в каталоге, аналог допускается только очень близкий по смыслу.
	rejectAbsentDense = "absent_not_close"
	// rejectAbsentTail — товара нет: показываются только пять ближайших аналогов.
	rejectAbsentTail = "absent_tail"
	// rejectTail — хвост выдачи: другая категория и скор заметно ниже первой позиции.
	rejectTail = "tail_low_relevance"
	// rejectNoise — запрос без смысла (разметка, шум) или не о товаре: товары не показываются.
	rejectNoise = "noise_query"
)

// tailRatio — позиция другой категории, чей итоговый скор ниже этой доли скора первой позиции, — шум хвоста.
// Внутри того же раздела каталога (офисные пакеты и PDF) порог строже — tailRatioSameRoot.
const (
	tailRatio         = 0.7
	tailRatioSameRoot = 0.5
)

var groupOrder = map[string]int{domain.GroupExact: 0, domain.GroupMatch: 1, domain.GroupAnalog: 2}

// rankCtx — всё, что нужно ранжированию одного запроса.
type rankCtx struct {
	lx    *searchLexicon
	attrs queryAttrs
	ent   queryEntities
	// purchased — клиент → купленные позиции (p:id) и семейства (f:id).
	purchased map[string]bool
	// purchasedQty — купленное количество по семейству: продление того же объёма.
	purchasedQty map[string]int
	// boughtVendors — вендоры купленного.
	boughtVendors map[string]bool
	// refCats — категории референсов аналога.
	refCats map[string]bool
	// productRel — релевантность позиции из слияния (0 — поиск её не нашёл).
	productRel map[string]float64
	// business — запрос бизнес-масштаба: количество от 10 или клиент-организация (smb, enterprise, gov).
	// Тогда домашние линейки стоят ниже бизнес-линеек (familyHit.scale).
	business bool
	// textTop — семейства из первых textTopN мест текстового поиска.
	textTop map[string]bool
	// hw — условия оборудования (hardware_search.go); nil — флаг выключен.
	hw *hwPlan
	// upgradeTargets — позиции-цели связей upgrade («старшая редакция»): у вариантов под сделку без названной
	// редакции заранее не выбираются (purchase_attrs.go).
	upgradeTargets map[string]bool
	// wantKinds — виды позиции, названные в запросе (техподдержка, модуль, курс…): они наравне с базовой
	// лицензией при выборе головы семейства (search_head.go).
	wantKinds map[string]bool
}

// textTopN — сколько первых мест BM25 считаются совпадением по названию (не отсекаются категорией).
const textTopN = 3

// businessQuantity — с какого количества лицензий запрос считается бизнес-закупкой.
const businessQuantity = 10

// businessSegments — сегменты клиента-организации.
var businessSegments = setOf("smb", "enterprise", "gov")

// isBusinessQuery — запрос бизнес-масштаба: явное количество от businessQuantity или клиент-организация с
// общим запросом («продлить антивирус»). Если продукт назван («касперский премиум»), сегмент порядок не меняет.
func isBusinessQuery(a queryAttrs, e queryEntities, segment string) bool {
	if a.quantity >= businessQuantity {
		return true
	}
	return businessSegments[segment] && len(e.vendors) == 0 && len(e.families) == 0 && e.parsedFamily == ""
}

// rejectReason — нарушенное обязательное условие позиции; "" — допустима.
func (rc rankCtx) rejectReason(d indexDoc) (string, string) {
	if reason, detail := rc.hw.reject(d); reason != "" {
		return reason, detail
	}
	a := rc.attrs
	if a.origin == "ru" && d.Origin != "ru" {
		return rejectOrigin, "нужно отечественное ПО"
	}
	if !a.analog && len(rc.ent.vendors) > 0 && !contains(rc.ent.vendors, d.VendorID) {
		return rejectVendor, "в запросе назван другой вендор"
	}
	if a.renewal && d.LicenseType != domain.LicenseRenewal {
		return rejectType, "нужно продление"
	}
	if a.metric != "" && d.LicenseMetric != nil && *d.LicenseMetric != a.metric && !isPerUnit(d) {
		return rejectMetric, "лицензия на " + *d.LicenseMetric + ", нужна на " + a.metric
	}
	if hidden, detail := rc.offerHidden(d); hidden {
		return rejectOffer, detail // вариант под сделку или организацию, которых запрос не называет (purchase_attrs.go)
	}
	return "", ""
}

// fit — насколько вариант лицензии подходит под условия запроса; ok — подходит полностью.
func (rc rankCtx) fit(d indexDoc) (int, bool) {
	a := rc.attrs
	score, ok := 0, true
	q := 0
	if d.LicenseQuantity != nil {
		q = *d.LicenseQuantity
	}
	if a.quantity > 0 {
		switch {
		case q == a.quantity:
			score += 8
		case quantityOK(d, a.quantity):
			score += 6
		default:
			ok = false
		}
	}
	if a.metric != "" && d.LicenseMetric != nil {
		if *d.LicenseMetric == a.metric {
			score += 4
		} else {
			ok = false
		}
	}
	if a.licType != "" {
		if d.LicenseType == a.licType {
			score += 3
		} else {
			ok = false
		}
	}
	if a.duration > 0 {
		if d.LicenseDuration != nil && *d.LicenseDuration == a.duration {
			score += 2
		} else {
			ok = false
		}
	}
	if a.metricPref != "" && d.LicenseMetric != nil && *d.LicenseMetric == a.metricPref {
		score += 2
	}
	if a.serverPref && d.LicenseMetric != nil && *d.LicenseMetric == domain.MetricServer {
		score += 2
	}
	if !a.renewal && d.LicenseType != domain.LicenseRenewal && d.LicenseType != domain.LicenseUpgrade {
		score++ // без «продлить» — новая лицензия, а не продление
	}
	if rc.purchased["p:"+d.ProductID] {
		score++
	}
	if a.analog && rc.refCats[d.CategoryID] {
		score++ // замена Windows — ОС для рабочих станций, а не серверная
	}
	if pq, bought := rc.purchasedQty[d.FamilyID]; bought && a.quantity == 0 && q == pq {
		score++ // продление того же объёма, что покупал клиент
	}
	score += rc.purchaseFit(d)
	return score, ok
}

// contextScore — контекст клиента: 1 — покупал это семейство, 0.5 — покупал у этого вендора.
func (rc rankCtx) contextScore(d indexDoc) float64 {
	switch {
	case rc.purchased["f:"+d.FamilyID]:
		return 1
	case rc.boughtVendors[d.VendorID]:
		return 0.5
	}
	return 0
}

// familyHit — семейство в выдаче: лучшая под условия лицензия и остальные варианты.
type familyHit struct {
	id      string
	rel     float64
	group   string
	head    indexDoc
	headOK  bool
	heads   []indexDoc // допустимые варианты по убыванию соответствия
	fits    map[string]bool
	final   float64
	parts   domain.ScoreParts
	context float64
	// tier — ступень внутри группы: 3 — названо целиком среди нескольких названных (fullNamed), 2 — семейство
	// названо в запросе, 1 — путь продления названного семейства (relations renewal), 0 — остальное (вендор назван,
	// название совпало частично).
	tier int
	// signals — почему семейство стоит выше (в трассу).
	signals []string
	// scale — 0: домашняя линейка в запросе бизнес-масштаба (ниже), 2 — семейство из подкатегории, названной словами
	// запроса (выше, namedCategory), 1 — остальное.
	scale int
	// exactQty — 1: лицензия головы ровно на запрошенное количество («Р7-Офис на 30 мест» на «офис на 30 мест»):
	// выше позиций, которые закрывают количество поштучно. Условие «учтено строго» — и в порядке выдачи.
	// Для одного места не действует: поштучная лицензия на 1 место — то же самое.
	exactQty int
	// core — основная линейка вендора в запросе «вендор + условия» (search_vendor_core.go): 3 — редакция линейки, 2 — общий продукт
	// основной категории, 1 — специализированный, 0 — остальное.
	core int
}

// buildFamily — семейство из всех его позиций: недопустимые уходят в отказы, лучшая по соответствию —
// во главу, остальные — в варианты. nil — допустимых позиций нет.
//
// При равном соответствии условиям голова — базовая лицензия, которую можно купить сейчас (search_head.go), затем
// позиция, которую поиск поставил выше (точное название, артикул), затем более популярная.
func (rc rankCtx) buildFamily(fid string, rel float64, rejected *[]domain.RejectedHit) *familyHit {
	type scored struct {
		d   indexDoc
		fit int
		ok  bool
	}
	var ok []scored
	var orgOnly []indexDoc
	for _, pid := range rc.lx.families[fid] {
		d := rc.lx.docs[pid]
		if reason, detail := rc.rejectReason(d); reason != "" {
			addRejected(rejected, d.ProductID, reason, detail)
			if reason == rejectOffer && rc.onlyOrgHidden(d) {
				orgOnly = append(orgOnly, d)
			}
			continue
		}
		f, full := rc.fit(d)
		ok = append(ok, scored{d, f, full})
	}
	if len(ok) == 0 && len(orgOnly) > 0 && len(orgOnly) == len(rc.lx.families[fid]) && rc.namesFamily(fid) {
		// названный продукт продаётся только под тип организации (Windows Server Standard — только для
		// госучреждений): он не скрывается, а показывается с этой оговоркой (orgOnlyNote) — по запросу.
		for _, d := range orgOnly {
			f, full := rc.fit(d)
			ok = append(ok, scored{d, f, full})
		}
	}
	if len(ok) == 0 {
		return nil
	}
	sort.SliceStable(ok, func(i, j int) bool {
		if bi, bj := rc.secondaryKind(ok[i].d), rc.secondaryKind(ok[j].d); bi != bj && ok[i].ok == ok[j].ok {
			// Продление, поддержка, модуль, курс — головой, только если базовой с тем же соответствием условиям нет
			// (search_head.go); вариант, который подходит под условия полностью, остаётся выше.
			return bj
		}
		if ok[i].fit != ok[j].fit {
			return ok[i].fit > ok[j].fit
		}
		if less, decided := rc.headLess(ok[i].d, ok[j].d); decided {
			return less
		}
		if ri, rj := rc.productRel[ok[i].d.ProductID], rc.productRel[ok[j].d.ProductID]; ri != rj {
			return ri > rj
		}
		return ok[i].d.Popularity > ok[j].d.Popularity
	})
	fh := &familyHit{id: fid, rel: rel, head: ok[0].d, headOK: ok[0].ok, fits: map[string]bool{}}
	for _, s := range ok {
		fh.heads = append(fh.heads, s.d)
		fh.fits[s.d.ProductID] = s.ok
	}
	return fh
}

func addRejected(rejected *[]domain.RejectedHit, pid, reason, detail string) {
	for _, r := range *rejected {
		if r.ProductID == pid {
			return
		}
	}
	if len(*rejected) < maxRejected {
		*rejected = append(*rejected, domain.RejectedHit{ProductID: pid, Reason: reason, Detail: detail})
	}
}

// assignGroup — exact: семейство или вендор названы в запросе и лицензия подходит полностью (или это
// продление купленного клиентом продукта); match — остальное релевантное.
func (rc rankCtx) assignGroup(fh *familyHit) {
	if fh.group == domain.GroupAnalog {
		return
	}
	fh.tier, fh.signals = rc.tierOf(fh.id)
	if t := rc.hw.compatTier(fh.head); t > 0 {
		fh.tier += t // «X для Y»: подтверждённая совместимость выше условной (hardware_search.go)
		fh.signals = append(fh.signals, signalCompatVerified)
	}
	fh.scale, fh.exactQty = 1, 0
	if n := rc.attrs.quantity; n > 1 && fh.head.LicenseQuantity != nil && *fh.head.LicenseQuantity == n && !isPerUnit(fh.head) {
		fh.exactQty = 1
	}
	if rc.business && rc.lx.consumer[fh.id] {
		fh.scale = 0
		fh.signals = append(fh.signals, signalConsumerScale)
	} else if rc.namedCategory(fh.id) {
		fh.scale = 2
		fh.signals = append(fh.signals, signalNamedCategory)
	}
	named := rc.exactNamed(fh.id)
	switch {
	case named && fh.headOK:
		fh.group = domain.GroupExact
	case rc.attrs.renewal && rc.purchased["f:"+fh.id] && fh.headOK:
		fh.group = domain.GroupExact
	default:
		fh.group = domain.GroupMatch
	}
}

// exactNamed — семейство названо в запросе для группы exact: вендор назван, семейство названо целиком, его
// назвал разбор или совпало слово названия, которое есть только у одного вендора («windows», «1с»). Слово
// нескольких вендоров («офис» → office: Microsoft Office и Kaspersky Small Office) точным совпадением не
// считается: иначе «офис на 30 мест» ставил Office LTSC на 1 ПК над Р7-Офис на 30 мест (QA M3).
func (rc rankCtx) exactNamed(fid string) bool {
	if contains(rc.ent.vendors, rc.lx.familyVendor[fid]) || contains(rc.ent.named, fid) || rc.ent.parsedFamily == fid {
		return true
	}
	if !contains(rc.ent.families, fid) {
		return false
	}
	for _, t := range rc.ent.entityTokens {
		for _, w := range wordsOf(t) {
			if rc.lx.familyWords[fid][w] && len(rc.lx.wordVendors[w]) == 1 {
				return true
			}
		}
	}
	return false
}

// Сигналы позиции в трассе (trace.items[].signals).
const (
	signalNamedFamily = "named_family"
	signalRenewalPath = "renewal_path"
	// signalConsumerScale — домашняя линейка ниже бизнес-линеек: количество от 10 или клиент-организация.
	signalConsumerScale = "consumer_scale"
	// signalNamedCategory — семейство из подкатегории, названной словами запроса («dr.web для бизнеса»: Enterprise
	// Security Suite из «Антивирус для бизнеса» выше Security Space из «Антивирус для дома»).
	signalNamedCategory = "named_category"
	// signalFullName — названо целиком среди нескольких названных: «windows server 2025» — сервер выше
	// «Windows Server 2025 CAL», у которой в запросе нет слова «cal».
	signalFullName = "full_name"
)

// namedCategory — вендор назван, и словами запроса названа подкатегория (мягкое условие, не корень каталога):
// семейство вендора из неё внутри своей ступени стоит выше его семейств из соседних подкатегорий («dr.web для
// бизнеса», «kaspersky … для бизнеса»). Без вендора не действует: слово категории без бренда («защита от утечек»)
// угадывается хуже, чем его находит текстовый поиск.
func (rc rankCtx) namedCategory(fid string) bool {
	want := rc.ent.category
	if !rc.ent.categoryByWords || want == "" || len(rc.ent.vendors) == 0 {
		return false
	}
	root := rc.lx.categoryRoot[want]
	return root != "" && root != want && rc.lx.familyCat[fid] == want
}

// fullNamed — среди нескольких семейств, названных в запросе, это названо целиком (все значимые слова названия, кроме
// имени вендора, есть в запросе), а хотя бы одно другое — нет: у него в названии есть слово, которого в запросе нет.
func (rc rankCtx) fullNamed(fid string) bool {
	if len(rc.ent.named) < 2 || !rc.namedWhole(fid) {
		return false
	}
	for _, f := range rc.ent.named {
		if f != fid && !rc.namedWhole(f) && !rc.pointsTo(f, fid) {
			return true
		}
	}
	return false
}

// pointsTo — в запросе есть слово, которое называет семейство f, но не fid: слово его названия или основа слова из
// его названий и описаний («клиентские лицензии windows server 2025» — это CAL, а не сервер).
func (rc rankCtx) pointsTo(f, fid string) bool {
	fs, own := rc.lx.familyStems(f), rc.lx.familyStems(fid)
	for _, t := range rc.ent.entityTokens {
		for _, w := range wordsOf(t) {
			if rc.lx.familyWords[f][w] && !rc.lx.familyWords[fid][w] {
				return true
			}
			if st := cqStem(w); st != "" && fs[st] && !own[st] {
				return true
			}
		}
	}
	return false
}

// namedWhole — каждое значимое слово названия семейства есть в запросе; слова имени вендора («Microsoft») можно
// не называть.
func (rc rankCtx) namedWhole(fid string) bool {
	in := map[string]bool{}
	for _, t := range rc.ent.entityTokens {
		for _, w := range wordsOf(t) {
			in[w] = true
		}
	}
	vendor := map[string]bool{}
	for _, w := range wordsOf(rc.lx.vendorName[rc.lx.familyVendor[fid]]) {
		vendor[w] = true
	}
	for w := range rc.lx.familyWords[fid] {
		if !in[w] && !vendor[w] {
			return false
		}
	}
	return len(rc.lx.familyWords[fid]) > 0
}

// tierOf — ступень семейства внутри группы. Семейство, названное в запросе целиком («kaspersky internet
// security продление»), стоит выше семейств того же вендора, у которых совпала только часть названия
// («Kaspersky Standard»): бизнес-слой внутри группы не может переставить их местами. При «продлить» сразу за
// названным идёт семейство, на которое оно продлевается по справочнику (KIS снят → продление переходом на Plus).
// Названо всё семейство вендора («касперский») — ступени равны, порядок прежний.
func (rc rankCtx) tierOf(fid string) (int, []string) {
	if len(rc.ent.named) == 0 || rc.attrs.analog {
		return 0, nil
	}
	if contains(rc.ent.named, fid) {
		if rc.fullNamed(fid) {
			return 3, []string{signalNamedFamily, signalFullName}
		}
		return 2, []string{signalNamedFamily}
	}
	if rc.attrs.renewal {
		for _, f := range rc.ent.named {
			if contains(rc.lx.renewals[f], fid) {
				return 1, []string{signalRenewalPath}
			}
		}
	}
	return 0, nil
}

// score — бизнес-слой: слагаемые итогового скора (уже умноженные на веса).
func (rc rankCtx) score(fh *familyHit) {
	d := fh.head
	fh.context = rc.contextScore(d)
	rel := fh.rel * (1 - licenseFitWeight + licenseFitWeight*rc.licenseFit(d))
	fh.parts = domain.ScoreParts{
		Relevance:  round4(businessWeights["relevance"] * rel),
		Popularity: round4(businessWeights["popularity"] * d.Popularity),
		Margin:     round4(businessWeights["margin"] * d.Margin),
		Context:    round4(businessWeights["context"] * fh.context),
	}
	fh.final = round4(fh.parts.Relevance + fh.parts.Popularity + fh.parts.Margin + fh.parts.Context)
}

// dropQuantityMisfits — количество и метрика — обязательные условия: если хотя бы одно семейство закрывает
// их своей лицензией, семейства без такой лицензии уходят в отказы. Если не закрывает никто — остаются, и
// покупателю говорится, что точного количества нет (relaxedMessages).
func (rc rankCtx) dropQuantityMisfits(fams []*familyHit, rejected *[]domain.RejectedHit) []*familyHit {
	a := rc.attrs
	if a.quantity == 0 {
		return fams
	}
	// Поштучная позиция закрывает и количество, и метрику: нужное число мест указывается при заказе — так же,
	// как в rejectReason. Раньше здесь метрика была строгой, и для «аналог фотошопа на 20 пользователей»
	// ФотоМАСТЕР и Affinity (поштучно, на устройство) уходили в отказы — оставался один CorelDRAW.
	strictOK := func(d indexDoc) bool {
		return quantityOK(d, a.quantity) && (a.metric == "" || d.LicenseMetric == nil || *d.LicenseMetric == a.metric || isPerUnit(d))
	}
	some := false
	for _, fh := range fams {
		some = some || strictOK(fh.head)
	}
	if !some {
		return fams
	}
	out := fams[:0]
	for _, fh := range fams {
		if strictOK(fh.head) {
			out = append(out, fh)
			continue
		}
		for _, d := range fh.heads {
			addRejected(rejected, d.ProductID, rejectQuantity, "нет лицензии на "+quantityLabel(a.quantity, a.metric))
		}
	}
	return out
}

// dropOtherCategories — категория названа в запросе («офис», «субд», «сзи»): если в выдаче есть семейства этой
// категории (по корню), семейства других категорий уходят в отказы.
func (rc rankCtx) dropOtherCategories(fams []*familyHit, rejected *[]domain.RejectedHit) []*familyHit {
	c := rc.ent.category
	if c == "" || rc.attrs.analog {
		return fams
	}
	root := rc.lx.categoryRoot[c]
	in := func(fh *familyHit) bool { return rc.lx.categoryRoot[rc.lx.familyCat[fh.id]] == root }
	some := false
	for _, fh := range fams {
		some = some || in(fh)
	}
	if !some {
		return fams
	}
	out := fams[:0]
	for _, fh := range fams {
		if in(fh) || rc.textTop[fh.id] {
			out = append(out, fh) // категория — по словам, но название совпало с запросом: «Альт Виртуализация»
			continue
		}
		addRejected(rejected, fh.head.ProductID, rejectCategory, "в запросе названа категория «"+rc.lx.categoryName[c]+"»")
	}
	return out
}

// dropTail — шум хвоста («винда 11 про» → Acrobat Pro, Zoom Pro): позиция другой категории, чем первая, со
// скором ниже tailRatio от первой, уходит в отказы с причиной. Аналоги и запрос аналога не трогаются.
func (rc rankCtx) dropTail(fams []*familyHit, rejected *[]domain.RejectedHit) []*familyHit {
	if len(fams) < 2 || rc.attrs.analog || fams[0].group == domain.GroupAnalog {
		return fams
	}
	top := fams[0]
	cat, vendor := rc.lx.familyCat[top.id], rc.lx.familyVendor[top.id]
	out := fams[:1]
	for _, fh := range fams[1:] {
		c := rc.lx.familyCat[fh.id]
		ratio := tailRatio
		if rc.lx.categoryRoot[c] == rc.lx.categoryRoot[cat] {
			ratio = tailRatioSameRoot // соседняя категория того же раздела — только явно слабое
		}
		// Категория названа в запросе: её позиции — не хвост, даже если первой встала позиция другого раздела
		// («операционная система … для 30 АРМ» → первым Р7-Офис по «30 рабочих мест», ОС не отсеиваются).
		root := rc.lx.categoryRoot[rc.ent.category]
		named := rc.ent.category != "" && rc.lx.categoryRoot[cat] != root && rc.lx.categoryRoot[c] == root
		keep := fh.group == domain.GroupAnalog || c == cat || rc.lx.familyVendor[fh.id] == vendor || named ||
			contains(rc.ent.families, fh.id) || contains(rc.ent.vendors, rc.lx.familyVendor[fh.id])
		if !keep && fh.final < ratio*top.final {
			addRejected(rejected, fh.head.ProductID, rejectTail,
				fmt.Sprintf("другая категория и вендор, скор %.2f < %.2f × %.2f первой позиции", fh.final, ratio, top.final))
			continue
		}
		out = append(out, fh)
	}
	return out
}

// licenseFit — насколько лицензия головы закрывает запрошенное количество: ровно столько — 1, поштучно
// или пакетами — 0.4, нет — 0. Входит в релевантность: «1С на 5 пользователей» — сначала пакет на 5 мест.
// Метрика не та (поштучная лицензия на устройство, когда просили на пользователя) — вдвое меньше.
func (rc rankCtx) licenseFit(d indexDoc) float64 {
	n := rc.attrs.quantity
	fit := 0.0
	switch {
	case n == 0:
		fit = 1
	case d.LicenseQuantity != nil && *d.LicenseQuantity == n:
		fit = 1
	case quantityOK(d, n):
		fit = 0.4
	}
	if m := rc.attrs.metric; m != "" && d.LicenseMetric != nil && *d.LicenseMetric != m {
		fit *= 0.5
	}
	// Тип и срок лицензии — мягкие условия: семейство без бессрочной лицензии, когда просили бессрочную,
	// остаётся в выдаче, но ниже тех, у кого она есть.
	if t := rc.attrs.licType; t != "" && d.LicenseType != t {
		fit *= licenseTypeMissFactor
	}
	if m := rc.attrs.duration; m > 0 && (d.LicenseDuration == nil || *d.LicenseDuration != m) {
		fit *= licenseDurationMissFactor
	}
	return fit
}

// sortFamilies — по группе, внутри группы — по ступени (названное семейство выше), затем по итоговому скору.
func sortFamilies(fs []*familyHit) {
	sort.SliceStable(fs, func(i, j int) bool {
		if gi, gj := groupOrder[fs[i].group], groupOrder[fs[j].group]; gi != gj {
			return gi < gj
		}
		if fs[i].tier != fs[j].tier {
			return fs[i].tier > fs[j].tier
		}
		if fs[i].scale != fs[j].scale {
			return fs[i].scale > fs[j].scale
		}
		if fs[i].core != fs[j].core {
			return fs[i].core > fs[j].core
		}
		if fs[i].exactQty != fs[j].exactQty {
			return fs[i].exactQty > fs[j].exactQty
		}
		if fs[i].final != fs[j].final {
			return fs[i].final > fs[j].final
		}
		return fs[i].id < fs[j].id
	})
}

// toItem — позиция ответа: голова семейства и остальные допустимые варианты.
func (rc rankCtx) toItem(fh *familyHit) out.SearchItem {
	mk := func(d indexDoc) out.SearchItem {
		return out.SearchItem{
			ProductID: d.ProductID, FamilyID: d.FamilyID, Title: d.Title, Vendor: d.Vendor, Category: d.Category,
			LicenseLine: licenseLine(d), PriceRub: d.PriceRub, PriceOnRequest: d.PriceOnRequest, Group: fh.group,
			MatchesQuery:    fh.fits[d.ProductID],
			PurchasedBefore: rc.purchased["p:"+d.ProductID] || rc.purchased["f:"+d.FamilyID],
			Final:           fh.final,
			Kind:            docKind(d), MPN: d.MPNRaw, PriceMode: docPriceMode(d), Required: []out.Companion{},
			OfferURL: d.OfferURL, FamilyURL: d.FamilyURL, VendorURL: d.VendorURL, Available: d.Available,
			OldPriceRub: d.OldPriceRub, LicenseKind: d.LicenseKind, TierMin: d.LicenseTierMin, TierMax: d.LicenseTierMax,
		}
	}
	it := mk(fh.head)
	for _, v := range fh.heads[1:] {
		it.Variants = append(it.Variants, mk(v))
	}
	return it
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// quantityOK — позиция закрывает нужное количество: ровно столько, поштучная продажа или пакеты ядер,
// которыми набирается нужное число.
func quantityOK(d indexDoc, n int) bool {
	if d.LicenseQuantity == nil {
		return false
	}
	q := *d.LicenseQuantity
	switch {
	case q == n, isPerUnit(d):
		return true
	case d.LicenseTierMin != nil && n >= *d.LicenseTierMin && (d.LicenseTierMax == nil || n <= *d.LicenseTierMax):
		return true // ступень цены за единицу фида: «от 100 до 149 узлов» закрывает 120
	case q > 0 && d.LicenseMetric != nil && *d.LicenseMetric == domain.MetricCore && n%q == 0:
		return true
	}
	return false
}
