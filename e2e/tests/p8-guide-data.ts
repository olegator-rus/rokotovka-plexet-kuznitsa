// Заглушки POST /api/guide для D1 и D2 по контракту ответа.
// Числа — из эталонных диалогов (фид магазина), не измерения стенда. Живой API: E2E_GUIDE_LIVE=1 — заглушка не
// ставится, тест идёт на ответах ядра.
import type { Page, Request } from '@playwright/test'

export const GUIDE_LIVE = process.env.E2E_GUIDE_LIVE === '1'

type Json = Record<string, unknown>

const KES_STD = {
  family_id: '3953',
  title: 'Стандартный',
  family_url:
    'https://store.softline.ru/kaspersky/kaspersky-endpoint-security-dlya-biznesa-standartnyy/',
}
const KES_ADV = {
  family_id: '3935',
  title: 'Расширенный',
  family_url:
    'https://store.softline.ru/kaspersky/kaspersky-endpoint-security-dlya-biznesa-rasshirennyy/',
}
const KES_SRC = {
  title: 'Kaspersky Endpoint Security для бизнеса — сравнение редакций',
  url: 'https://www.kaspersky.ru/small-to-medium-business-security/endpoint-select',
  fragment:
    'Шифрование файлов и дисков, управление установкой исправлений — в редакции «Расширенный».',
}

const claims = [
  {
    fact_id: 'kb_kes_adv_encryption',
    text: 'В Расширенном есть шифрование данных',
    source: KES_SRC,
    status: 'verified',
  },
  {
    fact_id: 'kb_kes_std_no_encryption',
    text: 'В Стандартном шифрования нет',
    source: KES_SRC,
    status: 'verified',
  },
  {
    fact_id: 'kb_kes_adv_patch',
    text: 'В Расширенном есть установка обновлений программ',
    source: KES_SRC,
    status: 'verified',
  },
]

const cond = (key: string, label: string, value: string, origin: string) => ({
  key,
  label,
  value,
  origin,
})
const defaults = [
  cond('operation', 'новая лицензия', 'new', 'default'),
  cond('term_months', '1 год', '12', 'default'),
]

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
  trace: [],
  state: { selection_id: 'e2e-sel', turn: 0 },
  ...over,
})

const kesOffer = (
  id: string,
  title: string,
  sku: string,
  tier: string,
  price: number,
  total: number,
) => ({
  offer_id: id,
  title,
  sku,
  tier,
  unit: 'за компьютер',
  quantity: null as number | null,
  role: 'match',
  price_per_unit: price,
  total,
  price_state: 'fixed',
  offer_url: `https://store.softline.ru/product/${id}/`,
})

/** D1, ход 1: «Помочь выбрать» — один вопрос с чипами. */
export const D1_ASK = base({
  type: 'clarification',
  clarification: {
    question: 'Сколько компьютеров нужно защитить?',
    key: 'count:workstation',
    required: false,
    chips: [
      ...['10', '30', '50', '100'].map((v) => ({ label: v, key: 'count:workstation', value: v })),
      { label: 'другое', key: 'count:workstation', value: '' },
    ],
  },
  conditions: defaults,
  trace: [
    {
      step: 'Найдено семейство защиты рабочих мест',
      tool: 'search_catalog',
      ms: 210,
    },
  ],
})

/** D1, ход 2: чип «30» — ступень 25–49, условия по умолчанию видны. */
export const D1_THIRTY = base({
  lead: 'Новые лицензии на год, ступень 25–49.',
  conditions: [cond('count:workstation', '30 ПК', '30', 'user'), ...defaults],
  offers: [
    kesOffer(
      '4980',
      'Kaspersky Endpoint Security для бизнеса Стандартный',
      '',
      '25–49',
      2813.4,
      84402,
    ),
    kesOffer(
      '4851',
      'Kaspersky Endpoint Security для бизнеса Расширенный',
      '',
      '25–49',
      4081.5,
      122445,
    ),
  ],
  trace: [
    {
      step: 'Найдено семейство защиты рабочих мест',
      tool: 'search_catalog',
      ms: 210,
    },
    { step: 'Подобрана ступень 25–49', tool: 'get_family_offers', ms: 95 },
  ],
  state: { selection_id: 'e2e-sel', turn: 2 },
})

