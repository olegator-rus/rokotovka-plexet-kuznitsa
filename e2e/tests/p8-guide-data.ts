// Заглушки POST /api/guide и /api/guide/route для сценариев контура экрана по
// контракту экрана. Числа — из текста сценариев (фид магазина), не измерения стенда.
// Живой API: E2E_GUIDE_LIVE=1 — заглушки не ставятся, тест идёт на ответах ядра.
import type { Page, Request } from '@playwright/test'

export const GUIDE_LIVE = process.env.E2E_GUIDE_LIVE === '1'

type Json = Record<string, unknown>

const cond = (key: string, label: string, value: string, origin: string) => ({
  key,
  label,
  value,
  origin,
})
const DIRECTION = cond('direction', 'антивирус для рабочих мест', 'endpoint', 'inferred')
const defaults = [
  cond('operation', 'новая лицензия', 'new', 'default'),
  cond('term_months', '1 год', '12', 'default'),
]
const count = (n: number) => cond('count:workstation', `${n} ПК`, String(n), 'user')

const KES_DOC = {
  label: 'Документация вендора',
  fragment:
    'Шифрование файлов и дисков, управление установкой исправлений — в редакции «Расширенный».',
  url: 'https://www.kaspersky.ru/small-to-medium-business-security/endpoint-select',
}
const CRYPTO_RULES = {
  label: 'Правила лицензирования КриптоПро',
  fragment: 'Лицензия на рабочее место не может использоваться в серверной операционной системе.',
  url: 'https://cryptopro.ru/buy/price',
}

interface OfferIn {
  id: string
  family: string
  title: string
  subtitle: string
  unit: string
  qty: number
  price: number | null
  total: number | null
  line?: string
  priceText: string
  role?: string
  reason?: { code: string; text: string }
  pack?: { size: number; packs: number; seats: number; spare: number; text: string }
}

const offer = (o: OfferIn) => ({
  offer_id: o.id,
  family_id: o.family,
  title: `${o.title} (позиция каталога)`,
  family_title: o.title,
  sku: `SKU-${o.id}`,
  tier: '',
  unit: o.unit,
  quantity: o.qty,
  price_per_unit: o.price,
  total: o.total,
  price_state: o.price ? 'priced' : 'on_request',
  offer_url: `https://store.softline.ru/product/${o.id}/`,
  term: '1 год',
  role: o.role ?? 'match',
  display: {
    title: o.title,
    subtitle: o.subtitle,
    details: [
      { label: 'Артикул', value: `SKU-${o.id}` },
      { label: 'Позиция каталога', value: `${o.title} (позиция каталога)` },
    ],
  },
  sale_unit: o.unit,
  sale_unit_kind: o.pack ? 'package' : 'per_object',
  unit_price: o.price,
  pay_quantity: o.pack ? o.pack.packs : o.qty,
  price_line: o.line ?? '',
  price_text: o.priceText,
  package: o.pack ?? null,
  reason: o.reason ?? null,
})

const base = (over: Json): Json => ({
  type: 'selection',
  lead: '',
  claims: [],
  comparison: null,
  offers: [],
  clarification: null,
  conditions: [],
  changes: [],
  next: ['add_to_spec', 'change_conditions', 'request_quote'],
  trace: [{ step: 'Найдены семейства', tool: 'search_catalog', ms: 120, result: 'семейств: 2' }],
  state: { selection_id: 'e2e-guide', turn: 1 },
  preliminary: false,
  notice: '',
  gaps: [],
  screen: null,
  ...over,
})

const summary = (id: string, total: number, text: string, formula: string) => ({
  offer_id: id,
  label: 'Стоимость закупки',
  total,
  total_text: text,
  formula,
  action: 'add_to_spec',
  hint: '',
})

// ── Сценарий 1: «подбери антивирь» ──

