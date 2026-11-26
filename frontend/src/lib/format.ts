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

/** Цена есть и больше нуля. Нулевая цена в фиде значит «по запросу» — как цена не показывается. */
export function isPositivePrice(rub: number | null | undefined): rub is number {
  return typeof rub === 'number' && Number.isFinite(rub) && rub > 0
}

/**
 * Цена в рублях: 12990 → «12 990 ₽»; null, 0 и отрицательное — пустая строка (вызывающий пишет «Цена по запросу»):
 * «0 ₽» на экран не выводится нигде.
 */
export function formatPrice(rub: number | null | undefined): string {
  if (!isPositivePrice(rub)) return ''
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

/**
 * Деньги в таблицах и сводках (запуски агента): всегда копейки, два знака — «6,66 ₽», «0,18 ₽»; меньше копейки —
 * «< 0,01 ₽»; ровно 0 — «0 ₽». formatCost с четырьмя знаками — для цены одного вызова модели.
 */
export function formatMoney(rub: number): string {
  if (!Number.isFinite(rub)) return ''
  if (rub === 0) return `0${NBSP}₽`
  if (Math.abs(rub) < 0.01) return `<${NBSP}0,01${NBSP}₽`
  return `${formatDecimal(rub, 2)}${NBSP}₽`
}

/** Прямые кавычки текста модели → «ёлочки»: "антивирус" → «антивирус». Непарная кавычка остаётся. */
export function russianQuotes(text: string): string {
  return text.replace(/"([^"\n]*)"/g, '«$1»')
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

/** Доля 0..1 → «12,3 %». Целые проценты без дробной части: 1 → «100 %», 0 → «0 %». */
export function formatPercent(share: number, digits = 1): string {
  if (!Number.isFinite(share)) return ''
  const pct = share * 100
  const rounded = Number(pct.toFixed(digits))
  const body = Number.isInteger(rounded) ? formatInt(rounded) : formatDecimal(pct, digits)
  return `${body}${NBSP}%`
}

/** Знак числа для изменений: «+», «−» или пусто для нуля. */
export function signOf(n: number): '+' | '−' | '' {
  if (n > 0) return '+'
  if (n < 0) return '−'
  return ''
}

/** Изменение доли в процентных пунктах: 0.032 → «+3,2 п.п.». */
export function formatPp(diffShare: number, digits = 1): string {
  if (!Number.isFinite(diffShare)) return ''
  const pp = Number((diffShare * 100).toFixed(digits))
  if (pp === 0) return `0${NBSP}п.п.`
  return `${signOf(pp)}${formatDecimal(Math.abs(pp), digits)}${NBSP}п.п.`
}

/** Число со знаком: 0.0512, 3 → «+0,051»; ноль — «0». */
export function formatSigned(n: number, digits = 2): string {
  if (!Number.isFinite(n)) return ''
  const r = Number(n.toFixed(digits))
  if (r === 0) return '0'
  return `${signOf(r)}${formatDecimal(Math.abs(r), digits)}`
}

/** Сумма в рублях для итогов: от 100 ₽ — целые с разрядами, меньше — как formatCost. */
export function formatRub(rub: number): string {
  if (!Number.isFinite(rub)) return ''
  if (Math.abs(rub) >= 100) return `${formatInt(rub)}${NBSP}₽`
  return formatCost(rub)
}

/** Дата и время ISO → «27.05.2026, 14:03» (местное время). Невалидная строка — как есть. */
export function formatDateTime(iso: string | null | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getDate())}.${p(d.getMonth() + 1)}.${d.getFullYear()}, ${p(d.getHours())}:${p(d.getMinutes())}`
}
