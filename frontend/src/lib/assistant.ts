// Помощник по выбору (контур помощника): логика без DOM — как открыли панель, что
// отправить серверу, стартовые подсказки по контексту, системная строка после добавления, хранение в sessionStorage.
// Сообщение помощника целиком собирает сервер; здесь ничего не выбирается и не считается.
import type {
  GuideChat,
  GuideChipAnswer,
  GuideCondition,
  GuideOffer,
  GuideRequest,
} from '@/api/typesGuide'
import { formatPrice, plural } from './format.ts'

// ── Лента ──

export interface UserEntry {
  id: number
  kind: 'user'
  text: string
}

export interface AssistantEntry {
  id: number
  kind: 'assistant'
  chat: GuideChat
  /** Подбор сервера, к которому относятся карточки (для POST /api/guide/spec). */
  selectionId: string
  /** Условия и офферы хода — для строки спецификации («В спецификацию» на старой карточке берёт их условия). */
  conditions: GuideCondition[]
  offers: GuideOffer[]
  /** Запрос, с которого начат подбор (уходит в заявку КП). */
  query: string
}

export interface SystemEntry {
  id: number
  kind: 'system'
  text: string
  /** Показать ссылку «Открыть спецификацию». */
  openSpec?: boolean
}

export interface ErrorEntry {
  id: number
  kind: 'error'
  text: string
}

export type ChatEntry = UserEntry | AssistantEntry | SystemEntry | ErrorEntry

// ── Как открыли панель (CHAT.md §1) ──

export type OpenSource = 'button' | 'invite' | 'product'

export interface OpenPlan {
  /** Отправить сразу (человек явно попросил: «Подобрать под мою задачу»). */
  send: string
  /** Подставить в поле ввода — отправляет человек. */
  prefill: string
}

/** Развёрнутый запрос: три слова и больше или число («нужен антивирус на 20 компьютеров»). */
export function isDetailedQuery(q: string): boolean {
  const t = q.trim()
  if (!t) return false
  return t.split(/\s+/).length >= 3 || /\d/.test(t)
}

/**
 * Что делать с запросом строки поиска при открытии. Кнопка у строки: короткий — приветствие, развёрнутый —
 * в поле ввода; строка-приглашение — отправить сразу; страница продукта — приветствие с темой продукта.
 */
export function openPlan(source: OpenSource, query: string, hasConversation: boolean): OpenPlan {
  const q = query.trim()
  if (source === 'invite' && q) return { send: q, prefill: '' }
  if (source === 'button' && !hasConversation && isDetailedQuery(q)) return { send: '', prefill: q }
  return { send: '', prefill: '' }
}

// ── Стартовые подсказки (CHAT.md §2) ──

const PROMPTS = {
  empty: [
    'Подобрать ПО для офиса',
    'Сравнить продукты',
    'Рассчитать стоимость лицензий',
    'Подобрать продление',
  ],
  antivirus: [
    'Подобрать антивирус для офиса',
    'Сравнить редакции Kaspersky',
    'Рассчитать на 20 компьютеров',
    'Перейти с другого антивируса',
  ],
  astra: [
    'Сравнить уровни защищённости',
    'Подобрать для рабочих станций',
    'Рассчитать лицензии',
    'Что ещё потребуется?',
  ],
  crypto: [
    'Подобрать для сервера',
    'Сравнить годовую и бессрочную',
    'Подобрать для рабочего места',
    'Какую лицензию выбрать для TLS?',
  ],
} as const

/** Реплика, которую отправляет стартовая подсказка: с темой продукта или запроса, чтобы сервер понял предмет. */
export interface StarterPrompt {
  label: string
  message: string
}

/** Стартовые подсказки по контексту: страница продукта, запрос в поиске, пустой поиск. */
export function starterPrompts(context: string): StarterPrompt[] {
  const c = context.trim().toLowerCase()
  const withTopic = (list: readonly string[], topic: string) =>
    list.map((label) => ({ label, message: topic ? `${label}: ${topic}` : label }))
  if (/astra|астра/.test(c)) return withTopic(PROMPTS.astra, 'Astra Linux Special Edition')
  if (/криптопро|cryptopro|crypto ?pro/.test(c)) return withTopic(PROMPTS.crypto, 'КриптоПро CSP')
  if (/антивир|касперск|kaspersky|dr\.?web|доктор|eset|pro32|защит/.test(c)) {
    return PROMPTS.antivirus.map((label) => ({
      label,
      message:
        label === 'Перейти с другого антивируса'
          ? 'хотим перейти с другого антивируса'
          : label === 'Рассчитать на 20 компьютеров'
            ? 'антивирус на 20 компьютеров'
            : label,
    }))
  }
  return PROMPTS.empty.map((label) => ({ label, message: label }))
}

// ── Запрос к серверу ──

/**
 * Тело хода. Первая реплика разговора (или первая после «Сбросить») — запрос без состояния; дальше — реплика с
 * состоянием подбора и исходным запросом (как в наборе оценки); кнопка вопроса или подсказка с чипом — чип.
 */
export function turnBody(
  input: { text?: string; chip?: GuideChipAnswer },
  state: unknown,
  startQuery = '',
): Omit<GuideRequest, 'session_id'> {
  const text = (input.text ?? '').trim()
  if (input.chip && state) return { query: startQuery, chip: input.chip, state }
  if (!state) return { query: text }
  return { query: startQuery, message: text, state }
}

// ── Строка после добавления ──

/** «Добавлено в спецификацию · 1 позиция · 197 010 ₽». */
export function addedLine(positions: number, knownRub: number | null): string {
  const head = `${positions} ${plural(positions, ['позиция', 'позиции', 'позиций'])}`
  const sum = knownRub !== null && knownRub > 0 ? ` · ${formatPrice(knownRub)}` : ''
  return `Добавлено в спецификацию · ${head}${sum}`
}

// ── Хранение (CHAT.md §1: закрытие сохраняет историю и условия; sessionStorage) ──

export const ASSISTANT_KEY = 'plexet.assistant.v1'

export interface StoredAssistant {
  entries: ChatEntry[]
  state: unknown
  product: string
}

/** Прочитать сохранённый разговор; битое или недоступное хранилище — пустой разговор. */
export function loadAssistant(storage: Pick<Storage, 'getItem'> | null): StoredAssistant {
  const empty: StoredAssistant = { entries: [], state: null, product: '' }
  try {
    const raw = storage?.getItem(ASSISTANT_KEY)
    if (!raw) return empty
    const v = JSON.parse(raw) as Partial<StoredAssistant>
    if (!Array.isArray(v.entries)) return empty
    return { entries: v.entries.slice(-60), state: v.state ?? null, product: v.product ?? '' }
  } catch {
    return empty
  }
}

export function saveAssistant(storage: Pick<Storage, 'setItem'> | null, v: StoredAssistant): void {
  try {
    storage?.setItem(ASSISTANT_KEY, JSON.stringify({ ...v, entries: v.entries.slice(-60) }))
  } catch {
    // Хранилище недоступно (приватный режим) — разговор живёт до перезагрузки.
  }
}

/** Последнее сообщение помощника — подсказки следующего шага активны только под ним. */
export function lastAssistantId(entries: readonly ChatEntry[]): number {
  for (let i = entries.length - 1; i >= 0; i--) {
    const e = entries[i]
    if (e && e.kind === 'assistant') return e.id
  }
  return -1
}
