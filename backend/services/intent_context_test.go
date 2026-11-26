package services

import (
	"slices"
	"strings"
	"testing"

	"kuznica/api/domain"
)

// Срез справочника в промпт разбора: кандидаты по словам запроса и правилам, а не весь каталог.
func TestIntentPromptSlice(t *testing.T) {
	p := newIntentEnv(t, "off", nil, nil, nil)
	d := p.dictionary(t.Context())
	if d == nil {
		t.Fatal("справочник не собран")
	}
	s := d.slice("антивирус касперского на 10 пк", domain.Intent{})
	if !slices.Contains(s.vendors, "kaspersky") {
		t.Errorf("касперского → вендор kaspersky среди кандидатов: %v", s.vendors)
	}
	if !slices.Contains(s.categories, "antivirus-home") && !slices.Contains(s.categories, "antivirus-business") {
		t.Errorf("антивирус → категория антивирусов среди кандидатов: %v", s.categories)
	}
	if len(s.vendors) > intentMaxVendors || len(s.vendors) >= len(d.vendors) {
		t.Errorf("вендоров в срезе %d из %d — не весь справочник", len(s.vendors), len(d.vendors))
	}
	// Правила нашли вендора и категорию, словами запрос их не называет — они всё равно в срезе.
	s = d.slice("что-нибудь для работы", domain.Intent{VendorID: "1c", CategoryID: "accounting"})
	if !slices.Contains(s.vendors, "1c") || !slices.Contains(s.categories, "accounting") {
		t.Errorf("найденное правилами — в срезе: %v / %v", s.vendors, s.categories)
	}
	// Слова запроса ничего не называют — в срезе корневые разделы: модель выберет по смыслу.
	s = d.slice("чем чертить планы квартир", domain.Intent{})
	if !slices.Contains(s.categories, "creative") || !slices.Contains(s.categories, "hardware") {
		t.Errorf("корневые разделы в срезе, если кандидатов мало: %v", s.categories)
	}
	// Фонетика и транслит: «фотошоп» — Adobe Photoshop.
	s = d.slice("аналог фотошопа", domain.Intent{})
	if !slices.Contains(s.vendors, "adobe") {
		t.Errorf("фотошоп → adobe среди кандидатов: %v", s.vendors)
	}
	prompt := d.promptFor("антивирус касперского", domain.Intent{})
	if strings.Contains(prompt, "dell — ") {
		t.Error("вендор, не связанный с запросом, попал в промпт")
	}
}

// Сужение промпта не пропускает выдумки: id вне справочника отбрасывается, id из справочника вне среза — принимается.
func TestIntentValidateOutsideSlice(t *testing.T) {
	p := newIntentEnv(t, "off", nil, nil, nil)
	d := p.dictionary(t.Context())
	raw, err := decodeIntent(`{"intent":"find_product","vendor":"no-such-vendor","category":"graphics","constraints":{}}`)
	if err != nil {
		t.Fatal(err)
	}
	in := d.validate(*raw, "графический редактор")
	if in.VendorID != "" {
		t.Errorf("выдуманный вендор принят: %q", in.VendorID)
	}
	if in.CategoryID != "graphics" {
		t.Errorf("категория из справочника отброшена: %q", in.CategoryID)
	}
}
