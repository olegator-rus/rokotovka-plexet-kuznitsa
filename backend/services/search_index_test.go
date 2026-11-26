package services

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"kuznica/api/domain"
)

// Маппинг: точное поле с нормализатором, морфология, n-граммы 2–15, вектор 1024 HNSW cosine.
func TestIndexBody(t *testing.T) {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false) // правила mapping содержат «=>»
	if err := enc.Encode(indexBody(indexMeta{CatalogVersion: "v", HasVectors: true})); err != nil {
		t.Fatal(err)
	}
	s := b.String()
	for _, want := range []string{
		`"exact":{"normalizer":"exact","type":"keyword"}`,
		`"ru":{"analyzer":"ru","type":"text"}`,
		`"ngram":{"analyzer":"edge","search_analyzer":"plain","type":"text"}`,
		`"edge":{"max_gram":15,"min_gram":2,"type":"edge_ngram"}`,
		`"dimension":1024`, `"space_type":"cosinesimil"`, `"engine":"lucene"`, `"name":"hnsw"`,
		`"knn":true`, `"dynamic":"strict"`, `"has_vectors":true`, `"ё=>е"`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("в маппинге нет %s", want)
		}
	}
}

// Свёртка символов индекса и Go-нормализации совпадает: иначе подстрока B0 разъедется.
func TestFoldMappingsMatchGo(t *testing.T) {
	m := foldMappings()
	if len(m) != len(foldRunes) {
		t.Fatalf("правил %d, замен %d", len(m), len(foldRunes))
	}
	for from, to := range foldRunes {
		if !slices.Contains(m, string(from)+"=>"+string(to)) {
			t.Fatalf("нет правила %c=>%c", from, to)
		}
	}
}

func fixtureSnapshot(t *testing.T) *domain.CatalogSnapshot {
	t.Helper()
	snap, err := fixtureSource().Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

// Документ: имена вендора и категории, путь категории, алиасы по вендору, семейству (с префиксом),
// категории-предку и термину; связи семейства; атрибуты строкой.
func TestBuildDocs(t *testing.T) {
	docs := buildDocs(fixtureSnapshot(t))
	by := map[string]indexDoc{}
	for _, d := range docs {
		by[d.ProductID] = d
	}
	kis := by["kis-2dev-1y"]
	if kis.Vendor != "Kaspersky" || kis.Category != "Антивирусы для дома" || kis.CategoryPath != "Безопасность / Антивирусы для дома" ||
		!slices.Equal(kis.CategoryIDs, []string{"security", "antivirus-home"}) {
		t.Fatalf("kis: %+v", kis)
	}
	if !slices.Equal(kis.Aliases, []string{"касперский", "каспер", "антивирус"}) {
		t.Fatalf("алиасы kis: %v", kis.Aliases)
	}
	if office := by["office-2021-hb"]; !slices.Contains(office.Aliases, "офис") || !slices.Contains(office.Aliases, "ворд") {
		t.Fatalf("алиасы офиса (префикс семейства, термин word): %v", office.Aliases)
	}
	if ph := by["photoshop-1u-1y"]; !slices.Equal(ph.Relations[domain.RelationSubstitute], []string{"fotomaster"}) {
		t.Fatalf("связи фотошопа: %v", ph.Relations)
	}
	if ksos := by["ksos-10dev-1y"]; ksos.Attributes != "platform: Windows, macOS, Android; servers: 1" {
		t.Fatalf("атрибуты: %q", ksos.Attributes)
	}
	if got := embeddingText(kis); got != "Kaspersky Internet Security. Kaspersky Internet Security, 2 устройства, 1 год. Безопасность / Антивирусы для дома. Защита домашних компьютеров и смартфонов от вирусов, фишинга и шифровальщиков." {
		t.Fatalf("текст вектора: %q", got)
	}
}

func TestIndexVersion(t *testing.T) {
	for name, want := range map[string]int{"products_v3": 3, "products_v12": 12, "products_vx": 0, "other_v1": 0, "products_v0": 0} {
		if got := indexVersion("products", name); got != want {
			t.Fatalf("%s → %d, ожидалось %d", name, got, want)
		}
	}
	if indexName("products", 4) != "products_v4" {
		t.Fatal(indexName("products", 4))
	}
}
