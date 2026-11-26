// Фикстуры панели поиска контура выдачи: пока бэкенд /api/suggest не отдаёт новые поля,
// e2e панели работает на них (page.route). Тексты и товары — из mock-каталога, числа — условные для раскладки.

const meta = (partial = false) => ({
  ms: { total: 9, suggestions: 2, products: 6, refinements: 1 },
  partial,
  products_cached: !partial,
  llm_calls: 0,
  embed_calls: 0,
  rerank_calls: 0,
  index_version: 3,
  catalog_version: 'mock-2',
})

const sw = (i: number, family: string, vendor: string, price: number | null, variants: number) => ({
  family_id: `fam-${i}`,
  family_name: family,
  product_id: `prod-${i}`,
  title: family,
  kind: 'software',
  vendor,
  category: 'Антивирусы',
  mpn: '',
  license_line: 'Подписка · 1 год',
  specs_line: '',
  price_from_rub: price,
  price_mode: price === null ? 'quote_required' : 'from',
  variants_count: variants,
  exact: false,
  sku_exact: false,
})

export const BROAD_PRODUCTS = [
  sw(1, 'Kaspersky Endpoint Security для бизнеса', 'Лаборатория Касперского', 2990, 12),
  sw(2, 'Dr.Web Enterprise Security Suite', 'Доктор Веб', 1850, 8),
  sw(3, 'ESET NOD32 Antivirus Business', 'ESET', 2100, 6),
  sw(4, 'Kaspersky Small Office Security', 'Лаборатория Касперского', 4300, 5),
  sw(5, 'PRO32 Total Security', 'PRO32', null, 3),
  sw(6, 'Kaspersky Security для файловых серверов', 'Лаборатория Касперского', 8900, 4),
]

export function broad(q: string, opts: { products?: boolean; partial?: boolean } = {}) {
  const typed = q.length
  const s = (text: string, kind = 'query') => ({
    text,
    highlight: [[0, Math.min(typed, text.length)]],
    kind,
    source: 'successful_query',
    hits: 10,
  })
  return {
    groups: [],
    q,
    normalized: q,
    mode: 'prefix',
    corrected: null,
    refinements: [
      {
        label: 'для бизнеса',
        query: 'антивирус для бизнеса',
        field: null,
        value: null,
        op: null,
        hits: 24,
      },
      {
        label: '1 год',
        query: 'антивирус 1 год',
        field: 'duration_months',
        value: 12,
        op: 'eq',
        hits: 18,
      },
      {
        label: 'продление',
        query: 'антивирус продление',
        field: null,
        value: null,
        op: null,
        hits: 9,
      },
      {
        label: '100 устройств',
        query: 'антивирус 100 устройств',
        field: 'license_quantity',
        value: 100,
        op: 'eq',
        hits: 6,
      },
    ],
    suggestions: [
      s('антивирус для бизнеса'),
      s('антивирус для серверов'),
      s('антивирус на 100 устройств'),
      s('антивирус продление'),
      s('антивирус — в Антивирусы для организаций', 'category'),
    ],
    categories: [
      { id: 'antivirus', name: 'Антивирусы для организаций', share: 0.72 },
      { id: 'endpoint', name: 'Защита конечных точек', share: 0.21 },
    ],
    vendors: [],
    products: opts.products === false || opts.partial ? [] : BROAD_PRODUCTS,
    total_hint: 37,
    meta: meta(Boolean(opts.partial)),
  }
}

/** Ответ на произвольный префикс: подсказки помечены самим префиксом — видно, на какой запрос ответ. */
export function tagged(q: string) {
  return {
    ...broad(q),
    suggestions: [
      {
        text: `${q} ответ`,
        highlight: [[0, q.length]],
        kind: 'query',
        hits: 3,
      },
    ],
    refinements: [],
    categories: [],
    products: [sw(10 + q.length, `Товар для «${q}»`, 'Вендор', 1000, 2)],
    total_hint: q.length,
  }
}

export const EXACT_MPN = 'SRV-R740-2X'

