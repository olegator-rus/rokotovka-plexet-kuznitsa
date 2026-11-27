package services

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
)

// Комплект для офиса (сценарии 1 и 8): консультант ведёт по частям — офисный пакет, защита, резервное
// копирование. Сценарий 1 — по шагам с добавлением в спецификацию; сценарий 8 («открываем офис») — со сметой, где
// непосчитанное честно помечено. Позиции и цены — код (подбор контура подбора или семейства из настройки), текст — модель.

// Части комплекта.
const (
	kitOffice    = "office"
	kitAntivirus = "antivirus"
	kitBackup    = "backup"
)

// kitPart — часть комплекта: подпись для сметы и запрос подбора.
type kitPart struct {
	key, label, query string
}

var kitParts = []kitPart{
	{kitOffice, "Офисный пакет", "офисный пакет"},
	{kitAntivirus, "Защита компьютеров", "антивирус"},
	{kitBackup, "Резервное копирование", "резервное копирование"},
}

func kitPartOf(key string) kitPart {
	for _, p := range kitParts {
		if p.key == key {
			return p
		}
	}
	return kitPart{}
}

// kitOfficeFamilies — настройка: семейства офисных решений для комплекта в порядке показа (каталог магазина):
// Microsoft Office для дома и бизнеса 2024 (бессрочная), Яндекс 360 для бизнеса «Основной», VK WorkSpace
// «Стартовый», «Р7-Офис» для юридических лиц. Семейства нет в каталоге — строка пропускается.
var kitOfficeFamilies = []string{"391338", "422091", "445423", "222227"}

// consultantKit — состояние комплекта в разговоре.
type consultantKit struct {
	estimate bool
	count    int
	asked    bool
	owned    map[string]bool
	chosen   map[string]consultantItem // часть → позиция для сметы (рекомендованная или добавленная)
	current  string
	pending  string // часть, о которой консультант спросил последней
}

var (
	kitStepRe     = regexp.MustCompile(`(^|[^\p{L}])(по|софт|программы|программное обеспечение)\s+для\s+офиса|для\s+офиса\s+(по|софт|программ)|(подобрать|подбери|собрать|собери)\s+(все\s+)?для\s+офиса`)
	kitEstimateRe = regexp.MustCompile(`открыва\p{L}*\s+(новый\s+)?офис|новый\s+офис|комплект\s+(для|на)\s+офис`)
	kitCountRe    = regexp.MustCompile(`(\d{1,5})`)
	kitNoneRe     = regexp.MustCompile(`ничего|с нуля|нет ничего|покупаем вс|всё покупаем|все покупаем|впервые`)
	kitOfficeRe   = regexp.MustCompile(`офисн|office|офис\b|word|excel|ворд|эксел|microsoft|майкрософт|р7|яндекс 360|workspace|привычн|бюджет|сравнить вариант|альтернатив`)
	kitAVRe       = regexp.MustCompile(`антивирус|защит|рабочие места|рабочих мест|сервер`)
	kitBackupRe   = regexp.MustCompile(`резервн|бэкап|backup|хранит|облак`)
)

// kitTurn — ход комплекта: начало («ПО для офиса», «открываем офис») или ответ на вопрос комплекта.
func (t *consultantTurn) kitTurn(ctx context.Context) (consultantPlan, bool) {
	text := guideNorm(t.req.Text)
	start := kitStepRe.MatchString(text) || kitEstimateRe.MatchString(text)
	k := t.conv.kit
	if start {
		k = &consultantKit{estimate: kitEstimateRe.MatchString(text), owned: map[string]bool{}, chosen: map[string]consultantItem{}}
		t.conv.kit = k
		k.count = kitCount(text)
		k.asked = true
		return t.kitQuestion(ctx, k), true
	}
	if k == nil || !t.kitAnswer(text, k) {
		return consultantPlan{}, false
	}
	if n := kitCount(text); n > 0 {
		k.count = n
	} else if k.count == 0 {
		if m := kitCountRe.FindString(text); m != "" {
			k.count, _ = strconv.Atoi(m)
		}
	}
	if kitNoneRe.MatchString(text) {
		k.owned = map[string]bool{}
	}
	for _, p := range kitParts {
		if strings.Contains(text, "есть") && kitNamesPart(text, p.key) && !kitNoneRe.MatchString(text) {
			k.owned[p.key] = true
		}
	}
	if k.count <= 0 {
		return t.kitQuestion(ctx, k), true
	}
	part := t.kitNextPart(text, k)
	if part == "" {
		return consultantPlan{messages: []string{"Основное для офиса подобрали. Если нужно что-то ещё — напишите, что именно."}}, true
	}
	return t.kitPartTurn(ctx, k, part, text), true
}

