package services

import "kuznica/api/externalApi/llm"

// ПДн перед внешней моделью (контур доводки, хвосты). Главная защита — в клиенте модели (externalApi/llm.Client.Chat
// маскирует каждое сообщение): через него идут разбор намерения, агент, судья и классификатор наборов. Здесь —
// то же маскирование раньше, на входе разбора намерения: ключ кэша разборов и строка кэша (intent_cache.normalized)
// строятся по замаскированному тексту, а число замен попадает в трассу (шаг intent → pii_masked).

// maskPII — текст с метками вместо ПДн и число замен (llm.MaskPII).
func maskPII(s string) (string, int) { return llm.MaskPII(s) }
