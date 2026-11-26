package domain

// Оценка «по цели» на фиде заказчика (контур эталонов): у запроса не оценки позиций 0–3, а ожидаемая цель
// ответа — семейство, вендор, вендор + категория, категория, SKU или «цели нет». Главная метрика — Success@1:
// первая цель ответа совпадает с ожидаемой. Наборы — JSONL в каталоге target_sets (mapping.yaml), закрытый набор —
// только по явному пути и только в CLI.

// Наборы «по цели» (EvalQuery.Set / evaluations.eval_set).
const (
	TargetSetEtalon        = "etalon"        // 10 эталонов заказчика
	TargetSetTZ            = "tz"            // 7 примеров из ТЗ
	TargetSetNeighbors     = "neighbors"     // 114 соседей эталонов (контур эталонов) — настройка
	TargetSetUnderstanding = "understanding" // контрольный набор понимания (контур фида) — настройка
	TargetSetClosed        = "closed"        // закрытый набор: новые формулировки, один прогон при заморозке
	// Наборы агента SearchOps: правка допускается, только если на них ухудшилось 0 запросов. Закрытый в них не входит.
	TargetSetFeedTune       = "feed_tune"       // neighbors + understanding
	TargetSetFeedRegression = "feed_regression" // etalon + tz
)

// TargetSetFiles — из каких файлов каталога target_sets собран набор. Закрытого здесь нет: его путь передаётся
// явно (`go run . eval --set closed --file …`), агенту и API он недоступен.
var TargetSetFiles = map[string][]string{
	TargetSetEtalon:         {"etalon.jsonl"},
	TargetSetTZ:             {"tz.jsonl"},
	TargetSetNeighbors:      {"neighbors.jsonl"},
	TargetSetUnderstanding:  {"understanding.jsonl"},
	TargetSetFeedTune:       {"neighbors.jsonl", "understanding.jsonl"},
	TargetSetFeedRegression: {"etalon.jsonl", "tz.jsonl"},
}

// TargetSets — все наборы «по цели», включая закрытый (для GET /api/evaluations?set=…).
var TargetSets = []string{TargetSetEtalon, TargetSetTZ, TargetSetNeighbors, TargetSetUnderstanding, TargetSetClosed,
	TargetSetFeedTune, TargetSetFeedRegression}

// IsTargetSet — набор оценки «по цели».
func IsTargetSet(set string) bool {
	for _, s := range TargetSets {
		if s == set {
			return true
		}
	}
	return false
}

// Типы ожидаемой цели (TargetExpect.Type) — те же, что у цели ответа (out.Target*), плюс none.
const (
	ExpectSKU            = "sku"
	ExpectFamily         = "family"
	ExpectVendor         = "vendor"
	ExpectVendorCategory = "vendor_category"
	ExpectCategory       = "category"
	// ExpectNone — бессмыслица или сервисный вопрос: товарной цели нет (цель отсутствует или сервисная).
	ExpectNone = "none"
)

// ConfigBM25 — чистый BM25: multi_match по тем же полям, что B1, по запросу как набран (только регистр и
// пробелы), без раскладки, опечаток, транслита, словарей и алиасов. Точка отсчёта «BM25» сравнения;
// в лестницу B0–B4 не входит.
const ConfigBM25 = "BM25"

// TargetEvalConfigs — сравнение на фиде: BM25 → BM25 + нормализатор (B1) → гибрид (B2) → полный продукт
// (live: конвейер B3 на действующих правилах, с принятыми правками агента — B4).
var TargetEvalConfigs = []string{ConfigBM25, ConfigB1, ConfigB2, ConfigLive}

// TargetQuery — строка набора «по цели» (JSONL).
type TargetQuery struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	// Group — часть набора: etalon | tz | neighbor | catalog | understanding.
	Group string `json:"group,omitempty"`
	// Vendor — слаг вендора эталона (соседи и закрытая часть по девяти вендорам).
	Vendor string `json:"vendor,omitempty"`
	// Kind — вид формулировки как в источнике; Distortion — вид искажения отчёта (склонение, транслит, опечатка,
	// раскладка, атрибут, написание, название) или вид запроса по каталогу.
	Kind       string       `json:"kind,omitempty"`
	Distortion string       `json:"distortion,omitempty"`
	Expect     TargetExpect `json:"expect"`
	// Why — почему ожидается именно эта цель (для подтверждения человеком).
	Why string `json:"why,omitempty"`
}

// TargetExpect — ожидаемая цель. Also — другие допустимые цели (засчитывается любая).
type TargetExpect struct {
	Type string `json:"type"`
	// URL — эталонный URL цели (эталоны заказчика); пусто — выводится из каталога (URL семейства, вендора).
	URL string `json:"url,omitempty"`
	// GroupIDs — family: одно из семейств; vendor_category: семейства, которые тоже засчитываются.
	GroupIDs []string `json:"group_ids,omitempty"`
	// VendorSlug — vendor, vendor_category.
	VendorSlug string `json:"vendor_slug,omitempty"`
	// CategoryIDs — vendor_category: категория второго уровня (одна); category: любая из.
	CategoryIDs []string `json:"category_ids,omitempty"`
	// OfferIDs — sku: одна из позиций.
	OfferIDs []string `json:"offer_ids,omitempty"`
	// Top > 1 — проверка по первым N карточкам, а не по цели ответа (набор понимания): family/vendor — хотя бы одна
	// из N, category — все N из этих категорий.
	Top int `json:"top,omitempty"`
	// Exclude — этих семейств нет среди первых 20 карточек.
	Exclude []string `json:"exclude,omitempty"`
	// AltMin — сколько карточек «Можно купить сейчас» (match_type alternative, с ценой) нужно в ответе («по запросу →
	// альтернатива»).
	AltMin int `json:"alt_min,omitempty"`
	// Normalized — нормализованный запрос ровно такой; RouteNot — маршрут ответа не этот (набор понимания).
	Normalized string `json:"normalized,omitempty"`
	RouteNot   string `json:"route_not,omitempty"`
	// Hard — жёсткие условия запроса: карточки, которые их нарушают, считаются нарушениями.
	Hard *HardExpect    `json:"hard,omitempty"`
	Also []TargetExpect `json:"also,omitempty"`
}

// HardExpect — жёсткие условия (: не ослабляются ни на одном шаге лестницы). Пустое поле — условия нет.
type HardExpect struct {
	LicenseKind    string `json:"license_kind,omitempty"` // renewal | base | support | …
	OrgType        string `json:"org_type,omitempty"`     // commercial | academic | government | …
	DurationMonths int    `json:"duration_months,omitempty"`
	// Quantity — число мест: ступень цены позиции должна его покрывать.
	Quantity int `json:"quantity,omitempty"`
}
