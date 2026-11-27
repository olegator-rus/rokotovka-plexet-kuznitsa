package services

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Экран подбора контура экрана: после хода код собирает готовые поля —
// заголовок и подзаголовок лицензии, как продаётся и сколько платить, основание рекомендации, строки сравнения в
// нужном порядке, причины у неподходящих, «что изменилось», полную стоимость задачи и итоговую строку. Фронт ничего
// не выбирает и не считает. Все числа — из карточек ответа (каталог), все «Есть/Нет» — из фактов базы знаний.

// Тексты экрана (UI §7).
const (
	screenTitle        = "Подбор лицензии"
	screenSourceLabel  = "Документация вендора"
	screenVendorsNote  = "Состав функций и условия лицензирования различаются"
	screenOnRequest    = "Цена по запросу"
	screenChooseHint   = "Выберите вариант, чтобы добавить в спецификацию"
	screenSummaryLabel = "Стоимость закупки"
	screenMaxRows      = 6
	screenMaxColumns   = 3
)

// screenHeadings — заголовок блока результата по основанию (UI 6.2).
var screenHeadings = map[string]string{
	out.BasisOnlyValid:       "Подходящая лицензия",
	out.BasisRequiredFeature: "Подходит под ваши условия",
	out.BasisLowestTotal:     "Меньшая стоимость закупки",
	out.BasisVendorNamed:     "Соответствует вашим условиям",
	out.BasisNone:            "Выберите подходящий вариант",
}

// screenCellText — значения ячейки (UI 6.3).
var screenCellText = map[string]string{
	out.MarkYes:         "Есть",
	out.MarkNo:          "Нет",
	out.MarkUnknown:     "Нет сведений",
	out.MarkConditional: "С условиями",
}

// featureShort — существительное функции для строки «Изменено: …, шифрование обязательно».
var featureShort = map[string]string{
	"encryption":       "шифрование",
	"patch_management": "установка обновлений программ",
	"tape_storage":     "хранение на лентах",
}

// reScreenDate — даты и годы в цитатах источников: на экране дат нет.
var (
	reScreenDate      = regexp.MustCompile(`\b\d{1,2}[./]\d{1,2}[./]\d{2,4}\b|\b\d{4}-\d{2}-\d{2}\b`)
	reScreenCopyright = regexp.MustCompile(`\s*\(?©[^)»]*\)?`)
	reScreenYear      = regexp.MustCompile(`(?i)\s*\b(от|на|по состоянию на)?\s*(19|20)\d{2}\s*(г\.|года|год)?\b`)
)

// screenSnapshot — состояние до хода: для строки «Что изменилось».
type screenSnapshot struct {
	turn int
	req  domain.GuideRequirements
}

// snapshotRequirements — копия требований (срезы копируются).
func snapshotRequirements(st domain.GuideState) screenSnapshot {
	r := st.Requirements
	r.Objects = append([]domain.GuideObject{}, r.Objects...)
	r.RequiredFeatures = append([]string{}, r.RequiredFeatures...)
	return screenSnapshot{turn: st.Turn, req: r}
}

// screen — поля экрана к готовому ответу: карточки обогащаются, собирается res.Screen.
func (t *guideTurn) screen(res *out.GuideResponse) {
	if res == nil {
		return
	}
	for i := range res.Offers {
		t.decorateOffer(&res.Offers[i])
	}
	for i := range res.Claims {
		res.Claims[i].Source.Label = screenSourceLabel
		res.Claims[i].Source.Fragment = cleanFragment(res.Claims[i].Source.Fragment)
	}
	if res.Type == out.GuideOutOfScope || res.Type == out.GuideFallback {
		return
	}
	sc := &out.GuideScreen{Title: screenTitle, Task: t.screenTask(res), Options: []string{}, Others: []out.GuideOther{}}
	var matches []*out.GuideOffer
	for i := range res.Offers {
		o := &res.Offers[i]
		switch o.Role {
		case out.OfferMatch:
			matches = append(matches, o)
			sc.Options = append(sc.Options, o.OfferID)
		case out.OfferAlternative, out.OfferRejected:
			sc.Others = append(sc.Others, out.GuideOther{OfferID: o.OfferID, Title: o.Display.Title, Subtitle: o.Display.Subtitle,
				PriceText: o.PriceText, Reason: o.Reason})
		}
	}
	sc.Changed = t.screenChanged(res)
	if len(matches) > 0 {
		sc.Result = t.screenResult(res, matches)
		if sc.Result != nil {
			sc.SelectedOfferID = sc.Result.OfferID
		}
		if sc.Result != nil && sc.Result.Basis == out.BasisRequiredFeature {
			// Требуемая функция не подтверждена — вариант уходит в «При изменении условий» с причиной (сценарий 1).
			kept := matches[:0:0]
			for _, o := range matches {
				if why := t.featureGap(o.FamilyID); why != nil {
					o.Reason = why
					sc.Others = append(sc.Others, out.GuideOther{OfferID: o.OfferID, Title: o.Display.Title,
						Subtitle: o.Display.Subtitle, PriceText: o.PriceText, Reason: why})
					continue
				}
				kept = append(kept, o)
			}
			matches = kept
			sc.Options = sc.Options[:0]
			for _, o := range matches {
				sc.Options = append(sc.Options, o.OfferID)
			}
		}
		sc.Comparison = t.screenComparison(res, matches, sc.Result)
		sc.TaskCost = t.screenTaskCost(matches)
		sc.Summary = screenSummary(matches, sc.SelectedOfferID)
		if sc.Changed != nil {
			sc.Changed.PriceChange = t.screenPriceChange(res, sc.SelectedOfferID)
		}
	}
	res.Screen = sc
}