export function exact(q: string) {
  return {
    groups: [],
    q,
    normalized: q.toLowerCase(),
    mode: 'prefix',
    corrected: null,
    refinements: [],
    suggestions: [],
    categories: [],
    vendors: [{ id: 'dell', name: 'Dell' }],
    products: [
      {
        family_id: 'fam-r740',
        family_name: 'Dell PowerEdge R740',
        product_id: 'dell-r740-2x',
        title: 'Сервер Dell PowerEdge R740, 2×Xeon Silver',
        kind: 'hardware',
        vendor: 'Dell',
        category: 'Серверы',
        mpn: EXACT_MPN,
        license_line: '',
        specs_line: '2×Xeon Silver 4214 · 64 ГБ',
        delivery_line: 'Поставка 10–14 дней',
        price_from_rub: 812000,
        price_mode: 'fixed',
        variants_count: 1,
        exact: true,
        sku_exact: true,
      },
    ],
    total_hint: 1,
    meta: meta(),
  }
}

export function layout(q: string) {
  return {
    ...broad('каспер'),
    q,
    corrected: { from: q, to: 'каспер', kind: 'layout' },
    refinements: [],
    suggestions: [
      {
        text: 'kaspersky endpoint security',
        highlight: null,
        kind: 'query',
        hits: 14,
      },
      {
        text: 'kaspersky small office security',
        highlight: null,
        kind: 'query',
        hits: 7,
      },
      {
        text: 'kaspersky для бизнеса',
        highlight: null,
        kind: 'query',
        hits: 6,
      },
    ],
    categories: [{ id: 'antivirus', name: 'Антивирусы для организаций', share: 0.9 }],
    vendors: [{ id: 'kaspersky', name: 'Лаборатория Касперского' }],
    products: BROAD_PRODUCTS.filter((p) => p.vendor === 'Лаборатория Касперского'),
  }
}

export const POPULAR = {
  groups: [],
  q: '',
  normalized: '',
  mode: 'prefix',
  corrected: null,
  refinements: [],
  suggestions: [
    { text: 'windows 11 pro', kind: 'query', hits: 40 },
    { text: 'kaspersky endpoint security', kind: 'query', hits: 31 },
    { text: 'microsoft 365 business', kind: 'query', hits: 22 },
  ],
  categories: [],
  vendors: [],
  products: [],
  total_hint: null,
  meta: meta(),
}

const companion = (id: string, title: string, label: string) => ({
  product_id: id,
  family_id: `fam-${id}`,
  title,
  vendor: 'Dell',
  category: 'Комплектующие',
  license_line: '',
  price_rub: 24000,
  price_mode: 'fixed',
  class: 'complement',
  compatibility: 'verified',
  reason: label,
  conditions: [],
  evidence_level: 'relation',
  source: 'relations',
  source_url: '',
  suggested_quantity: null,
  label,
  upgrade_adds: [],
})

export const RECS_EXACT = {
  basis: 'exact_sku',
  title: 'Дополните Dell PowerEdge R740',
  why: 'Проверенные связи из справочника совместимости',
  items: [
    companion('mem-32', 'Память Dell 32 ГБ DDR4 RDIMM', 'Совместимо с Dell PowerEdge R740'),
    companion('psu-750', 'Блок питания Dell 750 Вт', 'Совместимо с Dell PowerEdge R740'),
    companion('warranty-3y', 'Расширенная гарантия ProSupport 3 года', 'Дополняет решение'),
  ],
}

export const RECS_CATEGORY = {
  basis: 'category_popular',
  title: 'Вам может быть интересно',
  why: 'Популярные семейства категории за 30 дней',
  items: [
    companion('kes-b', 'Kaspersky Endpoint Security для бизнеса Стандартный', 'Популярное'),
    companion('drweb-ess', 'Dr.Web Enterprise Security Suite', 'Популярное'),
  ].map((c) => ({
    ...c,
    vendor: '',
    category: 'Антивирусы',
    label: '',
    reason: '',
  })),
}
