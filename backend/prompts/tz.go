package prompts

import _ "embed"

//go:embed tz.md
var tzText string

//go:embed tz_list.md
var tzListText string

// TZ — промпт извлечения одной позиции технического задания (контур ТЗ, services/tz_extract.go).
func TZ() Prompt { return parse(tzText) }

// TZList — промпт перечня позиций ТЗ без таблицы спецификации (номер и наименование).
func TZList() Prompt { return parse(tzListText) }