// --- карточка ---

// decorateOffer — заголовок, подзаголовок, единица продажи, цена единицы, к оплате, расчёт, пакет, причина.
func (t *guideTurn) decorateOffer(o *out.GuideOffer) {
	p := t.cat.products[o.OfferID]
	if p == nil {
		o.Display = &out.GuideOfferDisplay{Title: o.FamilyTitle, Subtitle: "", Details: []out.GuideDetail{}}
		return
	}
	v := t.cat.view(p, t.kb)
	kind := t.screenKind()
	qty := max(o.Quantity, 1)
	o.SaleUnit, o.SaleUnitKind = saleUnit(v, kind), saleUnitKind(v)
	o.PayQuantity = v.units(qty)
	if v.priced() {
		price := *p.PriceRub
		o.UnitPrice = &price
		o.PriceText = rubRU(price) + " ₽ " + saleUnitPhrase(o.SaleUnit)
		if o.Total != nil {
			o.PriceLine = fmt.Sprintf("%d × %s ₽", o.PayQuantity, rubRU(price))
		}
	} else {
		o.PriceText = screenOnRequest
	}
	if v.unit == "pack" && v.pack > 0 {
		seats := o.PayQuantity * v.pack
		pk := &out.GuideOfferPack{Size: v.pack, Packs: o.PayQuantity, Seats: seats, Spare: seats - qty}
		if pk.Spare > 0 {
			pk.Text = fmt.Sprintf("%s: %d %s в запасе", packPhrase(o.PayQuantity, v.pack, kind, true), pk.Spare, guidePlural(pk.Spare, "место", "места", "мест"))
		}
		o.Package = pk
	}
	o.Display = &out.GuideOfferDisplay{Title: t.familyDisplay(p.FamilyID), Subtitle: t.offerSubtitle(v, kind, qty, o.PayQuantity),
		Details: t.offerDetails(v, kind)}
	o.Reason = t.offerReason(o, v)
}

// screenKind — главный объект подбора (компьютеры по умолчанию).
func (t *guideTurn) screenKind() string {
	k, _ := mainObject(t.st.Requirements)
	return orDefault(k, "workstation")
}

// objectNoun — «компьютер», «сервер», «пользователь» в нужной форме.
func objectNoun(kind string, n int) string {
	switch kind {
	case "server":
		return guidePlural(n, "сервер", "сервера", "серверов")
	case "user":
		return guidePlural(n, "пользователь", "пользователя", "пользователей")
	case "connection":
		return guidePlural(n, "подключение", "подключения", "подключений")
	case "host":
		return guidePlural(n, "хост", "хоста", "хостов")
	case "workstation", "":
		return guidePlural(n, "компьютер", "компьютера", "компьютеров")
	}
	return guidePlural(n, "место", "места", "мест")
}

// objectCountText — «50 компьютеров».
func objectCountText(kind string, n int) string { return fmt.Sprintf("%d %s", n, objectNoun(kind, n)) }

// packPhrase — «пакет на 20 компьютеров» / «4 пакета на 5 компьютеров»; capital — с заглавной.
func packPhrase(packs, size int, kind string, capital bool) string {
	s := fmt.Sprintf("пакет на %d %s", size, objectNoun(kind, size))
	if packs > 1 {
		s = fmt.Sprintf("%d %s на %d %s", packs, guidePlural(packs, "пакет", "пакета", "пакетов"), size, objectNoun(kind, size))
	}
	if capital {
		s = guideUpperFirst(s)
	}
	return s
}

// saleUnit — как продаётся: цена пакета на места не делится.
func saleUnit(v guideOfferView, kind string) string {
	switch v.unit {
	case "pack":
		if v.pack > 0 {
			return fmt.Sprintf("пакет на %d %s", v.pack, objectNoun(kind, v.pack))
		}
		return "за пакет"
	case "node_tier":
		if kind == "workstation" {
			return "за компьютер"
		}
		return "за " + objectNoun(kind, 1)
	}
	return v.unitLabel(kind)
}

// saleUnitPhrase — «за компьютер», «за пакет на 20 компьютеров».
func saleUnitPhrase(unit string) string {
	if strings.HasPrefix(unit, "пакет") {
		return "за " + unit
	}
	return unit
}

func saleUnitKind(v guideOfferView) string {
	switch {
	case v.unit == "pack":
		return "package"
	case v.unit == "node_tier":
		return "per_object"
	case v.scope != "" || (v.p.License.Metric != nil && *v.p.License.Metric == "user"):
		return "per_object"
	}
	return "license"
}

