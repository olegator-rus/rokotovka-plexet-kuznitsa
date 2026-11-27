// Ответы API контура коммерции по контракту — пока бэкенд-треки не отдают
// поля, витрина проверяется на них. P2_LIVE=1 — подмены выключены, тесты идут на живом API (когда готово).
import type { Page, Route } from '@playwright/test'

export const P2_LIVE = process.env.P2_LIVE === '1'

type Json = Record<string, unknown>

const trace = (steps: Json[]) => ({
  request_id: 'p2-e2e',
  route: 'A',
  parse_source: 'rules',
  config: 'live',
  effective_config: 'B3',
  rules_version: 1,
  total_ms: 38,
  cost_rub: 0,
  steps: [
    { name: 'normalize', ms: 0, changes: [] },
    { name: 'attributes', ms: 0, found: [], unresolved: [] },
    { name: 'route', ms: 0, route: 'A', signals: [] },
    ...steps,
  ],
  items: [],
  rejected: [],
  llm: null,
})

function item(id: string, title: string, over: Json = {}): Json {
  return {
    product_id: id,
    family_id: id,
    title,
    vendor: 'Microsoft',
    category: 'Серверные ОС',
    license_line: 'Бессрочная · 16 ядер',
    price_rub: 120000,
    price_on_request: false,
    group: 'match',
    matches_query: true,
    purchased_before: false,
    final: 0.7,
    kind: 'software',
    mpn: '',
    price_mode: 'fixed',
    compatibility: null,
    required: [],
    variants: [],
    ...over,
  }
}

function companion(id: string, title: string, over: Json = {}): Json {
  return {
    product_id: id,
    family_id: id,
    title,
    vendor: 'Microsoft',
    category: 'Лицензии доступа',
    license_line: 'Бессрочная · 1 пользователь',
    price_rub: 3200,
    price_mode: 'fixed',
    class: 'conditional_required',
    compatibility: 'verified',
    reason: 'Нужны для доступа пользователей к серверу',
    conditions: ['access_license_required', 'no_existing_equivalent_license'],
    evidence_level: 'E4',
    source: 'vendor_documentation',
    source_url: 'https://www.microsoft.com/licensing/',
    suggested_quantity: 50,
    label: 'Проверьте необходимость',
    upgrade_adds: [],
    ...over,
  }
}

export const WS_QUERY = 'windows server 2025 standard на 50 пользователей'
export const MEM_QUERY = 'память 32 гб для Dell R740'

