// Консультант контура консультанта (docs/CONSULTANT.md): разбор текста сообщения,
// цены из копеек, чтение NDJSON, сборка ленты из событий, применение spec_event к спецификации, хранение разговора,
// флаг включения. Без DOM: tests/consultant.test.ts.
import type {
  Attachment,
  ConsultantPage,
  ConsultantSpecItem,
  ConversationEvent,
  ProductReference,
  SpecEventAttachment,
} from '@/api/typesConsultant'
import { lineFromOffer } from './guide.ts'
import { addLine, removeLine, setQuantity, clampQty, type SpecLine } from './spec.ts'

// ── Цены ──

const NBSP = ' '

/**
 * Цена из копеек: 8440200 → «84 402 ₽», 394020 → «3 940,20 ₽». 0, отрицательное, не число — '' (вызывающий пишет
 * «Цена по запросу»): «0 ₽» не показывается никогда.
 */
export function formatMinor(minor: number | null | undefined): string {
  if (typeof minor !== 'number' || !Number.isFinite(minor) || minor <= 0) return ''
  const kop = Math.round(minor)
  const whole = kop % 100 === 0
  const s = new Intl.NumberFormat('ru-RU', {
    minimumFractionDigits: whole ? 0 : 2,
    maximumFractionDigits: whole ? 0 : 2,
  }).format(kop / 100)
  // Разделитель разрядов ru-RU — узкий неразрывный пробел; приводим к обычному неразрывному, как в остальном магазине.
  return `${s.replace(/[   ]/g, NBSP)}${NBSP}₽`
}

/** Цена позиции для показа: полная стоимость, иначе «Цена по запросу». */
export function productPrice(p: Pick<ProductReference, 'totalPriceMinor' | 'priceStatus'>): string {
  const s = p.priceStatus === 'known' ? formatMinor(p.totalPriceMinor) : ''
  return s || 'Цена по запросу'
}

/** Расчёт «50 × 3 940,20 ₽» или '' (одна единица или цены нет). */
export function productCalc(
  p: Pick<ProductReference, 'quantity' | 'unitPriceMinor' | 'priceStatus'>,
): string {
  if (p.priceStatus !== 'known') return ''
  const unit = formatMinor(p.unitPriceMinor)
  if (!unit || !p.quantity || p.quantity < 2) return ''
  return `${p.quantity}${NBSP}×${NBSP}${unit}`
}

/** Прежняя цена за единицу при акции (только если больше текущей). */
export function productOldUnit(
  p: Pick<ProductReference, 'oldUnitPriceMinor' | 'unitPriceMinor' | 'priceStatus'>,
): string {
  if (p.priceStatus !== 'known') return ''
  const old = p.oldUnitPriceMinor
  const cur = p.unitPriceMinor
  if (typeof old !== 'number' || typeof cur !== 'number' || !(old > cur)) return ''
  return formatMinor(old)
}

/** Условия одной строкой: «Новая лицензия · 50 компьютеров · 1 год». */
export function productTerms(
  p: Pick<ProductReference, 'licenseType' | 'quantity' | 'duration' | 'offer'>,
): string {
  // Единица оффера бывает «за лицензию», «пакет на 5 мест» — склонять её нельзя, поэтому количество в штуках.
  const qty = p.quantity && p.quantity > 1 ? `${p.quantity}${NBSP}шт.` : ''
  return [p.licenseType, qty, p.duration ?? ''].filter(Boolean).join(' · ')
}

/** Причина несовместимости: «Не подходит: …», без повтора, если причина уже начинается с «Не подходит». */
export function unfitText(reason: string | undefined): string {
  const r = (reason ?? '').trim()
  if (!r) return 'Не подходит для вашей задачи'
  return /^не\s+подход/i.test(r) ? r : `Не подходит: ${r}`
}

