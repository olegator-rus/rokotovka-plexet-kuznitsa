package services

import "context"

// Флаги контура коммерции на один запрос (коммерческая лестница C0–C3): прогон оценки включает и
// выключает блоки по отдельности, не трогая флаги процесса. Код блоков читает флаги только через featuresFor —
// иначе C-лестница не различит конфигурации.

type featuresKey struct{}

// WithFeatures — флаги контура коммерции для поисков в ctx вместо флагов процесса. Флаг, выключенный в процессе
// (config), включить нельзя: итог — пересечение.
func WithFeatures(ctx context.Context, f Features) context.Context {
	return context.WithValue(ctx, featuresKey{}, f)
}

// featuresFor — действующие флаги контура коммерции для запроса: флаги процесса, суженные значением из ctx.
func (s *Services) featuresFor(ctx context.Context) Features {
	f := s.features
	if ctx == nil {
		return f
	}
	if o, ok := ctx.Value(featuresKey{}).(Features); ok {
		f.Hardware = f.Hardware && o.Hardware
		f.Companions = f.Companions && o.Companions
		f.MarginPolicy = f.MarginPolicy && o.MarginPolicy
		f.Solutions = f.Solutions && o.Solutions
	}
	return f
}
