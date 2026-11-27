package services

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"kuznica/api/domain"
	"kuznica/api/out"

	"github.com/google/uuid"
)

// Состояние подбора (SPEC §2): отдельно от истории; у каждого условия — происхождение; изменение условия
// сбрасывает только зависимое. Хранится в памяти процесса по selection_id, 30 минут без действий.

// Происхождение условия.
const (
	originUser     = "user"
	originDefault  = "default"
	originInferred = "inferred"
)

// Значения по умолчанию (SPEC §3): новая лицензия, 12 месяцев, коммерческая организация.
const (
	defaultOperation = "new"
	defaultTerm      = 12
	defaultOrg       = "commercial"
)

const (
	guideSessionTTL = 30 * time.Minute
	guideSessionCap = 5000
)

// newGuideState — новый подбор со значениями по умолчанию.
func newGuideState(feedVersion, kbVersion string) domain.GuideState {
	return domain.GuideState{
		SelectionID: uuid.NewString(), Intent: "license_selection",
		Requirements: domain.GuideRequirements{Operation: defaultOperation, TermMonths: defaultTerm, OrgType: defaultOrg},
		Origin:       map[string]string{"operation": originDefault, "term_months": originDefault, "org_type": originDefault},
		FeedVersion:  feedVersion, KBVersion: kbVersion,
		CandidateFamilyIDs: []string{}, SelectedOfferIDs: []string{}, FactIDs: []string{}, Unresolved: []string{},
	}
}

// guideSession — подбор в памяти: состояние и черновик спецификации.
type guideSession struct {
	state   domain.GuideState
	spec    guideSpecDraft
	touched time.Time
}

// guideSessions — подборы процесса.
type guideSessions struct {
	mu   sync.Mutex
	m    map[string]*guideSession
	now  func() time.Time
	lock sync.Map // selection_id → *sync.Mutex: ходы одного подбора по очереди
}

func (gs *guideSessions) get(id string) (*guideSession, bool) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	s, ok := gs.m[id]
	if !ok || gs.now().Sub(s.touched) > guideSessionTTL {
		return nil, false
	}
	return s, true
}

func (gs *guideSessions) put(s *guideSession) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	if gs.m == nil {
		gs.m = map[string]*guideSession{}
	}
	now := gs.now()
	s.touched = now
	if _, ok := gs.m[s.state.SelectionID]; !ok && len(gs.m) >= guideSessionCap {
		// Сначала выбросить просроченные; не помогло — самый старый.
		oldest, oldestAt := "", now
		for k, v := range gs.m {
			if now.Sub(v.touched) > guideSessionTTL {
				delete(gs.m, k)
				gs.lock.Delete(k)
				continue
			}
			if v.touched.Before(oldestAt) {
				oldest, oldestAt = k, v.touched
			}
		}
		if len(gs.m) >= guideSessionCap && oldest != "" {
			delete(gs.m, oldest)
			gs.lock.Delete(oldest)
		}
	}
	gs.m[s.state.SelectionID] = s
}

