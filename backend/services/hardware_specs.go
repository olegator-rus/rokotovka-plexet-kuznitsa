package services

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Числовые характеристики оборудования из атрибутов фида (правила, без модели). В mock характеристики приходят
// готовыми (specs), а в фиде заказчика — только строками атрибутов: «Объем установленной ОЗУ: 16 ГБ», «Высота
// Rackmount: 2U», «Диагональ экрана: 16». Без них условие «ноутбук 16 гб» не проходит ни одна позиция и выдача
// пустая. Значения из данных не перезаписываются: правила работают, только если specs у позиции нет.

// specRule — какой атрибут какую характеристику даёт: ключ атрибута (свёрнутый) содержит все слова need и ни одного
// из skip; unit — единица по умолчанию, если в значении её нет.
type specRule struct {
	key  string
	need []string
	skip []string
	unit string
}

// specRules — правила по порядку; несколько атрибутов на одну характеристику (SSD и HDD) складываются, если
// sum, иначе берётся первый разобранный.
var specRules = []specRule{
	{key: "ram_gb", need: []string{"озу"}, skip: []string{"макс", "слот"}, unit: "гб"},
	{key: "ram_gb", need: []string{"оперативн", "памят"}, skip: []string{"макс", "слот"}, unit: "гб"},
	{key: "storage_gb", need: []string{"объем установленного ssd"}, unit: "гб"},
	{key: "storage_gb", need: []string{"объем установленного hdd"}, unit: "гб"},
	{key: "screen_inch", need: []string{"диагональ"}, unit: "дюйм"},
	{key: "form_factor_u", need: []string{"rackmount"}, unit: "u"},
	{key: "form_factor_u", need: []string{"высота", "юнит"}, unit: "u"},
	{key: "ports_count", need: []string{"количество портов"}, unit: "порт"},
	{key: "ports_count", need: []string{"кол-во портов"}, unit: "порт"},
}

// sumSpecs — характеристики, которые складываются из нескольких атрибутов (SSD + HDD — общий объём).
var sumSpecs = setOf("storage_gb")

// specUnitFactor — множитель единицы значения атрибута к единице характеристики.
var specUnitFactor = map[string]float64{"гб": 1, "gb": 1, "тб": 1000, "tb": 1000, "мб": 1.0 / 1024, "mb": 1.0 / 1024,
	"u": 1, "юнит": 1, "дюйм": 1, "дюйма": 1, "дюймов": 1, `"`: 1, "порт": 1, "порта": 1, "портов": 1}

var reSpecValue = regexp.MustCompile(`^\s*(\d+(?:[.,]\d+)?)\s*([^\d\s]*)`)

// specValue — число и единица из значения атрибута («16 ГБ», «2U», «15.6»); ok=false — не число («не установлен»).
func specValue(v, defUnit string) (float64, bool) {
	m := reSpecValue.FindStringSubmatch(strings.ToLower(v))
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	unit := m[2]
	if unit == "" {
		unit = defUnit
	}
	f, ok := specUnitFactor[unit]
	if !ok {
		return 0, false
	}
	return n * f, true
}

// specsFromAttributes — характеристики позиции оборудования из её атрибутов; nil — ничего не разобрано.
func specsFromAttributes(attrs map[string]string) map[string]float64 {
	var out map[string]float64
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys) // порядок атрибутов — по ключу: «первый разобранный» один и тот же при каждом старте
	for _, k := range keys {
		v := attrs[k]
		fk := foldText(k)
		for _, r := range specRules {
			if !specKeyMatch(fk, r) {
				continue
			}
			n, ok := specValue(v, r.unit)
			if !ok {
				continue
			}
			if out == nil {
				out = map[string]float64{}
			}
			if sumSpecs[r.key] {
				out[r.key] += n
			} else if _, have := out[r.key]; !have {
				out[r.key] = n
			}
			break
		}
	}
	return out
}

func specKeyMatch(key string, r specRule) bool {
	for _, w := range r.need {
		if !strings.Contains(key, w) {
			return false
		}
	}
	for _, w := range r.skip {
		if strings.Contains(key, w) {
			return false
		}
	}
	return true
}

// docSpecs — характеристики позиции: из данных, а у оборудования без них — из атрибутов по правилам.
func docSpecs(p domain.Product) map[string]float64 {
	if len(p.Specs) > 0 || p.Kind != domain.KindHardware {
		return p.Specs
	}
	return specsFromAttributes(rawAttrs(p.Attributes))
}

// setIndexed — характеристики, под которые в индексе заведены поля specs.*; info nil — неизвестно.
func (p *hwPlan) setIndexed(info *indexMeta) {
	if p == nil || info == nil {
		return
	}
	p.indexed = map[string]bool{}
	for _, k := range info.SpecKeys {
		p.indexed[k] = true
	}
}

