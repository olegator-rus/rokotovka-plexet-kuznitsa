package catalogsource

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kuznica/api/domain"
)

func writeLexicon(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "lexicon.csv")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadLexicon(t *testing.T) {
	p := writeLexicon(t, "alias,canonical,kind,target,target_id,basis,note\n"+
		"# комментарий\n"+
		"винда,windows,jargon,category,445047|2050,ТЗ,\n"+
		"eset,pro32,successor,vendor,pro32,ComNews,на этом рынке — PRO32\n"+
		"сертификат,,drop,,,фид,\n")
	es, err := ReadLexicon(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 3 {
		t.Fatalf("записей %d, ожидалось 3", len(es))
	}
	if e := es[0]; e.Kind != domain.LexiconJargon || len(e.TargetIDs) != 2 || e.TargetIDs[1] != "2050" {
		t.Fatalf("винда: %+v", e)
	}
	if e := es[1]; e.Note == "" || e.Target != domain.AliasTargetVendor {
		t.Fatalf("eset: %+v", e)
	}
	if e := es[2]; e.Kind != domain.LexiconDrop || e.Target != domain.AliasTargetTerm {
		t.Fatalf("drop: %+v", e)
	}
}

func TestReadLexiconRejects(t *testing.T) {
	head := "alias,canonical,kind,target,target_id,basis\n"
	for name, row := range map[string]string{
		"тип связи":     "винда,windows,slang,term,,ТЗ\n",
		"без основания": "винда,windows,jargon,term,,\n",
		"цель без id":   "eset,pro32,successor,vendor,,ComNews\n",
		"без формы":     "винда,,jargon,term,,ТЗ\n",
	} {
		if _, err := ReadLexicon(writeLexicon(t, head+row)); err == nil || !strings.Contains(err.Error(), "lexicon.csv:2") {
			t.Errorf("%s: ожидалась ошибка со строкой 2, получено %v", name, err)
		}
	}
	if _, err := ReadLexicon(filepath.Join(t.TempDir(), "нет.csv")); !errors.Is(err, ErrNoFile) {
		t.Errorf("нет файла: %v", err)
	}
}

// Словарь продукта на фиде заказчика разбирается без ошибок.
func TestSoftlineLexiconFile(t *testing.T) {
	p := filepath.Join("..", "..", "..", "data", "softline", "lexicon.csv")
	if _, err := os.Stat(p); err != nil {
		t.Skip("нет data/softline/lexicon.csv")
	}
	es, err := ReadLexicon(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) < 40 {
		t.Fatalf("записей %d", len(es))
	}
}
