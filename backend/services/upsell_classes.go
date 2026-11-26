package services

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Допродажа по классам продуктов (контур эталонов, VENDORS.md, KICKOFF.md §4). Логику задал заказчик: «покупает офис — нужен
// антивирус», «обрастает инфраструктурой — нужен бэкап». Класс семейства — по категории второго уровня фида
// (upsell_classes.csv), таблица «класс запроса → класс дополнения → причина» — данные (upsell_links.csv).
// Внутри класса: тот же вендор → вендор с документом о совместимости (или SKU, который называет продукт якоря) →
// остальные с ценой; акция поднимает карточку. Только с ценой, не больше четырёх, одно семейство на вендора.
// Связь из таблицы — уровень B (вид class_logic); документ о совместимости поднимает карточку до A.

// evKindClass — основание «логика класса» (таблица классов заказчика).
const evKindClass = "class_logic"

// upRejectVendor — у вендора уже есть карточка в блоке.
const upRejectVendor = "vendor_taken"

type compiledClass struct {
	c        domain.UpsellClass
	cats     map[string]bool
	inc, exc *regexp.Regexp
}

// compatHit — запись совместимости для пары семейств; anchorIsPlatform — первое семейство пары — платформа.
type compatHit struct {
	i                int
	anchorIsPlatform bool
}

// buildClasses — классы, таблица и пары «семейство × семейство» с документом о совместимости.
func (ix *upsellIndex) buildClasses(data *upsellData) {
	ix.classByID = map[string]*compiledClass{}
	ix.links = map[string][]domain.UpsellLink{}
	ix.classFams = map[string][]string{}
	ix.familyClass = map[string]string{}
	ix.compatPair = map[string]compatHit{}
	for _, c := range data.classes {
		cc := &compiledClass{c: c, cats: map[string]bool{}}
		for _, id := range c.Categories {
			cc.cats[id] = true
		}
		if c.Include != "" {
			cc.inc = regexp.MustCompile(c.Include) // проверено загрузчиком
		}
		if c.Exclude != "" {
			cc.exc = regexp.MustCompile(c.Exclude)
		}
		ix.classes = append(ix.classes, cc)
		ix.classByID[c.ID] = cc
	}
	for _, l := range data.links {
		ix.links[l.From] = append(ix.links[l.From], l)
	}
	for _, fid := range ix.lx.familyOrder {
		cls := ix.classify(fid)
		ix.familyClass[fid] = cls
		if cls != "" && len(ix.buyable[fid]) > 0 {
			ix.classFams[cls] = append(ix.classFams[cls], fid)
		}
	}
	for i, cd := range ix.compat {
		plat, prod := ix.familiesNamed(cd.PlatformFamilies), ix.familiesNamed(cd.ProductFamilies)
		for _, p := range plat {
			for _, q := range prod {
				for _, k := range []struct {
					key string
					hit compatHit
				}{{p + "|" + q, compatHit{i, true}}, {q + "|" + p, compatHit{i, false}}} {
					old, ok := ix.compatPair[k.key]
					if !ok || ix.compat[old.i].Level != domain.EvidenceA && cd.Level == domain.EvidenceA {
						ix.compatPair[k.key] = k.hit
					}
				}
			}
		}
	}
}

// classify — первый класс, в категории которого семейство и чьи слова названия подходят.
func (ix *upsellIndex) classify(fid string) string {
	name := foldText(ix.lx.familyName[fid])
	cat := ix.lx.familyCat[fid]
	for _, cc := range ix.classes {
		if !cc.cats[cat] || cc.inc != nil && !cc.inc.MatchString(name) || cc.exc != nil && cc.exc.MatchString(name) {
			continue
		}
		return cc.c.ID
	}
	return ""
}

func (ix *upsellIndex) classOf(fid string) string {
	if c, ok := ix.familyClass[fid]; ok {
		return c
	}
	return ix.classify(fid)
}

func (ix *upsellIndex) classTitle(id string) string {
	if cc := ix.classByID[id]; cc != nil {
		return cc.c.Title
	}
	return id
}

// docA — документ уровня A для пары «якорь × семейство».
func (ix *upsellIndex) docA(anchorFam, fid string) (compatHit, bool) {
	h, ok := ix.compatPair[anchorFam+"|"+fid]
	if !ok || ix.compat[h.i].Level != domain.EvidenceA {
		return compatHit{}, false
	}
	return h, true
}

