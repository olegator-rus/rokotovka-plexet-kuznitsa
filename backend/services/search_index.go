package services

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"kuznica/api/domain"
)

// Индекс каталога в OpenSearch (ARCHITECTURE §2.4). Физический индекс products_v{N}, поиск ходит через
// алиас products: новый импорт строит следующий индекс и атомарно переключает алиас.

const (
	// defaultIndexAlias — алиас индекса каталога; физические индексы — <алиас>_v{N}.
	defaultIndexAlias = "products"
	// embeddingDim — размерность BGE-M3.
	embeddingDim = 1024
	// keepIndices — сколько последних индексов оставлять после импорта (текущий и предыдущий — для отката).
	keepIndices = 2
)

// indexMeta — _meta маппинга: что лежит в индексе.
type indexMeta struct {
	CatalogVersion string    `json:"catalog_version"`
	HasVectors     bool      `json:"has_vectors"`
	EmbeddingDim   int       `json:"embedding_dim,omitempty"`
	Products       int       `json:"products"`
	Source         string    `json:"source"`
	BuiltAt        time.Time `json:"built_at"`
}

// foldMappings — свёртка символов для char_filter индекса; те же замены, что foldRunes в Go.
func foldMappings() []string {
	out := make([]string, 0, len(foldRunes))
	for from, to := range foldRunes {
		f, t := string(from), string(to)
		// В mapping char_filter кавычка и обратная косая — обычные символы, экранировать не нужно.
		out = append(out, f+"=>"+t)
	}
	sort.Strings(out)
	return out
}

// indexBody — настройки и маппинг индекса.
//   - title.exact — keyword с нормализатором (нижний регистр, ё→е, дефисы, кавычки, пробелы): B0 и точное
//     совпадение; title.ru — русская морфология; title.ngram — edge n-граммы 2–15 для артикулов и
//     начала слов (поиск по ним — обычным анализатором, иначе n-граммится и запрос).
//   - Стеммер портит «Office 2021» и «8.3», поэтому рядом с морфологией всегда есть точное поле.
//   - embedding — HNSW на движке lucene, косинус: каталог маленький, фильтры по полям работают внутри kNN.
func indexBody(meta indexMeta) map[string]any {
	text := func(analyzer string) map[string]any { return map[string]any{"type": "text", "analyzer": analyzer} }
	keyword := map[string]any{"type": "keyword"}
	return map[string]any{
		"settings": map[string]any{
			"index": map[string]any{"number_of_shards": 1, "number_of_replicas": 0, "knn": true},
			"analysis": map[string]any{
				"char_filter": map[string]any{
					"fold":   map[string]any{"type": "mapping", "mappings": foldMappings()},
					"spaces": map[string]any{"type": "pattern_replace", "pattern": `\s+`, "replacement": " "},
					// split — двоеточие и точка между буквами разделяют слова: стандартный токенизатор
					// склеивает «1С:Бухгалтерия» и «Dr.Web» в одно слово, и «бухгалтерия» не находилась.
					// Точка между цифрами («8.3») остаётся частью версии.
					"split": map[string]any{"type": "pattern_replace", "pattern": `(?<=[\p{L}\p{N}])[:.](?=\p{L})`, "replacement": " "},
				},
				"normalizer": map[string]any{
					"exact": map[string]any{"type": "custom", "char_filter": []string{"fold", "spaces"}, "filter": []string{"lowercase", "trim"}},
				},
				"filter": map[string]any{
					"ru_stop": map[string]any{"type": "stop", "stopwords": "_russian_"},
					"ru_stem": map[string]any{"type": "stemmer", "language": "russian"},
					"edge":    map[string]any{"type": "edge_ngram", "min_gram": 2, "max_gram": 15},
				},
				"analyzer": map[string]any{
					"ru":    map[string]any{"type": "custom", "char_filter": []string{"fold", "split"}, "tokenizer": "standard", "filter": []string{"lowercase", "ru_stop", "ru_stem"}},
					"plain": map[string]any{"type": "custom", "char_filter": []string{"fold", "split"}, "tokenizer": "standard", "filter": []string{"lowercase"}},
					"edge":  map[string]any{"type": "custom", "char_filter": []string{"fold", "split"}, "tokenizer": "standard", "filter": []string{"lowercase", "edge"}},
				},
			},
		},
		"mappings": map[string]any{
			"_meta":   meta,
			"dynamic": "strict",
			"properties": map[string]any{
				"product_id": keyword,
				"sku":        map[string]any{"type": "keyword", "normalizer": "exact"},
				"family_id":  keyword,
				"family_name": map[string]any{"type": "text", "analyzer": "ru",
					"fields": map[string]any{"exact": map[string]any{"type": "keyword", "normalizer": "exact"}}},
				"title": map[string]any{"type": "text", "analyzer": "plain", "fields": map[string]any{
					"exact": map[string]any{"type": "keyword", "normalizer": "exact"},
					"ru":    text("ru"),
					"ngram": map[string]any{"type": "text", "analyzer": "edge", "search_analyzer": "plain"},
				}},
				"aliases":   text("plain"),
				"vendor_id": keyword,
				"vendor": map[string]any{"type": "text", "analyzer": "plain",
					"fields": map[string]any{"raw": keyword}},
				"category_id":   keyword,
				"category_ids":  keyword,
				"category":      keyword,
				"category_path": text("ru"),
				"description":   text("ru"),
				"attributes":    text("ru"),
				"relations": map[string]any{"properties": map[string]any{
					domain.RelationSubstitute: keyword, domain.RelationComplement: keyword,
					domain.RelationRenewal: keyword, domain.RelationCompatibility: keyword,
				}},
				"license_type":     keyword,
				"license_metric":   keyword,
				"license_quantity": map[string]any{"type": "integer"},
				"license_duration": map[string]any{"type": "integer"},
				"license_edition":  keyword,
				"price_rub":        map[string]any{"type": "float"},
				"price_on_request": map[string]any{"type": "boolean"},
				"origin":           keyword,
				"in_reestr":        map[string]any{"type": "boolean"},
				"popularity":       map[string]any{"type": "float"},
				"margin":           map[string]any{"type": "float"},
				"embedding": map[string]any{
					"type": "knn_vector", "dimension": embeddingDim,
					"method": map[string]any{"name": "hnsw", "engine": "lucene", "space_type": "cosinesimil",
						"parameters": map[string]any{"m": 16, "ef_construction": 128}},
				},
			},
		},
	}
}

