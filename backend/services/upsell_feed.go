package services

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"kuznica/api/domain"
	"kuznica/api/out"

	"go.uber.org/zap"
)

// Допродажа на фиде заказчика. На фиде связей, правил лицензий и
// совместимости нет — блок «Дополните решение» собирается из того, что доказуемо:
//   - B, тип лицензии внутри семейства: техподдержка, продление, модуль, льготный переход к позиции-якорю — с
//     проверкой редакции («Орёл» ≠ «Воронеж», «Стандартная редакция» ≠ «Расширенная»), назначения (рабочая
//     станция / сервер), архитектуры и типа организации;
//   - A/C, совместимость с первичным документом (compat_docs.csv): сертификат, протокол, таблица ОС вендора;
//   - B, SKU прямо называет платформу якоря («КриптоПро CSP для Astra Linux SE», «для рабочей станции Linux»);
//   - B, экосистема вендора: доступные семейства того же вендора из других категорий.
//
// LLM не вызывается: связь, которой нет в каталоге или документе, в блок не попадает. Маржа и популярность порядок
// не меняют. Акции магазина (promos.csv) — строка на карточке выдачи и блока, только с подтверждённым сроком.
// Блок строится после выбора цели (answerStage) и только на фиде; mock-путь (companions.go) не меняется.

// StepUpsell — шаг трассы допродажи на фиде.
const StepUpsell = "upsell"

// Вид основания карточки (CompanionEvidence.Kind).
const (
	evKindCompat    = "compat_document"
	evKindLicense   = "license_kind"
	evKindPlatform  = "sku_platform"
	evKindEcosystem = "vendor_ecosystem"
)

// Лимиты блока: всего и по видам.
const (
	upsellMax          = 6
	upsellMaxLicense   = 2
	upsellMaxCompat    = 3
	upsellMaxPlatform  = 2
	upsellMaxEcosystem = 2
	upsellMaxRejected  = 40
)

// Причины отказа кандидату допродажи (trace upsell.rejected[].reason).
const (
	upRejectEdition  = "edition_mismatch"
	upRejectPurpose  = "purpose_mismatch"
	upRejectArch     = "arch_mismatch"
	upRejectOrg      = "org_type_mismatch"
	upRejectOrgSize  = "org_size_mismatch"
	upRejectVersion  = "version_mismatch"
	upRejectPlatform = "platform_mismatch"
	upRejectCategory = "category_taken"
	upRejectNoPrice  = "not_buyable"
	upRejectShown    = "already_shown"
	upRejectLimit    = "limit"
	upRejectNoFamily = "family_not_in_catalog"
)

// UpsellStep — шаг трассы: якорь (семейство и позиция), рассмотренные и отклонённые кандидаты, акции.
type UpsellStep struct {
	domain.StepBase
	Error       string               `json:"error,omitempty"`
	Anchor      *out.CompanionAnchor `json:"anchor"`
	AnchorOffer string               `json:"anchor_offer,omitempty"`
	Considered  []UpsellConsidered   `json:"considered"`
	Rejected    []domain.RejectedHit `json:"rejected"`
	Promos      []string             `json:"promos"`
}

// UpsellConsidered — кандидат: вид основания, уровень, показан ли.
type UpsellConsidered struct {
	ProductID string `json:"product_id"`
	FamilyID  string `json:"family_id"`
	Kind      string `json:"kind"`
	Level     string `json:"level"`
	Source    string `json:"source,omitempty"` // id записи compat_docs
	Shown     bool   `json:"shown"`
}

// ---------- индекс каталога для допродажи (один на снимок словаря) ----------

// upsellIndexes — *searchLexicon → *upsellIndex; сбрасывается при смене справочников (SetUpsellData).
var upsellIndexes sync.Map

type upsellIndex struct {
	lx *searchLexicon
	// byName — свёрнутое название семейства → семейства.
	byName map[string][]string
	// buyable — семейство → позиции, которые можно купить сейчас (порядок каталога).
	buyable map[string][]string
	// vendorFams — вендор → семейства с позициями, которые можно купить (порядок каталога).
	vendorFams map[string][]string
	// lower — название позиции в нижнем регистре (для фраз «для …»), только покупаемые.
	lower map[string]string
	// compatByPlatform / compatByProduct — семейство → записи совместимости, где оно платформа / продукт.
	compatByPlatform map[string][]int
	compatByProduct  map[string][]int
	compat           []domain.CompatDoc
	// promoByFamily — семейство → акции.
	promoByFamily map[string][]domain.Promo
	// platformCache — семейство-платформа → найденные варианты SKU (считаются один раз).
	platformCache sync.Map
}

func (s *Services) upsellIndexFor(lx *searchLexicon) *upsellIndex {
	if v, ok := upsellIndexes.Load(lx); ok {
		return v.(*upsellIndex)
	}
	ix := buildUpsellIndex(lx, s.upsellData())
	v, _ := upsellIndexes.LoadOrStore(lx, ix)
	return v.(*upsellIndex)
}

