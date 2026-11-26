package services

import (
	"fmt"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Аналоги, реестр и честное «нет».
//   - Статус в реестре российского ПО — в трёх состояниях: confirmed (запись с номером, reestr.json),
//     not_matching (зарубежное ПО или запись «не соответствует»), unknown (отечественное без подтверждения).
//     Признак in_reestr каталога mock выведен из происхождения и подтверждением не считается.
//   - Явное требование реестра («из реестра», «в реестре») убирает неподтверждённые позиции из основного блока
//     в reestr_unconfirmed («Статус не установлен»).
//   - Запрос замены: исходный продукт — строкой «Вы искали», среди аналогов не повторяется.
//   - Товара нет в каталоге: empty.supply — «Запросить поставку»; у аналогов — строка отличий.

// reestrOf — статус семейства в реестре.
func (s *Services) reestrOf(lx *searchLexicon, familyID string) *out.Reestr {
	if e, ok := s.procurementData().reestr[familyID]; ok {
		if e.Status == domain.ReestrConfirmed {
			label := "В реестре российского ПО"
			if e.Number != "" {
				label += ", № " + e.Number
			}
			return &out.Reestr{Status: domain.ReestrConfirmed, Number: e.Number, Label: label, CheckedAt: e.CheckedAt}
		}
		return &out.Reestr{Status: domain.ReestrNotMatching, Label: "Не соответствует: в реестре российского ПО нет",
			CheckedAt: e.CheckedAt}
	}
	origin := ""
	if pids := lx.families[familyID]; len(pids) > 0 {
		origin = lx.docs[pids[0]].Origin
	}
	if origin == "foreign" {
		return &out.Reestr{Status: domain.ReestrNotMatching, Label: "Не соответствует: зарубежное ПО"}
	}
	return &out.Reestr{Status: domain.ReestrUnknown, Label: "Статус в реестре не установлен"}
}

// applyReestr — статус реестра у позиций ПО; при явном требовании реестра неподтверждённые уходят из основного
// блока в reestr_unconfirmed. Когда о реестре и происхождении не спрашивали (relevant = false), показывается
// только подтверждённая запись: «Не соответствует: зарубежное ПО» у Microsoft Office по запросу «microsoft
// office» — шум.
func (s *Services) applyReestr(lx *searchLexicon, resp *out.SearchResponse, required, relevant bool) {
	main := resp.Items[:0:0]
	for _, it := range resp.Items {
		if it.Kind == domain.KindHardware {
			main = append(main, it)
			continue
		}
		it.Reestr = s.reestrOf(lx, it.FamilyID)
		if !relevant && it.Reestr.Status != domain.ReestrConfirmed {
			it.Reestr = nil
		}
		if required && it.Reestr.Status != domain.ReestrConfirmed {
			resp.ReestrUnconfirmed = append(resp.ReestrUnconfirmed, it)
			continue
		}
		main = append(main, it)
	}
	resp.Items = main
	if required && len(resp.ReestrUnconfirmed) > 0 {
		resp.Total -= len(resp.ReestrUnconfirmed)
		if resp.Total < len(resp.Items) {
			resp.Total = len(resp.Items)
		}
	}
}

// analogOriginal — «Вы искали»: из референсов запроса замены — последняя версия по названию семейства
// («Microsoft Office 2024», а не 2021), в ней — самая популярная позиция.
func (s *Services) analogOriginal(lx *searchLexicon, refs []string) *out.AnalogOriginal {
	fam := ""
	for _, f := range refs {
		if len(lx.families[f]) > 0 && (fam == "" || lx.familyName[f] > lx.familyName[fam]) {
			fam = f
		}
	}
	var best *indexDoc
	for _, pid := range lx.families[fam] {
		d := lx.docs[pid]
		if best == nil || d.Popularity > best.Popularity {
			best = &d
		}
	}
	if best == nil {
		return nil
	}
	o := &out.AnalogOriginal{FamilyID: best.FamilyID, FamilyName: best.FamilyName, ProductID: best.ProductID,
		Title: best.Title, Vendor: best.Vendor, Category: best.Category, Reestr: s.reestrOf(lx, best.FamilyID)}
	for _, pid := range lx.families[best.FamilyID] {
		if p := lx.docs[pid].PriceRub; p != nil && (o.PriceFromRub == nil || *p < *o.PriceFromRub) {
			v := *p
			o.PriceFromRub = &v
		}
	}
	return o
}

// absentAnalogNotes — строка отличий у аналога отсутствующего товара: это другой продукт другого
// производителя — какой и из какой категории.
func absentAnalogNotes(resp *out.SearchResponse, name string) {
	for i := range resp.Items {
		it := &resp.Items[i]
		if it.Group != domain.GroupAnalog {
			continue
		}
		parts := []string{}
		if it.Vendor != "" {
			parts = append(parts, "производитель "+it.Vendor)
		}
		if it.Category != "" {
			parts = append(parts, strings.ToLower(it.Category))
		}
		if it.LicenseLine != "" {
			parts = append(parts, strings.ToLower(it.LicenseLine))
		}
		it.AnalogNote = fmt.Sprintf("Не «%s», а другой продукт: %s", name, strings.Join(parts, " · "))
	}
}
