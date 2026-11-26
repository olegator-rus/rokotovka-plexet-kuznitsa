// Оглавление вкладки «Теоретические основы»: части и разделы по порядку. Заголовки разделов и строки
// содержания берутся отсюда, чтобы не расходились. id — якорь раздела (#theory-<id>).

export interface TheorySectionRef {
  id: string
  title: string
}

export interface TheoryPart {
  title: string
  sections: readonly TheorySectionRef[]
}

export const THEORY_PARTS: readonly TheoryPart[] = [
  {
    title: 'Общая картина',
    sections: [
      { id: 'overview', title: 'Две части решения: поиск и агент' },
      { id: 'pipeline', title: 'Путь запроса целиком' },
    ],
  },
  {
    title: 'Путь запроса',
    sections: [
      { id: 'normalize', title: 'Нормализация: приводим запрос к словам каталога' },
      { id: 'understand', title: 'Что понял поиск: условия, названия, исключения, «или»' },
      { id: 'routes', title: 'Четыре маршрута: A, B, C, D' },
      { id: 'intent', title: 'Маршрут D: LLM и кэш разборов' },
      { id: 'retrieval', title: 'Два источника кандидатов: BM25 и векторы' },
      { id: 'rrf', title: 'Слияние RRF: из двух списков — один' },
      { id: 'families', title: 'От позиций к семействам: порог, условия, добор' },
      { id: 'rerank', title: 'Реранкер: второе мнение на сложных запросах' },
      { id: 'order', title: 'Порядок выдачи: группы, ступени, бизнес-слой' },
      { id: 'empty', title: 'Когда найти нечего: аналоги и честное «нет»' },
      { id: 'hardware', title: 'Техника: парт-номера, характеристики, совместимость' },
      { id: 'around', title: 'Вокруг выдачи: подсказки, дополнения, закупка' },
      { id: 'trace', title: 'Трасса: каждое решение видно' },
    ],
  },
  {
    title: 'Агент SearchOps',
    sections: [{ id: 'agent', title: 'Как поиск чинит сам себя' }],
  },
  {
    title: 'Как мы это проверяем',
    sections: [
      { id: 'quality', title: 'Как измеряется качество' },
      { id: 'cost', title: 'Из чего складывается стоимость' },
      { id: 'offline', title: 'Что будет, если что-то выключить' },
      { id: 'why', title: 'Почему так и чего пока нет' },
      { id: 'glossary', title: 'Словарь терминов' },
    ],
  },
]

const ALL = THEORY_PARTS.flatMap((p) => p.sections)

/** Заголовок раздела по id (для TheorySection). */
export function sectionTitle(id: string): string {
  return ALL.find((s) => s.id === id)?.title ?? id
}

export function anchorOf(id: string): string {
  return `theory-${id}`
}
