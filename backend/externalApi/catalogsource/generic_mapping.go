package catalogsource

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Файл сопоставления (mapping.yaml) для GenericSource: какие файлы — каталог, справочники и журналы,
// какие колонки — поля формата данных, словари значений. Пример с комментариями — data/mapping.example.yaml.

// Mapping — файл сопоставления целиком.
type Mapping struct {
	// Version — catalog_version снимка; пусто — "generic-<имя файла каталога>".
	Version string `yaml:"version"`
	// BaseDir — откуда считать пути файлов; пусто — каталог mapping.yaml, относительный — от него же.
	BaseDir string `yaml:"base_dir"`
	// CategorySeparator — разделитель пути категорий «ПО > Антивирусы»; пусто — авто (" > ", " / ", "/", "|", "\").
	CategorySeparator string `yaml:"category_separator"`
	// DateFormats — дополнительные форматы дат (Go layout) сверх встроенных.
	DateFormats []string `yaml:"date_formats"`
	// Timezone — пояс дат без смещения (IANA); пусто — Europe/Moscow.
	Timezone string `yaml:"timezone"`

	Catalog    *Table `yaml:"catalog"`
	Vendors    *Table `yaml:"vendors"`
	Categories *Table `yaml:"categories"`
	Relations  *Table `yaml:"relations"`
	Aliases    *Table `yaml:"aliases"`
	Clients    *Table `yaml:"clients"`
	Purchases  *Table `yaml:"purchases"`
	QueryLog   *Table `yaml:"query_log"`
	Events     *Table `yaml:"events"`
	// FamilyURLs — страницы семейств на сайте: family_id → url (контур фида). Необязательный файл: нет его —
	// предупреждение, URL семейства пустой.
	FamilyURLs *Table `yaml:"family_urls"`
	// CompanionPairs — размеченные пары блока допродажи (companion_pairs.json): файл или лист;
	// нет — метрики блока в `make eval SET=p2` не считаются («нет разметки»).
	CompanionPairs *Table `yaml:"companion_pairs"`
	// EvalQueries — готовый queries.json в формате контракта (разметка, наборы); пусто — оценки нет.
	EvalQueries string `yaml:"eval_queries"`
	// Справочники контура коммерции — JSON-файлы в формате данных; пусто — справочник пуст.
	LicenseRules         string `yaml:"license_rules"`
	CompatibilityRules   string `yaml:"compatibility_rules"`
	AttributeDefinitions string `yaml:"attribute_definitions"`
	SolutionTemplates    string `yaml:"solution_templates"`
	// Lexicon — словарь понимания запросов (CSV: alias, canonical, kind, target, target_id, basis, note; контур фида):
	// читается при старте API, а не импортом — правка словаря не требует переимпорта. Пусто — словаря нет.
	Lexicon string `yaml:"lexicon"`
	// CompatDocs, Promos — допродажа контура фида (CSV, upsell.go): совместимость с первичным документом и акции
	// магазина; читаются при старте API, как lexicon. Пусто — справочника нет.
	CompatDocs string `yaml:"compat_docs"`
	Promos     string `yaml:"promos"`
	// UpsellClasses, UpsellLinks — классы продуктов и таблица классов допродажи (контур эталонов, CSV, upsell.go).
	UpsellClasses string `yaml:"upsell_classes"`
	UpsellLinks   string `yaml:"upsell_links"`
	// Help — статьи справки магазина (JSON в формате data/mock/help.json): читаются при старте API, как lexicon.
	// Пусто — справки нет.
	Help string `yaml:"help"`
	// TargetSets — каталог наборов оценки «по цели» (JSONL: эталоны, примеры ТЗ, соседи, набор понимания; контур эталонов,
	// eval_target.go). Пусто — наборов нет. Закрытого набора здесь нет: он передаётся явным путём.
	TargetSets string `yaml:"target_sets"`
	// Synthetic — что в данных придумано ("margin", "orders") → плашка «Синтетические данные».
	Synthetic []string `yaml:"synthetic"`

	dir string // каталог файлов данных (BaseDir, приведённый к абсолютному)
}

// Table — один файл данных.
type Table struct {
	// File — путь к файлу (.csv, .tsv, .txt, .json, .jsonl, .ndjson, .xlsx); формат — по расширению или Format.
	File   string `yaml:"file"`
	Format string `yaml:"format"` // csv | json | jsonl | xlsx; пусто — по расширению
	// Sheet — лист XLSX; пусто — первый.
	Sheet string `yaml:"sheet"`
	// JSONPath — где в JSON лежит массив строк: "products" или "data.items"; пусто — корень.
	JSONPath string `yaml:"json_path"`
	// Delimiter — разделитель CSV; пусто или auto — по первой строке. "\t" — табуляция.
	Delimiter string `yaml:"delimiter"`
	// Encoding — кодировка CSV: utf-8 | cp1251 | auto (пусто — auto: не UTF-8 — значит cp1251).
	Encoding string `yaml:"encoding"`
	// HeaderRow — номер строки заголовка (с 1) для CSV/XLSX; 0 — первая непустая.
	HeaderRow int `yaml:"header_row"`
	// Columns — поле контракта → колонка источника (строка или список кандидатов: берётся первый непустой).
	// Вложенные поля JSON — через точку: "price.value".
	Columns map[string]Column `yaml:"columns"`
	// Values — словари значений: поле → {"как в источнике": "как в контракте"}; ключи сравниваются без
	// регистра и лишних пробелов.
	Values map[string]map[string]string `yaml:"values"`
	// Defaults — значение поля, если колонки нет или ячейка пуста.
	Defaults map[string]string `yaml:"defaults"`
	// Attributes — (только catalog) колонки в свободные атрибуты позиции: "Платформа" или "Платформа: platform".
	Attributes []string `yaml:"attributes"`
	// Specs — (только catalog) колонки в числовые характеристики позиции: "Память, ГБ: ram_gb". Нечисловая
	// ячейка пропускается.
	Specs []string `yaml:"specs"`
}

