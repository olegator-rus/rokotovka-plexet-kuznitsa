package catalogsource

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Справка из mapping.help (generic) и файл справки фида data/softline/help.json.
func TestGenericLoadHelp(t *testing.T) {
	ctx := context.Background()
	if _, err := (&Generic{mapping: &Mapping{}}).LoadHelp(ctx); !errors.Is(err, ErrNoFile) {
		t.Fatalf("без поля help: %v", err)
	}
	dir := t.TempDir()
	g := &Generic{mapping: &Mapping{Help: "help.json", dir: dir}}
	if _, err := g.LoadHelp(ctx); !errors.Is(err, ErrNoFile) {
		t.Fatalf("нет файла: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "help.json"), []byte(`[{"id":"x","title":"","triggers":["a"]}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := g.LoadHelp(ctx); err == nil {
		t.Fatal("статья без заголовка должна быть ошибкой")
	}

	feed := &Generic{mapping: &Mapping{Help: "help.json", dir: filepath.Join("..", "..", "..", "data", "softline")}}
	hs, err := feed.LoadHelp(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, h := range hs {
		ids[h.ID] = true
		if h.URL != "/help#"+h.ID {
			t.Errorf("%s: ссылка %q — только на свою статью страницы помощи", h.ID, h.URL)
		}
	}
	for _, id := range []string{"invoice", "license", "renewal", "documents", "reestr", "quote"} {
		if !ids[id] {
			t.Errorf("нет статьи %s", id)
		}
	}
}
