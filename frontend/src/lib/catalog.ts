// Витрина без DOM: типы каталога, временный поиск по витрине и формат цены. Без импортов —
// модуль проверяют node-тесты (tests/catalog.test.ts).
//
// Поиск здесь — заглушка на время, пока у API нет каталога: подстрочное совпадение по словам.
// Настоящий поиск (кейс хакатона) живёт на сервере; когда появится GET /api/search, страница
// зовёт его, а filterProducts уходит.

export type CategoryId = 'electronics' | 'home' | 'sport' | 'clothes' | 'books' | 'kids'

export interface Category {
  id: CategoryId
  title: string
}

export interface Product {
  id: string
  title: string
  category: CategoryId
  /** Цена в рублях. */
  price: number
  /** Цена до скидки; нет — скидки нет. */
  oldPrice?: number
}

/** Нижний регистр, ё → е, пробелы схлопнуты. */
export function normalize(s: string): string {
  return s.toLowerCase().replaceAll('ё', 'е').replace(/\s+/g, ' ').trim()
}

/**
 * Товары, у которых в названии или категории есть каждое слово запроса; category — только эта
 * категория. Пустой запрос — все товары категории. Порядок исходный.
 */
export function filterProducts(
  products: readonly Product[],
  categories: readonly Category[],
  query: string,
  category: CategoryId | null = null,
): Product[] {
  const words = normalize(query).split(' ').filter(Boolean)
  const titleOf = new Map(categories.map((c) => [c.id, c.title]))
  return products.filter((p) => {
    if (category && p.category !== category) return false
    if (words.length === 0) return true
    const hay = normalize(`${p.title} ${titleOf.get(p.category) ?? ''}`)
    return words.every((w) => hay.includes(w))
  })
}

const rub = new Intl.NumberFormat('ru-RU', {
  style: 'currency',
  currency: 'RUB',
  maximumFractionDigits: 0,
})

/** 6990 → «6 990 ₽» (неразрывные пробелы). */
export function formatPrice(value: number): string {
  return rub.format(value)
}

/** Категория из query-параметра: неизвестное значение — null (все товары). */
export function parseCategory(raw: unknown, categories: readonly Category[]): CategoryId | null {
  return typeof raw === 'string' && categories.some((c) => c.id === raw)
    ? (raw as CategoryId)
    : null
}