// indexDoc — документ индекса. Он же разбирается из _source выдачи: всё, что нужно позиции ответа,
// лежит в документе, и поиску не нужна база.
type indexDoc struct {
	ProductID       string              `json:"product_id"`
	SKU             string              `json:"sku"`
	FamilyID        string              `json:"family_id"`
	FamilyName      string              `json:"family_name"`
	Title           string              `json:"title"`
	Aliases         []string            `json:"aliases"`
	VendorID        string              `json:"vendor_id"`
	Vendor          string              `json:"vendor"`
	CategoryID      string              `json:"category_id"`
	CategoryIDs     []string            `json:"category_ids"`
	Category        string              `json:"category"`
	CategoryPath    string              `json:"category_path"`
	Description     string              `json:"description"`
	Attributes      string              `json:"attributes"`
	Relations       map[string][]string `json:"relations,omitempty"`
	LicenseType     string              `json:"license_type"`
	LicenseMetric   *string             `json:"license_metric"`
	LicenseQuantity *int                `json:"license_quantity"`
	LicenseDuration *int                `json:"license_duration"`
	LicenseEdition  string              `json:"license_edition"`
	PriceRub        *float64            `json:"price_rub"`
	PriceOnRequest  bool                `json:"price_on_request"`
	Origin          string              `json:"origin"`
	InReestr        bool                `json:"in_reestr"`
	Popularity      float64             `json:"popularity"`
	Margin          float64             `json:"margin"`
	Embedding       []float32           `json:"embedding,omitempty"`
}

