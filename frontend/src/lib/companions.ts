// Допродажа и совместимость на витрине («Правила блока»). Без DOM:
// tests/companions.test.ts. Решает сервер: что показать и в каком классе; фронт подписывает и отбирает
// только по флагам и по выбору покупателя («уже есть», RDS).
import type {
  Companion,
  CompanionBlock,
  EvidenceLevel,
  Features,
  ItemCompatibility,
} from '@/api/typesP2'

export const CLASS_LABELS: Record<string, string> = {
  conditional_required: 'Проверьте необходимость',
  compatible: 'Совместимо',
  complement: 'Дополняет решение',
  upgrade: 'Старшая редакция',
  renewal: 'Продление и поддержка',
}

/** Основание связи — подпись к иконке (иконку выбирает компонент по тому же ключу). */
export const SOURCE_LABELS: Record<string, string> = {
  vendor_documentation: 'Документация вендора',
  catalog_rule: 'Правило каталога',
  expert: 'Проверено экспертом',
  co_purchase: 'Часто берут вместе',
  agent_proposal: 'Предложено агентом, принято человеком',
}

export const sourceLabel = (s: string) => SOURCE_LABELS[s] ?? 'Справочник связей'

/** Подпись класса: сервер присылает готовую `label`, иначе — по классу. */
export function classLabel(c: Pick<Companion, 'label' | 'class'>): string {
  return c.label?.trim() || CLASS_LABELS[c.class] || 'Дополняет решение'
}

/** Без флагов в meta (старый сервер) — всё включено: ответ сам несёт пустые поля при выключенном флаге. */
export function featureOn(features: Partial<Features> | null | undefined, key: keyof Features) {
  return features?.[key] !== false
}

/**
 * Позиции блока «Дополните решение» к показу: несовместимые — никогда; то, что уже стоит строкой условно
 * обязательного в карточке якоря, в блоке не повторяется; не больше 4 из справочника связей (mock) и не больше 6
 * на фиде (§16.7: там позиции несут `evidence`, лимиты по видам основания ставит сервер). Пусто — блок скрыт
 * целиком (без добора популярными).
 */
export function blockItems(
  block: CompanionBlock | null | undefined,
  inCard: readonly string[] = [],
): Companion[] {
  const items = (block?.items ?? []).filter(
    (c) => c.compatibility !== 'incompatible' && !inCard.includes(c.product_id),
  )
  const feed = items.some((c) => c.evidence)
  return items.slice(0, feed ? 6 : 4)
}

export const BLOCK_TITLE = 'Дополните решение'

/** Заголовок блока — из `companions.title`; «Часто покупают вместе» не пишем: заказов в данных нет (§16.7). */
export function blockTitle(block: Pick<CompanionBlock, 'title'> | null | undefined): string {
  const t = block?.title?.trim() ?? ''
  return t && !/часто\s+(покупа|бер)/i.test(t) ? t : BLOCK_TITLE
}

/** Уровень основания: подпись бейджа без жаргона. */
export const EVIDENCE_TAGS: Record<EvidenceLevel, string> = {
  A: 'по документу вендора',
  B: 'из каталога',
  C: 'проверьте совместимость',
}

/** Подпись по виду основания (§16.8): вид точнее уровня. */
export const EVIDENCE_KIND_TAGS: Record<string, string> = {
  compat_document: 'по документу вендора',
  license_kind: 'из каталога: тип лицензии',
  sku_platform: 'из каталога: название позиции',
  class_logic: 'по логике класса',
}

export interface EvidenceView {
  level: EvidenceLevel
  /** «по документу вендора» / «из каталога: тип лицензии» / «по логике класса: <причина>» / по уровню. */
  tag: string
  /**
   * Вид `class_logic` (связь классов из таблицы заказчика): причина уже стоит в подписи «по логике класса: …»,
   * отдельной строкой не повторяется.
   */
  reasonInTag: boolean
  /** Ссылка на документ — только у уровня A и только http(s). */
  docUrl: string
  /** Что за документ — для подсказки и экранного диктора. */
  docTitle: string
}

const safeHttp = (u: string | null | undefined) =>
  /^https?:\/\//i.test(u ?? '') ? (u as string) : ''

/** Основание позиции: объект `evidence` (фид), иначе прежнее `evidence_level`; уровня нет — null. */
export function evidenceView(
  c: Pick<Companion, 'evidence' | 'evidence_level' | 'source_url'> &
    Partial<Pick<Companion, 'reason'>>,
): EvidenceView | null {
  const level = (c.evidence?.level || c.evidence_level || '').trim().toUpperCase()
  if (level !== 'A' && level !== 'B' && level !== 'C') return null
  const kind = c.evidence?.kind ?? ''
  const docUrl = level === 'A' ? safeHttp(c.evidence?.url || c.source_url) : ''
  const reason = noDates(c.reason ?? '')
  const classLogic = kind === 'class_logic' && level !== 'C'
  let tag = (level !== 'C' && EVIDENCE_KIND_TAGS[kind]) || EVIDENCE_TAGS[level]
  if (classLogic && reason) tag = `${tag}: ${lowerFirst(reason)}`
  return {
    level,
    tag,
    reasonInTag: classLogic && Boolean(reason),
    docUrl,
    docTitle: noDates(c.evidence?.title ?? '') || 'документ вендора',
  }
}

const lowerFirst = (s: string) =>
  /^[А-ЯЁA-Z][а-яёa-z]/.test(s) ? s.charAt(0).toLowerCase() + s.slice(1) : s