// familyDisplay — «{Семейство} — {Редакция}» по линейке базы знаний, иначе название семейства без служебных слов.
func (t *guideTurn) familyDisplay(fid string) string {
	name := suggestDisplay(t.cat.famName[fid])
	line, ok := t.kb.lines[fid]
	if !ok || line.Line == "" {
		return name
	}
	var mates []string
	for f, l := range t.kb.lines {
		if l.Line == line.Line && len(t.cat.byFamily[f]) > 0 {
			mates = append(mates, f)
		}
	}
	if len(mates) < 2 {
		return name
	}
	sort.Strings(mates)
	words := make([][]string, len(mates))
	for i, f := range mates {
		words[i] = strings.Fields(suggestDisplay(t.cat.famName[f]))
	}
	common := len(words[0])
	for _, w := range words[1:] {
		k := 0
		for k < common && k < len(w) && strings.EqualFold(w[k], words[0][k]) {
			k++
		}
		common = k
	}
	own := strings.Fields(name)
	if common == 0 || common >= len(own) {
		return name
	}
	base, edition := strings.Join(own[:common], " "), strings.Join(own[common:], " ")
	if !isEditionWord(edition) {
		return name
	}
	base = strings.TrimSuffix(base, " для бизнеса")
	edition = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(edition, " редакция"), " Редакция"))
	return base + " — " + guideUpperFirst(edition)
}

// isEditionWord — остаток названия — редакция («Расширенный», «Стандартная редакция»), а не другой продукт.
func isEditionWord(s string) bool {
	low := strings.ToLower(s)
	for _, e := range guideEditionStems {
		if strings.Contains(low, e.name) {
			return true
		}
	}
	return false
}

// offerSubtitle — «Новая лицензия · 50 компьютеров · 1 год»; у штучной лицензии с областью действия —
// «Серверная лицензия · бессрочная», «Рабочее место · 1 год».
func (t *guideTurn) offerSubtitle(v guideOfferView, kind string, qty, pay int) string {
	term := v.termLabel()
	if v.unit == "unit" {
		switch v.scope {
		case "server":
			return "Серверная лицензия · " + term
		case "tls_server":
			return "Для TLS-сервера · " + term
		case "workstation":
			return "Рабочее место · " + term
		}
	}
	op := "Новая лицензия"
	switch v.kind {
	case domain.LicenseKindRenewal:
		op = "Продление"
	case "upgrade":
		op = "Обновление"
	}
	obj := objectCountText(kind, qty)
	if v.unit == "pack" && v.pack > 0 {
		obj = packPhrase(pay, v.pack, kind, false)
	}
	return op + " · " + obj + " · " + term
}

// offerDetails — раскрытие «Сведения о лицензии»: артикул, ступень, тип организации, позиция каталога.
func (t *guideTurn) offerDetails(v guideOfferView, kind string) []out.GuideDetail {
	d := []out.GuideDetail{}
	if v.p.SKU != "" {
		d = append(d, out.GuideDetail{Label: "Артикул", Value: v.p.SKU})
	}
	if tl := v.tierLabel(); tl != "" {
		d = append(d, out.GuideDetail{Label: "Ступень", Value: tl + " " + objectNoun(kind, 5)})
	}
	if v.org != "" {
		d = append(d, out.GuideDetail{Label: "Тип организации", Value: guideUpperFirst(orgLabel(v.org))})
	}
	d = append(d, out.GuideDetail{Label: "Позиция каталога", Value: v.p.Title})
	return d
}

// offerReason — код причины и текст у неподходящих и альтернатив.
func (t *guideTurn) offerReason(o *out.GuideOffer, v guideOfferView) *out.GuideReason {
	r := t.st.Requirements
	switch o.Role {
	case out.OfferRejected:
		kind, _ := mainObject(r)
		switch {
		case v.scope == "workstation" && kind == "server":
			return &out.GuideReason{Code: "not_for_server_os", Text: "Не подходит для серверной ОС"}
		case (v.scope == "server" || v.scope == "tls_server") && kind == "workstation":
			return &out.GuideReason{Code: "server_only", Text: "Для серверной ОС"}
		}
		return &out.GuideReason{Code: "rule", Text: strings.TrimRight(orDefault(o.Note, "Не подходит под ваши условия лицензирования"), ".")}
	case out.OfferAlternative:
		if v.scope == "tls_server" && r.Use != "tls_server" {
			return &out.GuideReason{Code: "tls_server_only", Text: "Отдельная лицензия для TLS-сервера"}
		}
		if why := t.featureGap(o.FamilyID); why != nil {
			return why
		}
		if o.Note != "" {
			return &out.GuideReason{Code: "other_conditions", Text: strings.TrimRight(o.Note, ".")}
		}
	}
	return nil
}

// featureGap — причина, если у семейства не подтверждена требуемая функция; nil — все требуемые есть.
func (t *guideTurn) featureGap(fid string) *out.GuideReason {
	r := t.st.Requirements
	for _, axis := range r.RequiredFeatures {
		if t.kb.factYesAnyVariant(fid, axis) != nil {
			continue
		}
		f := t.kb.factOn(fid, axis, r.Variant)
		label := strings.ToLower(factLabel(f, axis))
		if f.verified() {
			if k, _ := f.factValue(); k == "no" {
				return &out.GuideReason{Code: "missing_feature", Text: "Не подходит: нет " + genitiveFeature(label)}
			}
		}
		return &out.GuideReason{Code: "feature_unknown", Text: "Нет сведений о " + prepositionalFeature(label)}
	}
	return nil
}

// factLabel — подпись оси: из факта, иначе справочник.
func factLabel(f *guideFact, axis string) string {
	if f != nil {
		if f.AxisLabel != "" {
			return f.AxisLabel
		}
		if f.Label != "" {
			return f.Label
		}
	}
	return axisLabel(axis)
}