// kitCount — «на 20 человек», «30 компьютеров», «30, ничего нет».
func kitCount(text string) int {
	m := regexp.MustCompile(`(\d{1,5})\s*(человек|сотрудник|компьютер|пк|рабоч|мест|пользоват|чел)`).FindStringSubmatch(text)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

func kitNamesPart(text, key string) bool {
	switch key {
	case kitOffice:
		return kitOfficeRe.MatchString(text)
	case kitAntivirus:
		return kitAVRe.MatchString(text)
	case kitBackup:
		return kitBackupRe.MatchString(text)
	}
	return false
}

// kitAnswer — реплика продолжает комплект: количество, «ничего нет», часть комплекта или ответ на вопрос о ней.
// Названный вендор или конкретный продукт вне комплекта — обычный подбор.
func (t *consultantTurn) kitAnswer(text string, k *consultantKit) bool {
	if len(guideVendorsIn(text)) > 0 && !strings.Contains(text, "microsoft") && !strings.Contains(text, "майкрософт") {
		return false
	}
	if kitNoneRe.MatchString(text) || (k.count == 0 && kitCountRe.MatchString(text)) {
		return true
	}
	for _, p := range kitParts {
		if kitNamesPart(text, p.key) {
			return true
		}
	}
	return k.pending != "" && len([]rune(text)) <= 40
}

// kitNextPart — какую часть подбирать: названная в реплике, затем та, о которой спрашивали, затем по порядку
// (в смете — сначала защита: офисный пакет требует уточнения).
func (t *consultantTurn) kitNextPart(text string, k *consultantKit) string {
	for _, p := range kitParts {
		if kitNamesPart(text, p.key) && !k.owned[p.key] {
			return p.key
		}
	}
	if k.pending != "" && !k.owned[k.pending] {
		return k.pending
	}
	order := []string{kitOffice, kitAntivirus, kitBackup}
	if k.estimate {
		order = []string{kitAntivirus, kitOffice, kitBackup}
	}
	for _, p := range order {
		if _, done := k.chosen[p]; !done && !k.owned[p] {
			return p
		}
	}
	return ""
}

// kitQuestion — первый вопрос комплекта: сколько мест и что уже есть.
func (t *consultantTurn) kitQuestion(ctx context.Context, k *consultantKit) consultantPlan {
	sh := &consultantSheet{Mode: "kit_question", Kind: "workstation", Qty: k.count}
	hint := "Скажи, что для офиса обычно нужны офисный пакет, антивирус и резервное копирование, и спроси, сколько рабочих мест и что из этого уже есть."
	if k.estimate {
		hint = "Скажи, что для нового офиса начал бы с трёх вещей: офисного пакета, защиты компьютеров и резервного копирования. Спроси, всё ли покупают с нуля или какие-то лицензии уже есть."
		if k.count == 0 {
			hint += " Количество рабочих мест неизвестно — спроси и его."
		}
	} else if k.count > 0 {
		hint = "Скажи, что для офиса обычно нужны офисный пакет, антивирус и резервное копирование, и спроси, что из этого уже есть."
	}
	sh.Question = hint
	if k.count > 0 {
		sh.Conditions = fmt.Sprintf("%d %s", k.count, objectNoun("workstation", k.count))
	}
	p := t.write(ctx, sh)
	if p.errReason != "" {
		// Вопрос без модели — тот же смысл кодом (чисел с ценами нет).
		msg := "Давайте соберём. Обычно для офиса нужны офисный пакет, антивирус и резервное копирование. Сколько у вас рабочих мест и что из этого уже есть?"
		if k.estimate {
			msg = "Для нового офиса я бы начал с трёх вещей: офисного пакета, защиты компьютеров и резервного копирования. Всё покупаете с нуля или какие-то лицензии уже есть?"
		}
		return consultantPlan{messages: []string{msg}}
	}
	p.attachments, p.quick = nil, nil // ответ на вопрос комплекта — свободным текстом
	return p
}

// kitPartTurn — позиции части комплекта и, в смете, смета.
func (t *consultantTurn) kitPartTurn(ctx context.Context, k *consultantKit, part, text string) consultantPlan {
	t.status(consultantStepSearching)
	k.current, k.pending = part, ""
	var sh *consultantSheet
	if part == kitOffice {
		sh = t.kitOfficeSheet(k.count, text)
	} else {
		q := fmt.Sprintf("%s для %d %s", kitPartOf(part).query, k.count, objectNoun("workstation", k.count))
		if part == kitAntivirus && strings.Contains(text, "сервер") && !strings.Contains(text, "только") {
			q += " и серверов"
		}
		res, err := t.s.Guide(ctx, t.requestID, in.GuideRequest{SessionID: t.req.SessionID, Message: q})
		if err != nil || res == nil {
			return t.fallback(consultantErrTurn, nil)
		}
		st := res.State
		t.conv.guide = &st
		sh = t.sheetFromGuide(res)
		sh.Assumption = ""
		if sh.Comparison != nil {
			sh.Comparison, sh.Mode = nil, "selection"
		}
	}
	t.status(consultantStepCalculating)
	sh.Mode = "kit"
	sh.ForceList = true
	sh.ListTitle = kitListTitle(part, k.count)
	if sh.Qty <= 0 {
		sh.Qty, sh.Kind = k.count, "workstation"
	}
	var owned []string
	for _, p := range kitParts {
		if k.owned[p.key] {
			owned = append(owned, strings.ToLower(p.label))
		}
	}
	ctxFact := "Комплект для офиса: офисный пакет, защита компьютеров, резервное копирование. Сейчас подбираем: " + strings.ToLower(kitPartOf(part).label) + "."
	if len(owned) > 0 {
		ctxFact += " Уже есть: " + strings.Join(owned, ", ") + "."
	}
	sh.Facts = append([]consultantFact{{Text: ctxFact}}, sh.Facts...)
	if len(sh.Items) > 0 {
		k.chosen[part] = sh.Items[0]
	}
	switch {
	case part == kitOffice && !k.estimate:
		sh.Question = "Начни с офисного пакета: назови первую позицию и её цену за компьютер и на всё количество (бессрочная — «можно купить один раз»). Во втором сообщении скажи, что есть варианты по подписке, и спроси, что важнее: привычные Word и Excel или возможность сократить расходы."
		sh.Chips = []string{"Привычный Office", "Важнее бюджет", "Сравнить варианты"}
	case k.estimate:
		next := t.kitNextPart("", k)
		switch next {
		case kitOffice:
			sh.Question = "Порекомендуй позицию 1 с основанием из mark (меньшая стоимость среди найденных) и её годовую стоимость на всё количество. Во втором сообщении спроси, где будут храниться рабочие документы и нужен ли Microsoft Office или подойдёт другой офисный пакет."
			sh.Chips = []string{"Нужен Microsoft Office", "Рассмотрим альтернативы"}
			k.pending = kitOffice
		case kitBackup:
			sh.Question = "Назови позицию 1 и её стоимость на всё количество. Во втором сообщении спроси, где будут храниться рабочие документы: на компьютерах, на сервере или в облаке."
			k.pending = kitBackup
		default:
			sh.Question = "Назови позицию 1 и её стоимость на всё количество."
		}
	}
	p := t.write(ctx, sh)
	t.conv.sheet = sh.Items
	if k.estimate && p.errReason == "" {
		p.attachments = append(p.attachments, t.kitEstimate(k))
	}
	return p
}

func kitListTitle(part string, n int) string {
	switch part {
	case kitOffice:
		return fmt.Sprintf("Офисные решения на %d %s", n, guidePlural(n, "место", "места", "мест"))
	case kitAntivirus:
		return fmt.Sprintf("Защита на %d %s", n, objectNoun("workstation", n))
	}
	return fmt.Sprintf("Резервное копирование на %d %s", n, objectNoun("workstation", n))
}

// kitOfficeSheet — офисные решения из настройки: по семейству — позиция для новой покупки под количество.
// «Привычный Office» — Microsoft первым; «бюджет» — по возрастанию стоимости.
func (t *consultantTurn) kitOfficeSheet(n int, text string) *consultantSheet {
	sh := &consultantSheet{Mode: "kit", Kind: "workstation", Qty: n}
	var items []consultantItem
	for _, fid := range kitOfficeFamilies {
		p := t.familyOffer(fid, n)
		if p == nil {
			continue
		}
		it := t.offerItem(p, n, consultantFit)
		if p.PriceRub == nil || *p.PriceRub <= 0 {
			it.Total, it.Unit, it.PriceState = nil, nil, out.PriceOnRequest
		}
		items = append(items, it)
	}
	if strings.Contains(text, "бюджет") || strings.Contains(text, "дешевл") || strings.Contains(text, "сократить") {
		sortItemsByTotal(items)
		if len(items) > 0 {
			items[0].Mark = "Меньшая стоимость среди найденных"
		}
	}
	for i := range items {
		items[i].N = i + 1
	}
	sh.Items = items
	return sh
}

func sortItemsByTotal(items []consultantItem) {
	key := func(it consultantItem) float64 {
		if it.Total == nil {
			return 1e18
		}
		return *it.Total
	}
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && key(items[j]) < key(items[j-1]); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}

// familyOffer — позиция семейства для новой покупки на n мест: сначала годовая, затем бессрочная; с ценой и в
// ступени — раньше; затем дешевле.
func (t *consultantTurn) familyOffer(fid string, n int) *domain.Product {
	var best *domain.Product
	var bestKey []float64
	for _, p := range t.cat.byFamily[fid] {
		if !consultantNewPurchase(p) || p.Kind == "hardware" {
			continue
		}
		if o := p.License.OrgType; o != "" && o != "commercial" {
			continue
		}
		v := t.cat.view(p, t.kb)
		termRank := 2.0
		switch {
		case p.License.DurationMonths != nil && *p.License.DurationMonths == 12:
			termRank = 0
		case p.License.DurationMonths == nil:
			termRank = 1
		}
		priced := v.priced()
		if priced && !v.inTier(n) {
			continue
		}
		price := 0.0
		if priced {
			price = *p.PriceRub
		}
		key := []float64{b2f(!priced), termRank, price}
		if best == nil || lessKey(key, bestKey) {
			best, bestKey = p, key
		}
	}
	return best
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// kitEstimate — смета: посчитанные части с суммой, остальные — «не рассчитано»; полной суммы нет, пока есть
// непосчитанные.
func (t *consultantTurn) kitEstimate(k *consultantKit) out.ConsultantEstimate {
	e := out.ConsultantEstimate{Type: "estimate", Lines: []out.ConsultantEstimateLine{}}
	total := 0.0
	missing := false
	for _, p := range kitParts {
		if k.owned[p.key] {
			continue
		}
		it, ok := k.chosen[p.key]
		if !ok || it.Total == nil {
			label := p.label
			if ok {
				label = it.Title
			}
			e.Lines = append(e.Lines, out.ConsultantEstimateLine{Label: label, Status: "not_calculated"})
			missing = true
			continue
		}
		total += *it.Total
		e.Lines = append(e.Lines, out.ConsultantEstimateLine{Label: it.Title, TotalPriceMinor: minor(it.Total), Status: "known"})
	}
	if m := minor(&total); m != nil {
		e.KnownTotalMinor = *m
	}
	if missing {
		e.Note = "Стоимость рассчитанных позиций " + rubRU(total) + " ₽. Полная стоимость пока неизвестна"
	} else {
		e.Note = "Стоимость комплекта " + rubRU(total) + " ₽"
	}
	return e
}

// kitAfterAdd — после «добавь» в комплекте по шагам: отметить часть и один раз предложить следующую.
func (t *consultantTurn) kitAfterAdd(p *consultantPlan, it consultantItem) {
	k := t.conv.kit
	if k == nil || k.current == "" {
		return
	}
	k.chosen[k.current] = it
	next := t.kitNextPart("", k)
	switch next {
	case kitAntivirus:
		p.messages = append(p.messages, "Теперь можно подобрать защиту для этих же компьютеров. Антивирус нужен только на рабочие места или у вас есть серверы?")
		p.quick = []string{"Только рабочие места", "Есть и серверы"}
	case kitBackup:
		p.messages = append(p.messages, "Осталось резервное копирование. Где будут храниться рабочие документы: на компьютерах, на сервере или в облаке?")
		p.quick = []string{"На компьютерах", "На сервере", "В облаке"}
	case kitOffice:
		p.messages = append(p.messages, "Теперь можно подобрать офисный пакет для этих же компьютеров. Нужен Microsoft Office или подойдёт другой офисный пакет?")
		p.quick = []string{"Нужен Microsoft Office", "Рассмотрим альтернативы"}
	}
	k.pending = next
}