/** Строка «Стоимость» таблицы сравнения — из columns[].totalPriceMinor (CONTRACT.md, «Вложения»). */
export function comparisonCostRow(columns: readonly ProductReference[]): string[] {
  return columns.map((c) => {
    if (c.priceStatus === 'known') return formatMinor(c.totalPriceMinor) || 'Нет данных'
    if (c.priceStatus === 'on_request') return 'Цена по запросу'
    return 'Нет данных'
  })
}

// ── Текст сообщения: только **полужирное** и [текст](https://store.softline.ru/…) ──

export type RichNode =
  | { kind: 'text'; text: string }
  | { kind: 'bold'; text: string }
  | { kind: 'link'; text: string; href: string }

const STORE_URL = /^https:\/\/store\.softline\.ru(\/[^\s<>"'`]*)?$/

export function isStoreUrl(url: string): boolean {
  return STORE_URL.test(url)
}

/** Неразрывные пробелы в числах и перед «₽»: «4 081,50 ₽» не переносится посередине. */
export function nbspNumbers(text: string): string {
  return text.replace(/(\d) (?=\d{3}(?!\d))/g, `$1${NBSP}`).replace(/(\d) ₽/g, `$1${NBSP}₽`)
}

/**
 * Разбор на узлы: `**…**` — полужирное, `[…](https://store.softline.ru/…)` — ссылка; остальное — текст как есть
 * (HTML не интерпретируется: узлы рендерятся интерполяцией). Ссылка на чужой адрес остаётся текстом её подписи.
 */
export function parseRich(src: string): RichNode[] {
  const out: RichNode[] = []
  const push = (raw: RichNode) => {
    const n = { ...raw, text: nbspNumbers(raw.text) }
    const last = out[out.length - 1]
    if (n.kind === 'text' && last?.kind === 'text') last.text += n.text
    else if (n.text) out.push(n)
  }
  const re = /\*\*([^*\n]+?)\*\*|\[([^\]\n]+)\]\(([^)\s]+)\)/g
  let pos = 0
  for (let m = re.exec(src); m; m = re.exec(src)) {
    if (m.index > pos) push({ kind: 'text', text: src.slice(pos, m.index) })
    if (m[1] !== undefined) push({ kind: 'bold', text: m[1] })
    else {
      const label = m[2] as string
      const href = m[3] as string
      push(isStoreUrl(href) ? { kind: 'link', text: label, href } : { kind: 'text', text: label })
    }
    pos = m.index + m[0].length
  }
  if (pos < src.length) push({ kind: 'text', text: src.slice(pos) })
  return out
}

// ── NDJSON ──

/** Разбор куска потока: целые строки — события, хвост без перевода строки — до следующего куска. */
export function splitNdjson(buffer: string, chunk: string): { lines: string[]; rest: string } {
  const all = buffer + chunk
  const parts = all.split('\n')
  const rest = parts.pop() ?? ''
  return { lines: parts.map((l) => l.trim()).filter(Boolean), rest }
}

const EVENT_TYPES = new Set([
  'status',
  'message.start',
  'message.delta',
  'message.attachment',
  'message.complete',
  'message.error',
])

/** Строка → событие; битая или чужая строка — null (пропускается). */
export function parseEvent(line: string): ConversationEvent | null {
  let data: unknown
  try {
    data = JSON.parse(line)
  } catch {
    return null
  }
  if (!data || typeof data !== 'object') return null
  const r = data as Record<string, unknown>
  if (typeof r.type !== 'string' || !EVENT_TYPES.has(r.type)) return null
  if (r.type === 'status') return typeof r.step === 'string' ? (r as ConversationEvent) : null
  if (typeof r.id !== 'string' || !r.id) return null
  if (r.type === 'message.delta' && typeof r.text !== 'string') return null
  if (r.type === 'message.attachment') {
    const a = r.attachment as Record<string, unknown> | undefined
    if (!a || typeof a !== 'object' || typeof a.type !== 'string') return null
  }
  return r as ConversationEvent
}

