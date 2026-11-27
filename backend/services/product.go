package services

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
)

// Страница товара: GET /api/products/{id} — позиция из каталога в памяти, как у GET /api/companions.
// Индекс поиска не нужен. Маржа и популярность в ответ не попадают никому: витрина их не показывает (как в поиске).

// maxProductAnalogs — аналогов на странице.
const maxProductAnalogs = 5

// maxProductID — длина id позиции в адресе.
const maxProductID = 128

// attrLabels — подписи ключей attributes, которых нет в attribute_definitions категории. Ключ без подписи на
// странице не показывается; тест сверяет словарь с data/mock.
var attrLabels = map[string]string{
	"edition":           "Редакция",
	"platform":          "Платформа",
	"purchase":          "Порядок покупки",
	"version":           "Версия",
	"language":          "Язык",
	"delivery":          "Поставка",
	"apps":              "Приложения",
	"certificate":       "Сертификаты",
	"protection":        "Защита",
	"usage":             "Использование",
	"deal":              "Тип сделки",
	"org_type":          "Тип организации",
	"servers":           "Серверов",
	"cpu":               "Процессор",
	"memory_type":       "Тип памяти",
	"speed_mts":         "Частота памяти, МТ/с",
	"module":            "Тип модуля",
	"drive_type":        "Тип накопителя",
	"interface":         "Интерфейс",
	"drive_form_factor": "Форм-фактор",
	"drive_bays":        "Отсеки для дисков",
	"raid":              "RAID-контроллер",
	"os":                "Операционная система",
	"poe":               "PoE",
	"layer":             "Уровень коммутации",
	"resolution":        "Разрешение",
	"panel":             "Тип матрицы",
	"service_level":     "Уровень сервиса",
	"topology":          "Топология",
	"mount":             "Монтаж",
}

// Product — страница товара по id позиции. Каталог не загружен — 503, позиции нет — 404, id длиннее 128 — 422.
func (s *Services) Product(ctx context.Context, id string) (*out.ProductPage, error) {
	if id == "" || len(id) > maxProductID {
		return nil, errs.NewValidationError(errs.MsgInvalidID).WithDetails(errs.FieldError{Field: "id",
			Code: errs.CodeOutOfRange, Message: "Идентификатор позиции — от 1 до 128 символов"})
	}
	cs := s.catalog.Load()
	cc := s.commerceFor(ctx)
	if cs == nil || cs.lex == nil || cc == nil {
		return nil, errs.NewUnavailableError("Каталог не загружен — сначала импорт (make import)")
	}
	p := cc.products[id]
	d, ok := cs.lex.docs[id]
	if p == nil || !ok {
		return nil, errs.NewNotFoundError("Позиция")
	}
	lx := cs.lex
	kind := docKind(d)
	res := &out.ProductPage{
		CatalogVersion: cs.version, ProductID: p.ID, SKU: p.SKU, MPN: p.MPN, Kind: kind,
		FamilyID: p.FamilyID, FamilyName: p.FamilyName, Title: p.Title,
		Vendor:      out.ProductVendor{ID: p.VendorID, Name: d.Vendor, Origin: p.Origin},
		Category:    out.ProductCategory{ID: p.CategoryID, Name: d.Category},
		Description: p.Description, LicenseLine: withOrgLine(d),
		License: out.ProductLicense{Type: p.License.Type, Metric: p.License.Metric, Quantity: p.License.Quantity,
			DurationMonths: p.License.DurationMonths, Edition: p.License.Edition, Kind: p.License.Kind,
			OrgType: p.License.OrgType, TierMin: p.License.TierMin, TierMax: p.License.TierMax,
			OrgLabel: orgLabelOf(d.LicenseOrgType)},
		PriceRub: p.PriceRub, OldPriceRub: discountedFrom(d), Promos: []out.ItemPromo{},
		PriceOnRequest: p.PriceOnRequest, PriceMode: docPriceMode(d), OfferURL: d.OfferURL, ImageURL: pictureOf(d.ProductID), Origin: p.Origin,
		Attributes: productPageAttrs(cc, p), Specs: productSpecs(cc, p),
		Variants: productVariants(lx, p), Compatibility: productCompat(cc, p),
		Companions: out.ProductPageCompanions{Required: []out.Companion{}, Items: []out.Companion{}},
		Analogs:    productAnalogs(cc, lx, p),
	}
	if kind != domain.KindHardware {
		res.Reestr = s.reestrOf(lx, p.FamilyID)
	}
	// Акции — как на карточке выдачи: только на фиде (у mock адресов магазина нет), по семейству и названию позиции.
	if d.OfferURL != "" {
		if pr := s.upsellIndexFor(lx).promosFor(p.FamilyID, []string{p.Title}, s.now()); len(pr) > 0 {
			res.Promos = pr
		}
	}
	qty := 1
	if p.License.Quantity != nil && *p.License.Quantity > 0 {
		qty = *p.License.Quantity
	}
	comp, err := s.ProductCompanions(ctx, p.ID, qty)
	if err != nil {
		return nil, err
	}
	if comp.Required != nil {
		res.Companions.Required = comp.Required
	}
	if comp.Items != nil {
		res.Companions.Items = comp.Items
	}
	return res, nil
}

