// Бэк-офис контура коммерции на живом API (без подмен): экраны открываются, данные сервера видны, ничего не
// меняется (применение маржи и откат связей — только до диалога). Методов контура коммерции на сервере нет — тест
// пропускается. E2E_SCREENS=1 — скриншоты live-* в artifacts/screens/p2/.
import type { Page } from '@playwright/test'
import { expect, test } from './fixtures'

const API = process.env.E2E_API_URL ?? 'http://127.0.0.1:8088'
const SHOTS = process.env.E2E_SCREENS === '1'
const shot = async (page: Page, name: string) => {
  if (SHOTS)
    await page.screenshot({ path: `../artifacts/screens/p2/${name}.png`, fullPage: true })
}

async function hasP2Api() {
  try {
    const r = await fetch(`${API}/api/margin/policy`, { signal: AbortSignal.timeout(5000) })
    return r.ok
  } catch {
    return false
  }
}

test.describe('Бэк-офис П2 — живой API', () => {
  test.beforeEach(async () => {
    test.skip(!(await hasP2Api()), 'методов контура коммерции на API пока нет')
  })

  test('F23 live: «Политика ранжирования» — рычаг, пример, диалог применения без применения', async ({
    page,
  }) => {
    test.setTimeout(120_000)
    await page.goto('/searchops?tab=policy')
    const lever = page.getByRole('group', { name: 'Приоритет маржи' })
    await expect(lever.getByRole('button', { name: /действует/ })).toBeVisible()
    // Первая симуляция собирает базовую выдачу (до ~20 с на tune).
    await expect(page.getByRole('region', { name: 'Пример: было / стало' })).toBeVisible({
      timeout: 90_000,
    })
    await expect(page.getByText(/Оценка на \d+ размеченн/)).toBeVisible()
    await expect(page.getByText('Синтетические данные').first()).toBeVisible()
    await shot(page, 'live-margin-1440')

    const free = lever.locator('button:not([disabled]):not([data-state="on"])').first()
    await free.click()
    await expect(page).toHaveURL(/w=/)
    await expect(page.getByText(/сменился первый результат:/)).toBeVisible()
    await page.getByRole('button', { name: /^Применить вес/ }).click()
    const dialog = page.getByRole('dialog', { name: /^Применить вес/ })
    await expect(dialog).toContainText('Будет создана политика')
    await dialog.getByRole('button', { name: 'Отмена' }).click()
    await expect(dialog).toBeHidden()
    await shot(page, 'live-margin-moved-1440')
  })

  test('F29 live: справочник связей и версии', async ({ page }) => {
    await page.goto('/searchops?tab=changes&view=relations')
    const table = page.getByRole('table', { name: 'Справочник связей' })
    await expect(table).toBeVisible()
    expect(await table.getByRole('row').count()).toBeGreaterThan(1)
    await expect(page.getByRole('heading', { name: 'Версии справочника' })).toBeVisible()
    await shot(page, 'live-relations-1440')
    await table.getByRole('row').nth(1).getByRole('button').click()
    await expect(page.getByRole('dialog')).toContainText('История версий этой связи')
  })

  test('F25 live: коммерческая лестница', async ({ page }) => {
    await page.goto('/proofs?tab=compare&view=commerce')
    const table = page.getByRole('table', { name: 'Коммерческая лестница конфигураций' })
    await expect(table).toBeVisible({ timeout: 30_000 })
    await expect(table.getByRole('rowheader', { name: /C0/ })).toBeVisible()
    await expect(page.getByText(/Воспроизводимость/)).toBeVisible()
    await shot(page, 'live-commerce-1440')
  })

  test('F26 live: очередь SearchOps показывает предложения связей, если они есть', async ({
    page,
  }) => {
    await page.goto('/searchops?tab=queue&status=all')
    await expect(page.getByRole('tab', { name: /Очередь/ })).toHaveAttribute('data-state', 'active')
    await page
      .getByRole('article')
      .first()
      .waitFor({ timeout: 15_000 })
      .catch(() => undefined)
    const card = page
      .getByRole('article')
      .filter({ hasText: /Сломанная связь|Нет обязательного дополнения/ })
      .first()
    if (await card.count()) {
      await expect(card).toContainText('Техническая корректность')
      await expect(card).toContainText('Эффект:')
      await card.scrollIntoViewIfNeeded()
      await shot(page, 'live-searchops-relation-1440')
    }
  })

  test('360 px live: без горизонтальной прокрутки', async ({ page }) => {
    test.setTimeout(120_000)
    await page.setViewportSize({ width: 360, height: 780 })
    for (const [url, name] of [
      ['/searchops?tab=policy', 'live-policy-360'],
      ['/searchops?tab=changes&view=relations', 'live-relations-360'],
      ['/proofs?tab=compare&view=commerce', 'live-commerce-360'],
    ]) {
      await page.goto(url)
      await expect(page.locator('main h1')).toBeVisible()
      await page.waitForLoadState('networkidle')
      const overflow = await page.evaluate(
        () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
      )
      expect(overflow, `${url}: горизонтальная прокрутка`).toBeLessThanOrEqual(0)
      await shot(page, name)
    }
  })
})