// genitiveFeature — «шифрование данных» → «шифрования данных» (первое слово на -ие/-ия).
func genitiveFeature(label string) string {
	w := strings.Fields(label)
	if len(w) == 0 {
		return label
	}
	switch {
	case strings.HasSuffix(w[0], "ие"):
		w[0] = strings.TrimSuffix(w[0], "ие") + "ия"
	case strings.HasSuffix(w[0], "ка"):
		w[0] = strings.TrimSuffix(w[0], "ка") + "ки"
	case strings.HasSuffix(w[0], "ль"):
		w[0] = strings.TrimSuffix(w[0], "ль") + "ля"
	default:
		return "функции «" + label + "»"
	}
	return strings.Join(w, " ")
}

// prepositionalFeature — «шифрование данных» → «шифровании данных».
func prepositionalFeature(label string) string {
	w := strings.Fields(label)
	if len(w) == 0 {
		return label
	}
	switch {
	case strings.HasSuffix(w[0], "ие"):
		w[0] = strings.TrimSuffix(w[0], "ие") + "ии"
	case strings.HasSuffix(w[0], "ка"):
		w[0] = strings.TrimSuffix(w[0], "ка") + "ке"
	default:
		return "функции «" + label + "»"
	}
	return strings.Join(w, " ")
}

// --- задача ---

// screenTask — понятая задача: «Антивирус для организации · Kaspersky», «КриптоПро CSP 5.0 для серверной ОС».
func (t *guideTurn) screenTask(res *out.GuideResponse) string {
	r := t.st.Requirements
	var parts []string
	switch dir := directionOf(r.Task); {
	case dir == "endpoint" && r.OrgType == "home":
		parts = append(parts, "Антивирус для дома")
	case dir == "endpoint":
		parts = append(parts, "Антивирус для организации")
	case dir != "" && t.kb.dirTitle[dir] != "":
		parts = append(parts, t.kb.dirTitle[dir])
	case dir != "":
		parts = append(parts, guideUpperFirst(directionLabel(dir)))
	}
	if len(parts) == 0 {
		fid := ""
		for _, o := range res.Offers {
			if o.Role == out.OfferMatch {
				fid = o.FamilyID
				break
			}
		}
		if fid == "" && len(t.st.CandidateFamilyIDs) > 0 {
			fid = t.st.CandidateFamilyIDs[0]
		}
		if fid != "" {
			name := suggestDisplay(t.cat.famName[fid])
			kind, _ := mainObject(r)
			switch {
			case r.Use == "tls_server":
				name += " для TLS-сервера"
			case kind == "server":
				name += " для серверной ОС"
			}
			return name
		}
		if r.Vendor != "" {
			return vendorLabel(r.Vendor)
		}
		return ""
	}
	if r.Vendor != "" {
		parts = append(parts, vendorLabel(r.Vendor))
	}
	return strings.Join(parts, " · ")
}

// --- результат ---

// screenResult — основание и одна рекомендация (UI 6.2): required_feature → only_valid → vendor_named →
// lowest_total → none.
func (t *guideTurn) screenResult(res *out.GuideResponse, matches []*out.GuideOffer) *out.GuideResult {
	r := t.st.Requirements
	first := matches[0]
	mk := func(basis string, o *out.GuideOffer, reason string, src *out.GuideScreenSource) *out.GuideResult {
		id := ""
		if o != nil {
			id = o.OfferID
		}
		return &out.GuideResult{Basis: basis, Heading: screenHeadings[basis], OfferID: id, Reason: reason, Source: src}
	}
	// 1) требуемая функция подтверждена только у первой подходящей среди сравниваемых.
	for _, axis := range r.RequiredFeatures {
		f := t.kb.factYesAnyVariant(first.FamilyID, axis)
		if f == nil {
			continue
		}
		unique, without := true, ""
		for _, o := range res.Offers {
			if o.FamilyID == first.FamilyID || (o.Role != out.OfferMatch && o.Role != out.OfferAlternative) {
				continue
			}
			if t.kb.factYesAnyVariant(o.FamilyID, axis) != nil {
				unique = false
				break
			}
			if g := t.kb.factOn(o.FamilyID, axis, r.Variant); g.verified() && without == "" && t.sameLine(o.FamilyID, first.FamilyID) {
				if k, _ := g.factValue(); k == "no" {
					without = t.editionName(o.FamilyID)
				}
			}
		}
		if !unique {
			continue
		}
		label := strings.ToLower(factLabel(f, axis))
		reason := "Включает " + label + "."
		if without != "" {
			reason += " В редакции «" + without + "» этой функции нет."
		}
		return mk(out.BasisRequiredFeature, first, reason, factScreenSource(f))
	}
	// 2) единственная допустимая лицензия.
	if len(matches) == 1 {
		reason, src := "", (*out.GuideScreenSource)(nil)
		if t.data != nil {
			for _, h := range t.data.ruleHits {
				if h.Message != "" {
					reason = strings.TrimRight(h.Message, ".") + "."
					src = t.ruleScreenSource(h, first.FamilyID)
					break
				}
			}
		}
		return mk(out.BasisOnlyValid, first, reason, src)
	}
	vendors := map[string]bool{}
	for _, o := range matches {
		vendors[t.cat.famVend[o.FamilyID]] = true
	}
	// 3) названная редакция или вендор (подходящие — одного названного вендора).
	if res.Recommendation != nil && res.Recommendation.Basis == "named" {
		for _, o := range matches {
			if o.OfferID == res.Recommendation.OfferID {
				return mk(out.BasisVendorNamed, o, "Редакция, которую вы назвали.", nil)
			}
		}
	}
	// 4) наименьшая полная стоимость среди подходящих одного вендора.
	if len(vendors) == 1 {
		var best, second *out.GuideOffer
		ok := true
		for _, o := range matches {
			if o.Total == nil {
				ok = false
				break
			}
			if best == nil || *o.Total < *best.Total {
				best, second = o, best
			} else if second == nil || *o.Total < *second.Total {
				second = o
			}
		}
		if ok && best != nil && second != nil && *best.Total < *second.Total {
			return mk(out.BasisLowestTotal, best, t.differenceText(best, second), nil)
		}
	}
	return mk(out.BasisNone, nil, "", nil)
}