// attrSpecs — подписи и единицы ключей категории позиции (attribute_definitions).
func attrSpecs(cc *commerceCatalog, categoryID string) ([]domain.AttributeSpec, map[string]domain.AttributeSpec) {
	defs := cc.attrDefs[categoryID]
	byKey := make(map[string]domain.AttributeSpec, len(defs))
	for _, a := range defs {
		byKey[a.Key] = a
	}
	return defs, byKey
}

// productPageAttrs — attributes позиции с подписями: сначала ключи из attribute_definitions категории (в их порядке),
// затем остальные по словарю attrLabels (по ключу). Ключ без подписи пропускается.
func productPageAttrs(cc *commerceCatalog, p *domain.Product) []out.ProductAttr {
	res := []out.ProductAttr{}
	var raw map[string]any
	if len(p.Attributes) == 0 || json.Unmarshal(p.Attributes, &raw) != nil {
		return res
	}
	defs, byKey := attrSpecs(cc, p.CategoryID)
	seen := map[string]bool{}
	add := func(key, label string) {
		v, ok := raw[key]
		if !ok || seen[key] || label == "" {
			return
		}
		val := attrValue(key, v)
		if val == "" {
			return
		}
		seen[key] = true
		res = append(res, out.ProductAttr{Key: key, Label: label, Value: val})
	}
	for _, a := range defs {
		add(a.Key, a.Label)
	}
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if a, ok := byKey[k]; ok && a.Label != "" {
			continue
		}
		add(k, attrLabels[k])
	}
	return res
}

