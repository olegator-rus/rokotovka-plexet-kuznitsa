package catalogsource

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"kuznica/api/domain"
)

// Контур доводки: справка, статус реестра и набор витрины — файлы рядом с каталогом. Нет файла — ErrNoFile.

// LoadHelp — help.json: статьи справки.
func (m MockJSON) LoadHelp(_ context.Context) ([]domain.HelpArticle, error) {
	var hs []domain.HelpArticle
	if err := readOptional(filepath.Join(m.Dir, "help.json"), &hs); err != nil {
		return nil, err
	}
	for i, h := range hs {
		if h.ID == "" || h.Title == "" || len(h.Triggers) == 0 {
			return nil, fmt.Errorf("help.json[%d]: пустой id, заголовок или триггеры", i)
		}
	}
	return hs, nil
}

// LoadReestr — reestr.json: статус семейств в реестре российского ПО.
func (m MockJSON) LoadReestr(_ context.Context) ([]domain.ReestrEntry, error) {
	var rs []domain.ReestrEntry
	if err := readOptional(filepath.Join(m.Dir, "reestr.json"), &rs); err != nil {
		return nil, err
	}
	for i, r := range rs {
		if r.FamilyID == "" || (r.Status != domain.ReestrConfirmed && r.Status != domain.ReestrNotMatching) {
			return nil, fmt.Errorf("reestr.json[%d]: пустое семейство или статус %q", i, r.Status)
		}
	}
	return rs, nil
}

// LoadShowcase — showcase.json: прогон витрины, перенесённый на mock-каталог.
func (m MockJSON) LoadShowcase(_ context.Context) ([]domain.ShowcaseQuery, error) {
	var qs []domain.ShowcaseQuery
	if err := readOptional(filepath.Join(m.Dir, "showcase.json"), &qs); err != nil {
		return nil, err
	}
	for i, q := range qs {
		switch q.Expect {
		case "product", "analogs", "absent", "help":
		default:
			return nil, fmt.Errorf("showcase.json[%d]: класс %q", i, q.Expect)
		}
		if q.ID == "" || q.Query == "" {
			return nil, fmt.Errorf("showcase.json[%d]: пустой id или запрос", i)
		}
	}
	return qs, nil
}

func readOptional(path string, v any) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ErrNoFile
		}
		return err
	}
	return readJSON(path, v)
}