func buildUpsellIndex(lx *searchLexicon, data *upsellData) *upsellIndex {
	ix := &upsellIndex{lx: lx, byName: map[string][]string{}, buyable: map[string][]string{},
		vendorFams: map[string][]string{}, lower: map[string]string{}, compatByPlatform: map[string][]int{},
		compatByProduct: map[string][]int{}, compat: data.compat, promoByFamily: map[string][]domain.Promo{}}
	for _, fid := range lx.familyOrder {
		ix.byName[nameKey(lx.familyName[fid])] = append(ix.byName[nameKey(lx.familyName[fid])], fid)
		for _, pid := range lx.families[fid] {
			d := lx.docs[pid]
			if docBuyPrice(d) == nil {
				continue
			}
			ix.buyable[fid] = append(ix.buyable[fid], pid)
			ix.lower[pid] = strings.ReplaceAll(strings.ToLower(d.Title), "ё", "е")
		}
		if len(ix.buyable[fid]) > 0 {
			v := lx.familyVendor[fid]
			ix.vendorFams[v] = append(ix.vendorFams[v], fid)
		}
	}
	missing := 0
	for i, cd := range data.compat {
		plat, prod := ix.familiesNamed(cd.PlatformFamilies), ix.familiesNamed(cd.ProductFamilies)
		if len(plat) == 0 || len(prod) == 0 {
			missing++
			zap.L().Warn("совместимость: семейства записи нет в каталоге", zap.String("id", cd.ID),
				zap.Strings("product", cd.ProductFamilies), zap.Strings("platform", cd.PlatformFamilies))
			continue
		}
		for _, f := range plat {
			ix.compatByPlatform[f] = append(ix.compatByPlatform[f], i)
		}
		for _, f := range prod {
			ix.compatByProduct[f] = append(ix.compatByProduct[f], i)
		}
	}
	for _, p := range data.promos {
		for _, f := range ix.familiesNamed(p.Families) {
			if p.Vendor != "" && lx.familyVendor[f] != p.Vendor {
				continue
			}
			ix.promoByFamily[f] = append(ix.promoByFamily[f], p)
		}
	}
	cov := ix.coverage()
	zap.L().Info("допродажа: покрытие каталога", zap.Int("families_buyable", cov.Families),
		zap.Int("with_support", cov.Support), zap.Int("with_renewal", cov.Renewal), zap.Int("with_module", cov.Module),
		zap.Int("with_migration", cov.Migration), zap.Int("with_ecosystem", cov.Ecosystem),
		zap.Int("compat_docs", len(data.compat)), zap.Int("compat_unresolved", missing))
	return ix
}

func nameKey(s string) string { return strings.Join(wordsOf(s), " ") }

func (ix *upsellIndex) familiesNamed(names []string) []string {
	var res []string
	for _, n := range names {
		res = append(res, ix.byName[nameKey(n)]...)
	}
	return res
}

// UpsellCoverage — покрытие связей B каталогом: семейства, которые можно купить, и сколько из них имеют
// покупаемую техподдержку, продление, модуль, льготный переход внутри семейства и экосистему вендора.
type UpsellCoverage struct {
	Families, Support, Renewal, Module, Migration, Ecosystem int
}

func (ix *upsellIndex) coverage() UpsellCoverage {
	var c UpsellCoverage
	for fid, pids := range ix.buyable {
		if strings.HasPrefix(fid, "offer-") {
			continue // электроника: семейство = сам оффер, связей типа лицензии нет (знаменатель BASIS — 1 809 семейств ПО)
		}
		c.Families++
		kinds := map[string]bool{}
		for _, pid := range pids {
			kinds[ix.lx.docs[pid].LicenseKind] = true
		}
		if kinds[domain.LicenseKindSupport] {
			c.Support++
		}
		if kinds[domain.LicenseKindRenewal] {
			c.Renewal++
		}
		if kinds[domain.LicenseKindModule] {
			c.Module++
		}
		if kinds[domain.LicenseKindMigration] {
			c.Migration++
		}
		cat := ix.lx.familyCat[fid]
		for _, f := range ix.vendorFams[ix.lx.familyVendor[fid]] {
			if f != fid && ix.lx.familyCat[f] != cat {
				c.Ecosystem++
				break
			}
		}
	}
	return c
}

// docBuyPrice — действующая цена позиции: > 0, не по запросу, в наличии; nil — купить сейчас нельзя.
func docBuyPrice(d indexDoc) *float64 {
	if d.PriceRub == nil || *d.PriceRub <= 0 || d.PriceOnRequest || d.Available != nil && !*d.Available {
		return nil
	}
	return d.PriceRub
}

// ---------- признаки позиции: редакция, назначение, архитектура ----------