/** F16 + F17: якорь с условно обязательным и блок «Дополните решение». */
export function wsResponse(query: string): Json {
  const cal = companion('ws-cal-2025-user', 'Windows Server 2025 CAL, лицензия на пользователя')
  const rds = companion(
    'ws-rds-cal-2025',
    'Windows Server 2025 RDS CAL, лицензия на пользователя',
    {
      category: 'Лицензии удалённого доступа',
      conditions: ['rds_planned'],
      reason: 'Нужны, если пользователи работают через удалённый рабочий стол',
      price_rub: 9800,
    },
  )
  return {
    request_id: `p2-ws-${Date.now()}`,
    query,
    normalized: query,
    config: 'live',
    chips: [
      {
        field: 'category',
        value: 'server-os',
        label: 'Серверная ОС',
        strict: true,
      },
      { field: 'edition', value: 'Standard', label: 'Standard', strict: true },
      { field: 'version', value: '2025', label: '2025', strict: true },
      {
        field: 'license_quantity',
        value: '50',
        label: '50 пользователей',
        strict: true,
      },
    ],
    total: 5,
    items: [
      item('ws-2025-std', 'Microsoft Windows Server 2025 Standard, 16 ядер', {
        group: 'exact',
        final: 0.93,
        required: [cal, rds],
      }),
      item('ws-2025-std-2core', 'Microsoft Windows Server 2025 Standard, дополнительные 2 ядра', {
        price_rub: 15000,
      }),
      item('ws-2025-dc', 'Microsoft Windows Server 2025 Datacenter, 16 ядер', {
        price_rub: null,
        price_on_request: true,
        price_mode: 'quote_required',
      }),
      item('ws-2022-std', 'Microsoft Windows Server 2022 Standard, 16 ядер', {
        final: 0.6,
      }),
      item('ws-2025-ess', 'Microsoft Windows Server 2025 Essentials', {
        final: 0.55,
      }),
    ],
    relaxed: [],
    related_queries: [],
    complements: [
      {
        family_id: 'old',
        family_name: 'Старый блок',
        product_id: 'old-1',
        title: 'Старый блок С этим покупают',
        vendor: 'X',
        category: 'Y',
        license_line: '',
        price_rub: 1,
        price_on_request: false,
        note: '',
      },
    ],
    companions: {
      anchor: {
        product_id: 'ws-2025-std',
        family_id: 'ws-2025-std',
        confidence: 'high',
      },
      items: [
        companion('veeam-backup', 'Veeam Backup & Replication Standard', {
          vendor: 'Veeam',
          category: 'Резервное копирование',
          class: 'complement',
          label: 'Дополняет решение — резервное копирование',
          reason: 'Резервные копии сервера и виртуальных машин',
          source: 'catalog_rule',
          source_url: '',
          suggested_quantity: null,
          price_rub: 89000,
        }),
        companion('impl-ws', 'Внедрение Windows Server, пакет «Старт»', {
          vendor: 'Softline',
          category: 'Услуги внедрения',
          class: 'complement',
          label: 'Дополняет решение — внедрение и поддержка',
          reason: 'Установка и настройка сервера под ключ',
          source: 'co_purchase',
          source_url: '',
          suggested_quantity: null,
          price_rub: null,
          price_mode: 'quote_required',
        }),
        companion('ws-sa', 'Software Assurance для Windows Server 2025, 1 год', {
          class: 'renewal',
          label: 'Продление и поддержка',
          reason: 'Обновления до новых версий в течение года',
          category: 'Продление',
          suggested_quantity: null,
          price_rub: 36000,
        }),
        companion('bad', 'Несовместимое — не должно показаться', {
          class: 'compatible',
          compatibility: 'incompatible',
        }),
      ],
    },
    facets: [],
    trace: trace([
      { name: 'hardware', ms: 0, skipped: true, reason: 'not_needed' },
      {
        name: 'companions',
        ms: 2,
        anchor: { product_id: 'ws-2025-std', confidence: 'high' },
        considered: [
          {
            product_id: 'ws-cal-2025-user',
            class: 'conditional_required',
            source: 'vendor_documentation',
          },
          {
            product_id: 'veeam-backup',
            class: 'complement',
            source: 'catalog_rule',
          },
        ],
        rejected: [{ product_id: 'ws-cal-2022-user', reason: 'version_mismatch' }],
      },
    ]),
  }
}