/** Прочитать поток NDJSON до конца: onEvent на каждое событие по порядку. */
export async function readNdjson(
  body: ReadableStream<Uint8Array>,
  onEvent: (e: ConversationEvent) => void,
): Promise<void> {
  const reader = body.getReader()
  const dec = new TextDecoder()
  let rest = ''
  for (;;) {
    const { done, value } = await reader.read()
    const chunk = done ? dec.decode() : dec.decode(value, { stream: true })
    const r = splitNdjson(rest, done ? `${chunk}\n` : chunk)
    rest = r.rest
    for (const l of r.lines) {
      const e = parseEvent(l)
      if (e) onEvent(e)
    }
    if (done) return
  }
}

// ── Лента ──

export interface AttachmentView {
  key: string
  data: Attachment
  /** spec_event, который не удалось применить: текст ошибки вместо «Добавлено». */
  failed?: string
}

export interface ChatMessage {
  id: string
  role: 'user' | 'assistant'
  text: string
  attachments: AttachmentView[]
  quickReplies?: string[]
  status: 'streaming' | 'complete' | 'error'
  /** Номер хода: сообщения консультанта одного хода — одна группа. */
  turn: number
  /** Ошибка хода целиком (ответа нет): кнопка «Попробовать ещё раз». */
  retry?: boolean
}

export const MAX_QUICK_REPLIES = 3

/** Ключ сообщения хода: id сервера повторяются от хода к ходу («m1»). */
export const msgKey = (turn: number, id: string) => `${turn}:${id}`

/**
 * Применить событие к ленте (без status — его показывает индикатор). Возвращает новую ленту; вложение получает ключ
 * по порядку. Неизвестный id у delta/attachment/complete — сообщение создаётся (сервер мог не прислать start).
 */
export function applyEvent(
  messages: readonly ChatMessage[],
  ev: ConversationEvent,
  turn: number,
): ChatMessage[] {
  if (ev.type === 'status') return messages as ChatMessage[]
  const key = msgKey(turn, ev.id)
  const next = [...messages]
  let i = next.findIndex((m) => m.id === key)
  if (i < 0) {
    next.push({ id: key, role: 'assistant', text: '', attachments: [], status: 'streaming', turn })
    i = next.length - 1
  }
  const m = { ...(next[i] as ChatMessage) }
  switch (ev.type) {
    case 'message.start':
      break
    case 'message.delta':
      m.text += ev.text
      break
    case 'message.attachment':
      m.attachments = [
        ...m.attachments,
        { key: `${key}:${m.attachments.length}`, data: ev.attachment },
      ]
      break
    case 'message.complete':
      m.status = 'complete'
      if (Array.isArray(ev.quickReplies)) {
        const qr = ev.quickReplies.filter((s) => typeof s === 'string' && s.trim())
        if (qr.length) m.quickReplies = qr.slice(0, MAX_QUICK_REPLIES)
      }
      break
    case 'message.error':
      m.status = 'error'
      break
  }
  next[i] = m
  return next
}

/** Сообщение показывается: есть текст или вложения (пустое ошибочное — нет, его заменяет запасной ответ). */
export const visibleMessage = (m: ChatMessage) =>
  m.role === 'user' || m.retry === true || !!m.text.trim() || m.attachments.length > 0

/** У хода нет ни одного показанного ответа консультанта — ход провален. */
export function turnFailed(messages: readonly ChatMessage[], turn: number): boolean {
  return !messages.some((m) => m.role === 'assistant' && m.turn === turn && visibleMessage(m))
}

/** Быстрые ответы: только у последней реплики консультанта и пока покупатель после неё не писал. */
export function activeQuickReplies(messages: readonly ChatMessage[]): {
  id: string
  replies: string[]
} | null {
  for (let i = messages.length - 1; i >= 0; i--) {
    const m = messages[i] as ChatMessage
    if (m.role === 'user') return null
    if (!visibleMessage(m)) continue
    if (m.status !== 'complete' || !m.quickReplies?.length) return null
    return { id: m.id, replies: m.quickReplies }
  }
  return null
}