export const S1_ASK = base({
  type: 'clarification',
  conditions: [DIRECTION, ...defaults],
  clarification: {
    key: 'count:workstation',
    question: 'Сколько компьютеров нужно защитить?',
    chips: ['10', '30', '50', '100'].map((v) => ({ label: v, key: 'count:workstation', value: v })),
    required: false,
  },
})

const S1_OFFERS = [
  offer({
    id: 'drweb20',
    family: '4069',
    title: 'Dr.Web Desktop Security Suite',
    subtitle: 'Новая лицензия · пакет на 30 компьютеров · 1 год',
    unit: 'пакет на 30 компьютеров',
    qty: 30,
    price: 51521.73,
    total: 51521.73,
    line: '1 × 51 521,73 ₽',
    priceText: '51 521,73 ₽ за пакет',
  }),
  offer({
    id: 'kesstd30',
    family: '3953',
    title: 'Kaspersky Endpoint Security — Стандартный',
    subtitle: 'Новая лицензия · 30 компьютеров · 1 год',
    unit: 'за компьютер',
    qty: 30,
    price: 2813.4,
    total: 84402,
    line: '30 × 2 813,40 ₽',
    priceText: '2 813,40 ₽ за компьютер',
  }),
  offer({
    id: 'kesadv30',
    family: '3935',
    title: 'Kaspersky Endpoint Security — Расширенный',
    subtitle: 'Новая лицензия · 30 компьютеров · 1 год',
    unit: 'за компьютер',
    qty: 30,
    price: 4081.5,
    total: 122445,
    line: '30 × 4 081,50 ₽',
    priceText: '4 081,50 ₽ за компьютер',
  }),
]

export const S1_THIRTY = base({
  conditions: [DIRECTION, count(30), ...defaults],
  offers: S1_OFFERS,
  clarification: {
    key: 'feature',
    question: 'Какие функции защиты обязательны?',
    chips: [
      { label: 'нужно: шифрование данных', key: 'feature', value: 'encryption' },
      { label: 'нужно: установка обновлений программ', key: 'feature', value: 'patch_management' },
      { label: 'ничего из этого', key: 'feature', value: '' },
    ],
    required: false,
  },
  screen: {
    title: 'Подбор лицензии',
    task: 'Антивирус для организации',
    result: {
      basis: 'none',
      heading: 'Выберите подходящий вариант',
      offer_id: '',
      reason: '',
      source: null,
    },
    selected_offer_id: '',
    options: S1_OFFERS.map((o) => o.offer_id),
    comparison: null,
    task_cost: {
      title: 'Стоимость защиты 30 компьютеров на 1 год',
      columns: ['Как продаётся', 'За 30 компьютеров'],
      rows: S1_OFFERS.map((o, i) => ({
        offer_id: o.offer_id,
        title: o.display.title,
        how_sold: o.sale_unit,
        total: o.total,
        total_text: ['51 521,73 ₽', '84 402 ₽', '122 445 ₽'][i],
        note: '',
      })),
      note: 'Состав функций и условия лицензирования различаются',
      cheapest: 'Меньшая стоимость закупки: Dr.Web Desktop Security Suite.',
    },
    changed: null,
    others: [],
    summary: {
      offer_id: '',
      label: 'Стоимость закупки',
      total: null,
      total_text: '',
      formula: '',
      action: 'choose',
      hint: 'Выберите вариант, чтобы добавить в спецификацию',
    },
  },
})

