package services

import (
	"encoding/json"
	"os"
	"strings"
	"sync/atomic"
)

// productPictures — картинки позиций из фида заказчика: id позиции → URL на CDN магазина (img.softline.com).
// Файл пишет конвертер фида (scripts/data/softline_feed.py → feed/pictures.json); в индекс и базу картинка не
// попадает, поэтому для неё не нужен переимпорт. Нет файла (mock, чистый клон без фида) — картинок нет, карточка
// показывает значок производителя.
var productPictures atomic.Pointer[map[string]string]

// loadPictures — прочитать файл картинок; ошибка чтения не мешает поиску — картинок просто нет.
func loadPictures(path string) {
	if path == "" {
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	m := map[string]string{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return
	}
	for id, u := range m {
		if !strings.HasPrefix(u, "https://") {
			delete(m, id) // только https-адреса: страница не грузит смешанный контент и не ходит по чужим схемам
		}
	}
	productPictures.Store(&m)
}

// pictureOf — URL картинки позиции; "" — картинки нет.
func pictureOf(productID string) string {
	m := productPictures.Load()
	if m == nil {
		return ""
	}
	return (*m)[productID]
}