// ── Спецификация ──

export type SpecOp =
  | { kind: 'add'; line: SpecLine }
  | { kind: 'remove'; offerId: string }
  | { kind: 'qty'; offerId: string; qty: number }

export const SPEC_ERRORS = {
  added: 'Не удалось добавить товар.',
  removed: 'Не удалось удалить товар.',
  updated: 'Не удалось изменить количество.',
} as const

/**
 * spec_event → операция над спецификацией (CONTRACT.md, «Спецификация»): added — lineFromOffer(offer) с количеством
 * события (повтор той же позиции меняет количество, а не создаёт дубль), removed — по offerId, updated — количество.
 * Применить нельзя — ошибка с текстом для ленты.
 */
export function resolveSpecEvent(
  lines: readonly SpecLine[],
  ev: SpecEventAttachment,
  query = '',
): { ok: true; op: SpecOp } | { ok: false; error: string } {
  const fail = { ok: false as const, error: SPEC_ERRORS[ev.action] ?? SPEC_ERRORS.added }
  if (!ev.offerId) return fail
  if (ev.action === 'added') {
    const o = ev.offer
    if (!o || o.offer_id !== ev.offerId) return fail
    const qty = typeof ev.quantity === 'number' && ev.quantity > 0 ? ev.quantity : o.quantity
    return {
      ok: true,
      op: { kind: 'add', line: lineFromOffer({ ...o, quantity: qty }, [], query) },
    }
  }
  const exists = lines.some((l) => l.product_id === ev.offerId)
  if (!exists) return fail
  if (ev.action === 'removed') return { ok: true, op: { kind: 'remove', offerId: ev.offerId } }
  if (ev.action === 'updated') {
    if (typeof ev.quantity !== 'number' || !(ev.quantity > 0)) return fail
    return { ok: true, op: { kind: 'qty', offerId: ev.offerId, qty: clampQty(ev.quantity) } }
  }
  return fail
}

/** Применить операцию к строкам (тот же результат, что у стора спецификации). */
export function applySpecOp(lines: readonly SpecLine[], op: SpecOp): SpecLine[] {
  if (op.kind === 'add') return addLine(lines, op.line)
  if (op.kind === 'remove') return removeLine(lines, op.offerId)
  return setQuantity(lines, op.offerId, op.qty)
}

/** Спецификация в запрос: [{offer_id, qty}]. */
export const specPayload = (lines: readonly SpecLine[]): ConsultantSpecItem[] =>
  lines.map((l) => ({ offer_id: l.product_id, qty: l.quantity }))

// ── Контекст страницы ──

export function pageContext(
  routeName: string | null | undefined,
  params: Record<string, unknown>,
  query: Record<string, unknown>,
  specOpen: boolean,
): ConsultantPage {
  const str = (v: unknown) => (typeof v === 'string' && v.trim() ? v.trim() : null)
  if (specOpen) return { type: 'spec', family_id: null, offer_id: null, query: null }
  if (routeName === 'search')
    return { type: 'search', family_id: null, offer_id: null, query: str(query.q) }
  if (routeName === 'product')
    return { type: 'product', family_id: null, offer_id: str(params.id), query: null }
  return { type: 'other', family_id: null, offer_id: null, query: null }
}

// ── Хранение (sessionStorage) ──

export interface StoredConversation {
  conversationId: string
  messages: ChatMessage[]
  draft: string
  open: boolean
  expanded: boolean
}

export const CONSULTANT_KEY = 'plexet.consultant'

export function newConversationId(): string {
  const c = (globalThis as { crypto?: { randomUUID?: () => string } }).crypto
  if (c?.randomUUID) return c.randomUUID()
  const hex = () =>
    Math.floor(Math.random() * 0x10000)
      .toString(16)
      .padStart(4, '0')
  return `${hex()}${hex()}-${hex()}-4${hex().slice(1)}-a${hex().slice(1)}-${hex()}${hex()}${hex()}`
}