/** D1, ход 3: «чем расширенный касперский отличается от стандартного» — таблица. */
export const D1_COMPARE = base({
  type: 'comparison',
  lead: 'В Расширенном есть шифрование данных и установка обновлений программ; в Стандартном их нет.',
  claims,
  conditions: [cond('count:workstation', '30 ПК', '30', 'user'), ...defaults],
  comparison: {
    columns: [KES_STD, KES_ADV],
    rows: [
      {
        key: 'encryption',
        label: 'Шифрование данных',
        cells: [
          { kind: 'no', fact_id: 'kb_kes_std_no_encryption' },
          { kind: 'yes', fact_id: 'kb_kes_adv_encryption' },
        ],
      },
      {
        key: 'patch_management',
        label: 'Установка обновлений программ',
        cells: [{ kind: 'no' }, { kind: 'yes', fact_id: 'kb_kes_adv_patch' }],
      },
      {
        key: 'console',
        label: 'Единая консоль управления',
        cells: [{ kind: 'yes' }, { kind: 'yes' }],
      },
      {
        key: 'certs',
        label: 'Сертификаты',
        cells: [{ kind: 'unknown' }, { kind: 'unknown' }],
      },
      {
        key: 'price_per_seat_year',
        label: 'Цена за компьютер в год, 25–49 мест',
        cells: [
          { kind: 'value', value: 2813.4 },
          { kind: 'value', value: 4081.5 },
        ],
      },
      {
        key: 'total',
        label: 'Итого на 30 ПК',
        cells: [
          { kind: 'value', value: 84402 },
          { kind: 'value', value: 122445 },
        ],
      },
    ],
  },
  trace: [
    {
      step: 'Найдено семейство защиты рабочих мест',
      tool: 'search_catalog',
      ms: 210,
    },
    {
      step: 'Проверены различия по документации вендора',
      tool: 'get_facts',
      ms: 140,
    },
  ],
  state: { selection_id: 'e2e-sel', turn: 3 },
})

/** D1, ход 4: «на самом деле компьютеров будет 50, и шифрование нужно» — Расширенный, ступень 50–99. */
export const D1_FIFTY = base({
  lead: 'Подходит Расширенный. Стандартный убран — в нём нет шифрования.',
  claims,
  conditions: [
    cond('count:workstation', '50 ПК', '50', 'user'),
    ...defaults,
    cond('required_features', 'нужно шифрование', 'encryption', 'user'),
  ],
  offers: [
    kesOffer(
      '4986',
      'Kaspersky Endpoint Security для бизнеса Расширенный',
      'KL4867RAQFS',
      '50–99',
      3940.2,
      197010,
    ),
  ],
  changes: [
    {
      condition: 'Без шифрования',
      effect: 'Стандартный — 2 715,30 ₽ за компьютер',
    },
  ],
  trace: [
    {
      step: 'Найдено семейство защиты рабочих мест',
      tool: 'search_catalog',
      ms: 210,
    },
    {
      step: 'Проверена функция по документации вендора',
      tool: 'get_facts',
      ms: 140,
    },
    { step: 'Подобрана ступень 50–99', tool: 'get_family_offers', ms: 90 },
  ],
  state: { selection_id: 'e2e-sel', turn: 4 },
})

const CRYPTO_SRC = {
  title: 'КриптоПро — условия лицензирования CSP',
  url: 'https://www.cryptopro.ru/products/csp/licensing',
  fragment:
    'Лицензия на рабочее место не даёт права использования в серверной операционной системе.',
}

