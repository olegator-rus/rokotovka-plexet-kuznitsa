// Package catalogsource — источники каталога для импорта. Сейчас один: mock-данные в JSON
//. Данные организаторов приедут вторым источником с тем же
// результатом — domain.CatalogSnapshot, и импорт менять не придётся.
package catalogsource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"kuznica/api/domain"
)

// MockJSON — каталог из каталога файлов: catalog.json обязателен, справочники — по возможности.
type MockJSON struct {
	Dir string
}

// Name — источник для сводки импорта.
func (m MockJSON) Name() string { return "mock-json:" + m.Dir }

// Load — снимок каталога. Битый JSON, повтор id или позиция без названия — ошибка: молча импортировать
// половину каталога хуже, чем не импортировать. Нет справочника или ссылка в никуда — предупреждение.
func (m MockJSON) Load(ctx context.Context) (*domain.CatalogSnapshot, error) {
	var catalog struct {
		Version   string           `json:"version"`
		Synthetic []string         `json:"synthetic"`
		Products  []domain.Product `json:"products"`
	}
	if err := readJSON(filepath.Join(m.Dir, "catalog.json"), &catalog); err != nil {
		return nil, err
	}
	snap := &domain.CatalogSnapshot{Version: catalog.Version, Products: catalog.Products, Synthetic: catalog.Synthetic}
	if snap.Version == "" {
		snap.Version = "unversioned"
		snap.Warnings = append(snap.Warnings, "catalog.json: нет version")
	}
	optional := []struct {
		file string
		dst  any
	}{
		{"vendors.json", &snap.Vendors},
		{"categories.json", &snap.Categories},
		{"relations.json", &snap.Relations},
		{"aliases.json", &snap.Aliases},
		{"clients.json", &snap.Clients},
	}
	for _, o := range optional {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		err := readJSON(filepath.Join(m.Dir, o.file), o.dst)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			snap.Warnings = append(snap.Warnings, o.file+": файла нет")
		case err != nil:
			return nil, err
		}
	}
	// Справочники контура коммерции: нет файла — пустой справочник без предупреждения, каталог контура поиска
	// без них полноценен.
	p2 := []struct {
		file string
		dst  any
	}{
		{"license_rules.json", &snap.LicenseRules},
		{"compatibility_rules.json", &snap.CompatibilityRules},
		{"attribute_definitions.json", &snap.AttributeDefinitions},
		{"solution_templates.json", &snap.SolutionTemplates},
	}
	for _, o := range p2 {
		if err := readJSON(filepath.Join(m.Dir, o.file), o.dst); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	if err := validate(snap); err != nil {
		return nil, err
	}
	return snap, nil
}

func readJSON(path string, dst any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return nil
}

// validate — обязательные поля и ссылочная целостность.
func validate(s *domain.CatalogSnapshot) error {
	if len(s.Products) == 0 {
		return errors.New("catalog.json: нет ни одной позиции")
	}
	vendors := make(map[string]bool, len(s.Vendors))
	for _, v := range s.Vendors {
		vendors[v.ID] = true
	}
	categories := make(map[string]bool, len(s.Categories))
	for _, c := range s.Categories {
		categories[c.ID] = true
	}
	seen := make(map[string]bool, len(s.Products))
	var dangling []string
	for i, p := range s.Products {
		switch {
		case strings.TrimSpace(p.ID) == "":
			return fmt.Errorf("catalog.json: позиция №%d без id", i+1)
		case seen[p.ID]:
			return fmt.Errorf("catalog.json: id %q повторяется", p.ID)
		case strings.TrimSpace(p.Title) == "":
			return fmt.Errorf("catalog.json: %q без title", p.ID)
		}
		seen[p.ID] = true
		if p.FamilyID == "" {
			s.Products[i].FamilyID = p.ID // без семейства позиция — сама себе семейство
		}
		if len(s.Vendors) > 0 && !vendors[p.VendorID] {
			dangling = append(dangling, fmt.Sprintf("%s → vendor %q", p.ID, p.VendorID))
		}
		if len(s.Categories) > 0 && !categories[p.CategoryID] {
			dangling = append(dangling, fmt.Sprintf("%s → category %q", p.ID, p.CategoryID))
		}
	}
	applyDefaults(s)
	if err := validateP2(s, seen); err != nil {
		return err
	}
	if len(dangling) > 0 {
		head := dangling
		if len(head) > 5 {
			head = head[:5]
		}
		s.Warnings = append(s.Warnings, fmt.Sprintf("ссылки в никуда: %d (%s)", len(dangling), strings.Join(head, "; ")))
	}
	return nil
}