export const S1_ENCRYPTION = base({
  conditions: [
    DIRECTION,
    count(30),
    ...defaults,
    cond('feature:encryption', 'нужно шифрование данных', 'encryption', 'user'),
  ],
  offers: [
    S1_OFFERS[2],
    { ...S1_OFFERS[1], role: 'alternative', reason: { code: 'missing_feature', text: 'Нет шифрования данных' } },
    { ...S1_OFFERS[0], role: 'alternative', reason: { code: 'feature_unknown', text: 'Нет сведений о шифровании' } },
  ],
  screen: {
    title: 'Подбор лицензии',
    task: 'Антивирус для организации',
    result: {
      basis: 'required_feature',
      heading: 'Подходит под ваши условия',
      offer_id: 'kesadv30',
      reason: 'Включает шифрование данных. В Стандартной редакции этой функции нет.',
      source: KES_DOC,
    },
    selected_offer_id: 'kesadv30',
    options: ['kesadv30'],
    comparison: null,
    task_cost: null,
    changed: { text: 'Изменено: шифрование обязательно', items: ['шифрование обязательно'], price_change: null },
    others: [
      {
        offer_id: 'kesstd30',
        title: 'Kaspersky Endpoint Security — Стандартный',
        subtitle: '',
        price_text: '2 813,40 ₽ за компьютер',
        reason: { code: 'missing_feature', text: 'Нет шифрования данных' },
      },
      {
        offer_id: 'drweb20',
        title: 'Dr.Web Desktop Security Suite',
        subtitle: '',
        price_text: '51 521,73 ₽ за пакет',
        reason: { code: 'feature_unknown', text: 'Нет сведений о шифровании' },
      },
    ],
    summary: summary('kesadv30', 122445, '122 445 ₽', '30 × 4 081,50 ₽'),
  },
})

// ── Сценарий 2: сравнение редакций и смена условий одной репликой ──

const STD30 = S1_OFFERS[1]!
const ADV30 = S1_OFFERS[2]!
const cell = (mark: string, text: string) => ({ mark, text })
const row = (key: string, label: string, priority: number, cells: Json[], kind = 'feature') => ({
  key,
  label,
  priority,
  asked: false,
  kind,
  cells,
})

export const S2_COMPARE = base({
  type: 'comparison',
  conditions: [DIRECTION, cond('vendor', 'Kaspersky', 'kaspersky', 'inferred'), count(30), ...defaults],
  offers: [STD30, ADV30],
  screen: {
    title: 'Подбор лицензии',
    task: 'Антивирус для организации · Kaspersky',
    result: {
      basis: 'lowest_total',
      heading: 'Меньшая стоимость закупки',
      offer_id: 'kesstd30',
      reason: 'Меньшая стоимость закупки: Стандартный, на 38 043 ₽ дешевле.',
      source: null,
    },
    selected_offer_id: 'kesstd30',
    options: ['kesstd30', 'kesadv30'],
    comparison: {
      title: 'Различия редакций',
      columns: [
        { offer_id: 'kesstd30', family_id: '3953', title: 'Стандартный' },
        { offer_id: 'kesadv30', family_id: '3935', title: 'Расширенный' },
      ],
      rows: [
        row('encryption', 'Шифрование данных', 1, [cell('no', 'Нет'), cell('yes', 'Есть')]),
        row('patch', 'Установка обновлений', 1, [cell('no', 'Нет'), cell('yes', 'Есть')]),
        row('console', 'Единая консоль', 4, [cell('yes', 'Есть'), cell('yes', 'Есть')]),
        row(
          'unit_price',
          'За компьютер',
          3,
          [cell('value', '2 813,40 ₽'), cell('value', '4 081,50 ₽')],
          'commercial',
        ),
      ],
      more_rows: [
        row('mobile', 'Защита мобильных устройств', 5, [cell('conditional', 'С условиями'), cell('yes', 'Есть')]),
      ],
      hidden_no_data: 2,
      hidden_label: 'Ещё 2 характеристики без сведений у вендора',
      vendors_note: '',
      difference: 'Меньшая стоимость закупки: Стандартный, на 38 043 ₽ дешевле.',
      source: KES_DOC,
    },
    task_cost: null,
    changed: null,
    others: [],
    summary: summary('kesstd30', 84402, '84 402 ₽', '30 × 2 813,40 ₽'),
  },
})