/** D2, ход 1: «самый дешёвый криптопро csp для сервера» — серверная первой, рабочее место — ниже. */
export const D2_SERVER = base({
  lead: 'Лицензия на рабочее место в серверной ОС не действует — нужна серверная.',
  claims: [
    {
      fact_id: 'kb_cryptopro_ws_not_server',
      text: 'Лицензия на рабочее место не действует в серверной ОС',
      source: CRYPTO_SRC,
      status: 'verified',
    },
  ],
  conditions: [
    cond('object', 'сервер', 'server', 'user'),
    cond('term_months', 'бессрочная', '0', 'inferred'),
  ],
  offers: [
    {
      offer_id: '197958',
      title: 'КриптоПро CSP 5.0: серверная лицензия СКЗИ',
      sku: '',
      tier: '',
      unit: 'за сервер',
      price_per_unit: 70000,
      total: 70000,
      price_state: 'priced',
      offer_url: 'https://store.softline.ru/product/197958/',
      role: 'match',
    },
    ...[
      ['411620', 'КриптоПро CSP 5.0: годовая лицензия на 1 рабочее место', 1850],
      ['25266', 'КриптоПро CSP: бессрочная лицензия на 1 рабочее место', 3700],
    ].map(([id, title, price]) => ({
      offer_id: id,
      title,
      sku: '',
      tier: '',
      unit: 'за рабочее место',
      price_per_unit: price,
      total: price,
      price_state: 'priced',
      offer_url: `https://store.softline.ru/product/${id}/`,
      role: 'rejected',
      note: 'Лицензия на рабочее место — для сервера не подходит',
    })),
  ],
  trace: [
    {
      step: 'Найдено семейство КриптоПро CSP',
      tool: 'search_catalog',
      ms: 180,
    },
    {
      step: 'Проверено правило «рабочее место / сервер»',
      tool: 'validate_license',
      ms: 12,
    },
    {
      step: 'Правила лицензирования',
      ms: 3,
      result: 'подходят: 1, не подходят по праву использования: 2',
    },
  ],
})

/** D2, ход 2: «а для tls-сервера?». */
export const D2_TLS = base({
  lead: 'Для TLS-сервера у КриптоПро отдельная лицензия.',
  claims: D2_SERVER.claims,
  conditions: [cond('object', 'TLS-сервер', 'tls_server', 'user')],
  offers: [
    {
      offer_id: '365870',
      title: 'КриптоПро CSP TLS, лицензия на право использования',
      sku: '',
      tier: '',
      unit: 'сервер',
      price_per_unit: 100000,
      total: 100000,
      price_state: 'fixed',
      offer_url: 'https://store.softline.ru/product/365870/',
    },
  ],
  trace: [
    {
      step: 'Найдено семейство КриптоПро CSP TLS',
      tool: 'search_catalog',
      ms: 160,
    },
  ],
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
  const m = (b.message ?? '').toLowerCase()
  if (/криптопро/i.test(b.query)) return /tls/.test(m) ? D2_TLS : D2_SERVER
  if (b.chip?.value === '30' || b.condition) return D1_THIRTY
  if (/отлича/.test(m)) return D1_COMPARE
  if (/50/.test(m)) return D1_FIFTY
  return D1_ASK
}

/**
 * Поставить заглушку /api/guide; возвращает журнал тел запросов. delayMs — задержка ответа, чтобы увидеть статус
 * хода. fail: сколько первых ходов ответить 503 (ошибка с повтором).
 */
export async function stubGuide(
  page: Page,
  opts: { delayMs?: number; fail?: number; route?: { mode: string; offer?: string } } = {},
) {
  const calls: GuideBody[] = []
  const missing = { error: { code: 'NOT_FOUND', message: 'нет' } }
  // Режим строки (POST /api/guide/route): route — ответ ядра; без него — 404, фронт решает своим правилом.
  await page.route('**/api/guide/route', (r) =>
    r.fulfill({
      status: opts.route ? 200 : 404,
      contentType: 'application/json',
      body: JSON.stringify(opts.route ?? missing),
    }),
  )
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
        body: JSON.stringify({
          error: { code: 'UNAVAILABLE', message: 'Подбор временно недоступен' },
        }),
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
