package services

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"kuznica/api/domain"
)

// Каталог для подбора: офферы по семействам и вид оффера с полями, которые нужны правилам лицензирования. Строится
// один раз на версию каталога (указатель catalogState) поверх справочника позиций commerce.products.

// guideCatalog — индекс каталога для подбора.
type guideCatalog struct {
	st       *catalogState
	version  string
	products map[string]*domain.Product
	byFamily map[string][]*domain.Product
	famName  map[string]string
	famURL   map[string]string
	famVend  map[string]string
	limMu    sync.Mutex
	limits   map[string]int // семейство → предел мест (familyLimit)
}

// guideCatalogCache — индекс текущего каталога.
type guideCatalogCache struct {
	mu  sync.Mutex
	cat *guideCatalog
}

// newGuideCatalog — индекс из справочника позиций (сервис и тесты).
func newGuideCatalog(version string, products map[string]*domain.Product) *guideCatalog {
	c := &guideCatalog{version: version, products: products, byFamily: map[string][]*domain.Product{},
		famName: map[string]string{}, famURL: map[string]string{}, famVend: map[string]string{}}
	for _, p := range products {
		if p.FamilyID == "" {
			continue
		}
		c.byFamily[p.FamilyID] = append(c.byFamily[p.FamilyID], p)
		if c.famName[p.FamilyID] == "" {
			c.famName[p.FamilyID] = p.FamilyName
		}
		if c.famURL[p.FamilyID] == "" && p.FamilyURL != "" {
			c.famURL[p.FamilyID] = p.FamilyURL
		}
		if c.famVend[p.FamilyID] == "" {
			c.famVend[p.FamilyID] = p.VendorID
		}
	}
	for _, ps := range c.byFamily {
		sort.Slice(ps, func(i, j int) bool { return ps[i].ID < ps[j].ID })
	}
	return c
}

// catalog — индекс текущего каталога; nil — каталог не загружен.
func (g *guideService) catalog() *guideCatalog {
	if g.fixedCatalog != nil {
		return g.fixedCatalog
	}
	st := g.s.catalog.Load()
	if st == nil || st.commerce == nil {
		return nil
	}
	g.cat.mu.Lock()
	defer g.cat.mu.Unlock()
	if g.cat.cat != nil && g.cat.cat.st == st {
		return g.cat.cat
	}
	c := newGuideCatalog(st.version, st.commerce.products)
	c.st = st
	g.cat.cat = c
	return c
}

// guideOfferView — оффер глазами правил: область действия, вариант, единица, ступень, срок, вид, организация.
type guideOfferView struct {
	p        *domain.Product
	scope    string // workstation | server | tls_server | ""
	variant  string
	unit     string // node_tier | pack | unit
	pack     int
	tierMin  int
	tierMax  int // 0 — без верхней границы
	term     int // месяцев; 0 — бессрочная
	kind     string
	org      string
	marked   bool   // область действия из разметки базы знаний, а не из названия
	platform string // any | astra_alt_ks3 | "" (разметка базы знаний)
}

// view — вид оффера: разметка базы знаний важнее признаков названия.
func (c *guideCatalog) view(p *domain.Product, kb *guideKB) guideOfferView {
	v := guideOfferView{p: p, kind: p.License.Kind, org: p.License.OrgType}
	title := strings.ToLower(p.Title)
	v.scope = scopeFromTitle(title)
	v.variant = variantFromTitle(title)
	if p.License.TierMin != nil {
		v.tierMin = *p.License.TierMin
		if p.License.TierMax != nil {
			v.tierMax = *p.License.TierMax
		}
	}
	if p.License.DurationMonths != nil {
		v.term = *p.License.DurationMonths
	}
	switch {
	case v.tierMin > 0:
		v.unit = "node_tier"
	default:
		v.unit = "unit"
	}
	if kb != nil {
		if m, ok := kb.marks[p.ID]; ok {
			if m.Scope != "" {
				v.scope, v.marked = m.Scope, true
			}
			if m.Variant != "" {
				v.variant = m.Variant
			}
			if v.scope == "" && m.Object != "" {
				v.scope = m.Object
			}
			v.platform = m.Platform
			switch m.SaleUnit {
			case "per_unit_in_tier":
				v.unit = "node_tier"
			case "package":
				v.unit, v.tierMin, v.tierMax = "pack", 0, 0
				if m.PackageSize != nil {
					v.pack = *m.PackageSize
				}
			}
		}
	}
	return v
}