const STD50 = offer({
  id: 'kesstd50',
  family: '3953',
  title: 'Kaspersky Endpoint Security — Стандартный',
  subtitle: 'Новая лицензия · 50 компьютеров · 1 год',
  unit: 'за компьютер',
  qty: 50,
  price: 2715.3,
  total: 135765,
  line: '50 × 2 715,30 ₽',
  priceText: '2 715,30 ₽ за компьютер',
  role: 'rejected',
  reason: { code: 'missing_feature', text: 'Не подходит: нет шифрования данных' },
})
const ADV50 = offer({
  id: 'kesadv50',
  family: '3935',
  title: 'Kaspersky Endpoint Security — Расширенный',
  subtitle: 'Новая лицензия · 50 компьютеров · 1 год',
  unit: 'за компьютер',
  qty: 50,
  price: 3940.2,
  total: 197010,
  line: '50 × 3 940,20 ₽',
  priceText: '3 940,20 ₽ за компьютер',
})

export const S2_FIFTY = base({
  conditions: [
    DIRECTION,
    cond('vendor', 'Kaspersky', 'kaspersky', 'inferred'),
    count(50),
    ...defaults,
    cond('feature:encryption', 'нужно шифрование данных', 'encryption', 'user'),
  ],
  offers: [ADV50, STD50],
  screen: {
    title: 'Подбор лицензии',
    task: 'Антивирус для организации · Kaspersky',
    result: {
      basis: 'required_feature',
      heading: 'Подходит под ваши условия',
      offer_id: 'kesadv50',
      reason: 'Включает шифрование данных.',
      source: KES_DOC,
    },
    selected_offer_id: 'kesadv50',
    options: ['kesadv50'],
    comparison: null,
    task_cost: null,
    changed: {
      text: 'Изменено: 30 → 50 компьютеров, шифрование обязательно',
      items: ['30 → 50 компьютеров', 'шифрование обязательно'],
      price_change: {
        offer_id: 'kesadv50',
        label: 'Цена за компьютер изменилась',
        from: { tier: '25–49 компьютеров', unit_price: 4081.5, text: '4 081,50 ₽' },
        to: { tier: '50–99 компьютеров', unit_price: 3940.2, text: '3 940,20 ₽' },
      },
    },
    others: [
      {
        offer_id: 'kesstd50',
        title: 'Kaspersky Endpoint Security — Стандартный',
        subtitle: '',
        price_text: '2 715,30 ₽ за компьютер',
        reason: { code: 'missing_feature', text: 'Не подходит: нет шифрования данных' },
      },
    ],
    summary: summary('kesadv50', 197010, '197 010 ₽', '50 × 3 940,20 ₽'),
  },
})

// ── Сценарий 3: «самый дешёвый криптопро csp для сервера» ──

const SERVER = offer({
  id: '197958',
  family: '22268',
  title: 'КриптоПро CSP 5.0',
  subtitle: 'Серверная лицензия · бессрочная',
  unit: 'за сервер',
  qty: 1,
  price: 70000,
  total: 70000,
  line: '1 × 70 000 ₽',
  priceText: '70 000 ₽ за сервер',
})
const notServer = { code: 'not_for_server_os', text: 'Не подходит для серверной ОС' }

export const S3_SERVER = base({
  lead: 'Лицензия на рабочее место не действует в серверной ОС — нужна серверная.',
  conditions: [
    cond('vendor', 'КриптоПро', 'kripto-pro', 'user'),
    cond('object', 'для сервера', 'server', 'user'),
    cond('operation', 'новая лицензия', 'new', 'default'),
  ],
  offers: [
    SERVER,
    offer({
      id: '411620',
      family: '22268',
      title: 'КриптоПро CSP 5.0',
      subtitle: 'Рабочее место · 1 год',
      unit: 'за рабочее место',
      qty: 1,
      price: 1850,
      total: 1850,
      priceText: '1 850 ₽',
      role: 'rejected',
      reason: notServer,
    }),
  ],
  screen: {
    title: 'Подбор лицензии',
    task: 'КриптоПро CSP для серверной ОС',
    result: {
      basis: 'only_valid',
      heading: 'Подходящая лицензия',
      offer_id: '197958',
      reason: 'Лицензия на рабочее место в серверной ОС не действует.',
      source: CRYPTO_RULES,
    },
    selected_offer_id: '197958',
    options: ['197958'],
    comparison: null,
    task_cost: null,
    changed: null,
    others: [
      {
        offer_id: '411620',
        title: 'КриптоПро CSP 5.0',
        subtitle: 'Рабочее место · 1 год',
        price_text: '1 850 ₽',
        reason: notServer,
      },
      {
        offer_id: '410137',
        title: 'КриптоПро CSP 5.0',
        subtitle: 'Рабочее место · бессрочная',
        price_text: '3 700 ₽',
        reason: notServer,
      },
      {
        offer_id: '365870',
        title: 'КриптоПро CSP TLS',
        subtitle: 'бессрочная',
        price_text: '100 000 ₽',
        reason: { code: 'tls_server_only', text: 'Отдельная лицензия для TLS-сервера' },
      },
    ],
    summary: summary('197958', 70000, '70 000 ₽', '1 × 70 000 ₽'),
  },
})

