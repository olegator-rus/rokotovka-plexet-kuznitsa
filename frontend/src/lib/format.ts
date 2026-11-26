// Форматирование чисел для экрана. Без DOM: проверяют tests/format.test.ts.
// Разделитель разрядов — неразрывный пробел, десятичный — запятая (ru-RU).

const NBSP = ' '

/** Русское склонение по числу: plural(5, ['товар', 'товара', 'товаров']) → 'товаров'. */
export function plural(n: number, forms: [string, string, string]): string {
  const abs = Math.abs(Math.trunc(n))
  const mod10 = abs % 10
  const mod100 = abs % 100
  if (mod10 === 1 && mod100 !== 11) return forms[0]
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return forms[1]
  return forms[2]
}

/** Целое с разрядами: 12990 → «12 990». */
export function formatInt(n: number): string {
  const sign = n < 0 ? '−' : ''
  const digits = String(Math.round(Math.abs(n)))
  return sign + digits.replace(/\B(?=(\d{3})+(?!\d))/g, NBSP)
}

/** Число с фиксированной точностью и запятой: 0.8234, 2 → «0,82». */
export function formatDecimal(n: number, digits = 2): string {
  const fixed = Math.abs(n).toFixed(digits)
  const [int, frac] = fixed.split('.')
  const sign = n < 0 && Number(fixed) !== 0 ? '−' : ''
  const intPart = formatInt(Number(int))
  return frac === undefined ? sign + intPart : `${sign}${intPart},${frac}`
}

/** Цена в рублях: 12990 → «12 990 ₽»; null — пустая строка (вызывающий пишет «Цена по запросу»). */
export function formatPrice(rub: number | null | undefined): string {
  if (rub === null || rub === undefined || !Number.isFinite(rub)) return ''
  const whole = Number.isInteger(rub)
  return `${whole ? formatInt(rub) : formatDecimal(rub, 2)}${NBSP}₽`
}

/** Время ответа: меньше миллисекунды — «<1 мс», до секунды — «41 мс», дальше — «1,2 с». */
export function formatMs(ms: number): string {
  if (!Number.isFinite(ms)) return ''
  if (ms > 0 && ms < 0.5) return `<1${NBSP}мс`
  if (ms < 1000) return `${Math.round(ms)}${NBSP}мс`
  return `${formatDecimal(ms / 1000, 1)}${NBSP}с`
}

/** Стоимость запроса: ровно 0 — «0 ₽», доли рубля — до 4 знаков без хвостовых нулей. */
export function formatCost(rub: number): string {
  if (!Number.isFinite(rub)) return ''
  if (rub === 0) return `0${NBSP}₽`
  if (Math.abs(rub) >= 1) return `${formatDecimal(rub, 2)}${NBSP}₽`
  const trimmed = formatDecimal(rub, 4).replace(/0+$/, '').replace(/,$/, '')
  return `${trimmed}${NBSP}₽`
}

/** Скор 0..1 → «0,82». */
export function formatScore(n: number): string {
  if (!Number.isFinite(n)) return ''
  return formatDecimal(n, 2)
}

/** «Найдено 7 · 41 мс». */
export function formatFound(total: number, ms: number | null | undefined): string {
  const head = `Найдено ${formatInt(total)}`
  return ms === null || ms === undefined ? head : `${head} · ${formatMs(ms)}`
}
