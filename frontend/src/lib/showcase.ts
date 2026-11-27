// Что витрина показывает гостю (§8). Рычага маржи на витрине нет (он в SearchOps);
// спецификация — действующий путь покупки, она остаётся.

const env = (import.meta as { env?: Record<string, string | undefined> }).env

export interface ShowcaseParts {
  /** «Вы покупали этот продукт» на карточке, подборка «по просмотренному» в панели поиска. */
  personalization: boolean
}

/**
 * Персонализация («Вы покупали», подборка по просмотренному) включена всегда — по решению владельца: она закрывает
 * требование заказчика «учитывать историю клиента — один запрос от разных людей даёт разную выдачу». Видна только
 * демо-покупателю после входа.
 */
export function showcaseParts(): ShowcaseParts {
  return { personalization: true }
}

export const SHOWCASE: ShowcaseParts = showcaseParts()

// По решению владельца над выдачей не показываются карточка «цели» запроса (сервер поле target отдаёт) и
// строка «Нужна помощь с выбором лицензии?». Вернуть: VITE_TARGET_CARD=1, VITE_ASSISTANT_INVITE=1.
export const SHOW_TARGET_CARD = env?.VITE_TARGET_CARD === '1'
export const SHOW_ASSISTANT_INVITE = env?.VITE_ASSISTANT_INVITE === '1'
