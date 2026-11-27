// Оглавление вкладки «Теоретические основы»: части и разделы по порядку. Заголовки разделов и строки
// содержания берутся отсюда, чтобы не расходились. id — якорь раздела (#theory-<id>).

export interface TheorySectionRef {
  id: string
  title: string
}

/** Часть вкладки: key выбирает компоненты разделов в TheoryTab. */
export interface TheoryPart {
  key: 'overview' | 'path' | 'guide' | 'agent' | 'proof'
  title: string
  sections: readonly TheorySectionRef[]
}

export const THEORY_PARTS: readonly TheoryPart[] = [
  {
    key: 'overview',
    title: 'Общая картина',
    sections: [
      { id: 'example', title: 'Один запрос от ввода до выдачи' },
      { id: 'overview', title: 'Из чего состоит решение' },
      { id: 'pipeline', title: 'Все шаги запроса по порядку' },
    ],
  },
  {
    key: 'path',
    title: 'Путь запроса',
    sections: [
      { id: 'normalize', title: 'Как исправляются опечатки, раскладка и сленг' },
      { id: 'understand', title: 'Как поиск находит в запросе количество, срок и названия' },
      { id: 'routes', title: 'Маршруты A–D: когда включаются модели' },
      { id: 'intent', title: 'Как LLM разбирает трудный запрос' },
      { id: 'retrieval', title: 'BM25 и векторы: поиск по словам и по смыслу' },
      { id: 'rrf', title: 'Слияние RRF: из двух списков — один' },
      { id: 'families', title: 'Как лицензии собираются в карточки продуктов' },
      { id: 'rerank', title: 'Как реранкер оценивает товары' },
      { id: 'order', title: 'Почему товары стоят в таком порядке' },
      { id: 'answer', title: 'Куда ведёт первая ссылка и откуда четыре товара' },
      { id: 'empty', title: 'Что видит покупатель, если подходящего мало или нет' },
      { id: 'hardware', title: 'Как ищется техника: парт-номера и характеристики' },
      { id: 'around', title: 'Подсказки, дополнения и закупка' },
      { id: 'trace', title: 'Что записывает трасса' },
    ],
  },
  {
    key: 'guide',
    title: 'Режим подбора',
    sections: [{ id: 'guide', title: 'Как подбор помогает выбрать лицензию' }],
  },
  {
    key: 'agent',
    title: 'Агент SearchOps',
    sections: [{ id: 'agent', title: 'Как SearchOps разбирает неудачные запросы' }],
  },
  {
    key: 'proof',
    title: 'Как мы это проверяем',
    sections: [
      { id: 'quality', title: 'Как измеряется качество' },
      { id: 'cost', title: 'Из чего складывается стоимость' },
      { id: 'offline', title: 'Что работает, если отказала модель или база' },
      { id: 'why', title: 'Почему выбрано так и чего пока нет' },
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
