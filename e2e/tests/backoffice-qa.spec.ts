// Проверки бэк-офиса после QA-прохода: на экранах нет календарных дат, строка отчёта на 360 px не рассыпается
// на столбик букв, песочница политики не крутится вечно, если сравнивать не на чем. Демо-состояние не меняется:
// только чтение, «Применить» и «Откатить» не нажимаются.
import type { Page } from '@playwright/test'
import { expect, test } from './fixtures'

/** Даты вида 28.05.2026, 26.11, 2026-05-28, «26 ноября». */
const DATE =
  /\b\d{1,2}\.\d{2}\.\d{4}\b|\b\d{4}-\d{2}-\d{2}\b|\b\d{1,2} (января|февраля|марта|апреля|мая|июня|июля|августа|сентября|октября|ноября|декабря)\b/

async function mainText(page: Page) {
  return page.locator('main').innerText()
}

test.describe('бэк-офис без дат', () => {
  for (const tab of ['memory', 'changes', 'runs']) {
    test(`SearchOps «${tab}»: время без календарной даты`, async ({ page }) => {
      await page.goto(`/searchops?tab=${tab}`)
      await expect(page.getByRole('tabpanel').first()).toBeVisible()
      await page.waitForLoadState('networkidle')
      expect(await mainText(page)).not.toMatch(DATE)
    })
  }

  test('журнал вызовов модели в «Стоимости» — без дат', async ({ page }) => {
    await page.goto('/proofs?tab=cost')
    await expect(page.getByRole('heading', { name: 'Журнал вызовов модели' })).toBeVisible()
    expect(await mainText(page)).not.toMatch(DATE)
  })
})

test('360 px: строка «Отчёта» — запрос строкой, частота и статус не наезжают', async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 780 })
  await page.goto('/searchops')
  const row = page.locator('.mrep__row-btn').first()
  await expect(row).toBeVisible()
  const box = await row.evaluate((el) => {
    const r = (s: string) => el.querySelector(s)!.getBoundingClientRect()
    return {
      query: r('.mrep__q'),
      freq: r('.mrep__freq'),
      status: r('.mrep__status'),
    }
  })
  expect(box.query.width, 'колонка запроса не сжата в столбик').toBeGreaterThan(200)
  // Частота и статус — разные строки: статус ниже частоты.
  expect(box.status.top).toBeGreaterThanOrEqual(box.freq.bottom - 1)
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
  )
  expect(overflow).toBeLessThanOrEqual(0)
})

test('«Политика ранжирования»: без размеченного набора — объяснение, а не вечный «Собираем базовую выдачу»', async ({
  page,
}) => {
  await page.goto('/searchops?tab=policy')
  await expect(page.getByRole('heading', { name: 'Рычаг маржи' })).toBeVisible()
  // Либо прогон положений рычага (есть набор), либо панель «не на чем сравнить» — но не бесконечный сбор.
  await expect(
    page.getByTestId('policy-no-set').or(page.getByText(/сменился первый результат/)),
  ).toBeVisible({ timeout: 40_000 })
  await expect(page.getByText(/Собираем базовую выдачу/)).toHaveCount(0)
})
