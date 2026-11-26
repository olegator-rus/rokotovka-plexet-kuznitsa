package catalogsource

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"kuznica/api/domain"
)

// Словарь понимания запросов (контур фида) — CSV рядом с mapping.yaml. Нет поля lexicon или файла —
// ErrNoFile: поиск работает на словаре импорта.

// lexiconKinds — допустимые типы связи.
var lexiconKinds = map[string]bool{domain.LexiconJargon: true, domain.LexiconSpelling: true,
	domain.LexiconRebrand: true, domain.LexiconSuccessor: true, domain.LexiconDrop: true}

// LoadLexicon — записи словаря из файла mapping.lexicon.
func (g *Generic) LoadLexicon(_ context.Context) ([]domain.LexiconEntry, error) {
	if g.mapping.Lexicon == "" {
		return nil, ErrNoFile
	}
	return ReadLexicon(g.mapping.path(g.mapping.Lexicon))
}

// ReadLexicon — разбор CSV словаря. Строка без alias или с неизвестным типом связи — ошибка: словарь с опечаткой
// в типе молча подставлял бы то, что должно было только пояснять.
func ReadLexicon(path string) ([]domain.LexiconEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNoFile
		}
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("%s: заголовок: %w", path, err)
	}
	col := map[string]int{}
	for i, h := range head {
		col[strings.TrimSpace(strings.TrimPrefix(h, "\ufeff"))] = i
	}
	for _, need := range []string{"alias", "kind", "basis"} {
		if _, ok := col[need]; !ok {
			return nil, fmt.Errorf("%s: нет колонки %q", path, need)
		}
	}
	get := func(rec []string, name string) string {
		if i, ok := col[name]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}
	var out []domain.LexiconEntry
	for line := 2; ; line++ {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		if len(rec) == 0 || strings.HasPrefix(strings.TrimSpace(rec[0]), "#") {
			continue
		}
		e := domain.LexiconEntry{Alias: get(rec, "alias"), Canonical: get(rec, "canonical"), Kind: get(rec, "kind"),
			Target: get(rec, "target"), Basis: get(rec, "basis"), Note: get(rec, "note")}
		if e.Target == "" {
			e.Target = domain.AliasTargetTerm
		}
		for _, id := range strings.Split(get(rec, "target_id"), "|") {
			if id = strings.TrimSpace(id); id != "" {
				e.TargetIDs = append(e.TargetIDs, id)
			}
		}
		switch {
		case e.Alias == "":
			return nil, fmt.Errorf("%s:%d: пустой alias", path, line)
		case !lexiconKinds[e.Kind]:
			return nil, fmt.Errorf("%s:%d: тип связи %q (jargon | spelling | rebrand | successor | drop)", path, line, e.Kind)
		case e.Basis == "":
			return nil, fmt.Errorf("%s:%d: у записи %q нет основания", path, line, e.Alias)
		case e.Kind != domain.LexiconDrop && e.Canonical == "":
			return nil, fmt.Errorf("%s:%d: у записи %q нет канонической формы", path, line, e.Alias)
		case e.Target != domain.AliasTargetTerm && e.Target != domain.AliasTargetVendor && e.Target != domain.AliasTargetCategory:
			return nil, fmt.Errorf("%s:%d: цель %q (term | vendor | category)", path, line, e.Target)
		case e.Target != domain.AliasTargetTerm && len(e.TargetIDs) == 0:
			return nil, fmt.Errorf("%s:%d: у записи %q цель %s без target_id", path, line, e.Alias, e.Target)
		}
		out = append(out, e)
	}
	return out, nil
}
