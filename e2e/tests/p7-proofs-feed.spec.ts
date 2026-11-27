// Контур эталонов (): «Доказательства → На данных заказчика» — один экран: эталоны заказчика, закрытая часть
// (Success@1 у BM25, гибрида и полного продукта или «Прогон не запускался»), доля ответов с ≥4 товарами, стоимость
// запроса при 15 тыс. в месяц с долями инфраструктуры и LLM, каталог «фид заказчика, N офферов». Числа — только из
// GET /api/evaluations/feed; дат на экране нет. Экран открыт гостю (жюри). Демо-состояние не меняется.
// E2E_SCREENS=1 — снимки 1440/360 в E2E_SHOTS_DIR (по умолчанию artifacts/screens/p7).
import type { Page } from '@playwright/test'
import { expect, test } from './fixtures'

const SHOTS = process.env.E2E_SCREENS === '1'
const OUT = process.env.E2E_SHOTS_DIR ?? '../artifacts/screens/p7'

interface FeedSummary {
  catalog: { source: string; offers: number }
  etalon: { matched: number; total: number } | null
  closed: { config: string }[]
  closed_detail?: { total: number; live: number; top4: number; model_searches: number; kinds: { kind: string }[] } | null
  four: { families4_n: number; answers: number } | null
  cost: { warm: { per_request_rub: number } | null; cold: { per_request_rub: number } | null } | null
}

async function summary(page: Page): Promise<FeedSummary> {
  const res = await page.request.get('/api/evaluations/feed')
  expect(res.ok()).toBe(true)
  return (await res.json()) as FeedSummary
}

async function noOverflow(page: Page, name: string) {
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
  )
  expect(overflow, `${name}: горизонтальная прокрутка`).toBeLessThanOrEqual(0)
}

/** Даты вида 28.05.2026, 2026-05-28, «26 ноября». */
const DATE = /\b\d{1,2}\.\d{1,2}\.\d{4}\b|\b\d{4}-\d{2}-\d{2}\b|\b\d{1,2} (января|февраля|марта|апреля|мая|июня|июля|августа|сентября|октября|ноября|декабря)\b/

test.describe('S6. Доказательства на данных заказчика', () => {
  test('сводка — первый экран раздела; числа из прогонов, без дат', async ({ page }) => {
    const s = await summary(page)
    await page.goto('/proofs')
    await expect(page.getByRole('tab', { name: /^На данных заказчика/ })).toHaveAttribute('data-state', 'active')
    const panel = page.getByRole('tabpanel')

    if (s.catalog.source === 'feed')
      await expect(panel.getByText(/фид заказчика, \d[\d\s ]* офферов/)).toBeVisible()

    const etalon = panel.getByRole('button', { name: /^Эталоны заказчика: .* Как посчитано/ })
    if (s.etalon) {
      await expect(etalon).toContainText(`${s.etalon.matched} из ${s.etalon.total}`)
      await etalon.click()
      await expect(page.getByRole('dialog').getByText('Как считается')).toBeVisible()
      await page.keyboard.press('Escape')
    } else {
      await expect(panel.getByText('прогон эталонов не запускался')).toBeVisible()
    }

    // Отложенный набор (закрытая часть): до финального прогона — «Прогон не запускался», после — три числа по конфигурациям.
    const closed = panel.getByRole('button', { name: /^Отложенный набор/ })
    if (s.closed.length === 0) await expect(closed).toContainText('Прогон не запускался')
    else for (const label of ['BM25', 'Гибрид', 'Полная конфигурация']) await expect(closed).toContainText(label)

    // Разбор отложенного набора по видам запросов: блок есть только с прогоном, числа — из ответа API.
    const heldOut = panel.getByRole('region', { name: 'Отложенный набор: проверка на незнакомых запросах' })
    const d = s.closed_detail
    if (d) {
      await expect(heldOut.getByText(`${d.live} из ${d.total}`, { exact: true })).toBeVisible()
      await expect(heldOut.getByText(`${d.top4} из ${d.total}`, { exact: true })).toBeVisible()
      await expect(heldOut.getByText(new RegExp(`на ${d.model_searches} запрос\\S+ из ${d.total},`))).toBeVisible()
      const table = heldOut.getByRole('region', { name: 'Отложенный набор по видам запросов' })
      await expect(table.getByRole('row')).toHaveCount(d.kinds.length + 2) // шапка, виды, «Всего»
      await expect(table.getByRole('row', { name: /^Всего/ })).toContainText(String(d.top4))
    } else {
      await expect(heldOut).toHaveCount(0)
    }

    if (s.four) await expect(panel.getByText(new RegExp(`${s.four.families4_n} из ${s.four.answers}`))).toBeVisible()
    const run = s.cost?.warm ?? s.cost?.cold
    if (run) {
      await expect(panel.getByRole('button', { name: /^Стоимость запроса: / })).toBeVisible()
      await expect(panel.getByText(/запросов в месяц · инфраструктура/)).toBeVisible()
      await expect(panel.getByRole('heading', { name: 'Стоимость запроса' })).toBeVisible()
    }

    // Дат на экране нет — ни в числах, ни в подписях каталога.
    expect(await panel.innerText()).not.toMatch(DATE)
  })

  test('360 и 1440 px: без горизонтальной прокрутки', async ({ page }) => {
    for (const [w, h] of [
      [360, 780],
      [1440, 900],
    ] as const) {
      await page.setViewportSize({ width: w, height: h })
      await page.goto('/proofs?tab=feed')
      await expect(page.getByRole('button', { name: /^Эталоны заказчика|^Отложенный набор/ }).first()).toBeVisible()
      await noOverflow(page, `S6 ${w}`)
      if (SHOTS) await page.screenshot({ path: `${OUT}/proofs-feed-${w}.png`, fullPage: true })
    }
  })

  test('гостю экран открыт, ошибка API — блок с повтором', async ({ browser }) => {
    const ctx = await browser.newContext({ storageState: { cookies: [], origins: [] } })
    const page = await ctx.newPage()
    await page.route('**/api/evaluations/feed', (r) => r.fulfill({ status: 503, json: { error: { code: 'UNAVAILABLE', message: 'Сервис недоступен' } } }))
    await page.goto('/proofs?tab=feed')
    await expect(page.getByText('Сводка на данных заказчика не загрузилась')).toBeVisible()
    await page.unroute('**/api/evaluations/feed')
    await page.getByRole('button', { name: /Повторить/ }).first().click()
    await expect(page.getByText('Сводка на данных заказчика не загрузилась')).toBeHidden()
    await ctx.close()
  })
})
