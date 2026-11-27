package ocr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Tesseract — локальное распознавание утилитой tesseract (Apache-2.0) с языками rus и eng: запасной путь, когда
// Vision OCR недоступен (нет роли ai.vision.user у ключа, сеть). В образе API — пакеты tesseract-ocr и
// tesseract-ocr-data-rus (deploy/Dockerfile.api). Картинка идёт через stdin, слова с рамками — из TSV.
type Tesseract struct{ bin string }

// NewTesseract — клиент, если утилита и русский язык есть; иначе nil.
func NewTesseract() *Tesseract {
	bin, err := exec.LookPath("tesseract")
	if err != nil {
		return nil
	}
	out, err := exec.Command(bin, "--list-langs").CombinedOutput()
	if err != nil || !strings.Contains(string(out), "\nrus") {
		return nil
	}
	return &Tesseract{bin: bin}
}

// Recognize — слова картинки (mime не важен: формат tesseract определяет сам).
func (t *Tesseract) Recognize(ctx context.Context, image []byte, _ string) ([]Word, error) {
	cmd := exec.CommandContext(ctx, t.bin, "stdin", "stdout", "-l", "rus+eng", "--psm", "3", "tsv")
	cmd.Stdin = bytes.NewReader(image)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("ocr: tesseract: %w", err)
	}
	return parseTSV(out.String())
}

// parseTSV — слова (level 5) из TSV tesseract: left, top, width, height, conf, text.
func parseTSV(s string) ([]Word, error) {
	lines := strings.Split(s, "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "level") {
		return nil, errors.New("ocr: tesseract вернул не TSV")
	}
	var words []Word
	lineIDs := map[string]int{}
	for _, l := range lines[1:] {
		f := strings.Split(l, "\t")
		if len(f) < 12 || f[0] != "5" {
			continue
		}
		text := strings.TrimSpace(f[11])
		if text == "" {
			continue
		}
		left, _ := strconv.ParseFloat(f[6], 64)
		top, _ := strconv.ParseFloat(f[7], 64)
		w, _ := strconv.ParseFloat(f[8], 64)
		h, _ := strconv.ParseFloat(f[9], 64)
		key := f[1] + "/" + f[2] + "/" + f[3] + "/" + f[4] // страница/блок/абзац/строка
		id, ok := lineIDs[key]
		if !ok {
			id = len(lineIDs) + 1
			lineIDs[key] = id
		}
		words = append(words, Word{Text: text, X0: left, Y0: top, X1: left + w, Y1: top + h, Line: id})
	}
	return words, nil
}

// Chain — распознавание по очереди: следующий — при ошибке предыдущего.
type Chain []interface {
	Recognize(ctx context.Context, image []byte, mime string) ([]Word, error)
}

// Recognize — первый успешный результат; ошибка — последняя.
func (c Chain) Recognize(ctx context.Context, image []byte, mime string) ([]Word, error) {
	err := errors.New("ocr: распознавание не настроено")
	for _, o := range c {
		var words []Word
		if words, err = o.Recognize(ctx, image, mime); err == nil {
			return words, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return nil, err
}