export interface GuideBody {
  session_id: string
  query: string
  message?: string
  chip?: { key: string; value: string }
  condition?: { key: string; value: string | null }
  state?: unknown
}

/** Ответ заглушки по телу хода. */
function reply(b: GuideBody): Json {
  const q = b.query.toLowerCase()
  const m = (b.message ?? '').toLowerCase()
  if (/криптопро/.test(q)) return S3_SERVER
  if (/отлича/.test(q)) {
    if (/50/.test(m) || b.condition) return S2_FIFTY
    return S2_COMPARE
  }
  if (b.chip?.key === 'feature' && b.chip.value === 'encryption') return S1_ENCRYPTION
  if (b.chip?.key === 'count:workstation' || b.condition) return S1_THIRTY
  return S1_ASK
}

/** Режим строки (POST /api/guide/route) по запросу — как правила ядра (UI §2). */
function routeOf(q: string): Json {
  const s = q.toLowerCase()
  if (/подбер|отлича|самый дешёв|для сервера/.test(s))
    return { mode: 'guide', reason: 'question', offer: '', open_now: true, open_reason: 'ask' }
  if (/^kl\d|^криптопро csp 5\.0$/.test(s))
    return { mode: 'search', reason: 'product_name', offer: '', open_now: false, open_reason: '' }
  return { mode: 'search_offer', reason: 'general_request', offer: '', open_now: false, open_reason: '' }
}

/**
 * Поставить заглушки /api/guide и /api/guide/route; возвращает журнал тел ходов. delayMs — задержка ответа, чтобы
 * увидеть ожидание и пересчёт. fail — сколько первых ходов ответить 503 (ошибка с повтором). route — постоянный
 * ответ режима строки вместо правил по запросу.
 */
export async function stubGuide(
  page: Page,
  opts: { delayMs?: number; fail?: number; route?: Json } = {},
) {
  const calls: GuideBody[] = []
  const missing = { error: { code: 'NOT_FOUND', message: 'нет' } }
  await page.route('**/api/guide/route', (r) => {
    const body = r.request().postDataJSON() as { query: string }
    return r.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(opts.route ?? routeOf(body.query)),
    })
  })
  // Черновик спецификации подбора (POST /api/guide/spec): нет — строка берётся из ответа хода.
  await page.route('**/api/guide/spec', (r) =>
    r.fulfill({ status: 404, contentType: 'application/json', body: JSON.stringify(missing) }),
  )
  let fails = opts.fail ?? 0
  await page.route('**/api/guide', async (route) => {
    const req: Request = route.request()
    const body = req.postDataJSON() as GuideBody
    calls.push(body)
    if (opts.delayMs) await new Promise((r) => setTimeout(r, opts.delayMs))
    if (fails > 0) {
      fails--
      return route.fulfill({
        status: 503,
        contentType: 'application/json',
        body: JSON.stringify({ error: { code: 'UNAVAILABLE', message: 'Подбор временно недоступен' } }),
      })
    }
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(reply(body)),
    })
  })
  return calls
}
