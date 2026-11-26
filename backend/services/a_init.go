// Package services — бизнес-логика по ограниченным контекстам: один контекст — один файл.
package services

import (
	"time"

	"kuznica/api/repositories"
)

// Services — граф сервисов. Слой знает про repositories и externalApi; про Fiber и HTTP — не знает.
//
// Новый контекст (каталог, поиск, корзина) приезжает полем этой структуры и строкой в NewServices —
// иначе хендлеру не через что звать логику.
type Services struct {
	repos *repositories.Repos
	now   func() time.Time
}

// Options — настройки сервисов из config.
type Options struct {
	// Now — часы; nil — time.Now (тесты подставляют свои).
	Now func() time.Time
}

// NewServices — граф сервисов. repos == nil — API без базы: health отдаёт db: down.
func NewServices(repos *repositories.Repos, opts Options) *Services {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Services{repos: repos, now: opts.Now}
}
