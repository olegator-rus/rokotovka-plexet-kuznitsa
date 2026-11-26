// Строка лицензии и варианты семейства. Без DOM: проверяют tests/license.test.ts.
// Сервер присылает готовую строку «Подписка · 10 устройств · 1 год» (`license_line`);
// фронт делит её на части для показа и упорядочивает варианты внутри раскрытой карточки.
import type { SearchItem, SearchItemBase } from '@/api/types'
import { formatPrice, isPositivePrice } from './format.ts'

export const LICENSE_SEPARATOR = ' · '

/** «Подписка · 10 устройств · 1 год» → ['Подписка', '10 устройств', '1 год']; пустые части отброшены. */
export function splitLicenseLine(line: string | null | undefined): string[] {
  if (!line) return []
  return line
    .split(/\s*[·•|]\s*/)
    .map((p) => p.trim())
    .filter(Boolean)
}

/** Части обратно в строку с единым разделителем (сервер мог прислать «|» или «•»). */
export function joinLicenseParts(parts: string[]): string {
  return parts.join(LICENSE_SEPARATOR)
}

/** Вид позиции фида (`license_kind`, §16.6), кроме базовой: с этих слов начинается `license_line` сервера. */
export const LICENSE_KIND_LABELS: Record<string, string> = {
  renewal: 'Продление',
  upgrade: 'Обновление',
  migration: 'Переход с другого продукта',
  support: 'Техподдержка',
  module: 'Модуль',
  course: 'Курс',
}

/** Базовая лицензия — по типу из атрибутов. */
export const LICENSE_TYPE_LABELS: Record<string, string> = {
  subscription: 'Новая · подписка',
  perpetual: 'Новая · бессрочная',
  renewal: 'Продление',
  upgrade: 'Обновление',
  oem: 'OEM',
  cloud: 'Облачная',
}

/** Вид из начала `license_line` («Переход с другого продукта · 1 год» → «Переход с другого продукта»). */
export function kindFromLine(line: string | null | undefined): string {
  const first = splitLicenseLine(line)[0] ?? ''
  return (
    Object.values(LICENSE_KIND_LABELS).find((l) => l.toLowerCase() === first.toLowerCase()) ?? ''
  )
}

/**
 * Подпись вида лицензии: `license_kind` сервера главнее (Cross-grade с `license_type=upgrade` — «Переход с другого
 * продукта», а не «Новая»); без вида (mock, страница товара) — по началу `license_line`; базовая — по типу атрибутов.
 * Ничего не известно — ''.
 */
export function licenseKindLabel(
  v: {
    license_kind?: string | null
    license_line?: string | null
    attrs?: { license_type?: string | null } | null
  },
  typeLabels: Record<string, string> = LICENSE_TYPE_LABELS,
): string {
  const kind = v.license_kind ?? ''
  if (LICENSE_KIND_LABELS[kind]) return LICENSE_KIND_LABELS[kind]
  if (kind !== 'base') {
    const fromLine = kindFromLine(v.license_line)
    if (fromLine) return fromLine
  }
  const type = v.attrs?.license_type ?? ''
  return typeLabels[type] ?? type
}

/** Цена для показа: «12 990 ₽» или «Цена по запросу». */
export function priceLabel(
  item: Pick<SearchItemBase, 'price_rub' | 'price_on_request'> & {
    price_mode?: string
    price_state?: string | null
  },
): string {
  // Контур коммерции: «от …» и «Нет в наличии» по price_mode; без поля — как в контуре поиска.
  if (item.price_mode === 'unavailable') return 'Нет в наличии'
  if (!isPurchasable(item)) return 'Цена по запросу'
  return item.price_mode === 'from'
    ? `от ${formatPrice(item.price_rub)}`
    : formatPrice(item.price_rub)
}

/** Можно положить в корзину: цена известна. Иначе действие — «Запросить цену». */
export function isPurchasable(
  item: Pick<SearchItemBase, 'price_rub' | 'price_on_request'> & { price_state?: string | null },
): boolean {
  // Контур фида: нулевая цена — «по запросу»; price_state сервера главнее.
  if (item.price_state === 'on_request') return false
  return !item.price_on_request && isPositivePrice(item.price_rub)
}

/**
 * Варианты лицензий в раскрытой карточке: подходящие под запрос первыми, дальше по цене
 * (по запросу — в конце), при равенстве — исходный порядок сервера.
 */
export function sortVariants<T extends SearchItemBase>(variants: readonly T[]): T[] {
  return variants
    .map((v, i) => ({ v, i }))
    .sort((a, b) => {
      if (a.v.matches_query !== b.v.matches_query) return a.v.matches_query ? -1 : 1
      const pa = isPurchasable(a.v) ? (a.v.price_rub as number) : Number.POSITIVE_INFINITY
      const pb = isPurchasable(b.v) ? (b.v.price_rub as number) : Number.POSITIVE_INFINITY
      if (pa !== pb) return pa - pb
      return a.i - b.i
    })
    .map(({ v }) => v)
}

/** Варианты без дубля основной позиции. */
export function otherVariants(item: SearchItem): SearchItemBase[] {
  return sortVariants((item.variants ?? []).filter((v) => v.product_id !== item.product_id))
}
