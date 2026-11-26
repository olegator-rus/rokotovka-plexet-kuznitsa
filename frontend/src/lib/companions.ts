// Допродажа и совместимость на витрине («Правила блока»). Без DOM:
// tests/companions.test.ts. Решает сервер: что показать и в каком классе; фронт подписывает и отбирает
// только по флагам и по выбору покупателя («уже есть», RDS).
import type { Companion, CompanionBlock, Features, ItemCompatibility } from '@/api/typesP2'

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
 * обязательного в карточке якоря, в блоке не повторяется; не больше 4. Пусто — блок скрыт целиком
 * (без добора популярными).
 */
export function blockItems(
  block: CompanionBlock | null | undefined,
  inCard: readonly string[] = [],
): Companion[] {
  const items = (block?.items ?? []).filter(
    (c) => c.compatibility !== 'incompatible' && !inCard.includes(c.product_id),
  )
  return items.slice(0, 4)
}

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