// assembleByClass — строки таблицы класса якоря по порядку: каждая даёт не больше max карточек; всего ≤ 4; одно
// семейство на вендора (тип лицензии и вариант того же семейства — отдельно).
func (r *upsellRun) assembleByClass(cls string) []upsellCand {
	var picked []upsellCand
	usedFam, usedVendor := map[string]bool{}, map[string]bool{}
	for _, l := range r.ix.links[cls] {
		var cs []upsellCand
		switch l.To {
		case domain.UpsellSelf:
			cs = r.licenseKindCands(l.Kinds, l.Reason)
		case domain.UpsellSelfVariant:
			cs = r.selfVariantCands(l)
		default:
			cs = r.classCands(cls, l)
		}
		n := 0
		for _, c := range cs {
			r.step.Considered = append(r.step.Considered, UpsellConsidered{ProductID: c.pid, FamilyID: c.fid,
				Kind: c.kind, Level: c.level, Source: c.source})
			if n >= l.Max || len(picked) >= upsellMax {
				r.reject(c.pid, upRejectLimit, "строка "+l.From+" → "+l.To+": не больше "+strconv.Itoa(l.Max))
				continue
			}
			if c.fid != r.anchor.FamilyID {
				v := r.lx.familyVendor[c.fid]
				switch {
				case usedFam[c.fid]:
					r.reject(c.pid, upRejectShown, "семейство уже в блоке")
					continue
				case usedVendor[v]:
					r.reject(c.pid, upRejectVendor, "у вендора уже есть карточка в блоке")
					continue
				}
				usedFam[c.fid], usedVendor[v] = true, true
			}
			picked = append(picked, c)
			n++
		}
	}
	return picked
}

// classCands — семейства класса дополнения по порядку: тот же вендор → документ A или SKU, называющий продукт якоря →
// остальные; внутри — акция выше, затем порядок записей документов, затем шире линейка. Только с ценой;
// require_doc — только с документом.
func (r *upsellRun) classCands(from string, l domain.UpsellLink) []upsellCand {
	key := r.anchor.ProductID + "|" + from + ">" + l.To + "|" + r.now.Format("2006-01-02")
	if v, ok := r.ix.cands.Load(key); ok {
		return append([]upsellCand(nil), v.([]upsellCand)...)
	}
	res := r.classCandsUncached(from, l)
	r.ix.remember(&r.ix.cands, key, res)
	return append([]upsellCand(nil), res...)
}

// upsellCacheMax — записей в кэшах кандидатов индекса допродажи (сверх — считается без кэша).
const upsellCacheMax = 4096

func (ix *upsellIndex) remember(m *sync.Map, key string, v any) {
	if ix.cached.Load() >= upsellCacheMax {
		return
	}
	if _, loaded := m.LoadOrStore(key, v); !loaded {
		ix.cached.Add(1)
	}
}

