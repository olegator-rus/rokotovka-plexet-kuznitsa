// Закупка под задачу: выбор позиции в слотах шаблона («заменить»), условные слоты,
// строки спецификации «Добавить всё». Количества и фасовки считает сервер (slot.quantities); фронт только
// выбирает из присланного. Без DOM: tests/solution.test.ts.
import type { SearchItemLite, Solution, SolutionSlot } from '@/api/typesP2'
import { type SpecLine, clampQty, priceModeOf, summarize } from './spec.ts'

/** Выбранная позиция по слотам: id слота → id позиции. */
export type SlotChoice = Record<string, string>

/** Варианты слота: выбранная по умолчанию и альтернативы. */
export function slotOptions(slot: SolutionSlot): SearchItemLite[] {
  return slot.item ? [slot.item, ...(slot.alternatives ?? [])] : []
}

export function defaultChoice(sol: Solution): SlotChoice {
  const out: SlotChoice = {}
  for (const s of sol.slots) if (s.item) out[s.id] = s.item.product_id
  return out
}

export function chosenItem(slot: SolutionSlot, choice: SlotChoice): SearchItemLite | null {
  const opts = slotOptions(slot)
  return opts.find((o) => o.product_id === choice[slot.id]) ?? opts[0] ?? null
}

/** Количество выбранной позиции слота (сервер прислал для каждой); нет — количество слота. */
export function slotQty(slot: SolutionSlot, choice: SlotChoice): number {
  const it = chosenItem(slot, choice)
  return clampQty((it && slot.quantities?.[it.product_id]) || slot.quantity || 1)
}

/** Слот нужен: есть позиция и (если слот условный) выбранная позиция слота-условия из списка сервера. */
export function slotActive(slot: SolutionSlot, sol: Solution, choice: SlotChoice): boolean {
  if (!slot.item) return false
  const c = slot.condition
  if (!c) return true
  const dep = sol.slots.find((s) => s.id === c.slot)
  const pick = dep ? chosenItem(dep, choice)?.product_id : undefined
  return Boolean(pick && c.product_ids.includes(pick))
}

/** Строки спецификации по нужным слотам (без убранных покупателем). */
export function solutionLines(
  sol: Solution,
  choice: SlotChoice,
  removed: ReadonlySet<string> = new Set(),
): SpecLine[] {
  const lines: SpecLine[] = []
  for (const s of sol.slots) {
    if (removed.has(s.id) || !slotActive(s, sol, choice)) continue
    const it = chosenItem(s, choice)
    if (!it) continue
    lines.push({
      product_id: it.product_id,
      title: it.title,
      vendor: it.vendor,
      category: it.category,
      license_line: it.license_line,
      price_rub: it.price_rub,
      price_mode: priceModeOf(it),
      quantity: slotQty(s, choice),
      origin: 'template',
      anchor: sol.title,
    })
  }
  return lines
}

/** Итог карточки по выбранным позициям — тем же правилом, что итог спецификации. */
export const solutionSummary = (
  sol: Solution,
  choice: SlotChoice,
  removed: ReadonlySet<string> = new Set(),
) => summarize(solutionLines(sol, choice, removed))
