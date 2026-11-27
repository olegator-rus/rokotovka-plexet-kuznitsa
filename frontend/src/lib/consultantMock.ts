// Заглушка событий консультанта для разработки и e2e: сценарии 2 и 3,
// ошибка модели с запасным ответом, ссылка на раздел, удаление из
// спецификации. Числа сверены с каталогом; номера позиций — условные, только для
// заглушки. Включается localStorage.consultant_mock === '1' или ?consultant=mock — не на стенде по умолчанию.
import type { GuideOffer } from '@/api/typesGuide'
import type {
  Attachment,
  ConsultantRequest,
  ConversationEvent,
  ProductReference,
} from '@/api/typesConsultant'

const STORE = 'https://store.softline.ru'

function offer(
  id: string,
  title: string,
  unitRub: number,
  qty: number,
  unit: string,
  tier = '',
): GuideOffer {
  return {
    offer_id: id,
    title,
    sku: '',
    tier,
    unit,
    price_per_unit: unitRub,
    total: Math.round(unitRub * qty * 100) / 100,
    price_state: 'fixed',
    offer_url: `${STORE}/`,
    quantity: qty,
  }
}

function product(
  id: string,
  title: string,
  vendor: string,
  unitMinor: number,
  qty: number,
  extra: Partial<ProductReference> = {},
  unit = 'компьютер',
  tier = '',
): ProductReference {
  return {
    offerId: id,
    sku: '',
    title,
    vendor,
    licenseType: 'Новая лицензия',
    quantity: qty,
    duration: '1 год',
    unitPriceMinor: unitMinor,
    totalPriceMinor: unitMinor * qty,
    priceStatus: 'known',
    compatibility: 'compatible',
    productUrl: `${STORE}/`,
    offer: offer(id, title, unitMinor / 100, qty, unit, tier),
    ...extra,
  }
}

const KES_STD_30 = product(
  'mock-kes-std',
  'Kaspersky Endpoint Security — Стандартный',
  'Kaspersky',
  281340,
  30,
  {},
  'компьютер',
  '25–49',
)
const KES_ADV_30 = product(
  'mock-kes-adv',
  'Kaspersky Endpoint Security — Расширенный',
  'Kaspersky',
  408150,
  30,
  {},
  'компьютер',
  '25–49',
)
const KES_ADV_50 = product(
  'mock-kes-adv',
  'Kaspersky Endpoint Security — Расширенный',
  'Kaspersky',
  394020,
  50,
  { reason: 'Поддерживает шифрование данных' },
  'компьютер',
  '50–99',
)

const CSP_SERVER = product(
  'mock-csp-server',
  'КриптоПро CSP 5.0, серверная лицензия',
  'КриптоПро',
  7000000,
  1,
  { reason: 'Действует в серверной ОС' },
  'сервер',
)
const CSP_TLS = product(
  'mock-csp-tls',
  'КриптоПро CSP 5.0, лицензия для TLS-сервера',
  'КриптоПро',
  10000000,
  1,
  {},
  'сервер',
)
const CSP_WS_YEAR = product(
  'mock-csp-ws-1y',
  'КриптоПро CSP 5.0, рабочее место · 1 год',
  'КриптоПро',
  185000,
  1,
  {
    compatibility: 'incompatible',
    reason: 'Не подходит для серверной ОС',
  },
  'рабочее место',
)
const CSP_WS_PERP = product(
  'mock-csp-ws-perp',
  'КриптоПро CSP 5.0, рабочее место · Бессрочная',
  'КриптоПро',
  370000,
  1,
  {
    compatibility: 'incompatible',
    reason: 'Не подходит для серверной ОС',
    duration: 'Бессрочно',
  },
  'рабочее место',
)

const CSP_LIST: Attachment = {
  type: 'product_list',
  title: 'КриптоПро CSP для сервера',
  groups: [
    { kind: 'fit', items: [CSP_SERVER, CSP_TLS] },
    { kind: 'unfit', title: 'Не подойдут', items: [CSP_WS_YEAR, CSP_WS_PERP] },
  ],
}

type Bubble = { text: string; attachments?: Attachment[]; quickReplies?: string[] }

function turn(bubbles: Bubble[], step = 'searching'): ConversationEvent[] {
  const out: ConversationEvent[] = [{ type: 'status', step }]
  bubbles.forEach((b, i) => {
    const id = `m${i + 1}`
    out.push({ type: 'message.start', id })
    out.push({ type: 'message.delta', id, text: b.text })
    for (const a of b.attachments ?? []) out.push({ type: 'message.attachment', id, attachment: a })
    out.push({
      type: 'message.complete',
      id,
      ...(b.quickReplies ? { quickReplies: b.quickReplies } : {}),
    })
  })
  return out
}