function parseMessage(x: unknown): ChatMessage | null {
  if (!x || typeof x !== 'object') return null
  const r = x as Record<string, unknown>
  if (typeof r.id !== 'string' || (r.role !== 'user' && r.role !== 'assistant')) return null
  const atts = Array.isArray(r.attachments)
    ? (r.attachments as unknown[]).filter(
        (a): a is AttachmentView =>
          !!a &&
          typeof a === 'object' &&
          typeof (a as AttachmentView).key === 'string' &&
          !!(a as AttachmentView).data &&
          typeof (a as AttachmentView).data.type === 'string',
      )
    : []
  const status = r.status === 'error' ? 'error' : 'complete'
  return {
    id: r.id,
    role: r.role,
    text: typeof r.text === 'string' ? r.text : '',
    attachments: atts,
    ...(Array.isArray(r.quickReplies)
      ? {
          quickReplies: (r.quickReplies as unknown[]).filter(
            (s): s is string => typeof s === 'string',
          ),
        }
      : {}),
    // Ответ, оборванный перезагрузкой, восстанавливается как завершённый: «Печатает…» не висит вечно.
    status,
    turn: typeof r.turn === 'number' ? r.turn : 0,
    ...(r.retry === true ? { retry: true } : {}),
  }
}

export function parseConversation(raw: string | null): StoredConversation {
  const empty: StoredConversation = {
    conversationId: newConversationId(),
    messages: [],
    draft: '',
    open: false,
    expanded: false,
  }
  if (!raw) return empty
  let data: unknown
  try {
    data = JSON.parse(raw)
  } catch {
    return empty
  }
  if (!data || typeof data !== 'object') return empty
  const r = data as Record<string, unknown>
  const messages = Array.isArray(r.messages)
    ? (r.messages as unknown[]).map(parseMessage).filter((m): m is ChatMessage => m !== null)
    : []
  return {
    conversationId:
      typeof r.conversationId === 'string' && r.conversationId
        ? r.conversationId
        : empty.conversationId,
    messages,
    draft: typeof r.draft === 'string' ? r.draft : '',
    open: r.open === true,
    expanded: r.expanded === true,
  }
}

// ── Флаг (CONTRACT.md, «Флаг») ──

export function consultantEnabled(opts: {
  env?: string | undefined
  search?: string
  stored?: string | null
}): boolean {
  if (opts.env === '1') return true
  if (opts.search && new URLSearchParams(opts.search).get('consultant') === '1') return true
  return opts.stored === '1'
}

// ── Тексты ──

export const GREETING = [
  'Здравствуйте! Помогу подобрать ПО, разобраться в лицензиях и посчитать стоимость.',
  'Расскажите, что ищете, — не обязательно знать точное название продукта.',
]

/** Четыре общие подсказки. */
export const STARTERS = [
  'Подобрать ПО для офиса',
  'Выбрать антивирус',
  'Разобраться в лицензиях',
  'Посчитать стоимость',
]

export function stepLabel(step: string | null): string {
  if (step === 'searching') return 'Ищу в каталоге…'
  if (step === 'checking') return 'Проверяю лицензии…'
  if (step === 'calculating') return 'Считаю стоимость…'
  return 'Печатает…'
}

export const VISIBLE_ROWS = 4

/** Список: первые четыре позиции видны сразу (сквозь группы), остальные — под «Ещё N вариантов». */
export function splitVisible<T>(groups: readonly { items: readonly T[] }[], expanded: boolean) {
  let left = expanded ? Infinity : VISIBLE_ROWS
  let hidden = 0
  const shown = groups.map((g) => {
    const take = Math.max(0, Math.min(g.items.length, left))
    left -= take
    hidden += g.items.length - take
    return g.items.slice(0, take)
  })
  return { shown, hidden }
}