// Column — одна колонка или список кандидатов.
type Column []string

// UnmarshalYAML — строка или список строк.
func (c *Column) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		*c = Column{n.Value}
		return nil
	case yaml.SequenceNode:
		var list []string
		if err := n.Decode(&list); err != nil {
			return err
		}
		*c = list
		return nil
	}
	return fmt.Errorf("строка %d: колонка — строка или список строк", n.Line)
}

// tableFields — поля, которые понимает каждая таблица (опечатка в имени поля — ошибка при загрузке).
var tableFields = map[string][]string{
	"catalog": {"id", "sku", "title", "description", "family_id", "family_name", "vendor", "category",
		"price_rub", "price_on_request", "license_type", "license_metric", "license_quantity",
		"license_duration_months", "license_duration", "license_edition", "origin", "in_reestr", "popularity", "margin",
		"kind", "mpn", "model_id", "compatible_with", "price_mode",
		// Контур фида (фид заказчика): наличие, прежняя цена, страницы, вид и ступень лицензии.
		"available", "old_price_rub", "offer_url", "family_url", "license_kind", "license_org_type",
		"license_tier_min", "license_tier_max"},
	"vendors":     {"id", "name", "origin", "url", "slug"},
	"family_urls": {"family_id", "url"},
	"categories":  {"id", "name", "parent_id", "parent", "path"},
	"relations": {"from", "to", "type", "note", "id", "from_product", "to_product", "requirement", "compatibility",
		"conditions", "quantity_strategy", "source", "source_url", "evidence_level", "review_status", "reviewed_at",
		"upgrade_adds"},
	"aliases":   {"alias", "canonical", "kind", "target", "target_id"},
	"clients":   {"id", "name", "segment"},
	"purchases": {"client_id", "product", "date"},
	"query_log": {"text", "freq", "ts", "session_id", "client_id", "results_count"},
	"events": {"ts", "session_id", "client_id", "type", "query", "product", "position", "results_count",
		"order_id", "account_id", "quantity"},
	"companion_pairs": {"anchor_product", "candidate_product", "class", "query"},
}

// requiredFields — без этих полей таблица бессмысленна.
var requiredFields = map[string][]string{
	"catalog":         {"title"},
	"vendors":         {"name"},
	"categories":      {"name"},
	"relations":       {"from", "to"},
	"aliases":         {"alias", "canonical"},
	"clients":         {"id"},
	"purchases":       {"client_id", "product"},
	"query_log":       {"text"},
	"events":          {"ts", "type"},
	"companion_pairs": {"anchor_product", "candidate_product", "class"},
	"family_urls":     {"family_id", "url"},
}

// LoadMapping — разобрать и проверить mapping.yaml.
func LoadMapping(path string) (*Mapping, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("файл сопоставления: %w", err)
	}
	var m Mapping
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	m.dir = filepath.Dir(abs)
	if m.BaseDir != "" {
		if filepath.IsAbs(m.BaseDir) {
			m.dir = m.BaseDir
		} else {
			m.dir = filepath.Join(m.dir, m.BaseDir)
		}
	}
	if err := m.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return &m, nil
}

func (m *Mapping) tables() map[string]*Table {
	return map[string]*Table{"catalog": m.Catalog, "vendors": m.Vendors, "categories": m.Categories,
		"relations": m.Relations, "aliases": m.Aliases, "clients": m.Clients, "purchases": m.Purchases,
		"query_log": m.QueryLog, "events": m.Events, "companion_pairs": m.CompanionPairs, "family_urls": m.FamilyURLs}
}

func (m *Mapping) validate() error {
	if m.Catalog == nil {
		return errors.New("нет раздела catalog")
	}
	var problems []string
	for name, t := range m.tables() {
		if t == nil {
			continue
		}
		if strings.TrimSpace(t.File) == "" {
			problems = append(problems, name+": не указан file")
		}
		switch strings.ToLower(t.Format) {
		case "", "csv", "json", "jsonl", "xlsx":
		default:
			problems = append(problems, fmt.Sprintf("%s: неизвестный format %q", name, t.Format))
		}
		known := tableFields[name]
		for f := range t.Columns {
			if !slices.Contains(known, f) {
				problems = append(problems, fmt.Sprintf("%s.columns: неизвестное поле %q (есть: %s)", name, f, strings.Join(known, ", ")))
			}
		}
		for f := range t.Defaults {
			if !slices.Contains(known, f) {
				problems = append(problems, fmt.Sprintf("%s.defaults: неизвестное поле %q", name, f))
			}
		}
		for _, f := range requiredFields[name] {
			if len(t.Columns[f]) == 0 && t.Defaults[f] == "" {
				problems = append(problems, fmt.Sprintf("%s: нужна колонка %q", name, f))
			}
		}
		if len(t.Attributes) > 0 && name != "catalog" {
			problems = append(problems, name+": attributes бывают только у catalog")
		}
		if len(t.Specs) > 0 && name != "catalog" {
			problems = append(problems, name+": specs бывают только у catalog")
		}
	}
	if len(problems) > 0 {
		slices.Sort(problems)
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

// path — абсолютный путь файла данных.
func (m *Mapping) path(file string) string {
	if filepath.IsAbs(file) {
		return file
	}
	return filepath.Join(m.dir, file)
}