/** Бейджи карточки блока как на выдаче: «Скидка N%»; «Акция» — только если строки акции нет. */
export function companionBadges(
  c: Pick<Companion, 'badges' | 'promos'>,
): { text: string; discount: boolean }[] {
  const hasPromo = (c.promos ?? []).length > 0
  const out: { text: string; discount: boolean }[] = []
  for (const raw of c.badges ?? []) {
    const text = noDates(typeof raw === 'string' ? raw : '').trim()
    if (!text || out.some((b) => b.text === text)) continue
    const discount = /скидк/i.test(text)
    if (!discount && hasPromo && /^акция$/i.test(text)) continue
    out.push({ text, discount })
  }
  return out
}

/** Даты в интерфейсе не пишем: «… № 29073/2025 от 23.01.2025» → «… № 29073/2025». */
export function noDates(text: string): string {
  return text
    .replace(/\s*(?:от|до)?\s*\b\d{1,2}\.\d{1,2}\.\d{4}(?:\s*г\.?)?/gi, '')
    .replace(/\s*(?:от|до)?\s*\b\d{4}-\d{2}-\d{2}\b/gi, '')
    .replace(/\s{2,}/g, ' ')
    .trim()
}

/** Строка «почему» под карточкой блока: причина сервера без дат. */
export const reasonText = (c: Pick<Companion, 'reason'>) => noDates(c.reason ?? '')

/** Условие «RDS планируется»: такие позиции строки появляются только после переключателя. */
export function isRdsCompanion(c: Pick<Companion, 'conditions'>): boolean {
  return (c.conditions ?? []).some((x) => /rds|remote_desktop/i.test(x))
}

export interface RequiredView {
  /** Позиции, показанные сразу. */
  base: Companion[]
  /** Позиции, которые добавляет переключатель «Планируется RDS». */
  rds: Companion[]
}

export function splitRequired(required: readonly Companion[] | null | undefined): RequiredView {
  const list = (required ?? []).filter((c) => c.compatibility !== 'incompatible')
  return {
    base: list.filter((c) => !isRdsCompanion(c)),
    rds: list.filter(isRdsCompanion),
  }
}

/** Совместимость не подтверждена — товар не подставляется, строка зовёт к менеджеру. */
export const isUnconfirmed = (c: Pick<Companion, 'compatibility'>) => c.compatibility === 'unknown'

/** Свёрнутая строка условно обязательного: «Проверьте необходимость: лицензии доступа CAL × 50». */
export function requiredHeadline(c: Companion, quantity: number | null): string {
  if (isUnconfirmed(c)) {
    const what = c.category ? c.category.toLowerCase() : 'дополнительные лицензии'
    return `Могут потребоваться ${what} — уточните у менеджера`
  }
  const qty = quantity && quantity > 0 ? ` × ${quantity}` : ''
  return `${classLabel(c)}: ${c.title}${qty}`
}

export const QTY_HINT = 'по числу пользователей или устройств'

/** Значок совместимости в карточке выдачи. incompatible в выдачу не попадает — значка нет. */
export interface CompatView {
  tone: 'ok' | 'cond' | 'unknown'
  text: string
}

export function compatView(c: ItemCompatibility | null | undefined, model = ''): CompatView | null {
  if (!c) return null
  const target = model || c.target_model
  switch (c.state) {
    case 'verified':
      return {
        tone: 'ok',
        text: target ? `Подтверждено для ${target}` : 'Совместимость подтверждена',
      }
    case 'conditional':
      return {
        tone: 'cond',
        text: c.condition ? `При условии: ${c.condition}` : 'Совместимо при условии',
      }
    case 'unknown':
      return {
        tone: 'unknown',
        text: target
          ? `Совместимость с ${target} не подтверждена`
          : 'Совместимость не подтверждена',
      }
    default:
      return null
  }
}

/** Над выдачей: «Для точной проверки укажите конфигурацию сервера», если среди позиций есть условные. */
export function hasConditional(items: readonly { compatibility?: ItemCompatibility | null }[]) {
  return items.some((i) => i.compatibility?.state === 'conditional')
}

/** Каноническая модель «dell-poweredge-r740» → «Dell PowerEdge R740» для подписи, если имени нет. */
export function modelTitle(id: string): string {
  return id
    .split(/[-_]+/)
    .filter(Boolean)
    .map((w) => {
      if (/\d/.test(w)) return w.toUpperCase()
      if (w.toLowerCase() === 'poweredge') return 'PowerEdge'
      if (w.toLowerCase() === 'proliant') return 'ProLiant'
      if (w.toLowerCase() === 'thinksystem') return 'ThinkSystem'
      return w.charAt(0).toUpperCase() + w.slice(1)
    })
    .join(' ')
}

/**
 * Показ блока пишется один раз на поиск («События»): ключ — номер поиска и место показа.
 * Возвращает true, если событие надо отправить, и запоминает ключ.
 */
export function firstImpression(seen: Set<string>, requestId: string, place: string): boolean {
  if (!requestId) return false
  const key = `${requestId}:${place}`
  if (seen.has(key)) return false
  seen.add(key)
  return true
}

/** Полезная нагрузка companion_impression (anchor, items). */
export function impressionPayload(anchor: string, items: readonly Companion[], place: string) {
  return {
    anchor,
    place,
    items: items.map((c) => ({ product_id: c.product_id, class: c.class })),
  }
}