// buildDocs — документы индекса из снимка каталога: имена вендора и категории, путь категории, алиасы
// и связи семейства подставляются здесь.
func buildDocs(s *domain.CatalogSnapshot) []indexDoc {
	vendors := make(map[string]domain.Vendor, len(s.Vendors))
	for _, v := range s.Vendors {
		vendors[v.ID] = v
	}
	cats := make(map[string]domain.Category, len(s.Categories))
	for _, c := range s.Categories {
		cats[c.ID] = c
	}
	relations := map[string]map[string][]string{}
	for _, r := range s.Relations {
		if relations[r.FromFamily] == nil {
			relations[r.FromFamily] = map[string][]string{}
		}
		relations[r.FromFamily][r.Type] = append(relations[r.FromFamily][r.Type], r.ToFamily)
	}
	docs := make([]indexDoc, 0, len(s.Products))
	for _, p := range s.Products {
		ids, names := categoryChain(cats, p.CategoryID)
		d := indexDoc{
			ProductID:       p.ID,
			SKU:             p.SKU,
			FamilyID:        p.FamilyID,
			FamilyName:      p.FamilyName,
			Title:           p.Title,
			VendorID:        p.VendorID,
			Vendor:          vendors[p.VendorID].Name,
			CategoryID:      p.CategoryID,
			CategoryIDs:     ids,
			CategoryPath:    strings.Join(names, " / "),
			Description:     p.Description,
			Attributes:      attributesText(p.Attributes),
			Relations:       relations[p.FamilyID],
			LicenseType:     p.License.Type,
			LicenseMetric:   p.License.Metric,
			LicenseQuantity: p.License.Quantity,
			LicenseDuration: p.License.DurationMonths,
			LicenseEdition:  p.License.Edition,
			PriceRub:        p.PriceRub,
			PriceOnRequest:  p.PriceOnRequest,
			Origin:          p.Origin,
			InReestr:        p.InReestr,
			Popularity:      p.Popularity,
			Margin:          p.Margin,
		}
		if d.Vendor == "" {
			d.Vendor = p.VendorID
		}
		if len(names) > 0 {
			d.Category = names[len(names)-1]
		}
		d.Aliases = aliasesFor(s.Aliases, p, ids)
		docs = append(docs, d)
	}
	return docs
}

// categoryChain — id и имена категорий от корня до листа. Цикл в справочнике обрывается.
func categoryChain(cats map[string]domain.Category, id string) ([]string, []string) {
	var ids, names []string
	seen := map[string]bool{}
	for id != "" && !seen[id] {
		seen[id] = true
		c, ok := cats[id]
		if !ok {
			if len(ids) == 0 {
				ids, names = []string{id}, []string{id}
			}
			break
		}
		ids = append([]string{c.ID}, ids...)
		names = append([]string{c.Name}, names...)
		if c.ParentID == nil {
			break
		}
		id = *c.ParentID
	}
	return ids, names
}

// aliasesFor — алиасы, которые относятся к позиции: вендора, семейства (id совпадает или семейство —
// вариант цели: «microsoft-office» → «microsoft-office-2021»), категории или её предка, термина (каноническое
// слово есть в названии, семействе или описании).
func aliasesFor(all []domain.Alias, p domain.Product, categoryIDs []string) []string {
	var out []string
	seen := map[string]bool{}
	text := ""
	for _, a := range all {
		hit := false
		switch a.Target {
		case domain.AliasTargetVendor:
			hit = a.TargetID == p.VendorID
		case domain.AliasTargetFamily:
			hit = a.TargetID == p.FamilyID || strings.HasPrefix(p.FamilyID, a.TargetID+"-")
		case domain.AliasTargetCategory:
			for _, id := range categoryIDs {
				hit = hit || id == a.TargetID
			}
		case domain.AliasTargetTerm:
			if text == "" {
				text = " " + strings.Join(wordsOf(p.Title+" "+p.FamilyName+" "+p.Description), " ") + " "
			}
			c := strings.Join(wordsOf(a.Canonical), " ")
			hit = c != "" && strings.Contains(text, " "+c+" ")
		}
		if hit && !seen[a.Alias] {
			seen[a.Alias] = true
			out = append(out, a.Alias)
		}
	}
	return out
}

var wordRe = regexp.MustCompile(`[\p{L}\p{N}]+`)

// wordsOf — слова в нормализованном виде.
func wordsOf(s string) []string { return wordRe.FindAllString(foldText(s), -1) }

// attributesText — атрибуты одной строкой «ключ: значение; …» в порядке ключей.
func attributesText(raw domain.JSONB) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		if m[k] == nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %v", k, m[k]))
	}
	return strings.Join(parts, "; ")
}

// embeddingText — текст для вектора: семейство, название, категория, описание.
func embeddingText(d indexDoc) string {
	parts := []string{d.FamilyName, d.Title, d.CategoryPath, d.Description}
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ". ")
}

// indexName — физический индекс версии n для алиаса.
func indexName(alias string, n int) string { return fmt.Sprintf("%s_v%d", alias, n) }

// indexVersion — номер версии из имени индекса алиаса; чужой индекс — 0.
func indexVersion(alias, name string) int {
	prefix := alias + "_v"
	if !strings.HasPrefix(name, prefix) {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimPrefix(name, prefix))
	if err != nil || n < 1 {
		return 0
	}
	return n
}
