package services

import "kuznica/api/domain"

// Основная линейка вендора. Запрос называет только вендора и условия покупки («продлить
// касперский на 100 пк на 2 года», «касперский на 10 пк»): под условия подходят десятки семейств вендора, и все
// они точные. Первыми идут семейства его основной категории — той, где у него больше всего подходящих семейств
// (у «Лаборатории Касперского» — «Антивирусы для организаций», а не Symphony из «Защиты информации» или курсы), и
// среди них — общие продукты выше специализированных: название «… для почтовых серверов», «… для интернет-шлюзов»
// называет объект защиты, которого в запросе нет. Сегмент покупателя («для бизнеса», «для дома») объектом не
// считается. Порядок между уровнями релевантности по-прежнему решает sortFamilies: это ступень после scale.

// segmentWords — слово после «для» / «for», которое называет покупателя, а не объект: продукт остаётся общим.
var segmentWords = setOf("бизнеса", "дома", "организаций", "предприятий", "малого", "среднего", "крупного", "семьи",
	"офиса", "корпоративных", "компаний", "business", "home", "family", "office", "enterprise", "smb")

// specialisedName — название семейства называет объект применения («для почтовых серверов», «for xSP»).
func specialisedName(name string) bool {
	ws := wordsOf(name)
	for i, w := range ws {
		if (w == "для" || w == "for") && i+1 < len(ws) && !segmentWords[ws[i+1]] {
			return true
		}
	}
	return false
}

// markVendorCore — ступень core у семейств запроса «вендор + условия покупки»: 3 — редакция линейки основной
// категории, 2 — общий продукт основной категории, 1 — специализированный, 0 — остальное. Другие запросы — без изменений.
func (rc rankCtx) markVendorCore(fams []*familyHit) {
	a := rc.attrs
	if len(rc.ent.vendors) != 1 || len(rc.ent.named) > 0 || rc.ent.parsedFamily != "" ||
		a.quantity == 0 && !a.renewal && a.duration == 0 && !rc.vendorOnly {
		return
	}
	rc.markCoreByCategory(fams)
	rc.markKnownLines(fams)
}

// knownLineBonus — прибавка к ступени core у семейства основной линейки из базы знаний: выше любой ступени по
// категории (максимум 3), порядок внутри линеек по категории сохраняется.
const knownLineBonus = 3

// markKnownLines — основная линейка вендора из базы знаний (vendor_families.json): «астра» — Astra Linux SE, а не
// DCImanager из категории, где у Группы Астра больше семейств; «криптопро» — КриптоПро CSP, а не аппаратная
// платформа NGate. Категория этого не различает: у КриптоПро все семейства в одной. Носители и услуги — не линейка.
func (rc rankCtx) markKnownLines(fams []*familyHit) {
	if len(rc.vendorLines) == 0 {
		return
	}
	for _, fh := range fams {
		if fh.group != domain.GroupExact || !rc.vendorLines[fh.id] || rc.lx.familyVendor[fh.id] != rc.ent.vendors[0] ||
			rc.serviceFamily(fh.id) || mediaName(rc.lx.familyName[fh.id]) {
			continue
		}
		fh.core += knownLineBonus
	}
}

// vendorLineFamilies — семейства основных линеек из базы знаний подбора; нет базы — пусто.
func (s *Services) vendorLineFamilies() map[string]bool {
	kb := s.guide().knowledge()
	if kb == nil || len(kb.lines) == 0 {
		return nil
	}
	out := make(map[string]bool, len(kb.lines))
	for f, l := range kb.lines {
		if l.Line != "" {
			out[f] = true
		}
	}
	return out
}

// markCoreByCategory — ступень core по основной категории вендора (см. markVendorCore).
func (rc rankCtx) markCoreByCategory(fams []*familyHit) {
	count := map[string]int{}
	best, bestN := "", 0
	for _, fh := range fams {
		if fh.group != domain.GroupExact || rc.lx.familyVendor[fh.id] != rc.ent.vendors[0] {
			continue
		}
		c := rc.lx.familyCat[fh.id]
		count[c]++
		if count[c] > bestN {
			best, bestN = c, count[c]
		}
	}
	if bestN < 2 {
		return
	}
	var core []*familyHit
	for _, fh := range fams {
		if fh.group != domain.GroupExact || rc.lx.familyCat[fh.id] != best || rc.serviceFamily(fh.id) ||
			mediaName(rc.lx.familyName[fh.id]) {
			continue // носитель дистрибутива и услуги — не основная линейка
		}
		fh.core = 1
		if !specialisedName(rc.lx.familyName[fh.id]) {
			fh.core = 2
			core = append(core, fh)
		}
	}
	// Редакции одной линейки («Endpoint Security для бизнеса Стандартный / Расширенный / Расширенный Плюс») —
	// основной продукт вендора в этой категории: выше одиночных семейств.
	for _, fh := range core {
		for _, o := range core {
			if o != fh && sameLine(rc.lx.familyName[fh.id], rc.lx.familyName[o.id]) {
				fh.core = 3
				break
			}
		}
	}
}

// mediaName — семейство — носитель дистрибутива («Certified media Pack», «Медиапакет Dr.Web»), а не лицензия.
func mediaName(name string) bool {
	ws := wordsOf(name)
	for i, w := range ws {
		if w == "медиапакет" || w == "media" && i+1 < len(ws) && ws[i+1] == "pack" {
			return true
		}
	}
	return false
}