// scopeFromTitle — область действия по названию позиции каталога.
func scopeFromTitle(t string) string {
	switch {
	case strings.Contains(t, "tls"):
		return "tls_server"
	case (strings.Contains(t, "сервер") || strings.Contains(t, "server")) &&
		!strings.Contains(t, "пк") && !strings.Contains(t, "рабоч") && !strings.Contains(t, "компьютер"):
		return "server" // «для ПК, мобильных устройств и файловых серверов» — не серверная лицензия
	case strings.Contains(t, "рабоч"):
		return "workstation"
	}
	return ""
}

// variantFromTitle — уровень защищённости Astra Linux SE по названию.
func variantFromTitle(t string) string {
	switch {
	case strings.Contains(t, "орёл") || strings.Contains(t, "орел"):
		return "Орел"
	case strings.Contains(t, "воронеж"):
		return "Воронеж"
	case strings.Contains(t, "смоленск"):
		return "Смоленск"
	}
	return ""
}

// priced — у оффера действующая цена.
func (v guideOfferView) priced() bool {
	return priceKnown(v.p) && (v.p.Available == nil || *v.p.Available)
}

// tierLabel — «25–49» или «от 500»; "" — без ступени.
func (v guideOfferView) tierLabel() string {
	switch {
	case v.tierMin > 0 && v.tierMax > 0:
		return fmt.Sprintf("%d–%d", v.tierMin, v.tierMax)
	case v.tierMin > 0:
		return fmt.Sprintf("от %d", v.tierMin)
	}
	return ""
}

// inTier — количество попадает в ступень оффера (без ступени — любое).
func (v guideOfferView) inTier(n int) bool {
	if v.tierMin == 0 {
		return true
	}
	return n >= v.tierMin && (v.tierMax == 0 || n <= v.tierMax)
}

// unitLabel — единица продажи для карточки.
func (v guideOfferView) unitLabel(object string) string {
	switch v.unit {
	case "pack":
		if v.pack > 0 {
			return fmt.Sprintf("за пакет на %d мест", v.pack)
		}
		return "за пакет"
	case "node_tier":
		if object == "workstation" {
			return "за компьютер"
		}
		return "за узел"
	}
	if v.p.License.Metric != nil && *v.p.License.Metric == "user" {
		return "за пользователя"
	}
	switch v.scope {
	case "server", "tls_server":
		return "за сервер"
	case "workstation":
		return "за рабочее место"
	}
	return "за лицензию"
}

// termLabel — «1 год», «2 года», «бессрочная».
func (v guideOfferView) termLabel() string {
	if v.term == 0 {
		return "бессрочная"
	}
	return termWords(v.term)
}

func termWords(m int) string {
	if m%12 == 0 {
		y := m / 12
		switch {
		case y == 1:
			return "1 год"
		case y >= 2 && y <= 4:
			return fmt.Sprintf("%d года", y)
		}
		return fmt.Sprintf("%d лет", y)
	}
	return fmt.Sprintf("%d мес.", m)
}

// units — сколько единиц продажи нужно на n объектов (пакет — округление вверх).
func (v guideOfferView) units(n int) int {
	if n <= 0 {
		n = 1
	}
	if v.unit == "pack" && v.pack > 0 {
		return int(math.Ceil(float64(n) / float64(v.pack)))
	}
	return n
}

// round2 — до копеек.
func guideRound2(v float64) float64 { return math.Round(v*100) / 100 }

// perSeat — цена за одно место: у пакета — цена комплекта / число мест.
func (v guideOfferView) perSeat() float64 {
	if v.p.PriceRub == nil {
		return 0
	}
	if v.unit == "pack" && v.pack > 0 {
		return *v.p.PriceRub / float64(v.pack)
	}
	return *v.p.PriceRub
}
