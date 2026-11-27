package services

import (
	"slices"
	"strings"

	"kuznica/api/domain"
)

// Голова семейства ( шаг 3): семейство представляет базовая лицензия коммерческого
// типа, в наличии с ценой. Продления, техподдержка, модули и курсы головой не становятся, если о них не просили;
// академические, государственные и некоммерческие варианты скрыты, пока тип организации не назван
// (purchase_attrs.go offerHidden). Опора — License.kind и License.org_type фида; в mock полей нет, порядок прежний.

// kindStems — начало слова запроса → вид позиции, о котором прямо просят («техподдержка astra linux»).
var kindStems = []struct{ stem, kind string }{
	{"техподдерж", domain.LicenseKindSupport}, {"поддержк", domain.LicenseKindSupport},
	{"сопровожд", domain.LicenseKindSupport}, {"support", domain.LicenseKindSupport},
	{"модул", domain.LicenseKindModule}, {"module", domain.LicenseKindModule}, {"addon", domain.LicenseKindModule},
	{"курс", domain.LicenseKindCourse}, {"обучени", domain.LicenseKindCourse}, {"training", domain.LicenseKindCourse},
	{"обновлени", domain.LicenseKindUpgrade}, {"апгрейд", domain.LicenseKindUpgrade}, {"upgrade", domain.LicenseKindUpgrade},
	{"продлен", domain.LicenseKindRenewal}, {"продлить", domain.LicenseKindRenewal}, {"renewal", domain.LicenseKindRenewal},
}

// wantedKinds — виды позиции, названные в запросе: тогда они наравне с базовой лицензией.
func wantedKinds(tokens []string, a queryAttrs) map[string]bool {
	want := map[string]bool{}
	for _, t := range tokens {
		for _, w := range wordsOf(t) {
			for _, ks := range kindStems {
				if strings.HasPrefix(w, ks.stem) {
					want[ks.kind] = true
				}
			}
		}
	}
	if a.renewal {
		want[domain.LicenseKindRenewal] = true
	}
	if a.deal == DealMigration {
		want[domain.LicenseKindMigration] = true
	}
	if a.deal == DealExtension {
		want[domain.LicenseKindModule] = true
	}
	return want
}

// kindRank — место вида позиции при выборе головы: 0 — базовая (или названная в запросе), выше — хуже.
// "" (источник вид не сообщает) и other — сразу за базовой: так mock и позиции без типа не теряют места.
func kindRank(kind string, want map[string]bool) int {
	if want[kind] {
		return 0
	}
	switch kind {
	case domain.LicenseKindBase:
		return 0
	case "", domain.LicenseKindOther:
		return 1
	case domain.LicenseKindUpgrade, domain.LicenseKindMigration, domain.LicenseKindRenewal:
		return 2
	case domain.LicenseKindModule, domain.LicenseKindSupport:
		return 3
	}
	return 4 // course
}

// buyable — позицию можно купить сразу: цена > 0 и не «по запросу» (: цена из фида у позиции не в
// наличии ценой не считается — импорт её уже снял).
func buyable(d indexDoc) bool {
	return d.PriceRub != nil && *d.PriceRub > 0 && !d.PriceOnRequest && (d.Available == nil || *d.Available)
}

// headLess — порядок вариантов с равным соответствием условиям запроса: вид позиции, затем «можно купить сейчас».
// ok=false — по этим признакам варианты равны.
func (rc rankCtx) headLess(a, b indexDoc) (less, ok bool) {
	if ka, kb := kindRank(rc.lx.kindOf(a), rc.wantKinds), kindRank(rc.lx.kindOf(b), rc.wantKinds); ka != kb {
		return ka < kb, true
	}
	if ba, bb := buyable(a), buyable(b); ba != bb {
		return ba, true
	}
	// Сборка под отдельную платформу («на базе п/а Эльбрус», «для Байкал-М», ARM) — головой, только если
	// платформу назвали: «астра линукс» — это x86-64, а не редакция для Эльбруса.
	if pa, pb := rc.otherPlatform(a.Title), rc.otherPlatform(b.Title); pa != pb {
		return pb, true
	}
	// Носитель дистрибутива («Certified media Pack») — не лицензия: головой семейства, только если другого нет.
	if ma, mb := mediaName(a.Title), mediaName(b.Title); ma != mb {
		return mb, true
	}
	return false, false
}

// secondaryKind — вид позиции, который не представляет семейство, если о нём не просили: продление, обновление,
// переход, техподдержка, модуль, курс. Такой вариант уступает базовому независимо от мелких добавок соответствия.
func (rc rankCtx) secondaryKind(d indexDoc) bool {
	return kindRank(rc.lx.kindOf(d), rc.wantKinds) >= 2
}

// courseCategoryStems — начало слова названия категории, по которому всё в ней — обучение («Курсы по информационной
// безопасности»): у таких позиций фид пишет тип «базовая» или «продление», а это курс (Kaspersky Express Course).
var courseCategoryStems = []string{"курс", "обучени", "тренинг"}

// kindOf — вид позиции с поправкой на категорию: позиция из категории курсов — курс, что бы ни стояло в фиде.
func (lx *searchLexicon) kindOf(d indexDoc) string {
	if lx != nil && d.LicenseKind != "" {
		for _, w := range wordsOf(lx.categoryName[d.CategoryID]) {
			for _, st := range courseCategoryStems {
				if strings.HasPrefix(w, st) {
					return domain.LicenseKindCourse
				}
			}
		}
	}
	return d.LicenseKind
}

// platformStems — начала слов названия позиции, по которым это сборка под отдельную аппаратную платформу.
var platformStems = []string{"эльбрус", "elbrus", "байкал", "baikal", "aarch64", "комдив", "мцст"}

// platformWords — короткие названия платформ: только целым словом («arm», а не «armor»).
var platformWords = setOf("arm", "arm64", "armv8", "risc-v", "riscv", "мцст")

// otherPlatform — позиция собрана под платформу, которой в запросе нет.
func (rc rankCtx) otherPlatform(title string) bool {
	for _, w := range wordsOf(title) {
		if platformWords[w] {
			for _, t := range rc.ent.textTokens {
				if slices.Contains(wordsOf(t), w) {
					return false
				}
			}
			return true
		}
		for _, st := range platformStems {
			if !strings.HasPrefix(w, st) {
				continue
			}
			for _, t := range rc.ent.textTokens {
				for _, q := range wordsOf(t) {
					if strings.HasPrefix(q, st) || strings.HasPrefix(st, q) && len([]rune(q)) >= 4 {
						return false
					}
				}
			}
			return true
		}
	}
	return false
}