// relaxSpecs — «коммутатор 24 порта», а позиций категории с таким числом портов нет: вместо пустой выдачи — похожие
// позиции категории, как при снятом сроке или количестве у лицензий (relaxedMessages). Проверка — в памяти по
// всем позициям категории запроса; снимаются только числовые условия, категория, цена, вендор и совместимость
// остаются. Без категории («16 гб») условия не снимаются: похожие было бы не из чего выбирать.
func (p *hwPlan) relaxSpecs() bool {
	if !p.active() || p.category == "" || p.compatMode == "filter" || p.mpn != nil || p.model != "" {
		return false
	}
	var nums []hwCond
	for _, c := range p.conds {
		if c.typ == "number" {
			nums = append(nums, c)
		}
	}
	if len(nums) == 0 {
		return false
	}
	ids := p.hx.products[p.category]
	if len(ids) == 0 {
		return false // категория не листовая: позиции не перебрать, условия как есть
	}
	for _, id := range ids {
		if r, _ := p.reject(p.lx.docs[id]); r == "" {
			return false // есть позиция под все условия: снимать нечего
		}
	}
	var keep []hwCond
	for _, c := range p.conds {
		if c.typ != "number" {
			keep = append(keep, c)
			continue
		}
		missing := true
		for _, id := range ids {
			if _, ok := p.lx.docs[id].Specs[c.Key]; ok {
				missing = false
				break
			}
		}
		p.relaxed = append(p.relaxed, hwRelaxed{cond: c, missing: missing})
	}
	p.conds = keep
	return true
}

// relaxedChips — чипы снятых условий: не строгие (условие показано, но выдачу не ограничивает).
func (p *hwPlan) relaxedChips(chips []out.Chip) []out.Chip {
	if p == nil || len(p.relaxed) == 0 {
		return chips
	}
	for i := range chips {
		for _, r := range p.relaxed {
			if chips[i].Field == r.cond.Key {
				chips[i].Strict = false
			}
		}
	}
	return chips
}

// relaxedMessages — что снято и почему: характеристики нет в данных каталога или нет позиций с таким значением.
func (p *hwPlan) relaxedMessages() []out.Relaxed {
	if p == nil {
		return nil
	}
	var res []out.Relaxed
	for _, r := range p.relaxed {
		label := strings.TrimSpace(r.cond.Text)
		if label == "" {
			label = p.condChip(r.cond).Label
		}
		msg := fmt.Sprintf("Позиций с условием «%s» нет, показаны похожие с другими значениями", label)
		if r.missing {
			msg = fmt.Sprintf("Условие «%s» не проверить: в карточках раздела «%s» этой характеристики нет. Показаны похожие — уточните в карточке товара",
				label, p.hx.catNames[p.category])
		}
		res = append(res, out.Relaxed{Field: r.cond.Key, Message: msg})
	}
	return res
}

// nearestHits — после снятия условий: позиции категории, ближайшие по снятым характеристикам (без значения — в
// конце), затем популярность. Это список RRF наравне с текстом и векторами, как structuredHits.
func (p *hwPlan) nearestHits() []domain.RankedHit {
	type cand struct {
		id   string
		dist float64
		pop  float64
	}
	var cs []cand
	for _, id := range p.hx.products[p.category] {
		d := p.lx.docs[id]
		if r, _ := p.reject(d); r != "" {
			continue
		}
		dist := 0.0
		for _, r := range p.relaxed {
			v, ok := d.Specs[r.cond.Key]
			if !ok {
				dist = math.Inf(1)
				break
			}
			if r.cond.Value > 0 {
				dist += math.Abs(v-r.cond.Value) / r.cond.Value
			}
		}
		if math.IsInf(dist, 1) && !p.allMissing() {
			continue // значение есть у других позиций: без значения — не «похожие»
		}
		cs = append(cs, cand{id, dist, d.Popularity})
	}
	sort.Slice(cs, func(i, j int) bool {
		if cs[i].dist != cs[j].dist {
			return cs[i].dist < cs[j].dist
		}
		if cs[i].pop != cs[j].pop {
			return cs[i].pop > cs[j].pop
		}
		return cs[i].id < cs[j].id
	})
	var hits []domain.RankedHit
	for i, c := range cs {
		if i == exactListTop {
			break
		}
		hits = append(hits, domain.RankedHit{ProductID: c.id, Rank: i + 1})
	}
	return hits
}

// allMissing — ни у одной снятой характеристики нет значений в категории: ближайших не выбрать, порядок — популярность.
func (p *hwPlan) allMissing() bool {
	for _, r := range p.relaxed {
		if !r.missing {
			return false
		}
	}
	return true
}
