package services

import (
	"encoding/json"
	"fmt"
	"strings"

	"kuznica/api/domain"
)

// validate_license (SPEC §4, KNOWLEDGE.md «Правила лицензирования»): оффер против условий покупателя. Правила
// кода — вид позиции под операцию, тип организации, срок, область действия под объект, ступень количества,
// вариант; правила базы знаний — when / forbid / require из license_rules. Правило в статусе draft применяется
// (не показать неподходящее безопаснее), но его текст покупателю не идёт.

// guideViolation — нарушенное правило.
type guideViolation struct {
	RuleID  string
	Message string // текст для покупателя
	Source  string
	Status  string // verified | draft | code
}

// Коды правил кода.
const (
	ruleCodeOperation = "code_operation"
	ruleCodeOrg       = "code_org_type"
	ruleCodeTerm      = "code_term"
	ruleCodeScope     = "code_scope"
	ruleCodeTier      = "code_quantity_in_tier"
	ruleCodeVariant   = "code_variant"
	ruleCodeUnit      = "code_unit_unknown"
)

// licenseTypeRU — вид позиции каталога → «Тип лицензии» в правилах базы знаний.
var licenseTypeRU = map[string]string{
	domain.LicenseKindRenewal:   "Продление",
	domain.LicenseKindUpgrade:   "Обновление",
	domain.LicenseKindMigration: "Миграция",
	domain.LicenseKindBase:      "Полная версия",
	domain.LicenseKindSupport:   "Техподдержка",
	domain.LicenseKindModule:    "Модуль",
}

// guideTermPolicy — как проверять срок: exact — только названный срок; any — любой (срок по умолчанию, а в
// семействе нет позиций на этот срок).
type guideTermPolicy int

const (
	termExact guideTermPolicy = iota
	termAny
	// termDefault — срок по умолчанию: названный срок или бессрочная (бессрочная не «другой срок», а без срока).
	termDefault
)

// validateLicense — подходит ли оффер под условия; пустой список нарушений — подходит.
func validateLicense(v guideOfferView, r domain.GuideRequirements, kb *guideKB, term guideTermPolicy) []guideViolation {
	var bad []guideViolation
	code := func(id, msg string) { bad = append(bad, guideViolation{RuleID: id, Message: msg, Status: "code"}) }

	switch r.Operation {
	case "renewal":
		if v.kind != domain.LicenseKindRenewal {
			code(ruleCodeOperation, "Это не продление")
		}
	default:
		if v.kind != domain.LicenseKindBase && v.kind != "" {
			code(ruleCodeOperation, "Для новой покупки нужна полная лицензия, а не "+strings.ToLower(licenseTypeRU[v.kind]))
		}
	}
	if org := r.OrgType; org != "" && v.org != "" && v.org != org {
		code(ruleCodeOrg, "Лицензия для другого типа организации: "+orgLabel(v.org))
	}
	if term != termAny && r.TermMonths > 0 && v.term != r.TermMonths && !(term == termDefault && v.term == 0) {
		code(ruleCodeTerm, "Срок "+v.termLabel()+", а нужен "+termWords(r.TermMonths))
	}
	kind, n := mainObject(r)
	if r.Use == "tls_server" {
		kind = "server"
	}
	switch {
	case kind == "server" && v.scope == "workstation":
		code(ruleCodeScope, "Лицензия на рабочее место — для сервера не подходит")
	case kind == "workstation" && (v.scope == "server" || v.scope == "tls_server"):
		code(ruleCodeScope, "Серверная лицензия — для рабочих мест не нужна")
	}
	if kind != "" && n == 0 {
		n = 1 // объект назван без числа — одна единица
	}
	if n > 0 && !v.inTier(n) {
		code(ruleCodeTier, fmt.Sprintf("Ступень %s — не для %d", v.tierLabel(), n))
	}
	if v.scope == "distribution" || v.scope == "unknown" {
		code(ruleCodeScope, "Дистрибутив или позиция без известной области действия — не лицензия на использование")
	}
	if r.Use != "tls_server" && v.scope == "tls_server" {
		code(ruleCodeScope, "Лицензия для TLS-сервера — для обычной установки не нужна")
	}
	if v.platform == "astra_alt_ks3" && !strings.Contains(guideNorm(r.Platform), "astra") && !strings.Contains(guideNorm(r.Platform), "альт") {
		code(ruleCodeScope, "Исполнение для Astra Linux SE / Альт (класс КС3) — для другой ОС не нужно")
	}
	if r.Variant != "" && v.variant != "" && guideNorm(v.variant) != guideNorm(r.Variant) {
		code(ruleCodeVariant, "Другой уровень защищённости: «"+v.variant+"»")
	}
	if v.unit == "pack" && v.pack <= 0 {
		code(ruleCodeUnit, "Неизвестно, на сколько мест пакет")
	}
	if kb != nil {
		for _, rule := range kb.rules {
			if !ruleApplies(rule, v, r) {
				continue
			}
			if ruleViolated(rule, v, r) {
				st := kbDraft
				if rule.show {
					st = kbVerified
				}
				bad = append(bad, guideViolation{RuleID: rule.ID, Message: rule.Message, Source: rule.Source, Status: st})
			}
		}
	}
	return bad
}