// differenceText — «Меньшая стоимость закупки: Стандартный, на 38 043 ₽ дешевле.»
func (t *guideTurn) differenceText(best, second *out.GuideOffer) string {
	return fmt.Sprintf("Меньшая стоимость закупки: %s, на %s ₽ дешевле.", t.editionName(best.FamilyID),
		rubRU(guideRound2(*second.Total-*best.Total)))
}

// sameLine — семейства одной линейки базы знаний (или одного вендора с общим началом названия).
func (t *guideTurn) sameLine(a, b string) bool {
	la, lb := t.kb.lines[a].Line, t.kb.lines[b].Line
	if la != "" && la == lb {
		return true
	}
	return t.cat.famVend[a] == t.cat.famVend[b] && guideCommonPrefix(guideNorm(t.cat.famName[a]), guideNorm(t.cat.famName[b])) >= 8
}

// editionName — короткое имя редакции («Стандартный»); не редакция — заголовок семейства.
func (t *guideTurn) editionName(fid string) string {
	d := t.familyDisplay(fid)
	if i := strings.Index(d, " — "); i >= 0 {
		return d[i+len(" — "):]
	}
	return d
}

// factScreenSource — «Документация вендора» с фрагментом без дат.
func factScreenSource(f *guideFact) *out.GuideScreenSource {
	if f == nil || f.SourceURL == "" {
		return nil
	}
	return &out.GuideScreenSource{Label: screenSourceLabel, Fragment: cleanFragment(f.Excerpt), URL: f.SourceURL}
}

// ruleScreenSource — «Правила лицензирования {вендор}» у правила базы знаний.
func (t *guideTurn) ruleScreenSource(h guideViolation, fid string) *out.GuideScreenSource {
	if !strings.HasPrefix(h.Source, "http") {
		return nil
	}
	rule := t.kb.ruleByID(h.RuleID)
	frag := rule.Note
	if frag == "" {
		frag = h.Message
	}
	label := "Правила лицензирования"
	if v := vendorLabel(t.cat.famVend[fid]); v != "" {
		label += " " + v
	}
	return &out.GuideScreenSource{Label: label, Fragment: cleanFragment(frag), URL: h.Source}
}

// cleanFragment — цитата без дат, годов и строк «©…».
func cleanFragment(s string) string {
	s = reScreenDate.ReplaceAllString(s, "")
	s = reScreenCopyright.ReplaceAllString(s, "")
	s = reScreenYear.ReplaceAllString(s, "")
	return strings.Join(strings.Fields(s), " ")
}

// --- сравнение ---

// screenRowDraft — строка до отбора.
type screenRowDraft struct {
	row     out.GuideScreenRow
	hasData bool
	order   int
	src     *guideFact
}

// screenAxisRank — функции, о которых покупатели спрашивают (словарь условий), — выше внутри приоритета.
func screenAxisRank(axis string) int {
	for i, f := range guideFeatureStems {
		if f.axis == axis {
			return i
		}
	}
	return len(guideFeatureStems)
}