/** События ответа на реплику (без задержек; их добавляет транспорт заглушки). */
export function mockEvents(req: Pick<ConsultantRequest, 'text' | 'spec'>): ConversationEvent[] {
  const t = req.text.toLowerCase()
  if (t.includes('будет 50') || (t.includes('50') && t.includes('шифрован'))) {
    return turn(
      [
        {
          text: 'Тогда подходит Расширенный. На 50 компьютеров цена за место меняется: теперь это **3 940,20 ₽** вместо 4 081,50 ₽. Всего получится **197 010 ₽** на год.',
          attachments: [{ type: 'product_card', product: KES_ADV_50 }],
        },
      ],
      'calculating',
    )
  }
  if (t.includes('расширенн') || t.includes('касперск')) {
    return turn([
      {
        text: 'В обеих редакциях есть единая консоль управления. В Расширенном дополнительно есть шифрование данных и установка обновлений программ.',
      },
      {
        text: 'На 30 компьютеров Стандартный стоит **84 402 ₽**, Расширенный — **122 445 ₽**. Если эти дополнительные функции вам не нужны, я бы не переплачивал и взял Стандартный.',
        attachments: [
          {
            type: 'comparison',
            columns: [KES_STD_30, KES_ADV_30],
            rows: [
              { label: 'Единая консоль', values: ['✓', '✓'] },
              { label: 'Шифрование', values: ['—', '✓'] },
              { label: 'Обновления ПО', values: ['—', '✓'] },
            ],
          },
          { type: 'source', title: 'Документация вендора', url: 'https://support.kaspersky.ru/' },
        ],
        quickReplies: ['Шифрование нужно', 'Не нужно'],
      },
    ])
  }
  if (t.includes('электронн') && t.includes('подпис')) {
    return turn([
      {
        text: 'Тогда стоит проверить обычную серверную лицензию за **70 000 ₽**. Уточним ещё вашу серверную ОС, чтобы не ошибиться с совместимостью.',
        attachments: [{ type: 'product_card', product: CSP_SERVER }],
      },
    ])
  }
  if (t.includes('криптопро') || t.includes('csp')) {
    return turn(
      [
        {
          text: 'Тут важно не ошибиться с лицензией. КриптоПро CSP за **1 850 ₽** на год рассчитан на рабочее место — в серверной ОС такая лицензия не действует.',
        },
        {
          text: 'Серверная лицензия стоит **70 000 ₽**, а лицензия для TLS-сервера — **100 000 ₽**. Вы хотите использовать КриптоПро для электронной подписи или для TLS?',
          attachments: [
            CSP_LIST,
            {
              type: 'source',
              title: 'Правила лицензирования КриптоПро',
              url: 'https://www.cryptopro.ru/',
            },
          ],
          quickReplies: ['Электронная подпись', 'TLS', 'Пока не знаю'],
        },
      ],
      'checking',
    )
  }
  if (t.includes('оплат')) {
    return turn([
      {
        text: 'Инструкцию по оплате по счёту можно посмотреть в разделе оплаты магазина. Вот ссылка.',
        attachments: [
          { type: 'link', topic: 'payment', title: 'Способы оплаты', url: `${STORE}/` },
        ],
      },
    ])
  }
  if (t.includes('убери')) {
    const first = req.spec[0]
    if (!first) return turn([{ text: 'Сейчас спецификация пустая.' }])
    return turn([
      {
        text: 'Убрал позицию из спецификации.',
        attachments: [
          { type: 'spec_event', action: 'removed', offerId: first.offer_id, title: 'Позиция' },
        ],
      },
    ])
  }
  if (t.includes('добавь')) {
    return turn([
      {
        text: 'Добавил Kaspersky Endpoint Security, Расширенный, на 50 компьютеров.',
        attachments: [
          {
            type: 'spec_event',
            action: 'added',
            offerId: KES_ADV_50.offerId,
            title: KES_ADV_50.title,
            quantity: 50,
            totalPriceMinor: KES_ADV_50.totalPriceMinor,
            offer: KES_ADV_50.offer,
          },
        ],
      },
    ])
  }
  if (t.includes('ошибк')) {
    return [
      { type: 'status', step: 'searching' },
      { type: 'message.start', id: 'm1' },
      { type: 'message.error', id: 'm1', reason: 'model_unavailable' },
      { type: 'message.start', id: 'm2' },
      {
        type: 'message.delta',
        id: 'm2',
        text: 'Не получилось сформулировать ответ, но вот что нашлось в каталоге',
      },
      { type: 'message.attachment', id: 'm2', attachment: CSP_LIST },
      { type: 'message.complete', id: 'm2' },
    ]
  }
  if (t.includes('сбой')) {
    return [
      { type: 'status', step: 'searching' },
      { type: 'message.start', id: 'm1' },
      { type: 'message.error', id: 'm1', reason: 'model_unavailable' },
    ]
  }
  return turn([
    {
      text: 'Расскажите чуть подробнее о задаче: сколько компьютеров и что нужно защитить или автоматизировать.',
    },
  ])
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

/** Поток событий заглушки с паузами, как у настоящего ответа. */
export async function* mockStream(
  req: Pick<ConsultantRequest, 'text' | 'spec'>,
  signal?: AbortSignal,
): AsyncGenerator<ConversationEvent> {
  const events = mockEvents(req)
  for (const [i, e] of events.entries()) {
    if (signal?.aborted) return
    await sleep(i === 0 ? 150 : e.type === 'message.start' ? 400 : 40)
    yield e
  }
}
