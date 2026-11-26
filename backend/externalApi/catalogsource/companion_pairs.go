package catalogsource

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"kuznica/api/domain"
)

// LoadCompanionPairs — размеченные пары блока допродажи (companion_pairs.json). Нет файла —
// ErrNoFile; пустой id или неизвестный класс — ошибка: оценка на битой разметке хуже, чем её отсутствие.
func (m MockJSON) LoadCompanionPairs(_ context.Context) ([]domain.CompanionPair, error) {
	var ps []domain.CompanionPair
	if err := readJSON(filepath.Join(m.Dir, "companion_pairs.json"), &ps); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNoFile
		}
		return nil, err
	}
	for i, p := range ps {
		switch {
		case strings.TrimSpace(p.AnchorProduct) == "" || strings.TrimSpace(p.CandidateProduct) == "":
			return nil, fmt.Errorf("companion_pairs.json: пара №%d без anchor_product или candidate_product", i+1)
		case !slices.Contains(domain.CompanionPairClasses, p.Class):
			return nil, fmt.Errorf("companion_pairs.json: пара №%d — неизвестный класс %q", i+1, p.Class)
		}
	}
	return ps, nil
}