// screenComparison — 2–3 колонки, 4–6 строк по приоритетам UI 6.3, остальное — в раскрытие.
func (t *guideTurn) screenComparison(res *out.GuideResponse, matches []*out.GuideOffer, result *out.GuideResult) *out.GuideScreenTable {
	var cols []*out.GuideOffer
	seen := map[string]bool{}
	add := func(o *out.GuideOffer) {
		if len(cols) < screenMaxColumns && !seen[o.FamilyID] {
			seen[o.FamilyID] = true
			cols = append(cols, o)
		}
	}
	if res.Comparison != nil && len(res.Comparison.Columns) >= 2 {
		for _, c := range res.Comparison.Columns {
			for _, o := range matches {
				if o.FamilyID == c.FamilyID {
					add(o)
				}
			}
		}
	} else {
		for _, o := range matches {
			add(o)
		}
		// Редакция той же линейки, исключённая по функции, — колонкой рядом («Различия редакций»).
		for i := range res.Offers {
			o := &res.Offers[i]
			if o.Role == out.OfferAlternative && len(matches) > 0 && t.sameLine(o.FamilyID, matches[0].FamilyID) &&
				o.Reason != nil && (o.Reason.Code == "missing_feature" || o.Reason.Code == "feature_unknown") {
				add(o)
			}
		}
		// Одна линейка — дешевле левее (Стандартный, затем Расширенный).
		if t.oneVendor(cols) {
			sort.SliceStable(cols, func(i, j int) bool { return unitPriceOf(cols[i]) < unitPriceOf(cols[j]) })
		}
	}
	if len(cols) < 2 {
		return nil
	}
	r := t.st.Requirements
	tbl := &out.GuideScreenTable{Title: "Различия редакций", Rows: []out.GuideScreenRow{}, MoreRows: []out.GuideScreenRow{}}
	oneVendor := t.oneVendor(cols)
	if !oneVendor {
		tbl.Title, tbl.VendorsNote = "Сравнение вариантов", screenVendorsNote
	}
	fids := make([]string, len(cols))
	for i, o := range cols {
		fids[i] = o.FamilyID
	}
	shorts := t.cat.shortTitles(fids)
	for _, o := range cols {
		title := o.Display.Title
		if oneVendor {
			title = t.editionName(o.FamilyID)
			if title == o.Display.Title && shorts[o.FamilyID] != "" {
				title = suggestDisplay(shorts[o.FamilyID])
			}
		}
		tbl.Columns = append(tbl.Columns, out.GuideScreenCol{OfferID: o.OfferID, FamilyID: o.FamilyID, Title: title})
	}
	asked := map[string]bool{}
	for _, a := range r.RequiredFeatures {
		asked[a] = true
	}
	for _, f := range guideFeatureStems {
		if strings.Contains(guideNorm(t.query+" "+t.text), f.stem) {
			asked[f.axis] = true
		}
	}
	// Оси в порядке базы знаний: сначала первой колонки, затем остальных.
	var axes []string
	axisSeen := map[string]bool{}
	for _, fid := range fids {
		for _, f := range t.kb.byFamily[fid] {
			if !axisSeen[f.Axis] && f.applies(r.Platform) {
				axisSeen[f.Axis] = true
				axes = append(axes, f.Axis)
			}
		}
	}
	for a := range asked {
		if !axisSeen[a] {
			axisSeen[a] = true
			axes = append(axes, a)
		}
	}
	var drafts []screenRowDraft
	for i, axis := range axes {
		row := out.GuideScreenRow{Key: axis, Kind: "feature", Asked: asked[axis]}
		marks := map[string]int{}
		texts := map[string]bool{}
		data := false
		var src *guideFact
		for _, fid := range fids {
			f := t.kb.factOn(fid, axis, r.Variant)
			if row.Label == "" && f != nil {
				row.Label = factLabel(f, axis)
			}
			cell := screenCell(f)
			if cell.Mark != out.MarkUnknown {
				data = true
				if src == nil {
					src = f
				}
			}
			marks[cell.Mark]++
			texts[cell.Mark+"|"+cell.Text] = true
			row.Cells = append(row.Cells, cell)
		}
		if row.Label == "" {
			row.Label = axisLabel(axis)
		}
		same := len(texts) == 1
		switch {
		case row.Asked:
			row.Priority = 0
		case marks[out.MarkYes] > 0 && marks[out.MarkNo] > 0:
			row.Priority = 1
		case !same && (marks[out.MarkValue] > 0 || marks[out.MarkConditional] > 0) && marks[out.MarkUnknown] == 0:
			row.Priority = 2
		case same && marks[out.MarkYes] == len(fids):
			row.Priority = 4
		default:
			row.Priority = 5
		}
		drafts = append(drafts, screenRowDraft{row: row, hasData: data, order: screenAxisRank(axis)*1000 + i, src: src})
	}
	if pr := t.unitPriceRow(cols); pr != nil {
		drafts = append(drafts, screenRowDraft{row: *pr, hasData: true, order: 1 << 30})
	}
	sort.SliceStable(drafts, func(i, j int) bool {
		if drafts[i].row.Priority != drafts[j].row.Priority {
			return drafts[i].row.Priority < drafts[j].row.Priority
		}
		return drafts[i].order < drafts[j].order
	})
	features := 0
	for _, d := range drafts {
		switch {
		case !d.hasData && !d.row.Asked:
			tbl.HiddenNoData++
		case d.row.Priority <= 4 && len(tbl.Rows) < screenMaxRows:
			tbl.Rows = append(tbl.Rows, d.row)
			if d.row.Kind == "feature" {
				features++
			}
			if tbl.Source == nil && d.src != nil && d.row.Priority <= 1 {
				tbl.Source = factScreenSource(d.src)
			}
		default:
			tbl.MoreRows = append(tbl.MoreRows, d.row)
		}
	}
	if features == 0 {
		return nil // одни цены — это не сравнение функций: стоимость задачи — в task_cost
	}
	if tbl.HiddenNoData > 0 {
		tbl.HiddenLabel = fmt.Sprintf("Ещё %d %s без сведений у вендора", tbl.HiddenNoData,
			guidePlural(tbl.HiddenNoData, "характеристика", "характеристики", "характеристик"))
	}
	if result != nil && result.Basis == out.BasisLowestTotal {
		tbl.Difference = result.Reason
	}
	return tbl
}

func (t *guideTurn) oneVendor(cols []*out.GuideOffer) bool {
	v := map[string]bool{}
	for _, o := range cols {
		v[t.cat.famVend[o.FamilyID]] = true
	}
	return len(v) <= 1
}

func unitPriceOf(o *out.GuideOffer) float64 {
	if o.UnitPrice == nil {
		return 1e18
	}
	return *o.UnitPrice
}