// docAttrs — атрибуты документа («ключ: значение; …») в карту.
func docAttrs(d indexDoc) map[string]string {
	m := map[string]string{}
	for _, part := range strings.Split(d.Attributes, "; ") {
		if k, v, ok := strings.Cut(part, ": "); ok {
			m[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return m
}

// editionRe — редакция в названии: «Стандартная редакция», «редакция Standard», «(расширенная Редакция)».
var editionRe = regexp.MustCompile(`([\p{L}]+)\s+редакци\p{L}*|редакци\p{L}*\s+"?([\p{L}]+)`)

// editionStems — основы слов-редакций (первые 6 букв), которые различают лицензии.
var editionStems = map[string]bool{"станда": true, "расшир": true, "базова": true, "корпор": true, "промыш": true,
	"сертиф": true, "профес": true, "образо": true, "термин": true, "сервер": true, "десктоп": true, "standa": true,
	"advanc": true, "base": true, "enterp": true, "profes": true, "premiu": true, "pve": true}

func stem6(w string) string {
	r := []rune(w)
	if len(r) > 6 {
		r = r[:6]
	}
	return string(r)
}

// titleEditions — редакции, названные в названии позиции.
func titleEditions(title string) map[string]bool {
	res := map[string]bool{}
	for _, m := range editionRe.FindAllStringSubmatch(foldText(title), -1) {
		for _, w := range m[1:] {
			if st := stem6(w); w != "" && editionStems[st] {
				res[st] = true
			}
		}
	}
	return res
}

// versionRe — номер версии в названии: «5.0», «1.8», «12.0» (не часть большего числа).
var versionRe = regexp.MustCompile(`(?:^|[^\d.,])(\d{1,2}\.\d{1,2})(?:[^\d.]|$)`)

// titleVersions — версии, названные в названии позиции.
func titleVersions(title string) map[string]bool {
	res := map[string]bool{}
	for _, m := range versionRe.FindAllStringSubmatch(title, -1) {
		res[m[1]] = true
	}
	return res
}

// skuPlatformWords — платформы, которые SKU называет фразой «для …»: позиция «КриптоПро CSP для Astra Linux SE» — не та же
// лицензия, что «КриптоПро CSP 5.0», и её техподдержка — не «к этой лицензии».
var skuPlatformWords = []platformTerm{{"astra linux", 1}, {"linux", 1}, {"windows", 1}, {"macos", 1}, {"mac os", 1},
	{"ред ос", 1}, {"альт", 1}, {"роса", 1}, {"android", 1}, {"эльбрус", 1}}

// titlePlatform — платформа из фразы «для …» в названии ("" — не названа).
func titlePlatform(title string) string {
	low := strings.ReplaceAll(strings.ToLower(title), "ё", "е")
	for _, w := range skuPlatformWords {
		if p, n := forPhrase(low, low, []platformTerm{w}); n > 0 && p != "" {
			return w.term
		}
	}
	return ""
}

// titlePurpose — назначение по названию: рабочее место / рабочая станция или сервер.
func titlePurpose(title string) map[string]bool {
	low := foldText(title)
	res := map[string]bool{}
	if strings.Contains(low, "рабоч") {
		res["рабочая станция"] = true
	}
	if strings.Contains(low, "сервер") {
		res["сервер"] = true
	}
	return res
}

func splitSet(v string) map[string]bool {
	res := map[string]bool{}
	for _, x := range strings.Split(v, ",") {
		if x = strings.ToLower(strings.TrimSpace(x)); x != "" {
			res[x] = true
		}
	}
	return res
}

// overlap — у множеств есть общий элемент; пустое множество не противоречит ничему (true).
func overlap(a, b map[string]bool) bool {
	if len(a) == 0 || len(b) == 0 {
		return true
	}
	for k := range a {
		if b[k] {
			return true
		}
	}
	return false
}

// offerFit — совпадение позиции-кандидата с якорем внутри семейства: отказ с причиной или число совпавших
// признаков (редакция, назначение, размер организации, архитектура). Признак, которого нет у одной из сторон, не противоречит.
func offerFit(anchor, cand indexDoc) (int, string, string) {
	score := 0
	if av, cv := titleVersions(anchor.Title), titleVersions(cand.Title); !overlap(av, cv) {
		return 0, upRejectVersion, "версия в названии другая"
	}
	// Кандидат называет платформу, которой нет у якоря: «… для Astra Linux SE» к «КриптоПро CSP 5.0» — не «к этой
	// лицензии». Обратное (у якоря «для рабочей станции Windows», у сертификата сопровождения платформы нет) — не отказ.
	if ap, cp := titlePlatform(anchor.Title), titlePlatform(cand.Title); cp != "" && ap != cp {
		return 0, upRejectPlatform, "платформа в названии «" + cp + "» ≠ «" + ap + "»"
	}
	ae, ce := strings.ToLower(anchor.LicenseEdition), strings.ToLower(cand.LicenseEdition)
	switch {
	case ae != "" && ce != "" && ae != ce:
		return 0, upRejectEdition, "редакция " + cand.LicenseEdition + " ≠ " + anchor.LicenseEdition
	case ae != "" && ae == ce:
		score++
	}
	ate, cte := titleEditions(anchor.Title), titleEditions(cand.Title)
	if !overlap(ate, cte) {
		return 0, upRejectEdition, "в названии другая редакция"
	} else if len(ate) > 0 && len(cte) > 0 {
		score++
	}
	if anchor.LicenseOrgType != "" && cand.LicenseOrgType != "" && anchor.LicenseOrgType != cand.LicenseOrgType {
		return 0, upRejectOrg, "тип организации " + cand.LicenseOrgType + " ≠ " + anchor.LicenseOrgType
	}
	aa, ca := docAttrs(anchor), docAttrs(cand)
	ap, cp := splitSet(aa["Назначение"]), splitSet(ca["Назначение"])
	if len(ap) == 0 || len(cp) == 0 {
		ap, cp = titlePurpose(anchor.Title), titlePurpose(cand.Title) // назначения в атрибутах нет — по названию
	}
	if !overlap(ap, cp) {
		return 0, upRejectPurpose, "назначение «" + ca["Назначение"] + "» ≠ «" + aa["Назначение"] + "»"
	} else if len(ap) > 0 && len(cp) > 0 {
		score++
	}
	as, cs := splitSet(aa["Размер организации"]), splitSet(ca["Размер организации"])
	if !overlap(as, cs) {
		return 0, upRejectOrgSize, "размер организации «" + ca["Размер организации"] + "» ≠ «" + aa["Размер организации"] + "»"
	} else if len(as) > 0 && len(cs) > 0 {
		score++
	}
	ar, cr := splitSet(aa["Совместимая архитектура/платформа"]), splitSet(ca["Совместимая архитектура/платформа"])
	if !overlap(ar, cr) {
		return 0, upRejectArch, "архитектура " + ca["Совместимая архитектура/платформа"]
	} else if len(ar) > 0 && len(cr) > 0 {
		score++
	}
	return score, "", ""
}

// ---------- сборка блока ----------

type upsellCand struct {
	pid, fid string
	kind     string // evKind*
	level    string
	label    string
	class    string
	compat   string
	reason   string
	evTitle  string
	evURL    string
	docNo    string
	source   string // id записи compat_docs
	rank     int    // порядок внутри вида
}

type upsellRun struct {
	ix       *upsellIndex
	lx       *searchLexicon
	anchor   indexDoc
	shown    map[string]bool // семейства выдачи и блока
	step     *UpsellStep
	rejected map[string]bool
}

func (r *upsellRun) reject(pid, reason, detail string) {
	if r.rejected[pid] || len(r.step.Rejected) >= upsellMaxRejected {
		return
	}
	r.rejected[pid] = true
	r.step.Rejected = append(r.step.Rejected, domain.RejectedHit{ProductID: pid, Reason: reason, Detail: detail})
}

// licenseKindCands — техподдержка, продление, модуль, льготный переход внутри семейства якоря.
func (r *upsellRun) licenseKindCands() []upsellCand {
	type spec struct {
		kind, label, class, reason string
	}
	specs := []spec{
		{domain.LicenseKindSupport, "Техподдержка", CompanionRenewal, "Техподдержка к этой лицензии"},
		{domain.LicenseKindRenewal, "Продление", CompanionRenewal, "Продление для действующей лицензии"},
		{domain.LicenseKindModule, "Модуль", CompanionComplement, "Модуль к этой редакции"},
		{domain.LicenseKindMigration, "Льготный переход", CompanionComplement,
			"Льготный переход: в каталоге — лицензия типа «Миграция»"},
	}
	var res []upsellCand
	for i, sp := range specs {
		bestPID, bestScore, bestDur := "", -1, -1
		var bestPrice float64
		for _, pid := range r.ix.buyable[r.anchor.FamilyID] {
			d := r.lx.docs[pid]
			if d.LicenseKind != sp.kind || pid == r.anchor.ProductID {
				continue
			}
			score, why, detail := offerFit(r.anchor, d)
			if why != "" {
				r.reject(pid, why, detail)
				continue
			}
			dur := 0
			if d.LicenseDuration != nil && *d.LicenseDuration == 12 {
				dur = 1
			}
			price := *docBuyPrice(d)
			if score > bestScore || score == bestScore && (dur > bestDur || dur == bestDur && price < bestPrice) {
				bestPID, bestScore, bestDur, bestPrice = pid, score, dur, price
			}
		}
		if bestPID == "" {
			continue
		}
		reason := sp.reason + editionNote(r.anchor)
		res = append(res, upsellCand{pid: bestPID, fid: r.anchor.FamilyID, kind: evKindLicense, level: domain.EvidenceB,
			label: sp.label, class: sp.class, compat: domain.CompatVerified, reason: reason,
			evTitle: "Каталог: тип лицензии «" + licenseKindLabels[sp.kind] + "» в семействе " + r.anchor.FamilyName,
			evURL:   firstURL(r.anchor.FamilyURL, r.lx.docs[bestPID].OfferURL), rank: i})
	}
	return res
}

// editionNote — уточнение к причине: редакция и назначение якоря («Орёл», рабочая станция).
func editionNote(a indexDoc) string {
	var parts []string
	if a.LicenseEdition != "" {
		parts = append(parts, "«"+a.LicenseEdition+"»")
	}
	if p := docAttrs(a)["Назначение"]; p != "" && !strings.Contains(p, ",") {
		parts = append(parts, strings.ToLower(p))
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

func firstURL(xs ...string) string {
	for _, x := range xs {
		if strings.HasPrefix(x, "http") {
			return x
		}
	}
	return ""
}

// compatCands — совместимость с документом: якорь — платформа (продукт работает на ней) или продукт (платформа,
// на которой он работает). Уровень — из записи (A подтверждает версию, C — проверить совместимость).
func (r *upsellRun) compatCands() []upsellCand {
	var res []upsellCand
	add := func(i int, targets []string, asPlatform bool) {
		cd := r.ix.compat[i]
		var vers []string
		if asPlatform {
			for v := range titleVersions(cd.ProductVersion + " " + cd.ProductLabel) {
				vers = append(vers, v)
			}
			sort.Strings(vers)
		}
		// Одна позиция на запись — лучшая среди всех семейств записи (KES: Стандартный / Расширенный / Total).
		pid := r.pickOffer(targets, asPlatform, vers...)
		if pid == "" {
			r.step.Considered = append(r.step.Considered, UpsellConsidered{Kind: evKindCompat, Level: cd.Level, Source: cd.ID})
			r.reject("compat:"+cd.ID, upRejectNoPrice, "нет позиции, которую можно купить сейчас, без чужой платформы в названии")
			return
		}
		c := upsellCand{pid: pid, fid: r.lx.docs[pid].FamilyID, kind: evKindCompat, level: cd.Level, source: cd.ID,
			evURL: cd.URL, docNo: cd.DocNumber, evTitle: docTitle(cd), rank: len(res)}
		if cd.Level == domain.EvidenceA {
			c.label, c.class, c.compat = "Совместимо по данным вендора", CompanionCompatible, domain.CompatVerified
			c.reason = compatReason(cd, asPlatform)
		} else {
			c.label, c.class, c.compat = "Проверьте совместимость", CompanionComplement, domain.CompatUnknown
			c.reason = "Решает смежную задачу. Проверьте совместимость — " + docTitle(cd)
		}
		res = append(res, c)
	}
	for _, i := range r.ix.compatByPlatform[r.anchor.FamilyID] {
		add(i, r.ix.familiesNamed(r.ix.compat[i].ProductFamilies), true)
	}
	for _, i := range r.ix.compatByProduct[r.anchor.FamilyID] {
		add(i, r.ix.familiesNamed(r.ix.compat[i].PlatformFamilies), false)
	}
	// Уровень A раньше C, внутри — порядок записей.
	sort.SliceStable(res, func(i, j int) bool { return res[i].level < res[j].level })
	return res
}

// docTitle — подпись документа без даты: «Сертификат Ready for Astra № 29073/2025».
func docTitle(cd domain.CompatDoc) string {
	t := cd.DocLabel
	if t == "" {
		t = cd.DocTitle
	}
	if t == "" {
		t = "документ вендора"
	}
	if cd.DocNumber != "" && !strings.Contains(t, cd.DocNumber) {
		t += " № " + cd.DocNumber
	}
	return t
}

// compatReason — строка причины уровня A: с версиями из документа, без обобщения и без даты.
func compatReason(cd domain.CompatDoc, anchorIsPlatform bool) string {
	ver := func(name, v string) string {
		if v == "" {
			return name
		}
		return name + " " + v
	}
	plat, prod := cd.PlatformLabel, cd.ProductLabel
	if plat == "" {
		plat = ver(cd.Platform, cd.PlatformVersion)
	}
	if prod == "" {
		prod = ver(cd.Product, cd.ProductVersion)
	}
	if anchorIsPlatform {
		return "Совместимо с " + plat + " — " + docTitle(cd)
	}
	return "Платформа для " + prod + ": " + plat + " — " + docTitle(cd)
}

// pickOffer — позиция для карточки среди семейств fids: можно купить сейчас; выше — право использования (не
// медиапакет), без чужой платформы в названии («для рабочей станции Windows» под записью о Linux — нет), с версией из
// документа, базовая лицензия, с именем платформы якоря (если preferPlatform), коммерческая (или тип организации
// якоря), дешевле. "" — купить нечего.
func (r *upsellRun) pickOffer(fids []string, preferPlatform bool, versions ...string) string {
	var named []string
	own := map[string]bool{}
	if preferPlatform {
		for _, t := range r.ix.platformTerms(r.anchor.FamilyID) {
			own[t.term] = true
			if t.weight > 1 {
				named = append(named, t.term)
			}
		}
	}
	foreign := func(low string) bool {
		tp := titlePlatform(low)
		if tp == "" || !preferPlatform {
			return false
		}
		for t := range own {
			if strings.Contains(t, tp) || strings.Contains(tp, t) {
				return false
			}
		}
		return true
	}
	org := r.anchor.LicenseOrgType
	if org == "" {
		org = "commercial"
	}
	best, bestKey := "", []float64(nil)
	for _, fid := range fids {
		if fid == r.anchor.FamilyID {
			continue
		}
		for _, pid := range r.ix.buyable[fid] {
			d := r.lx.docs[pid]
			low := r.ix.lower[pid]
			key := make([]float64, 8)
			key[7] = -*docBuyPrice(d)
			if !notLicense(low) {
				key[0] = 1
			}
			if !foreign(low) {
				key[1] = 1
			}
			for _, v := range versions {
				if v != "" && titleVersions(low)[v] {
					key[2] = 1
				}
			}
			if d.LicenseKind == domain.LicenseKindBase || d.LicenseKind == "" {
				key[3] = 1
			}
			for _, t := range named {
				if strings.Contains(low, t) {
					key[4] = 1
				}
			}
			if d.LicenseOrgType == "" || d.LicenseOrgType == org {
				key[5] = 1
			}
			if len(fids) > 1 {
				key[6] = -float64(upIndexOf(fids, fid)) // при равенстве — порядок семейств записи
			}
			if best == "" || lessKey(bestKey, key) {
				best, bestKey = pid, key
			}
		}
	}
	if best != "" && bestKey[1] == 0 {
		return "" // все позиции названы для другой платформы — карточку не показываем
	}
	return best
}

func upIndexOf(xs []string, x string) int {
	for i, y := range xs {
		if y == x {
			return i
		}
	}
	return len(xs)
}

// notLicense — позиция — носитель или документация, а не право использования.
func notLicense(low string) bool {
	for _, w := range []string{"media pack", "медиапакет", "медиа пакет", "дистрибутив", "комплект документации", "формуляр"} {
		if strings.Contains(low, w) {
			return true
		}
	}
	return false
}

func lessKey(a, b []float64) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// platformTerm — как платформа якоря пишется в названиях SKU: своё имя (вес 2) или семейство ОС (linux, вес 1).
type platformTerm struct {
	term   string
	weight int
}

// platformGeneric — служебные слова названия ОС, которые в SKU не пишут.
var platformGeneric = map[string]bool{"операционная": true, "система": true, "программа": true, "платформа": true}

// isPlatform — семейство — платформа (ОС): так помечено в записях совместимости или категория — операционные
// системы, а в названии есть «linux» / «ос».
func (ix *upsellIndex) isPlatform(fid string) bool {
	if len(ix.compatByPlatform[fid]) > 0 {
		return true
	}
	if !strings.Contains(strings.ToLower(ix.lx.categoryName[ix.lx.familyCat[fid]]), "операционн") {
		return false
	}
	for _, w := range wordsOf(ix.lx.familyName[fid]) {
		if w == "linux" || w == "ос" || w == "os" {
			return true
		}
	}
	return false
}

func (ix *upsellIndex) platformTerms(fid string) []platformTerm {
	if !ix.isPlatform(fid) {
		return nil
	}
	var words []string
	for _, w := range wordsOf(ix.lx.familyName[fid]) {
		if !platformGeneric[w] {
			words = append(words, w)
		}
	}
	var res []platformTerm
	if len(words) >= 2 {
		res = append(res, platformTerm{term: words[0] + " " + words[1], weight: 2})
	} else if len(words) == 1 && len([]rune(words[0])) >= 4 {
		res = append(res, platformTerm{term: words[0], weight: 2})
	}
	linux := false
	for _, w := range words {
		linux = linux || w == "linux"
	}
	if !linux {
		n, l := 0, 0
		for _, pid := range ix.buyable[fid] {
			n++
			if strings.Contains(strings.ToLower(ix.lx.docs[pid].Description), "платформа: linux") {
				l++
			}
		}
		linux = n > 0 && 2*l > n
	}
	if linux {
		res = append(res, platformTerm{term: "linux", weight: 1})
	}
	return res
}

// platformVariant — вариант SKU другого вендора, прямо называющий платформу якоря.
type platformVariant struct {
	pid, fid, phrase string
	weight           int
}

// solutionCategoryHint — к ОС в первую очередь — защита, антивирус, резервное копирование, криптография: при равном
// весе фразы эти категории выше (прикладные программы «для Linux» — после).
var solutionCategoryHint = []string{"защиты информации", "антивирус", "резервного копирования",
	"информационной безопасности", "криптограф"}

// platformVariants — по одному варианту на семейство других вендоров; кэш на семейство-платформу.
func (ix *upsellIndex) platformVariants(fid string) []platformVariant {
	if v, ok := ix.platformCache.Load(fid); ok {
		return v.([]platformVariant)
	}
	terms := ix.platformTerms(fid)
	vendor := ix.lx.familyVendor[fid]
	best := map[string]platformVariant{}
	if len(terms) > 0 {
		for pid, low := range ix.lower {
			d := ix.lx.docs[pid]
			if d.FamilyID == fid || ix.lx.familyVendor[d.FamilyID] == vendor || !strings.Contains(low, "для ") {
				continue
			}
			phrase, w := forPhrase(d.Title, low, terms)
			if w == 0 || w == 1 && !ix.solutionCategory(d.FamilyID) {
				continue // «для Linux» у прикладной программы — не про решение на этой ОС
			}
			cur, ok := best[d.FamilyID]
			better := !ok || w > cur.weight
			if ok && w == cur.weight {
				// В семействе — базовая лицензия, коммерческая, дешевле.
				better = lessKey(variantKey(ix.lx.docs[cur.pid]), variantKey(d))
			}
			if better {
				best[d.FamilyID] = platformVariant{pid: pid, fid: d.FamilyID, phrase: phrase, weight: w}
			}
		}
	}
	res := make([]platformVariant, 0, len(best))
	for _, v := range best {
		res = append(res, v)
	}
	hint := func(v platformVariant) int {
		if ix.solutionCategory(v.fid) {
			return 1
		}
		return 0
	}
	sort.Slice(res, func(i, j int) bool {
		a, b := res[i], res[j]
		if a.weight != b.weight {
			return a.weight > b.weight
		}
		if ha, hb := hint(a), hint(b); ha != hb {
			return ha > hb
		}
		if na, nb := len(ix.buyable[a.fid]), len(ix.buyable[b.fid]); na != nb {
			return na < nb // узкое семейство — точнее про платформу, чем широкий прикладной пакет
		}
		return a.fid < b.fid
	})
	ix.platformCache.Store(fid, res)
	return res
}

// solutionCategory — категория семейства — защита информации, антивирус, резервное копирование, криптография.
func (ix *upsellIndex) solutionCategory(fid string) bool {
	cat := strings.ToLower(ix.lx.categoryName[ix.lx.familyCat[fid]])
	for _, h := range solutionCategoryHint {
		if strings.Contains(cat, h) {
			return true
		}
	}
	return false
}

func variantKey(d indexDoc) []float64 {
	k := []float64{0, 0, 0}
	if d.LicenseKind == domain.LicenseKindBase || d.LicenseKind == "" {
		k[0] = 1
	}
	if d.LicenseOrgType == "" || d.LicenseOrgType == "commercial" {
		k[1] = 1
	}
	if p := docBuyPrice(d); p != nil {
		k[2] = -*p
	}
	return k
}

// forPhrase — фраза «для …» из названия, если в ней названа платформа: текст фразы в регистре оригинала и вес.
func forPhrase(title, low string, terms []platformTerm) (string, int) {
	bestW, bestPhrase := 0, ""
	for from := 0; ; {
		i := strings.Index(low[from:], "для ")
		if i < 0 {
			break
		}
		start := from + i
		from = start + len("для ")
		if start > 0 {
			if r := rune(low[start-1]); r != ' ' && r != '(' && r != ',' {
				continue
			}
		}
		seg := low[from:]
		end := len(seg)
		for _, stop := range []string{",", "(", ")", ";", " - ", " – ", " — ", "."} {
			if k := strings.Index(seg, stop); k >= 0 && k < end {
				end = k
			}
		}
		if end > 60 {
			end = 60
			for end > 0 && !utf8Start(seg[end]) {
				end--
			}
		}
		seg = seg[:end]
		for _, t := range terms {
			if t.weight > bestW && containsWord(seg, t.term) {
				orig := seg
				if len(title) == len(low) {
					orig = title[from : from+end]
				}
				bestW, bestPhrase = t.weight, strings.TrimRight(strings.TrimSpace(orig), " -–—")
			}
		}
	}
	return bestPhrase, bestW
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }

// containsWord — term в s целыми словами.
func containsWord(s, term string) bool {
	for from := 0; ; {
		i := strings.Index(s[from:], term)
		if i < 0 {
			return false
		}
		a, b := from+i, from+i+len(term)
		okL := a == 0 || !isWordByte(s, a-1, true)
		okR := b == len(s) || !isWordByte(s, b, false)
		if okL && okR {
			return true
		}
		from = a + 1
	}
}

// isWordByte — у границы стоит буква или цифра (для UTF-8 — по руне на границе).
func isWordByte(s string, i int, before bool) bool {
	var r rune
	if before {
		j := i
		for j > 0 && !utf8Start(s[j]) {
			j--
		}
		r = []rune(s[j : i+1])[0]
	} else {
		r = []rune(s[i:])[0]
	}
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func (r *upsellRun) platformCands() []upsellCand {
	var res []upsellCand
	for i, v := range r.ix.platformVariants(r.anchor.FamilyID) {
		if i >= 12 {
			break
		}
		res = append(res, upsellCand{pid: v.pid, fid: v.fid, kind: evKindPlatform, level: domain.EvidenceB,
			label: "Вариант под платформу", class: CompanionCompatible, compat: domain.CompatConditional,
			reason:  "Вариант для " + v.phrase,
			evTitle: "Каталог: название позиции называет платформу («для " + v.phrase + "»)",
			evURL:   r.lx.docs[v.pid].OfferURL, rank: i})
	}
	return res
}

// ecosystemCands — доступные семейства того же вендора из других категорий.
func (r *upsellRun) ecosystemCands() []upsellCand {
	vendor := r.lx.familyVendor[r.anchor.FamilyID]
	cat := r.lx.familyCat[r.anchor.FamilyID]
	fams := append([]string(nil), r.ix.vendorFams[vendor]...)
	sort.SliceStable(fams, func(i, j int) bool { return len(r.ix.buyable[fams[i]]) > len(r.ix.buyable[fams[j]]) })
	name := r.anchor.Vendor
	if name == "" {
		name = r.lx.vendorName[vendor]
	}
	quoted := name
	if !strings.Contains(name, "«") {
		quoted = "«" + name + "»"
	}
	var res []upsellCand
	for _, fid := range fams {
		if fid == r.anchor.FamilyID || r.lx.familyCat[fid] == cat {
			continue
		}
		pid := r.pickOffer([]string{fid}, false)
		if pid == "" {
			continue
		}
		res = append(res, upsellCand{pid: pid, fid: fid, kind: evKindEcosystem, level: domain.EvidenceB,
			label: "Экосистема вендора", class: CompanionComplement, compat: domain.CompatUnknown,
			reason:  "Из экосистемы " + quoted,
			evTitle: "Каталог: другие продукты вендора " + name,
			evURL:   firstURL(r.anchor.VendorURL, r.lx.docs[pid].FamilyURL), rank: len(res)})
		if len(res) >= 6 {
			break
		}
	}
	return res
}

// ---------- шаг поиска ----------

// feedUpsellStage — блок «Дополните решение» и акции на фиде. Срабатывает, когда mock-подбор (companions.go)
// блока не дал, а каталог — фид (у позиций есть страницы на сайте). Сбой не ломает поиск: шаг с error.
func (s *Services) feedUpsellStage(lx *searchLexicon, analog bool, resp *out.SearchResponse) (step *UpsellStep) {
	t := time.Now()
	step = &UpsellStep{StepBase: domain.StepBase{Name: StepUpsell}, Considered: []UpsellConsidered{},
		Rejected: []domain.RejectedHit{}, Promos: []string{}}
	defer func() {
		if rec := recover(); rec != nil {
			step = &UpsellStep{StepBase: domain.StepBase{Name: StepUpsell, Reason: "error"}, Error: fmt.Sprint(rec),
				Considered: []UpsellConsidered{}, Rejected: []domain.RejectedHit{}, Promos: []string{}}
		}
		if step != nil {
			step.Ms = msSince(t)
		}
	}()
	if lx == nil || len(resp.Items) == 0 || lx.docs[resp.Items[0].ProductID].OfferURL == "" {
		return nil // не фид (mock) или пусто: шага нет, выдача как раньше
	}
	ix := s.upsellIndexFor(lx)
	s.applyPromos(ix, resp, step)
	discountFromVariants(resp)
	if resp.Companions != nil {
		step.Skipped, step.Reason = true, "блок собран по связям справочника"
		return step
	}
	idx, conf, why := upsellAnchor(resp, analog)
	if idx < 0 {
		step.Skipped, step.Reason = true, why
		return step
	}
	it := resp.Items[idx]
	anchorPID := anchorOffer(it, resp.Query)
	a, ok := lx.docs[anchorPID]
	if !ok {
		step.Skipped, step.Reason = true, "позиции якоря нет в словаре"
		return step
	}
	step.Anchor = &out.CompanionAnchor{ProductID: anchorPID, FamilyID: it.FamilyID, Confidence: conf}
	step.AnchorOffer = a.Title
	r := &upsellRun{ix: ix, lx: lx, anchor: a, shown: map[string]bool{}, step: step, rejected: map[string]bool{}}
	for _, x := range resp.Items {
		r.shown[x.FamilyID] = true
	}
	items := r.assemble()
	for i := range items {
		items[i].Promos = ix.promosFor(items[i].FamilyID, []string{items[i].Title}, s.now())
	}
	if len(items) == 0 {
		step.Reason = "связей у якоря нет: ни типа лицензии, ни документа, ни варианта под платформу"
		return step
	}
	resp.Companions = &out.CompanionBlock{Title: s.upsellBlockTitle(), Anchor: *step.Anchor, Items: items}
	return step
}

// assemble — кандидаты всех видов в блок: тип лицензии, документ, платформа, экосистема; по одному семейству.
func (r *upsellRun) assemble() []out.Companion {
	lic := r.licenseKindCands()
	cmp := r.compatCands()
	plat := r.platformCands()
	eco := r.ecosystemCands()
	var picked []upsellCand
	inBlock := map[string]bool{}
	cats := map[string]bool{} // категории карточек других семейств: вариант под платформу и экосистема — по одной на категорию
	take := func(cs []upsellCand, max int, allowShown, onePerCat bool) {
		n := 0
		for _, c := range cs {
			r.step.Considered = append(r.step.Considered, UpsellConsidered{ProductID: c.pid, FamilyID: c.fid,
				Kind: c.kind, Level: c.level, Source: c.source})
			switch {
			case inBlock[c.fid] && c.fid != r.anchor.FamilyID:
				r.reject(c.pid, upRejectShown, "семейство уже в блоке")
				continue
			case !allowShown && r.shown[c.fid]:
				r.reject(c.pid, upRejectShown, "семейство уже в выдаче")
				continue
			case onePerCat && cats[r.lx.familyCat[c.fid]]:
				r.reject(c.pid, upRejectCategory, "в блоке уже есть карточка этой категории")
				continue
			case n >= max || len(picked) >= upsellMax:
				r.reject(c.pid, upRejectLimit, fmt.Sprintf("в блоке не больше %d карточек этого вида", max))
				continue
			}
			if c.fid != r.anchor.FamilyID {
				inBlock[c.fid] = true
				cats[r.lx.familyCat[c.fid]] = true
			}
			picked = append(picked, c)
			n++
		}
	}
	take(lic, upsellMaxLicense, true, false)
	// Документ — основание сильнее, чем вид в выдаче: семейство из выдачи с подтверждённой совместимостью остаётся.
	// Документы — по одному на категорию: KES и Dr.Web к Astra — оба антивирусы, второй уступает место бэкапу.
	before := len(picked)
	take(cmp, upsellMaxCompat, true, true)
	platMax := upsellMaxPlatform
	if len(picked)-before >= 2 {
		platMax = 1 // документов достаточно — вариантов под платформу меньше
	}
	take(plat, platMax, false, true)
	ecoMax := upsellMaxEcosystem
	if len(picked) >= 4 {
		ecoMax = 1
	}
	take(eco, ecoMax, false, true)
	shown := map[string]bool{}
	res := make([]out.Companion, 0, len(picked))
	for _, c := range picked {
		shown[c.pid] = true
		res = append(res, r.companionOf(c))
	}
	for i := range r.step.Considered {
		r.step.Considered[i].Shown = shown[r.step.Considered[i].ProductID]
	}
	return res
}

func (r *upsellRun) companionOf(c upsellCand) out.Companion {
	d := r.lx.docs[c.pid]
	price := docBuyPrice(d)
	mode := domain.PriceFixed
	if price == nil {
		mode = domain.PriceQuoteRequired
	}
	source := "catalog_rule"
	if c.kind == evKindCompat {
		source = "vendor_documentation"
	}
	return out.Companion{ProductID: c.pid, FamilyID: c.fid, Title: d.Title, Vendor: d.Vendor, Category: d.Category,
		LicenseLine: licenseLine(d), PriceRub: price, PriceMode: mode, Class: c.class, Compatibility: c.compat,
		Reason: c.reason, Conditions: []string{}, EvidenceLevel: c.level, Source: source, SourceURL: c.evURL,
		Label: c.label, UpgradeAdds: []string{}, OfferURL: d.OfferURL, FamilyURL: d.FamilyURL,
		Evidence: &out.CompanionEvidence{Level: c.level, Kind: c.kind, Title: c.evTitle, URL: c.evURL, DocNumber: c.docNo}}
}

// upsellAnchor — карточка-якорь: первая точная карточка, когда цель — названный товар (семейство или SKU,
// уверенно) или вендор / вендор + категория (средне). Категория, справка, аналог, уточнение — без якоря.
func upsellAnchor(resp *out.SearchResponse, analog bool) (int, string, string) {
	switch {
	case analog:
		return -1, "", "запрос аналога"
	case resp.Clarify != nil:
		return -1, "", "запрос неоднозначный — нужно уточнение"
	case resp.Target == nil:
		return -1, "", "цели ответа нет"
	}
	conf := "high"
	switch resp.Target.Type {
	case out.TargetFamily, out.TargetSKU:
	case out.TargetVendor, out.TargetVendorCategory:
		conf = "medium"
	default:
		return -1, "", "цель — " + resp.Target.Type + ": товар не назван"
	}
	for i, it := range resp.Items {
		if it.MatchType == out.MatchExact {
			return i, conf, ""
		}
	}
	return -1, "", "точной карточки нет"
}

// anchorOffer — позиция якоря: самая дешёвая базовая лицензия карточки, которую можно купить и которая проходит
// условия запроса (её цена — «от» на карточке); нет — позиция карточки.
func anchorOffer(it out.SearchItem, query string) string {
	var nums []string
	for _, w := range strings.Fields(foldText(query)) {
		if strings.ContainsAny(w, "0123456789") {
			nums = append(nums, w)
		}
	}
	best, bestNums, bestPrice := "", -1, math.Inf(1)
	consider := func(x out.SearchItem) {
		if x.LicenseKind != "" && x.LicenseKind != domain.LicenseKindBase || !x.MatchesQuery {
			return
		}
		p := itemPrice(x)
		if p == nil {
			return
		}
		// Числа запроса («5.0») — в названии позиции: якорь «КриптоПро CSP 5.0», а не 4.0 той же карточки.
		n := 0
		low := foldText(x.Title)
		for _, w := range nums {
			if containsWord(low, w) {
				n++
			}
		}
		if n > bestNums || n == bestNums && *p < bestPrice {
			best, bestNums, bestPrice = x.ProductID, n, *p
		}
	}
	consider(it)
	for _, v := range it.Variants {
		consider(v)
	}
	if best == "" {
		return it.ProductID
	}
	return best
}

// discountFromVariants — бейдж «Скидка N%» по old_price у позиции, которая задаёт цену «от» карточки, если у
// головной позиции скидки нет: карточка Astra Linux SE «от 3 100 ₽» показывает скидку «Орла».
func discountFromVariants(resp *out.SearchResponse) {
	for i := range resp.Items {
		it := &resp.Items[i]
		if len(it.Badges) > 0 || it.PriceFrom == nil {
			continue
		}
		for _, v := range append([]out.SearchItem{*it}, it.Variants...) {
			p := itemPrice(v)
			if p == nil || *p != *it.PriceFrom || v.OldPriceRub == nil || *v.OldPriceRub <= *p || !v.MatchesQuery {
				continue
			}
			if n := int(math.Round((*v.OldPriceRub - *p) / *v.OldPriceRub * 100)); n >= 1 {
				it.Badges = append(it.Badges, "Скидка "+strconv.Itoa(n)+"%")
				break
			}
		}
	}
}

// ---------- акции ----------

// promosFor — действующие акции семейства; при offer_match — если его называет хотя бы одно из названий.
func (ix *upsellIndex) promosFor(fid string, titles []string, now time.Time) []out.ItemPromo {
	var res []out.ItemPromo
	for _, p := range ix.promoByFamily[fid] {
		if !p.ActiveOn(now) {
			continue
		}
		if len(p.OfferMatch) > 0 && !anyTitleHas(titles, p.OfferMatch) {
			continue
		}
		res = append(res, out.ItemPromo{ID: p.ID, Title: p.Title, Text: p.Text, URL: p.URL})
	}
	return res
}

func anyTitleHas(titles, subs []string) bool {
	for _, t := range titles {
		lt := strings.ToLower(t)
		for _, s := range subs {
			if strings.Contains(lt, strings.ToLower(s)) {
				return true
			}
		}
	}
	return false
}

// applyPromos — акции на карточках выдачи (по семейству; offer_match — по названиям позиций карточки).
func (s *Services) applyPromos(ix *upsellIndex, resp *out.SearchResponse, step *UpsellStep) {
	now := s.now()
	for i := range resp.Items {
		it := &resp.Items[i]
		titles := []string{it.Title}
		for _, v := range it.Variants {
			titles = append(titles, v.Title)
		}
		it.Promos = ix.promosFor(it.FamilyID, titles, now)
		for _, p := range it.Promos {
			step.Promos = append(step.Promos, p.ID)
		}
	}
}
