// на живом API (P2_LIVE=1): старшая редакция, «К этой позиции» в спецификации и шаг трассы.
// Ожидания — из ответа API: что сервер подобрал, то и должно быть на экране.
// Скриншоты — artifacts/screens/p2/live-k2-*.png. Запрос КП не отправляется.
import { mkdirSync } from 'node:fs'
import type { Page } from '@playwright/test'
import { expect, openSearch, resultCards, search, test, tracePanel } from './fixtures'
import { P2_LIVE } from './p2-fixtures'

const SCREENS = '../artifacts/screens/p2'
mkdirSync(SCREENS, { recursive: true })

type Lite = { product_id: string; title: string }
type Body = {
  items: { product_id: string; title: string; group: string }[]
  upgrade: { for_product_id: string; product_id: string; title: string; upgrade_adds: string[] } | null
}

const specButton = (page: Page) => page.getByRole('button', { name: /^Спецификация, позиций/ })
const specSheet = (page: Page) => page.getByRole('dialog', { name: 'Спецификация' })
const overflow = (page: Page) =>
  page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)

test.describe('старшая редакция — живой API', () => {
  test.skip(!P2_LIVE, 'только P2_LIVE=1')

  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      try {
        if (!sessionStorage.getItem('k2-init')) {
          localStorage.removeItem('plexet.spec')
          sessionStorage.setItem('k2-init', '1')
        }
      } catch {
        /* без хранилища */
      }
    })
  })

  test('F22: старшая редакция рядом с выдачей, запрошенная позиция первой', async ({ page }) => {
    await openSearch(page)
    const body = (await search(page, 'windows server 2025 standard')) as unknown as Body
    const up = body.upgrade!
    expect(up.for_product_id).toBe(body.items[0]!.product_id)
    expect(up.product_id).not.toBe(body.items[0]!.product_id)
    expect(up.upgrade_adds.length).toBeGreaterThan(0)
    expect(up.upgrade_adds.length).toBeLessThanOrEqual(2)

    await expect(resultCards(page).first()).toContainText(body.items[0]!.title)
    const region = page.getByRole('region', { name: 'Старшая редакция' })
    await expect(region).toBeVisible()
    await expect(region).toContainText(up.title)
    for (const a of up.upgrade_adds) await expect(region).toContainText(a)
    // Не в блоке дополнений.
    await expect(page.getByRole('region', { name: /^Дополните решение/ }).getByText(up.title)).toHaveCount(0)
    await region.scrollIntoViewIfNeeded()
    await page.screenshot({ path: `${SCREENS}/live-k2-f22-1440.png` })

    await page.setViewportSize({ width: 360, height: 780 })
    expect(await overflow(page)).toBeLessThanOrEqual(0)
    await region.scrollIntoViewIfNeeded()
    await page.screenshot({ path: `${SCREENS}/live-k2-f22-360.png` })
  })

  test('F17: «К этой позиции» в спецификации после добавления из выдачи', async ({ page }) => {
    await openSearch(page)
    await search(page, 'windows server 2025 standard')
    const first = resultCards(page).first()
    const companions = page.waitForResponse((r) => r.url().includes('/api/companions?product_id='))
    await first.getByRole('button', { name: /^В спецификацию:/ }).click()
    await specButton(page).click()
    const resp = await companions
    expect(resp.status()).toBe(200)
    const data = (await resp.json()) as { required: Lite[]; items: Lite[] }
    const sheet = specSheet(page)
    const offered = [...data.required, ...data.items]
    if (offered.length) {
      await expect(sheet.getByText('К этой позиции:')).toBeVisible()
      await sheet.getByRole('button', { name: new RegExp(`^В спецификацию: ${offered[0]!.title.slice(0, 20)}`) }).click()
      await expect(
        sheet.getByRole('list', { name: 'Позиции спецификации' }).getByRole('listitem'),
      ).toHaveCount(2)
    } else {
      await expect(sheet.getByText('К этой позиции:')).toHaveCount(0)
    }
    await page.waitForTimeout(400)
    await page.screenshot({ path: `${SCREENS}/live-k2-f17-spec-1440.png` })
  })

  test('Трасса: шаг «Старшая редакция», шага решения по шаблону нет', async ({ page }) => {
    await openSearch(page)
    const body = (await search(page, 'windows server 2025 standard')) as unknown as Body & Record<string, unknown>
    expect('solution' in body).toBe(false)
    await page.getByRole('switch', { name: 'Трассировка' }).click()
    const panel = tracePanel(page)
    await expect(panel.getByRole('button', { name: /Старшая редакция/ })).toBeVisible()
    await expect(panel.getByRole('button', { name: /Решение по шаблону/ })).toHaveCount(0)
    await page.screenshot({ path: `${SCREENS}/live-k2-trace-1440.png` })
  })
})