// screenCell — ячейка из факта: «Есть», «Нет», «Нет сведений», «С условиями» или значение.
func screenCell(f *guideFact) out.GuideScreenCell {
	if !f.verified() {
		return out.GuideScreenCell{Mark: out.MarkUnknown, Text: screenCellText[out.MarkUnknown]}
	}
	switch kind, val := f.factValue(); kind {
	case "yes":
		return out.GuideScreenCell{Mark: out.MarkYes, Text: screenCellText[out.MarkYes], FactID: f.ID}
	case "no":
		return out.GuideScreenCell{Mark: out.MarkNo, Text: screenCellText[out.MarkNo], FactID: f.ID}
	case "depends":
		return out.GuideScreenCell{Mark: out.MarkConditional, Text: screenCellText[out.MarkConditional], FactID: f.ID}
	case "value":
		return out.GuideScreenCell{Mark: out.MarkValue, Text: val, FactID: f.ID}
	}
	return out.GuideScreenCell{Mark: out.MarkUnknown, Text: screenCellText[out.MarkUnknown]}
}

// unitPriceRow — «За компьютер»: цена единицы при одинаковых условиях (единица, срок и ступень совпадают).
func (t *guideTurn) unitPriceRow(cols []*out.GuideOffer) *out.GuideScreenRow {
	unit, tier, term := "", "", ""
	for i, o := range cols {
		if o.UnitPrice == nil {
			return nil
		}
		if i == 0 {
			unit, tier, term = o.SaleUnit, o.Tier, o.Term
			continue
		}
		if o.SaleUnit != unit || o.Tier != tier || o.Term != term {
			return nil
		}
	}
	row := &out.GuideScreenRow{Key: "unit_price", Label: guideUpperFirst(saleUnitPhrase(unit)), Priority: 3, Kind: "commercial"}
	for _, o := range cols {
		row.Cells = append(row.Cells, out.GuideScreenCell{Mark: out.MarkValue, Text: rubRU(*o.UnitPrice) + " ₽"})
	}
	return row
}

// --- полная стоимость задачи ---

// screenTaskCost — несколько вендоров: стоимость всей задачи у каждого (цена пакета не делится на места).
func (t *guideTurn) screenTaskCost(matches []*out.GuideOffer) *out.GuideTaskCost {
	if t.oneVendor(matches) || len(matches) < 2 {
		return nil
	}
	kind, n := mainObject(t.st.Requirements)
	kind = orDefault(kind, "workstation")
	n = max(n, 1)
	terms := map[string]bool{}
	for _, o := range matches {
		terms[o.Term] = true
	}
	what := "Стоимость закупки на " + objectCountText(kind, n)
	if directionOf(t.st.Requirements.Task) == "endpoint" {
		what = "Стоимость защиты " + fmt.Sprintf("%d %s", n, objectNounGen(kind, n))
	}
	if len(terms) == 1 {
		for term := range terms {
			if term != "бессрочная" {
				what += " на " + term
			}
		}
	}
	tc := &out.GuideTaskCost{Title: what, Columns: []string{"Как продаётся", "За " + objectCountText(kind, n)},
		Rows: []out.GuideTaskCostRow{}, Note: screenVendorsNote}
	var best *out.GuideOffer
	tie := false
	for _, o := range matches {
		row := out.GuideTaskCostRow{OfferID: o.OfferID, Title: o.Display.Title, HowSold: o.SaleUnit, Total: o.Total}
		if o.Total != nil {
			row.TotalText = rubRU(*o.Total) + " ₽"
			switch {
			case best == nil || *o.Total < *best.Total:
				best, tie = o, false
			case *o.Total == *best.Total:
				tie = true
			}
		} else {
			row.TotalText = screenOnRequest
		}
		if o.Package != nil {
			row.Note = o.Package.Text
		}
		tc.Rows = append(tc.Rows, row)
	}
	if best != nil && !tie {
		tc.Cheapest = "Меньшая стоимость закупки: " + best.Display.Title + "."
	}
	return tc
}

// objectNounGen — «защиты 20 компьютеров» (родительный падеж после «защиты»).
func objectNounGen(kind string, n int) string {
	switch kind {
	case "server":
		return guidePlural(n, "сервера", "серверов", "серверов")
	case "workstation", "":
		return guidePlural(n, "компьютера", "компьютеров", "компьютеров")
	}
	return guidePlural(n, "места", "мест", "мест")
}

// --- итог ---

// screenSummary — итоговая строка выбранной позиции; без выбора — подсказка выбрать вариант.
func screenSummary(matches []*out.GuideOffer, selected string) *out.GuideSummary {
	if selected == "" {
		return &out.GuideSummary{Label: screenSummaryLabel, Action: "choose", Hint: screenChooseHint}
	}
	for _, o := range matches {
		if o.OfferID != selected {
			continue
		}
		s := &out.GuideSummary{OfferID: o.OfferID, Label: screenSummaryLabel}
		if o.Total != nil {
			v := *o.Total
			s.Total, s.TotalText, s.Formula, s.Action = &v, rubRU(v)+" ₽", o.PriceLine, "add_to_spec"
			return s
		}
		s.TotalText, s.Action, s.Hint = screenOnRequest, "request_quote", "Запросить КП"
		return s
	}
	return nil
}

// --- что изменилось ---

