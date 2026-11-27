// Что витрина показывает гостю (§8). Один флаг: по умолчанию витрина узкая — без
// персонализации по истории и без шаблонов решений. Код этих блоков не удалён: VITE_SHOWCASE_FULL=1 возвращает
// их разом. Рычага маржи на витрине нет (он в SearchOps); спецификация — действующий путь покупки, она остаётся.

const env = (import.meta as { env?: Record<string, string | undefined> }).env

/** Полная витрина контуров коммерции–5. false — узкая витрина контура фида. */
export const SHOWCASE_FULL = env?.VITE_SHOWCASE_FULL === '1'

export interface ShowcaseParts {
  /** «Вы покупали этот продукт» на карточке, подборка «по просмотренному» в панели поиска. */
  personalization: boolean
  /** Карточка «Закупка под задачу» (шаблон решения) над выдачей. */
  solutionTemplates: boolean
}

export function showcaseParts(full: boolean = SHOWCASE_FULL): ShowcaseParts {
  return { personalization: full, solutionTemplates: full }
}

export const SHOWCASE: ShowcaseParts = showcaseParts()

// По решению владельца над выдачей не показываются карточка «цели» запроса (сервер поле target отдаёт) и
// строка «Нужна помощь с выбором лицензии?». Вернуть: VITE_TARGET_CARD=1, VITE_ASSISTANT_INVITE=1.
export const SHOW_TARGET_CARD = env?.VITE_TARGET_CARD === '1'
export const SHOW_ASSISTANT_INVITE = env?.VITE_ASSISTANT_INVITE === '1'
