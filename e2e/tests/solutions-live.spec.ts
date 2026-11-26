// на живом API (P2_LIVE=1): закупка под задачу, старшая редакция, «К этой позиции» в
// спецификации и шаги трассы. Ожидания — из ответа API: что сервер подобрал, то и должно быть на экране.
// Скриншоты — artifacts/screens/p2/live-k2-*.png. Запрос КП не отправляется.
import { mkdirSync } from 'node:fs'
import type { Page } from '@playwright/test'
import { expect, openSearch, resultCards, search, test, tracePanel, understanding } from './fixtures'
import { P2_LIVE } from './p2-fixtures'

const SCREENS = '../artifacts/screens/p2'
mkdirSync(SCREENS, { recursive: true })

type Lite = { product_id: string; title: string }
type Slot = {
  id: string
  label: string
  quantity: number
  item: Lite | null
  alternatives: Lite[]
  quantities: Record<string, number>
  active: boolean
}
type Body = {
  items: { product_id: string; title: string; group: string }[]
  chips: { field: string; label: string }[]
  solution: { template_id: string; title: string; seats: number; slots: Slot[] } | null
  upgrade: { for_product_id: string; product_id: string; title: string; upgrade_adds: string[] } | null
}

const card = (page: Page) => page.getByRole('region', { name: /^Рабочее место сотрудника|^Удалённый офис|^Сервер под 1С/ })
const slots = (page: Page) => page.getByRole('list', { name: 'Слоты решения' }).getByRole('listitem')
const specButton = (page: Page) => page.getByRole('button', { name: /^Спецификация, позиций/ })
const specSheet = (page: Page) => page.getByRole('dialog', { name: 'Спецификация' })
const overflow = (page: Page) =>
  page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)

test.describe('закупка под задачу и старшая редакция — живой API', () => {
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

  test('F18: «рабочее место для 20 сотрудников» — шаблон из трёх слотов на 20 мест над выдачей', async ({
    page,
  }) => {
    await openSearch(page)
    const body = (await search(page, 'рабочее место для 20 сотрудников')) as unknown as Body
    const sol = body.solution!
    expect(sol.template_id).toBe('tmpl-workplace')
    expect(sol.seats).toBe(20)
    const active = sol.slots.filter((s) => s.active)
    expect(active.map((s) => s.id)).toEqual(['laptop', 'office', 'security'])
    for (const s of active) expect(s.item).not.toBeNull()
    expect(body.chips.some((c) => c.label === 'Комплексная закупка')).toBe(true)

    await expect(understanding(page).getByText('Комплексная закупка')).toBeVisible()
    const c = card(page)
    await expect(c).toBeVisible()
    await expect(c).toContainText('На 20 мест')
    await expect(c).toContainText('Собрано по шаблону Softline')
    await expect(slots(page)).toHaveCount(3)
    for (const [i, s] of active.entries()) {
      const row = slots(page).nth(i)
      await expect(row).toContainText(s.label)
      await expect(row).toContainText(s.item!.title)
      await expect(row).toContainText(`× ${s.quantity}`)
    }
    // Обычная выдача под шаблоном — та же, что прислал сервер.
    await expect(resultCards(page)).toHaveCount(body.items.length)
    await page.screenshot({ path: `${SCREENS}/live-k2-f18-1440.png`, fullPage: true })

    // «Заменить» ноутбук: вторая позиция из альтернатив, количество — её собственное.
    const laptop = active[0]!
    const alt = laptop.alternatives[0]!
    await slots(page).nth(0).getByRole('button', { name: /Заменить/ }).click()
    await slots(page).nth(0).getByRole('radio', { name: new RegExp(alt.title.slice(0, 20)) }).check()
    await expect(slots(page).nth(0)).toContainText(`× ${laptop.quantities[alt.product_id]}`)
    await page.screenshot({ path: `${SCREENS}/live-k2-f18-replace-1440.png` })

    await c.getByRole('button', { name: 'Добавить всё в спецификацию' }).click()
    await expect(specButton(page)).toHaveAccessibleName('Спецификация, позиций: 3')
    await expect(c.getByText('В спецификации')).toBeVisible()
    await specButton(page).click()
    const rows = specSheet(page).getByRole('list', { name: 'Позиции спецификации' }).getByRole('listitem')
    await expect(rows).toHaveCount(3)
    await expect(rows.nth(0)).toContainText(alt.title)
    for (let i = 0; i < 3; i++) await expect(rows.nth(i)).toContainText('из шаблона')
    await expect(rows.nth(0).getByRole('spinbutton')).toHaveValue(String(laptop.quantities[alt.product_id]))
    await page.waitForTimeout(400)
    await page.screenshot({ path: `${SCREENS}/live-k2-f18-spec-1440.png` })
    await page.keyboard.press('Escape')

    await page.setViewportSize({ width: 360, height: 780 })
    expect(await overflow(page)).toBeLessThanOrEqual(0)
    await page.screenshot({ path: `${SCREENS}/live-k2-f18-360.png`, fullPage: true })
  })

  test('F18: запрос не под шаблон — обычная выдача, карточки нет; снятый чип убирает шаблон', async ({
    page,
  }) => {
    await openSearch(page)
    const body = (await search(page, 'антивирус')) as unknown as Body
    expect(body.solution).toBeNull()
    await expect(page.getByRole('list', { name: 'Слоты решения' })).toHaveCount(0)

    await search(page, 'удаленный офис на 5 человек')
    await expect(card(page)).toBeVisible()
    await understanding(page)
      .getByRole('button', { name: /Снять условие «Комплексная закупка»/ })
      .first()
      .click()
    await expect(page.getByRole('list', { name: 'Слоты решения' })).toHaveCount(0)
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

  test('Трасса: шаги «Решение по шаблону» и «Старшая редакция»', async ({ page }) => {
    await openSearch(page)
    await search(page, 'сервер под 1с на 50 пользователей')
    await page.getByRole('switch', { name: 'Трасса' }).click()
    const panel = tracePanel(page)
    await expect(panel.getByRole('button', { name: /Решение по шаблону/ })).toContainText(
      'tmpl-1c-server · 50 мест',
    )
    await expect(panel.getByRole('button', { name: /Старшая редакция/ })).toBeVisible()
    await page.screenshot({ path: `${SCREENS}/live-k2-trace-1440.png` })
  })
})
