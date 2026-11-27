package services

import (
	"context"
	"strings"

	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
)

// update_spec (SPEC §4): черновик спецификации подбора. Версия черновика растёт с каждым изменением; ключ
// операции делает повтор безопасным: тот же ключ — тот же ответ, строка не дублируется. Цена перечитывается из
// каталога при каждом добавлении. Черновик — то, что фронт кладёт в существующую спецификацию покупателя;
// запрос КП — существующий POST /api/quotes (origin guide).

const (
	guideSpecMaxLines = 20
	guideOpKeyMax     = 64
	guideOpKeysKept   = 64
)

// Тексты.
const (
	MsgGuideSpecConflict = "Черновик спецификации изменился — обновите подбор"
	MsgGuideNoSelection  = "Подбор не найден или истёк — начните подбор заново"
)

// guideSpecDraft — черновик спецификации подбора.
type guideSpecDraft struct {
	version int
	lines   []guideSpecLine
	ops     map[string]*out.GuideSpec // ключ операции → ответ
	opOrder []string
}

type guideSpecLine struct {
	offerID  string
	quantity int
}

// UpdateGuideSpec — POST /api/guide/spec.
func (s *Services) UpdateGuideSpec(_ context.Context, req in.GuideSpecRequest) (*out.GuideSpec, error) {
	req.OpKey = strings.TrimSpace(req.OpKey)
	var bad []errs.FieldError
	if req.SelectionID == "" || len(req.SelectionID) > maxIDLen {
		bad = append(bad, errs.FieldError{Field: "selection_id", Code: errs.CodeRequired, Message: "Нужен идентификатор подбора"})
	}
	if req.OfferID == "" || len(req.OfferID) > maxIDLen {
		bad = append(bad, errs.FieldError{Field: "offer_id", Code: errs.CodeRequired, Message: "Нужен оффер"})
	}
	if req.Quantity < 0 || req.Quantity > maxQuoteQuantity {
		bad = append(bad, errs.FieldError{Field: "quantity", Code: errs.CodeOutOfRange, Message: "Количество от 0 до 100000 (0 — убрать строку)"})
	}
	if req.OpKey == "" || len(req.OpKey) > guideOpKeyMax {
		bad = append(bad, errs.FieldError{Field: "op_key", Code: errs.CodeRequired, Message: "Нужен ключ операции"})
	}
	if len(bad) > 0 {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(bad...)
	}
	g := s.guide()
	cat := g.catalog()
	if cat == nil {
		return nil, errs.NewUnavailableError("Каталог не загружен: цены проверить нельзя")
	}
	p := cat.products[req.OfferID]
	if p == nil {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(errs.FieldError{Field: "offer_id",
			Code: errs.CodeInvalidFormat, Message: "Позиции нет в каталоге"})
	}
	mu := g.sessions.turnLock(req.SelectionID)
	mu.Lock()
	defer mu.Unlock()
	sess, ok := g.sessions.get(req.SelectionID)
	if !ok {
		return nil, errs.NewNotFoundError(MsgGuideNoSelection)
	}
	d := &sess.spec
	if prev, ok := d.ops[req.OpKey]; ok {
		dup := *prev
		dup.Duplicate = true
		return &dup, nil
	}
	if req.Version != d.version {
		return nil, errs.NewConflictError(MsgGuideSpecConflict)
	}
	qty := req.Quantity
	idx := -1
	for i, l := range d.lines {
		if l.offerID == req.OfferID {
			idx = i
		}
	}
	switch {
	case qty == 0 && idx >= 0:
		d.lines = append(d.lines[:idx], d.lines[idx+1:]...)
	case qty == 0:
	case idx >= 0:
		d.lines[idx].quantity = qty // та же позиция — количество, не вторая строка
	case len(d.lines) >= guideSpecMaxLines:
		return nil, errs.NewValidationError("В черновике не больше 20 строк")
	default:
		d.lines = append(d.lines, guideSpecLine{offerID: req.OfferID, quantity: qty})
	}
	d.version++
	res := g.specView(cat, d)
	if d.ops == nil {
		d.ops = map[string]*out.GuideSpec{}
	}
	d.ops[req.OpKey] = res
	d.opOrder = append(d.opOrder, req.OpKey)
	if len(d.opOrder) > guideOpKeysKept {
		delete(d.ops, d.opOrder[0])
		d.opOrder = d.opOrder[1:]
	}
	g.sessions.put(sess)
	cp := *res
	return &cp, nil
}

// specView — черновик с ценами из каталога сейчас.
func (g *guideService) specView(cat *guideCatalog, d *guideSpecDraft) *out.GuideSpec {
	kb := g.knowledge()
	res := &out.GuideSpec{Version: d.version, Lines: []out.GuideSpecLine{}, Unresolved: []string{}}
	total := 0.0
	for _, l := range d.lines {
		p := cat.products[l.offerID]
		if p == nil {
			res.Unresolved = append(res.Unresolved, "Позиции "+l.offerID+" больше нет в каталоге")
			res.OnRequest++
			continue
		}
		v := cat.view(p, kb)
		line := out.GuideSpecLine{OfferID: p.ID, FamilyID: p.FamilyID, Title: p.Title, SKU: p.SKU, Tier: v.tierLabel(),
			Quantity: l.quantity, PriceState: out.PriceOnRequest, OfferURL: p.OfferURL}
		if v.priced() {
			price := *p.PriceRub
			sum := guideRound2(price * float64(v.units(l.quantity)))
			line.PricePerUnit, line.Total, line.PriceState = &price, &sum, out.PricePriced
			if v.unit == "pack" && v.pack > 0 {
				pkg, seat := price, guideRound2(price/float64(v.pack))
				line.PackagePrice, line.PackageSize, line.PricePerUnit = &pkg, v.pack, &seat
			}
			total += sum
		} else {
			res.OnRequest++
		}
		if !v.inTier(l.quantity) {
			res.Unresolved = append(res.Unresolved, "Количество "+itoa(l.quantity)+" не попадает в ступень "+v.tierLabel()+": "+p.Title)
		}
		res.Lines = append(res.Lines, line)
	}
	if res.OnRequest == 0 {
		t := guideRound2(total)
		res.TotalRub = &t
	}
	return res
}