/** F20 + F21: оборудование, совместимость, фасеты. */
export function memResponse(query: string, filters: Json[] = []): Json {
  const mem = (id: string, title: string, over: Json = {}) =>
    item(id, title, {
      vendor: 'Dell',
      category: 'Серверная память',
      license_line: 'DDR4 · 32 ГБ · 2933 МГц',
      kind: 'hardware',
      price_rub: 41000,
      ...over,
    })
  return {
    request_id: `p2-mem-${Date.now()}`,
    query,
    normalized: query,
    config: 'live',
    chips: [
      {
        field: 'category',
        value: 'server-ram',
        label: 'Серверная память',
        strict: true,
      },
      {
        field: 'ram_gb',
        value: '32',
        label: 'Память 32 ГБ',
        strict: true,
        op: 'eq',
        unit: 'ГБ',
      },
      {
        field: 'compat_target',
        value: 'dell-poweredge-r740',
        label: 'Совместимо с Dell PowerEdge R740',
        strict: true,
      },
    ],
    total: 3,
    items: [
      mem('dell-32g-2933', 'Модуль памяти Dell 32 ГБ DDR4-2933 RDIMM', {
        group: 'exact',
        mpn: 'SNS-R740-32G',
        compatibility: {
          target_model: 'dell-poweredge-r740',
          state: 'verified',
          condition: '',
        },
      }),
      mem('hmaa-32g', 'Модуль памяти SK hynix 32 ГБ DDR4-2933 RDIMM', {
        vendor: 'SK hynix',
        mpn: 'HMA84GR7CJR4N-WM',
        compatibility: {
          target_model: 'dell-poweredge-r740',
          state: 'conditional',
          condition: 'BIOS не ниже 2.12',
        },
      }),
      mem('kingston-32g', 'Модуль памяти Kingston 32 ГБ DDR4-3200 RDIMM', {
        vendor: 'Kingston',
        mpn: 'KSM32RD4/32HDR',
        price_mode: 'from',
        compatibility: {
          target_model: 'dell-poweredge-r740',
          state: 'unknown',
          condition: '',
        },
      }),
    ],
    relaxed: [],
    related_queries: [],
    complements: [],
    companions: null,
    facets: [
      {
        key: 'ram_gb',
        label: 'Объём',
        unit: 'ГБ',
        type: 'number',
        min: 8,
        max: 128,
        values: [],
      },
      {
        key: 'memory_type',
        label: 'Тип памяти',
        unit: '',
        type: 'enum',
        min: null,
        max: null,
        values: [
          { value: 'DDR4', count: 3 },
          { value: 'DDR5', count: 1 },
        ],
      },
    ],
    trace: trace([
      {
        name: 'hardware',
        ms: 1,
        mpn_detected: false,
        units: [{ text: '32 гб', key: 'ram_gb', op: 'eq', value: 32, unit: 'ГБ' }],
        compat_target: 'dell-poweredge-r740',
        filters,
      },
      { name: 'companions', ms: 0, anchor: null, considered: [], rejected: [] },
    ]),
  }
}

export interface Captured {
  events: Json[]
  searches: Json[]
  quotes: Json[]
}

/**
 * Подмены: /api/search по запросу отдаёт фикстуру (прочие запросы — живой API), /api/meta — живой ответ
 * плюс поля контура коммерции, /api/events и /api/quotes перехватываются. Регистрируется после авто-стража fixtures.ts,
 * поэтому срабатывает первым.
 */
export async function mockP2(
  page: Page,
  opts: {
    features?: Json
    synthetic?: string[]
    quote?: 'ok' | 'error'
  } = {},
): Promise<Captured> {
  const cap: Captured = { events: [], searches: [], quotes: [] }
  await page.route('**/api/events', (route) => {
    cap.events.push(route.request().postDataJSON() as Json)
    return route.fulfill({ status: 204, body: '' })
  })
  await page.route('**/api/quotes', (route: Route) => {
    cap.quotes.push(route.request().postDataJSON() as Json)
    if (opts.quote === 'error')
      return route.fulfill({
        status: 503,
        json: {
          error: {
            code: 'UNAVAILABLE',
            message: 'Сервис запросов КП недоступен',
            traceId: 'e2e',
          },
        },
      })
    return route.fulfill({
      status: 201,
      json: {
        id: 'Q-2026-0042',
        status: 'new',
        positions: 3,
        total_rub: null,
        on_request: 1,
      },
    })
  })
  if (P2_LIVE) return cap
  await page.route('**/api/meta', async (route) => {
    const resp = await route.fetch()
    const body = (await resp.json()) as Json
    return route.fulfill({
      response: resp,
      json: {
        ...body,
        features: opts.features ?? {
          hardware: true,
          companions: true,
          margin_policy: true,
          solutions: false,
        },
        synthetic: opts.synthetic ?? [],
      },
    })
  })
  await page.route('**/api/search', (route) => {
    const body = route.request().postDataJSON() as Json
    cap.searches.push(body)
    const q = String(body.query ?? '')
    if (q === WS_QUERY) return route.fulfill({ json: wsResponse(q) })
    if (q === MEM_QUERY)
      return route.fulfill({
        json: memResponse(q, (body.filters as Json[]) ?? []),
      })
    return route.fallback()
  })
  return cap
}

export const ofType = (cap: Captured, type: string) => cap.events.filter((e) => e.type === type)