// ruleApplies — правило касается семейства оффера и условия when совпали.
func ruleApplies(rule guideRule, v guideOfferView, r domain.GuideRequirements) bool {
	if len(rule.FamilyIDs) > 0 && !containsStr(rule.FamilyIDs, v.p.FamilyID) {
		return false
	}
	if len(rule.FamilyIDs) == 0 && rule.Vendor != "" && rule.Vendor != "*" && rule.Vendor != v.p.VendorID {
		return false
	}
	kind, _ := mainObject(r)
	if r.Use == "tls_server" {
		kind = "server"
	}
	for k, want := range rule.When {
		ws := fmt.Sprint(want)
		if strings.HasSuffix(k, "_not") {
			// «ОС не Windows»: неизвестное значение — правило применяется (осторожно), результат предварительный.
			base := strings.TrimSuffix(k, "_not")
			if base == "os" || base == "server_os" {
				base = "platform"
			}
			if cur := reqField(r, base); cur != "" && strings.EqualFold(cur, ws) {
				return false
			}
			continue
		}
		switch k {
		case "os":
			if !strings.EqualFold(r.Platform, ws) {
				return false
			}
		case "object":
			if !hasObject(r, ws) && kind != ws {
				return false
			}
		case "objects":
			list, _ := want.([]any)
			for _, x := range list {
				if !hasObject(r, fmt.Sprint(x)) {
					return false
				}
			}
		case "use":
			if r.Use != ws {
				return false
			}
		case "platform":
			if !strings.EqualFold(r.Platform, ws) {
				return false
			}
		case "operation":
			op := r.Operation
			if op == "" {
				op = defaultOperation
			}
			if op != ws {
				return false
			}
		case "variant":
			if guideNorm(v.variant) != guideNorm(ws) && guideNorm(r.Variant) != guideNorm(ws) {
				return false
			}
		default:
			return false // условие, которого код не знает, — правило не применяется молча, а не ложно
		}
	}
	return true
}

// ruleViolated — оффер нарушает forbid или require правила.
func ruleViolated(rule guideRule, v guideOfferView, r domain.GuideRequirements) bool {
	for k, val := range rule.Forbid {
		switch k {
		case "license_scope":
			if v.scope != "" && v.scope == fmt.Sprint(val) {
				return true
			}
		case "license_type":
			ru := licenseTypeRU[v.kind]
			attr := ""
			if v.p.Attributes != nil {
				attr = attrString(v.p, "Тип лицензии")
			}
			for _, x := range anyList(val) {
				if x != "" && (x == ru || strings.EqualFold(x, attr)) {
					return true
				}
			}
		}
	}
	for k, val := range rule.Require {
		switch k {
		case "family_id":
			if v.p.FamilyID != fmt.Sprint(val) {
				return true
			}
		case "license_scope":
			if v.scope != fmt.Sprint(val) {
				return true
			}
		case "license_type":
			if fmt.Sprint(val) == "Продление" && v.kind != domain.LicenseKindRenewal {
				return true
			}
		case "users_max":
			if n, ok := val.(float64); ok && objectCount(r, "user") > int(n) {
				return true
			}
		}
	}
	return false
}

func anyList(v any) []string {
	switch x := v.(type) {
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			out = append(out, fmt.Sprint(e))
		}
		return out
	case string:
		return []string{x}
	}
	return nil
}

// attrString — строковый атрибут позиции.
func attrString(p *domain.Product, key string) string {
	m := map[string]any{}
	if err := json.Unmarshal([]byte(p.Attributes), &m); err != nil {
		return ""
	}
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}

// requiredFamilies — семейства, которые правило требует при текущих условиях (TLS-сервер → КриптоПро CSP TLS).
func requiredFamilies(kb *guideKB, r domain.GuideRequirements, candidates []string) []guideRule {
	if kb == nil {
		return nil
	}
	var out []guideRule
	for _, rule := range kb.rules {
		fid, ok := rule.Require["family_id"]
		if !ok {
			continue
		}
		hit := len(rule.FamilyIDs) == 0
		for _, c := range candidates {
			if containsStr(rule.FamilyIDs, c) {
				hit = true
			}
		}
		if !hit {
			continue
		}
		match := true
		for k, want := range rule.When {
			switch k {
			case "use":
				match = match && r.Use == fmt.Sprint(want)
			case "object":
				match = match && hasObject(r, fmt.Sprint(want))
			default:
				match = false
			}
		}
		if match && fmt.Sprint(fid) != "" {
			out = append(out, rule)
		}
	}
	return out
}

// reqField — условие покупателя по имени поля правила.
func reqField(r domain.GuideRequirements, k string) string {
	switch k {
	case "platform":
		return r.Platform
	case "use":
		return r.Use
	case "variant":
		return r.Variant
	case "operation":
		return r.Operation
	}
	return ""
}
