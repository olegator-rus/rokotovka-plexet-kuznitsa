package services

import (
	"encoding/json"

	"kuznica/api/domain"
	"kuznica/api/externalApi/llm"
)

// ПДн перед внешней моделью (контур доводки, хвосты). Главная защита — в клиенте модели (externalApi/llm.Client.Chat
// маскирует каждое сообщение): через него идут разбор намерения, агент, судья и классификатор наборов. Здесь —
// то же маскирование раньше, на входе разбора намерения: ключ кэша разборов и строка кэша (intent_cache.normalized)
// строятся по замаскированному тексту, а число замен попадает в трассу (шаг intent → pii_masked).

// maskPII — текст с метками вместо ПДн и число замен (llm.MaskPII).
func maskPII(s string) (string, int) { return llm.MaskPII(s) }

// maskJSONStrings — сохранённая трасса без ПДн: каждая строка JSON (запрос в шаге нормализации, найденные слова,
// исправления) проходит maskPII; числа и ключи не трогаются, поэтому JSON остаётся валидным. Без замен — исходные
// байты.
func maskJSONStrings(raw domain.JSONB) (domain.JSONB, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	n := 0
	var walk func(any) any
	walk = func(x any) any {
		switch t := x.(type) {
		case string:
			m, k := maskPII(t)
			n += k
			return m
		case []any:
			for i := range t {
				t[i] = walk(t[i])
			}
		case map[string]any:
			for key, val := range t {
				t[key] = walk(val)
			}
		}
		return x
	}
	v = walk(v)
	if n == 0 {
		return raw, nil
	}
	return json.Marshal(v)
}