func (r *upsellRun) classCandsUncached(from string, l domain.UpsellLink) []upsellCand {
	type scored struct {
		c   upsellCand
		key []float64
	}
	anchorFam, anchorVendor := r.anchor.FamilyID, r.lx.familyVendor[r.anchor.FamilyID]
	terms := anchorNameTerms(r.anchor.FamilyName)
	evTitle := "Логика заказчика: " + r.ix.classTitle(from) + " → " + r.ix.classTitle(l.To) + " (" + l.Source + ")"
	var res []scored
	for _, fid := range r.ix.classFams[l.To] {
		if fid == anchorFam {
			continue
		}
		hit, doc := r.ix.docA(anchorFam, fid)
		if l.RequireDoc && !doc {
			continue
		}
		var vers []string
		if doc && hit.anchorIsPlatform {
			cd := r.ix.compat[hit.i]
			for v := range titleVersions(cd.ProductVersion + " " + cd.ProductLabel) {
				vers = append(vers, v)
			}
			sort.Strings(vers)
		}
		// Тот же вендор — только позиция не дороже якоря в sameVendorRatio раз (RuBackup OneClick за 761 000 ₽ к ОС за
		// 5 100 ₽ — нет). Дешёвой позиции нет — семейство идёт по общим правилам ярусов.
		same := r.lx.familyVendor[fid] == anchorVendor
		pid := ""
		if same && r.sameVendorRatio > 0 {
			if ap := docBuyPrice(r.anchor); ap != nil {
				r.priceCap = r.sameVendorRatio * *ap
				pid = r.pickOffer([]string{fid}, true, vers...)
				r.priceCap = 0
				if pid == "" {
					same = false
				}
			}
		}
		if pid == "" {
			pid = r.pickOffer([]string{fid}, true, vers...)
		}
		if pid == "" {
			continue
		}
		d := r.lx.docs[pid]
		phrase := namesAnchor(r.lx.familyName[fid], terms)
		if phrase == "" {
			phrase = namesAnchor(d.Title, terms)
		}
		promo := len(r.ix.promosFor(fid, []string{d.Title}, r.now)) > 0 || len(discountBadge(d)) > 0
		// Ярус: тот же вендор → документ A или SKU называет продукт якоря → остальные; в ярусе — акция, затем порядок
		// записей compat_docs.csv (выверенный человеком список документов), затем шире линейка. Цена входа не годится:
		// цены за единицу несравнимы («Хранение данных в ДЦ» за 4 ₽ выиграло бы у лицензии бэкапа).
		docRank := 0.0
		if doc {
			docRank = -float64(hit.i) - 1
		}
		key := []float64{0, 0, 0, docRank, float64(len(r.ix.buyable[fid]))}
		if same {
			key[0] = 1
		}
		if doc || phrase != "" {
			key[1] = 1
		}
		if promo {
			key[2] = 1
		}
		c := upsellCand{pid: pid, fid: fid, kind: evKindClass, level: domain.EvidenceB, label: r.ix.classTitle(l.To),
			class: CompanionComplement, compat: domain.CompatUnknown, reason: l.Reason, evTitle: evTitle}
		if phrase != "" {
			c.reason += ". Вариант для " + phrase
			c.compat = domain.CompatConditional
		}
		if doc {
			cd := r.ix.compat[hit.i]
			c.kind, c.level, c.compat, c.class = evKindCompat, domain.EvidenceA, domain.CompatVerified, CompanionCompatible
			c.reason = l.Reason + ". " + compatReason(cd, hit.anchorIsPlatform)
			c.evTitle, c.evURL, c.docNo, c.source = docTitle(cd), cd.URL, cd.DocNumber, cd.ID
		}
		res = append(res, scored{c, key})
	}
	sort.SliceStable(res, func(i, j int) bool {
		if lessKey(res[j].key, res[i].key) {
			return true
		}
		if lessKey(res[i].key, res[j].key) {
			return false
		}
		return res[i].c.fid < res[j].c.fid
	})
	out := make([]upsellCand, len(res))
	for i, s := range res {
		s.c.rank = i
		out[i] = s.c
	}
	return out
}

// selfVariantCands — позиции того же семейства под другую платформу («КриптоПро CSP для Astra Linux SE» к «CSP 5.0»):
// по одной на платформу, базовая и дешевле; имя ОС выше общего «linux».
func (r *upsellRun) selfVariantCands(l domain.UpsellLink) []upsellCand {
	own := titlePlatform(r.anchor.Title)
	type v struct {
		pid, phrase string
		w           int
		price       float64
	}
	best := map[string]v{}
	for _, pid := range r.ix.buyable[r.anchor.FamilyID] {
		d := r.lx.docs[pid]
		low := r.ix.lower[pid]
		tp := r.ix.feat[pid].platform
		if tp == "" || tp == own || pid == r.anchor.ProductID || d.LicenseKind != domain.LicenseKindBase && d.LicenseKind != "" {
			continue
		}
		phrase, _ := forPhrase(d.Title, low, []platformTerm{{tp, 1}})
		w := 1
		if tp != "linux" {
			w = 2
		}
		p := *docBuyPrice(d)
		if cur, ok := best[tp]; !ok || p < cur.price {
			best[tp] = v{pid, phrase, w, p}
		}
	}
	var xs []v
	for _, x := range best {
		xs = append(xs, x)
	}
	sort.Slice(xs, func(i, j int) bool {
		if xs[i].w != xs[j].w {
			return xs[i].w > xs[j].w
		}
		return xs[i].pid < xs[j].pid
	})
	var res []upsellCand
	for i, x := range xs {
		res = append(res, upsellCand{pid: x.pid, fid: r.anchor.FamilyID, kind: evKindPlatform, level: domain.EvidenceB,
			label: "Вариант под платформу", class: CompanionCompatible, compat: domain.CompatConditional,
			reason:  "Вариант для " + x.phrase,
			evTitle: "Каталог: позиция того же семейства называет платформу (" + l.Source + ")",
			evURL:   r.lx.docs[x.pid].OfferURL, rank: i})
	}
	return res
}

var numRe = regexp.MustCompile(`\d+(?:\.\d+)?`)

