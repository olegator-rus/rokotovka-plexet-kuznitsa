// Семь причин неудачного поиска и их владельцы (контур доводки). Коды диагнозов агента
// остаются подпричинами внутри карточки. Сервер может прислать причину и владельца сам (cause, owner) — тогда они
// главнее; без них причина выводится из кода по той же таблице. Без DOM: tests/causes.test.ts.

export type CauseId =
  | 'not_understood'
  | 'ranked_low'
  | 'content_gap'
  | 'not_in_catalog'
  | 'relation_rule'
  | 'non_product'
  | 'noise'

/** Вид карточки очереди: правка поиска (утвердить / отклонить) или задача владельцу (отправить). */
export type CauseKind = 'search_fix' | 'owner_task'

export interface Cause {
  id: CauseId
  label: string
  owner: string
  kind: CauseKind
  codes: readonly string[]
}

export const CAUSES: readonly Cause[] = [
  {
    id: 'not_understood',
    label: 'Поиск не понял запрос',
    owner: 'Администратор поиска',
    kind: 'search_fix',
    codes: ['TYPO', 'ALIAS_GAP', 'SUGGESTION_GAP'],
  },
  {
    id: 'ranked_low',
    label: 'Нужный товар стоит низко или задублирован',
    owner: 'Команда поиска',
    kind: 'search_fix',
    codes: ['RANKING_FAILURE', 'FAMILY_DUPLICATE', 'SUGGESTION_DEGRADED'],
  },
  {
    id: 'content_gap',
    label: 'В карточке не хватает данных',
    owner: 'Каталог',
    kind: 'owner_task',
    codes: ['CONTENT_GAP'],
  },
  {
    id: 'not_in_catalog',
    label: 'Товара нет в каталоге',
    owner: 'Категорийный менеджер',
    kind: 'owner_task',
    codes: ['ASSORTMENT_GAP', 'SUBSTITUTE_AVAILABLE'],
  },
  {
    id: 'relation_rule',
    label: 'Ошибка в связи или лицензионном правиле',
    owner: 'Лицензионный специалист',
    kind: 'search_fix',
    codes: ['BROKEN_RELATION', 'MISSING_REQUIRED', 'CONSTRAINT_CONFLICT'],
  },
  {
    id: 'non_product',
    label: 'Запрос не о товаре',
    owner: 'Поддержка и справка',
    kind: 'owner_task',
    codes: ['NON_PRODUCT'],
  },
  {
    id: 'noise',
    label: 'Шум или недостаточно данных',
    owner: 'Ручной разбор',
    kind: 'owner_task',
    codes: ['NOISE', 'UNKNOWN'],
  },
]

export const KIND_LABELS: Record<CauseKind, string> = {
  search_fix: 'Правка поиска',
  owner_task: 'Задача владельцу',
}

const byCode = new Map(CAUSES.flatMap((c) => c.codes.map((code) => [code, c] as const)))
const byId = new Map(CAUSES.map((c) => [c.id as string, c]))
const byLabel = new Map(CAUSES.map((c) => [c.label.toLowerCase(), c]))

/** Причина по коду диагноза (подпричине); неизвестный код — null. */
export function causeOfCode(code: string | null | undefined): Cause | null {
  return code ? (byCode.get(code.trim().toUpperCase()) ?? null) : null
}

export interface CauseView {
  /** Причина из таблицы; null — сервер прислал свою, которой нет в таблице, или причины нет. */
  cause: Cause | null
  label: string
  owner: string
  kind: CauseKind
  /** Коды подпричин — мелко. */
  codes: string[]
}

export interface CauseInput {
  cause?: string | null
  owner?: string | null
  diagnosis?: string | null
  action?: string | null
}

/**
 * Причина, владелец и вид карточки. Порядок: cause сервера (id или подпись) → код диагноза → «ручной разбор».
 * owner сервера главнее владельца из таблицы. Вид: action=ticket — всегда задача владельцу (правила поиска не
 * меняются); иначе — по причине.
 */
export function causeView(p: CauseInput): CauseView {
  const server = (p.cause ?? '').trim()
  const fromServer = server ? (byId.get(server) ?? byLabel.get(server.toLowerCase()) ?? null) : null
  const fromCode = causeOfCode(p.diagnosis)
  const c = fromServer ?? fromCode ?? (server ? null : byId.get('noise')!)
  const kind: CauseKind =
    p.action === 'ticket' ? 'owner_task' : p.action ? 'search_fix' : (c?.kind ?? 'owner_task')
  return {
    cause: c,
    label: c?.label ?? server,
    owner: displayOwner(p.owner) || c?.owner || 'Ручной разбор',
    kind,
    codes: p.diagnosis ? [p.diagnosis] : [],
  }
}

/** Код роли (ecom, product…) — не подпись владельца: показываем владельца причины из таблицы. */
const ROLE_IDS = new Set(['ecom', 'product', 'content', 'head', 'system'])
function displayOwner(raw: string | null | undefined): string {
  const v = (raw ?? '').trim()
  return ROLE_IDS.has(v) ? '' : v
}

/** Похоже на код диагноза (ALIAS_GAP), а не на текст («поиск уже отвечает»). */
export const isDiagnosisCode = (v: string | null | undefined) =>
  !!v && /^[A-Z][A-Z_]+$/.test(v.trim())

/** Варианты фильтров «Причина» и «Владелец» (значение — id причины / подпись владельца). */
export const CAUSE_OPTIONS = CAUSES.map((c) => ({ value: c.id, label: c.label }))
export const OWNER_OPTIONS = [...new Set(CAUSES.map((c) => c.owner))].map((o) => ({
  value: o,
  label: o,
}))