// attrValue — значение атрибута строкой: строка как есть (тип сделки и организации — подписью покупки), число —
// без лишних нулей, bool — «да» / «нет».
func attrValue(key string, v any) string {
	switch x := v.(type) {
	case string:
		switch key {
		case "deal":
			if l := dealLabels[x]; l != "" {
				return l
			}
		case "org_type":
			if l := orgLabels[x]; l != "" {
				return l
			}
		}
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		if x {
			return "да"
		}
		return "нет"
	case nil:
		return ""
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// productSpecs — числовые характеристики с подписью и единицей из attribute_definitions категории; ключ без
// определения пропускается. Порядок — порядок определений.
func productSpecs(cc *commerceCatalog, p *domain.Product) []out.ProductSpec {
	res := []out.ProductSpec{}
	if len(p.Specs) == 0 {
		return res
	}
	defs, _ := attrSpecs(cc, p.CategoryID)
	for _, a := range defs {
		if v, ok := p.Specs[a.Key]; ok && a.Label != "" {
			res = append(res, out.ProductSpec{Key: a.Key, Label: a.Label, Value: v, Unit: a.Unit})
		}
	}
	return res
}

// productVariants — остальные позиции семейства: по цене (без цены — в конце), затем по названию.
func productVariants(lx *searchLexicon, p *domain.Product) []out.ProductVariant {
	res := []out.ProductVariant{}
	for _, pid := range lx.families[p.FamilyID] {
		if pid == p.ID {
			continue
		}
		d := lx.docs[pid]
		res = append(res, out.ProductVariant{ProductID: pid, Title: d.Title, LicenseLine: withOrgLine(d),
			License: out.ProductLicense{Type: d.LicenseType, Metric: d.LicenseMetric, Quantity: d.LicenseQuantity,
				DurationMonths: d.LicenseDuration, Edition: d.LicenseEdition, Kind: d.LicenseKind,
				OrgType: d.LicenseOrgType, TierMin: d.LicenseTierMin, TierMax: d.LicenseTierMax,
				OrgLabel: orgLabelOf(d.LicenseOrgType)},
			PriceRub: d.PriceRub, OldPriceRub: discountedFrom(d), PriceOnRequest: d.PriceOnRequest,
			PriceMode: docPriceMode(d)})
	}
	priced := func(v out.ProductVariant) bool { return v.PriceRub != nil && !v.PriceOnRequest }
	sort.SliceStable(res, func(i, j int) bool {
		a, b := res[i], res[j]
		if pa, pb := priced(a), priced(b); pa != pb {
			return pa
		} else if pa && *a.PriceRub != *b.PriceRub {
			return *a.PriceRub < *b.PriceRub
		}
		return a.Title < b.Title
	})
	return res
}

// productCompat — правила совместимости позиции (по product_id; правило на всё семейство — тоже), по модели.
func productCompat(cc *commerceCatalog, p *domain.Product) []out.ProductCompat {
	res := []out.ProductCompat{}
	for _, r := range cc.compatRules {
		if r.ProductID != p.ID && (r.ProductID != "" || r.FamilyID != p.FamilyID) {
			continue
		}
		res = append(res, out.ProductCompat{TargetModel: r.TargetModel, State: r.State, Condition: r.Condition,
			SourceURL: r.SourceURL})
	}
	sort.SliceStable(res, func(i, j int) bool { return res[i].TargetModel < res[j].TargetModel })
	return res
}

// productAnalogs — аналоги: связи substitute от семейства в действующем справочнике связей (кроме отклонённых и
// ждущих проверки), до maxProductAnalogs семейств. От семейства — позиция связи или самая популярная позиция;
// число популярности в ответ не идёт.
func productAnalogs(cc *commerceCatalog, lx *searchLexicon, p *domain.Product) []out.ProductAnalog {
	res := []out.ProductAnalog{}
	seen := map[string]bool{p.FamilyID: true}
	for _, rel := range cc.relationsFrom[p.FamilyID] {
		if len(res) == maxProductAnalogs {
			break
		}
		if rel.Type != domain.RelationSubstitute || rel.ReviewStatus == domain.ReviewPending ||
			rel.ReviewStatus == domain.ReviewRejected || seen[rel.ToFamily] {
			continue
		}
		if rel.FromProduct != nil && *rel.FromProduct != p.ID {
			continue
		}
		var best *indexDoc
		if rel.ToProduct != nil {
			if d, ok := lx.docs[*rel.ToProduct]; ok {
				best = &d
			}
		}
		if best == nil {
			for _, pid := range lx.families[rel.ToFamily] {
				d := lx.docs[pid]
				if best == nil || d.Popularity > best.Popularity {
					best = &d
				}
			}
		}
		if best == nil {
			continue
		}
		seen[rel.ToFamily] = true
		res = append(res, out.ProductAnalog{FamilyID: rel.ToFamily, FamilyName: best.FamilyName,
			ProductID: best.ProductID, Title: best.Title, Vendor: best.Vendor, LicenseLine: licenseLine(*best),
			PriceRub: best.PriceRub, PriceOnRequest: best.PriceOnRequest, PriceMode: docPriceMode(*best),
			Note: rel.Note})
	}
	return res
}

// orgLabelOf — для кого вариант фида («Образовательное учреждение»); "" — без ограничения организацией.
func orgLabelOf(feedOrgType string) string {
	return orgLabels[feedOrg[feedOrgType]]
}

// discountedFrom — цена до скидки, если она выше текущей (как зачёркнутая цена карточки выдачи); иначе nil.
func discountedFrom(d indexDoc) *float64 {
	if d.OldPriceRub == nil || d.PriceRub == nil || d.PriceOnRequest || *d.PriceRub <= 0 || *d.OldPriceRub <= *d.PriceRub {
		return nil
	}
	return d.OldPriceRub
}

// withOrgLine — строка лицензии с типом организации, если вариант не для всех («для: образование»): на странице товара
// иначе стоят рядом два «Продление · 100 устройств · 2 года» с разной ценой (коммерческий и академический).
func withOrgLine(d indexDoc) string {
	line := licenseLine(d)
	if o := feedOrg[d.LicenseOrgType]; o != "" && orgLabels[o] != "" {
		line += " · для: " + strings.ToLower(orgLabels[o])
	}
	return line
}
