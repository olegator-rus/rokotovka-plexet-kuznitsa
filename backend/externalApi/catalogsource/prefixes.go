package catalogsource

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"kuznica/api/domain"
)

// LoadPrefixes — набор префиксов панели prefixes.json и его байты (хэш набора). Нет файла —
// ErrNoFile; пустой id, префикс, цель или неизвестный тип — ошибка.
func (m MockJSON) LoadPrefixes(_ context.Context) ([]domain.PanelPrefix, []byte, error) {
	path := filepath.Join(m.Dir, "prefixes.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, ErrNoFile
		}
		return nil, nil, err
	}
	var ps []domain.PanelPrefix
	if err := readJSON(path, &ps); err != nil {
		return nil, nil, err
	}
	seen := map[string]bool{}
	for i, p := range ps {
		switch {
		case p.ID == "" || seen[p.ID]:
			return nil, nil, fmt.Errorf("prefixes.json[%d]: пустой или повторный id", i)
		case strings.TrimSpace(p.Prefix) == "" || p.TargetQuery == "":
			return nil, nil, fmt.Errorf("prefixes.json %s: пустой префикс или цель", p.ID)
		case !slices.Contains(domain.PrefixTypes, p.Type):
			return nil, nil, fmt.Errorf("prefixes.json %s: тип %q", p.ID, p.Type)
		}
		seen[p.ID] = true
	}
	return ps, raw, nil
}