// anchorNameTerms — как продукт якоря называют другие SKU: числа его названия («365»), иначе два первых значимых
// слова («astra linux»).
func anchorNameTerms(name string) []string {
	low := foldText(name)
	if i := strings.Index(low, "("); i > 0 {
		low = low[:i] // «Microsoft 365 Family (для семьи)» — пояснение в скобках не часть имени
	}
	if nums := numRe.FindAllString(low, -1); len(nums) > 0 {
		return nums
	}
	var words []string
	for _, w := range wordsOf(low) {
		if !platformGeneric[w] {
			words = append(words, w)
		}
	}
	if len(words) < 2 {
		return nil
	}
	return []string{words[0] + " " + words[1]}
}

// namesAnchor — фраза «для …» в тексте, где названы все термины имени якоря: «для Microsoft Office 365».
func namesAnchor(text string, terms []string) string {
	if len(terms) == 0 {
		return ""
	}
	low := strings.ReplaceAll(strings.ToLower(text), "ё", "е")
	phrase, w := forPhrase(text, low, []platformTerm{{terms[0], 1}})
	if w == 0 {
		return ""
	}
	pl := strings.ToLower(phrase)
	for _, t := range terms[1:] {
		if !containsWord(pl, t) {
			return ""
		}
	}
	return phrase
}

// discountBadge — «Скидка N%» по old_price позиции.
func discountBadge(d indexDoc) []string {
	p := docBuyPrice(d)
	if p == nil || d.OldPriceRub == nil || *d.OldPriceRub <= *p {
		return nil
	}
	if n := int(math.Round((*d.OldPriceRub - *p) / *d.OldPriceRub * 100)); n >= 1 {
		return []string{"Скидка " + strconv.Itoa(n) + "%"}
	}
	return nil
}

// ---------- «Вместе с этим» в подсказках ----------

// suggestTogetherMax — дополнений в строке подсказок.
const suggestTogetherMax = 2

// suggestTogether — одно-два дополнения из таблицы классов для семейства (первое семейство подсказки): строки
// таблицы класса по порядку, кроме типа лицензии и варианта того же семейства — в подсказке нужен другой товар.
func (s *Services) suggestTogether(lx *searchLexicon, p out.PanelProduct) []out.PanelTogether {
	res := []out.PanelTogether{}
	if lx == nil || p.FamilyID == "" {
		return res
	}
	ix := s.upsellIndexFor(lx)
	cls := ix.classOf(p.FamilyID)
	if cls == "" {
		return res
	}
	key := p.FamilyID + "|" + s.now().Format("2006-01-02")
	if v, ok := ix.together.Load(key); ok {
		return append([]out.PanelTogether{}, v.([]out.PanelTogether)...)
	}
	defer func() { ix.remember(&ix.together, key, append([]out.PanelTogether{}, res...)) }()
	pids := ix.buyable[p.FamilyID]
	anchorPID := p.ProductID
	if _, ok := lx.docs[anchorPID]; !ok && len(pids) > 0 {
		anchorPID = pids[0]
	}
	a, ok := lx.docs[anchorPID]
	if !ok {
		return res
	}
	r := &upsellRun{ix: ix, lx: lx, anchor: a, shown: map[string]bool{}, rejected: map[string]bool{}, now: s.now(),
		sameVendorRatio: s.upsellRatio,
		step:            &UpsellStep{Considered: []UpsellConsidered{}, Rejected: []domain.RejectedHit{}}}
	usedVendor := map[string]bool{}
	// Первый проход — по одной карточке на строку таблицы; если строк меньше двух — второй проход добирает из тех же
	// строк (видеосвязь → офисный пакет, max 2).
	for pass := 0; pass < 2 && len(res) < suggestTogetherMax; pass++ {
		for _, l := range ix.links[cls] {
			if l.To == domain.UpsellSelf || l.To == domain.UpsellSelfVariant || pass == 1 && l.Max < 2 {
				continue
			}
			for _, c := range r.classCands(cls, l) {
				v := lx.familyVendor[c.fid]
				if usedVendor[v] {
					continue
				}
				usedVendor[v] = true
				d := lx.docs[c.pid]
				res = append(res, out.PanelTogether{AnchorFamilyID: p.FamilyID, FamilyID: c.fid,
					FamilyName: lx.familyName[c.fid], ProductID: c.pid, Title: d.Title, Vendor: d.Vendor,
					PriceRub: docBuyPrice(d), Reason: c.reason, Level: c.level, Kind: c.kind, Label: c.label,
					OfferURL: d.OfferURL, FamilyURL: d.FamilyURL, SourceURL: c.evURL})
				break // одна карточка на строку за проход
			}
			if len(res) >= suggestTogetherMax {
				break
			}
		}
	}
	return res
}