// turnLock — замок подбора: два хода одного подбора не идут параллельно.
func (gs *guideSessions) turnLock(id string) *sync.Mutex {
	v, _ := gs.lock.LoadOrStore(id, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// --- условия ---

// objectCount — количество объектов вида kind (0 — не задано).
func objectCount(r domain.GuideRequirements, kind string) int {
	for _, o := range r.Objects {
		if o.Kind == kind {
			return o.Count
		}
	}
	return 0
}

// mainObject — первый объект подбора ("" и 0 — не задан).
func mainObject(r domain.GuideRequirements) (string, int) {
	if len(r.Objects) == 0 {
		return "", 0
	}
	return r.Objects[0].Kind, r.Objects[0].Count
}

// guideDelta — изменения условий из чипа, правил или модели.
type guideDelta struct {
	objects   []domain.GuideObject // число по виду; Count 0 — убрать
	objKind   string               // вид объекта без числа («для сервера»)
	operation string
	term      int
	org       string
	vendor    string
	features  []string
	dropFeat  []string
	platform  string
	use       string
	variant   string
	cheapest  bool
	task      string
	clear     []string // ключи условий, которые убрать (вернуть по умолчанию)
	fromText  bool     // извлечено правилами из текста покупателя (не из ответа модели)
}

func (d guideDelta) empty() bool {
	return len(d.objects) == 0 && d.objKind == "" && d.operation == "" && d.term == 0 && d.org == "" && d.vendor == "" &&
		len(d.features) == 0 && len(d.dropFeat) == 0 && d.platform == "" && d.use == "" && d.variant == "" &&
		!d.cheapest && d.task == "" && len(d.clear) == 0
}

// applyDelta — изменения в состояние: что изменилось (для changes) и сброс зависимого. origin — user | inferred.
func applyDelta(st *domain.GuideState, d guideDelta, origin string) []out.GuideChange {
	var ch []out.GuideChange
	r := &st.Requirements
	resetOffers := func() { st.SelectedOfferIDs = []string{} }
	for _, k := range d.clear {
		switch {
		case k == "count" || strings.HasPrefix(k, "count:"):
			kind := strings.TrimPrefix(strings.TrimPrefix(k, "count"), ":")
			r.Objects = slices.DeleteFunc(r.Objects, func(o domain.GuideObject) bool { return kind == "" || o.Kind == kind })
			delete(st.Origin, k)
			resetOffers()
		case k == "operation":
			r.Operation, st.Origin[k] = defaultOperation, originDefault
			resetOffers()
		case k == "term_months":
			// Снять срок — любой срок, а не снова «1 год по умолчанию».
			r.TermMonths, st.Origin[k] = 0, originUser
			resetOffers()
		case k == "org_type":
			r.OrgType, st.Origin[k] = defaultOrg, originDefault
			resetOffers()
		case k == "vendor":
			r.Vendor, r.Variant = "", ""
			delete(st.Origin, k)
			st.CandidateFamilyIDs = []string{}
			resetOffers()
		case strings.HasPrefix(k, "feature:"):
			f := strings.TrimPrefix(k, "feature:")
			r.RequiredFeatures = slices.DeleteFunc(r.RequiredFeatures, func(x string) bool { return x == f })
			delete(st.Origin, k)
			resetOffers()
		case k == "platform", k == "use", k == "variant":
			setStr(r, k, "")
			delete(st.Origin, k)
			resetOffers()
		}
	}
	for _, o := range d.objects {
		prev := objectCount(*r, o.Kind)
		if o.Count == prev {
			continue
		}
		found := false
		for i := range r.Objects {
			if r.Objects[i].Kind == o.Kind {
				r.Objects[i].Count, found = o.Count, true
			}
		}
		if !found {
			r.Objects = append(r.Objects, o)
		}
		st.Origin["count:"+o.Kind] = origin
		resetOffers()
		if prev > 0 {
			effect := fmt.Sprintf("было %d — ступень и сумма пересчитаны", prev)
			if len(r.Objects) > 1 {
				effect = "пересчитана строка «" + map[bool]string{true: "серверы", false: "рабочие станции"}[o.Kind == "server"] + "», остальные без изменений"
			}
			cond := countLabel(o.Kind, o.Count)
			if r.Task == "webinar" && o.Kind == "user" {
				// Вебинар: участники, а не пользователи; тариф выбирается по вместимости, ступени нет.
				cond = fmt.Sprintf("%d %s", o.Count, guidePlural(o.Count, "участник", "участника", "участников"))
				effect = fmt.Sprintf("было %d — тарифы подобраны заново", prev)
			}
			ch = append(ch, out.GuideChange{Condition: cond, Effect: effect})
		}
	}
	if d.objKind != "" && objectCount(*r, d.objKind) == 0 && !hasObject(*r, d.objKind) {
		r.Objects = append(r.Objects, domain.GuideObject{Kind: d.objKind})
		st.Origin["count:"+d.objKind] = origin
		resetOffers()
	}
	if d.operation != "" && d.operation != r.Operation {
		r.Operation, st.Origin["operation"] = d.operation, origin
		resetOffers()
		ch = append(ch, out.GuideChange{Condition: operationLabel(d.operation), Effect: "офферы подобраны заново"})
	} else if d.operation != "" && origin == originUser && st.Origin["operation"] == originDefault && d.fromText {
		st.Origin["operation"] = origin // покупатель сам назвал то же значение
	}
	if d.term > 0 && d.term != r.TermMonths {
		r.TermMonths, st.Origin["term_months"] = d.term, origin
		resetOffers()
		ch = append(ch, out.GuideChange{Condition: termWords(d.term), Effect: "офферы подобраны заново"})
	} else if d.term > 0 && origin == originUser && st.Origin["term_months"] == originDefault && d.fromText {
		st.Origin["term_months"] = origin
	}
	if d.org != "" && d.org != r.OrgType {
		r.OrgType, st.Origin["org_type"] = d.org, origin
		resetOffers()
	}
	if d.vendor != "" && d.vendor != r.Vendor {
		if r.Vendor != "" {
			// Новый вендор — редакция и кандидаты прошлого вендора не годятся.
			st.CandidateFamilyIDs, r.Variant = []string{}, ""
			ch = append(ch, out.GuideChange{Condition: "производитель", Effect: "редакции подобраны заново"})
		}
		r.Vendor, st.Origin["vendor"] = d.vendor, origin
		resetOffers()
	}
	for _, f := range d.features {
		if !slices.Contains(r.RequiredFeatures, f) {
			r.RequiredFeatures = append(r.RequiredFeatures, f)
			st.Origin["feature:"+f] = origin
			resetOffers()
		}
	}
	for _, f := range d.dropFeat {
		r.RequiredFeatures = slices.DeleteFunc(r.RequiredFeatures, func(x string) bool { return x == f })
		delete(st.Origin, "feature:"+f)
	}
	if d.variant != "" {
		st.Unresolved = slices.DeleteFunc(st.Unresolved, func(x string) bool { return x == "variant" })
	}
	for k, v := range map[string]string{"platform": d.platform, "use": d.use, "variant": d.variant} {
		if v != "" && getStr(*r, k) != v {
			setStr(r, k, v)
			st.Origin[k] = origin
			resetOffers()
		}
	}
	if d.cheapest {
		r.Cheapest = true
	}
	if d.task != "" && r.Task == "" {
		r.Task = d.task
		if st.Origin["task"] == "" {
			st.Origin["task"] = originInferred // направление понято из запроса — видно чипом, можно убрать
		}
	}
	return ch
}

func hasObject(r domain.GuideRequirements, kind string) bool {
	for _, o := range r.Objects {
		if o.Kind == kind {
			return true
		}
	}
	return false
}

func getStr(r domain.GuideRequirements, k string) string {
	switch k {
	case "platform":
		return r.Platform
	case "use":
		return r.Use
	case "variant":
		return r.Variant
	}
	return ""
}

func setStr(r *domain.GuideRequirements, k, v string) {
	switch k {
	case "platform":
		r.Platform = v
	case "use":
		r.Use = v
	case "variant":
		r.Variant = v
	}
}

// chipDelta — чип в изменение условия; value "" — убрать условие.
func chipDelta(st domain.GuideState, c *guideChipIn) (guideDelta, bool) {
	var d guideDelta
	key, val := strings.TrimSpace(c.Key), strings.TrimSpace(c.Value)
	if val == "" {
		switch key {
		case "count", "operation", "term_months", "org_type", "vendor", "platform", "use", "variant":
			d.clear = []string{key}
			return d, true
		}
		if strings.HasPrefix(key, "count:") || strings.HasPrefix(key, "feature:") {
			d.clear = []string{key}
			return d, true
		}
		return d, false
	}
	switch {
	case key == "count" || strings.HasPrefix(key, "count:"):
		n, err := strconv.Atoi(val)
		if err != nil || n <= 0 || n > maxQuoteQuantity {
			return d, false
		}
		kind := strings.TrimPrefix(strings.TrimPrefix(key, "count"), ":")
		if kind == "" {
			kind = st.Pending
			if !strings.HasPrefix(kind, "count:") {
				kind, _ = mainObject(st.Requirements)
			} else {
				kind = strings.TrimPrefix(kind, "count:")
			}
		}
		if kind == "" {
			kind = "workstation"
		}
		d.objects = []domain.GuideObject{{Kind: kind, Count: n}}
	case key == "operation" && (val == "new" || val == "renewal"):
		d.operation = val
	case key == "term_months":
		n, err := strconv.Atoi(val)
		if err != nil || n <= 0 || n > 120 {
			return d, false
		}
		d.term = n
	case key == "org_type" && slices.Contains([]string{"commercial", "government", "academic", "nonprofit", "home"}, val):
		d.org = val
	case key == "vendor":
		d.vendor = val
	case key == "feature":
		d.features = []string{val}
	case key == "object" && slices.Contains([]string{"workstation", "server", "user", "connection"}, val):
		d.objKind = val
	case key == "use":
		d.use = val
	case key == "variant":
		if val == "unknown" {
			return d, true // «не знаю»: вариант остаётся не выбран, результат предварительный
		}
		d.variant = val
	case key == "platform":
		d.platform = val
	default:
		return d, false
	}
	return d, true
}

// guideChipIn — чип запроса (in.GuideChip без импорта in в состояние).
type guideChipIn struct{ Key, Value string }

// --- чипы условий на экране ---

// conditionsOf — условия подбора как чипы: введённые и значения по умолчанию, у каждого происхождение.
func conditionsOf(st domain.GuideState) []out.GuideCondition {
	r := st.Requirements
	origin := func(k string) string {
		if o := st.Origin[k]; o != "" {
			return o
		}
		return originInferred
	}
	var cs []out.GuideCondition
	if r.Vendor != "" {
		cs = append(cs, out.GuideCondition{Key: "vendor", Label: vendorLabel(r.Vendor), Value: r.Vendor, Origin: origin("vendor")})
	}
	for _, o := range r.Objects {
		if o.Count > 0 {
			label := countLabel(o.Kind, o.Count)
			if r.Task == "webinar" && o.Kind == "user" {
				label = fmt.Sprintf("%d %s", o.Count, guidePlural(o.Count, "участник", "участника", "участников"))
			}
			cs = append(cs, out.GuideCondition{Key: "count:" + o.Kind, Label: label,
				Value: strconv.Itoa(o.Count), Origin: origin("count:" + o.Kind)})
		} else {
			cs = append(cs, out.GuideCondition{Key: "object", Label: objectLabel(o.Kind), Value: o.Kind, Origin: origin("count:" + o.Kind)})
		}
	}
	if r.Use != "" {
		cs = append(cs, out.GuideCondition{Key: "use", Label: useLabel(r.Use), Value: r.Use, Origin: origin("use")})
	}
	if r.Variant != "" {
		cs = append(cs, out.GuideCondition{Key: "variant", Label: "«" + r.Variant + "»", Value: r.Variant, Origin: origin("variant")})
	}
	cs = append(cs, out.GuideCondition{Key: "operation", Label: operationLabel(r.Operation), Value: r.Operation, Origin: origin("operation")})
	if r.TermMonths > 0 {
		cs = append(cs, out.GuideCondition{Key: "term_months", Label: termWords(r.TermMonths), Value: strconv.Itoa(r.TermMonths), Origin: origin("term_months")})
	}
	if r.Task != "" {
		cs = append([]out.GuideCondition{{Key: "direction", Label: directionLabel(r.Task), Value: directionOf(r.Task), Origin: origin("task")}}, cs...)
	}
	if r.OrgType != defaultOrg || st.Origin["org_type"] == originUser || directionOf(r.Task) == "endpoint" {
		// Для антивируса тип покупателя меняет выбор (организация или дом) — показываем и значение по умолчанию.
		cs = append(cs, out.GuideCondition{Key: "org_type", Label: orgLabel(r.OrgType), Value: r.OrgType, Origin: origin("org_type")})
	}
	for _, f := range r.RequiredFeatures {
		cs = append(cs, out.GuideCondition{Key: "feature:" + f, Label: "нужно " + strings.ToLower(axisLabel(f)), Value: f, Origin: origin("feature:" + f)})
	}
	if r.Platform != "" {
		cs = append(cs, out.GuideCondition{Key: "platform", Label: r.Platform, Value: r.Platform, Origin: origin("platform")})
	}
	return cs
}

// plural — форма слова по числу: 1 компьютер, 2 компьютера, 5 компьютеров.
func guidePlural(n int, one, few, many string) string {
	n10, n100 := n%10, n%100
	switch {
	case n10 == 1 && n100 != 11:
		return one
	case n10 >= 2 && n10 <= 4 && (n100 < 12 || n100 > 14):
		return few
	}
	return many
}

func countLabel(kind string, n int) string {
	switch kind {
	case "workstation":
		return fmt.Sprintf("%d ПК", n)
	case "server":
		return fmt.Sprintf("%d %s", n, guidePlural(n, "сервер", "сервера", "серверов"))
	case "user":
		return fmt.Sprintf("%d %s", n, guidePlural(n, "пользователь", "пользователя", "пользователей"))
	case "connection":
		return fmt.Sprintf("%d %s", n, guidePlural(n, "подключение", "подключения", "подключений"))
	case "host":
		return fmt.Sprintf("%d %s", n, guidePlural(n, "хост", "хоста", "хостов"))
	}
	return fmt.Sprintf("%d %s", n, guidePlural(n, "место", "места", "мест"))
}

func objectLabel(kind string) string {
	switch kind {
	case "server":
		return "для сервера"
	case "workstation":
		return "для рабочего места"
	case "user":
		return "на пользователей"
	}
	return kind
}

func operationLabel(op string) string {
	if op == "renewal" {
		return "продление"
	}
	return "новая лицензия"
}

func orgLabel(o string) string {
	switch o {
	case "government":
		return "государственная организация"
	case "academic":
		return "образование"
	case "nonprofit":
		return "некоммерческая организация"
	case "home":
		return "для дома"
	}
	return "коммерческая организация"
}

func useLabel(u string) string {
	if u == "tls_server" {
		return "TLS-сервер"
	}
	return u
}

func vendorLabel(v string) string {
	switch v {
	case "kaspersky":
		return "Kaspersky"
	case "kripto-pro":
		return "КриптоПро"
	case "astralinux":
		return "Astra Linux"
	case "drweb", "doctor-web":
		return "Dr.Web"
	case "webinar":
		return "МТС Линк" // слаг вендора на витрине не показывается
	case "cyberprotect":
		return "Киберпротект"
	case "anydesk":
		return "AnyDesk"
	case "pro32":
		return "PRO32"
	case "microsoft":
		return "Microsoft"
	}
	return v
}

// axisLabels — подписи осей сравнения; ось без подписи — подпись из факта (axis_label) или сама ось.
var axisLabels = map[string]string{
	"encryption":             "Шифрование данных",
	"patch_management":       "Установка обновлений программ",
	"console":                "Единая консоль управления",
	"adaptive_anomaly":       "Адаптивный контроль аномалий",
	"app_control":            "Контроль программ",
	"device_control":         "Контроль устройств",
	"web_control":            "Веб-контроль",
	"sysadmin_tools":         "Инструменты системного администрирования",
	"mail_server":            "Защита почтовых серверов",
	"web_gateway":            "Защита интернет-шлюзов",
	"edr":                    "EDR",
	"purpose":                "Назначение",
	"platforms":              "Платформы",
	"certificates":           "Сертификаты",
	"license_unit":           "Единица лицензирования",
	"support":                "Техподдержка",
	"mandatory_access":       "Мандатное управление доступом",
	"integrity_control":      "Мандатный контроль целостности",
	"managed_devices":        "Управляемые устройства",
	"concurrent_connections": "Одновременные подключения",
	"licensed_users":         "Сотрудники",
	"tape_storage":           "Хранение на лентах",
}

func axisLabel(axis string) string {
	if l, ok := axisLabels[axis]; ok {
		return l
	}
	return axis
}

// directionLabel — подпись чипа направления.
func directionLabel(task string) string {
	switch directionOf(task) {
	case "endpoint":
		return "антивирус для рабочих мест"
	case "mail_gateway":
		return "защита почты и интернет-шлюза"
	case "network":
		return "защита сети"
	case "backup":
		return "резервное копирование"
	case "crypto":
		return "электронная подпись и шифрование"
	case "vuln":
		return "поиск уязвимостей"
	case "access":
		return "защита от несанкционированного доступа"
	case "webinar":
		return "вебинары"
	}
	return task
}