// screenChanged — «Изменено: 30 → 50 компьютеров, шифрование обязательно»: только на ходе со сменой условий.
func (t *guideTurn) screenChanged(res *out.GuideResponse) *out.GuideChanged {
	if t.before.turn == 0 {
		return nil
	}
	a, b := t.before.req, t.st.Requirements
	var items []string
	for _, ob := range b.Objects {
		was := objectCount(a, ob.Kind)
		if was != ob.Count {
			if was > 0 {
				items = append(items, fmt.Sprintf("%d → %s", was, objectCountText(ob.Kind, ob.Count)))
			} else {
				items = append(items, objectCountText(ob.Kind, ob.Count))
			}
		}
	}
	for _, f := range b.RequiredFeatures {
		if !containsStr(a.RequiredFeatures, f) {
			items = append(items, featureNoun(f, t.kb)+" обязательно")
		}
	}
	for _, f := range a.RequiredFeatures {
		if !containsStr(b.RequiredFeatures, f) {
			items = append(items, featureNoun(f, t.kb)+" не обязательно")
		}
	}
	if a.Operation != b.Operation && b.Operation != "" {
		items = append(items, operationLabel(b.Operation))
	}
	if a.TermMonths != b.TermMonths && b.TermMonths > 0 && t.st.Origin["term_months"] == originUser {
		items = append(items, "срок "+termWords(b.TermMonths))
	}
	if a.Vendor != b.Vendor && b.Vendor != "" && t.st.Origin["vendor"] == originUser {
		items = append(items, vendorLabel(b.Vendor))
	}
	if a.OrgType != b.OrgType && b.OrgType != "" {
		items = append(items, orgLabel(b.OrgType))
	}
	if a.Use != b.Use && b.Use != "" {
		items = append(items, useLabel(b.Use))
	}
	if len(items) == 0 {
		return nil
	}
	return &out.GuideChanged{Text: "Изменено: " + strings.Join(items, ", "), Items: items}
}

// featureNoun — «шифрование» для строки изменений.
func featureNoun(axis string, kb *guideKB) string {
	if s, ok := featureShort[axis]; ok {
		return s
	}
	for _, fs := range kb.byFamily {
		for _, f := range fs {
			if f.Axis == axis {
				return strings.ToLower(factLabel(f, axis))
			}
		}
	}
	return strings.ToLower(axisLabel(axis))
}

// screenPriceChange — у выбранного семейства сменилась ступень: прежняя и новая цена за единицу.
func (t *guideTurn) screenPriceChange(res *out.GuideResponse, selected string) *out.GuidePriceChange {
	if selected == "" {
		return nil
	}
	var o *out.GuideOffer
	for i := range res.Offers {
		if res.Offers[i].OfferID == selected {
			o = &res.Offers[i]
		}
	}
	if o == nil || o.UnitPrice == nil {
		return nil
	}
	prevID, ok := t.prevOffers[o.FamilyID]
	if !ok || prevID == o.OfferID {
		return nil
	}
	prev := t.cat.products[prevID]
	if prev == nil || prev.PriceRub == nil {
		return nil
	}
	pv := t.cat.view(prev, t.kb)
	nv := t.cat.view(t.cat.products[o.OfferID], t.kb)
	if pv.tierLabel() == "" || nv.tierLabel() == "" || pv.tierLabel() == nv.tierLabel() {
		return nil
	}
	kind := t.screenKind()
	return &out.GuidePriceChange{OfferID: o.OfferID, Label: "Цена " + saleUnitPhrase(o.SaleUnit) + " изменилась",
		From: out.GuidePriceTier{Tier: pv.tierLabel() + " " + objectNoun(kind, 5), UnitPrice: *prev.PriceRub, Text: rubRU(*prev.PriceRub) + " ₽"},
		To:   out.GuidePriceTier{Tier: nv.tierLabel() + " " + objectNoun(kind, 5), UnitPrice: *o.UnitPrice, Text: rubRU(*o.UnitPrice) + " ₽"}}
}

// guideUpperFirst — первая буква заглавная.
func guideUpperFirst(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// tlsAlternative — подбор для сервера (не TLS): лицензия для TLS-сервера того же вендора — в «При изменении условий»
// с причиной «Отдельная лицензия для TLS-сервера» (сценарий 3: четвёртый товар из каталога).
func (t *guideTurn) tlsAlternative(res *out.GuideResponse, data *guideData) {
	r := t.st.Requirements
	if kind, _ := mainObject(r); kind != "server" || r.Use == "tls_server" || r.Operation == "renewal" {
		return
	}
	var top *out.GuideOffer
	for i := range res.Offers {
		if res.Offers[i].Role == out.OfferMatch {
			top = &res.Offers[i]
			break
		}
	}
	if top == nil {
		return
	}
	vendor := t.cat.famVend[top.FamilyID]
	var best *guideOfferView
	for _, f := range t.cat.withTLSFamilies(nil, vendor) {
		for _, p := range t.cat.byFamily[f] {
			v := t.cat.view(p, t.kb)
			if v.scope != "tls_server" || !v.priced() || v.kind != domain.LicenseKindBase {
				continue
			}
			if best == nil || *v.p.PriceRub < *best.p.PriceRub || (*v.p.PriceRub == *best.p.PriceRub && v.p.ID < best.p.ID) {
				vv := v
				best = &vv
			}
		}
	}
	if best == nil {
		return
	}
	for _, o := range res.Offers {
		if o.OfferID == best.p.ID {
			return
		}
	}
	if t.altFams == nil {
		t.altFams = map[string]bool{}
	}
	t.altFams[best.p.FamilyID] = true
	res.Offers = append(res.Offers, t.offerOut(*best, data.qty, data.kind, out.OfferAlternative, "Отдельная лицензия для TLS-сервера"))
}
