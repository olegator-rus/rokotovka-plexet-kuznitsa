// Витрина контура коммерции на живом API (P2_LIVE=1): F16, F17, F19, F21, F24 на данных mock-2 и живые скриншоты
// artifacts/screens/p2/live-*.png. Ожидания — из ответа API (что пришло, то и должно быть на экране), без
// подписей фикстур. Запрос КП не отправляется: демо-база не меняется.
import { mkdirSync } from 'node:fs'
import type { Page } from '@playwright/test'
import { expect, openSearch, resultCards, search, test, tracePanel, understanding } from './fixtures'
import { MEM_QUERY, P2_LIVE, WS_QUERY } from './p2-fixtures'

const SCREENS = '../artifacts/screens/p2'
mkdirSync(SCREENS, { recursive: true })

/** F19: парт-номер в нижнем регистре и с пробелом вместо дефиса (в каталоге — P20249-B21). */
const MPN_QUERY = 'p20249 b21'

type Companion = {
  product_id: string
  family_id: string
  title: string
  reason?: string
  suggested_quantity?: number | null
}
type LiveItem = {
  product_id: string
  family_id: string
  title: string
  group: string
  required?: Companion[]
  compatibility?: { state: string } | null
}
type LiveBody = {
  items: LiveItem[]
  chips: { field: string; label: string }[]
  companions: { anchor: { product_id: string }; items: Companion[] } | null
}

const specButton = (page: Page) => page.getByRole('button', { name: /^Спецификация, позиций/ })
const specSheet = (page: Page) => page.getByRole('dialog', { name: 'Спецификация' })
const block = (page: Page) => page.getByRole('region', { name: /^Дополните решение к/ })
const overflow = (page: Page) =>
  page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
const esc = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')

async function liveSearch(page: Page, q: string) {
  return (await search(page, q)) as unknown as LiveBody
}

test.describe('Витрина контура коммерции — живой API', () => {
  test.skip(!P2_LIVE, 'только P2_LIVE=1: на фикстурах — p2-buyer.spec.ts')

  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      try {
        if (!sessionStorage.getItem('p2-live-init')) {
          localStorage.removeItem('plexet.spec')
          sessionStorage.setItem('p2-live-init', '1')
        }
      } catch {
        /* без хранилища */
      }
    })
  })

  test('F16: сервер первым, CAL 2025 × 50 в строке, CAL 2022 не выше сервера', async ({ page }) => {
    await openSearch(page)
    const body = await liveSearch(page, WS_QUERY)

    // Сервер — точный товар первым; количество из запроса ушло в CAL, а не в отказ сервера.
    const top = body.items[0]!
    expect(top.family_id).toBe('windows-server-2025')
    expect(top.group).toBe('exact')
    const req = top.required?.[0]
    expect(req?.family_id).toBe('windows-server-cal-2025')
    expect(req?.suggested_quantity).toBe(50)
    const cal22 = body.items.findIndex((i) => i.family_id === 'windows-server-cal')
    if (cal22 >= 0) expect(body.items[cal22]!.group).not.toBe('exact')
    expect(body.chips.some((c) => c.label === '50 пользователей')).toBe(true)

    const first = resultCards(page).first()
    await expect(first).toContainText('Windows Server 2025 Standard')
    const line = first.getByRole('button', {
      name: new RegExp(`Проверьте необходимость: ${esc(req!.title)} × 50`),
    })
    await expect(line).toBeVisible()
    await page.screenshot({ path: `${SCREENS}/live-f16-1440.png` })

    await line.click()
    if (req?.reason) await expect(first.getByText(req.reason)).toBeVisible()
    await expect(first.getByRole('link', { name: /Основание/ })).toHaveAttribute('href', /^https:/)
    await expect(first.getByRole('spinbutton', { name: `Количество: ${req!.title}` })).toHaveValue('50')
    await expect(first.getByText(/Windows Server 2022 CAL/)).toHaveCount(0)
    await page.screenshot({ path: `${SCREENS}/live-f16-open-1440.png` })

    // «Уже есть» сворачивает строку; вернуть — и строка снова предлагает CAL.
    await first.getByRole('switch', { name: 'Уже есть подходящие лицензии' }).click()
    await expect(first.getByRole('button', { name: 'Не требуется, по вашим данным' })).toBeVisible()
    await first.getByRole('button', { name: 'Не требуется, по вашим данным' }).click()
    await first.getByRole('switch', { name: 'Уже есть подходящие лицензии' }).click()
    await expect(first.getByRole('spinbutton', { name: `Количество: ${req!.title}` })).toHaveValue('50')

    // 360 px: та же выдача без горизонтальной прокрутки.
    await page.setViewportSize({ width: 360, height: 780 })
    expect(await overflow(page)).toBeLessThanOrEqual(0)
    await page.screenshot({ path: `${SCREENS}/live-f16-360.png`, fullPage: true })
  })

  test('F17: «Дополните решение» (если есть связи) и спецификация: сервер + CAL × 50', async ({
    page,
  }) => {
    await openSearch(page)
    const body = await liveSearch(page, WS_QUERY)
    const first = resultCards(page).first()

    if (body.companions && body.companions.items.length > 0) {
      const b = block(page)
      await expect(b).toBeVisible()
      await expect(b.getByRole('group')).toHaveCount(body.companions.items.length)
      await expect(b.getByText('В спецификации')).toHaveCount(0) // ничего не выбрано заранее
      await expect(page.getByText(/Рекомендуем/)).toHaveCount(0)
    } else {
      // Связей-дополнений у якоря нет — блок скрыт целиком, без добора популярными (Правила блока).
      await expect(block(page)).toHaveCount(0)
    }

    await first.getByRole('button', { name: /^В спецификацию:/ }).click()
    await first.getByRole('button', { name: /Проверьте необходимость/ }).click()
    await first.getByRole('button', { name: 'Добавить в спецификацию' }).first().click()
    await expect(specButton(page)).toHaveAccessibleName('Спецификация, позиций: 2')
    await page.screenshot({ path: `${SCREENS}/live-f17-1440.png`, fullPage: true })

    await specButton(page).click()
    const sheet = specSheet(page)
    await expect(sheet).toBeVisible()
    const rows = sheet.getByRole('list', { name: 'Позиции спецификации' }).getByRole('listitem')
    await expect(rows).toHaveCount(2)
    await expect(rows.nth(0)).toContainText('из поиска')
    await expect(rows.nth(1)).toContainText('условно обязательное')
    // Серверная лицензия на ядра: «50 пользователей» не переносится в её количество; CAL — 50.
    await expect(rows.nth(0).getByRole('spinbutton')).toHaveValue('1')
    await expect(rows.nth(1).getByRole('spinbutton')).toHaveValue('50')
    await page.waitForTimeout(400)
    await page.screenshot({ path: `${SCREENS}/live-f17-spec-1440.png` })

    await page.setViewportSize({ width: 360, height: 780 })
    expect(await overflow(page)).toBeLessThanOrEqual(0)
    await page.screenshot({ path: `${SCREENS}/live-f17-spec-360.png` })
  })

  test('F21: память для Dell R740 — чип совместимости, только подтверждённые и условные', async ({
    page,
  }) => {
    await openSearch(page)
    const body = await liveSearch(page, MEM_QUERY)
    expect(body.items.length).toBeGreaterThan(0)
    for (const it of body.items) expect(['verified', 'conditional']).toContain(it.compatibility?.state)

    await expect(understanding(page).getByText('Память 32 ГБ')).toBeVisible()
    await expect(understanding(page).getByText('Совместимо с Dell PowerEdge R740')).toBeVisible()
    const cards = resultCards(page)
    await expect(cards.first()).toContainText(/Подтверждено для Dell PowerEdge R740|При условии/)
    if (body.items.some((i) => i.compatibility?.state === 'conditional')) {
      await expect(page.getByText('Для точной проверки укажите конфигурацию сервера')).toBeVisible()
    }
    await page.screenshot({ path: `${SCREENS}/live-f21-1440.png`, fullPage: true })

    await page.setViewportSize({ width: 360, height: 780 })
    expect(await overflow(page)).toBeLessThanOrEqual(0)
    await page.screenshot({ path: `${SCREENS}/live-f21-360.png`, fullPage: true })
  })

  test('F19: парт-номер с другими разделителями — чип «Парт-номер», точный товар первым', async ({
    page,
  }) => {
    await openSearch(page)
    const body = await liveSearch(page, MPN_QUERY)
    expect(body.items[0]?.product_id).toBe('srv-dl380g10-5218')
    await expect(understanding(page).getByText('Парт-номер P20249-B21')).toBeVisible()
    await expect(resultCards(page).first()).toContainText('HPE ProLiant DL380 Gen10')
    await expect(page.getByText(/Исправлено на/)).toHaveCount(0)
    if (body.companions) await expect(block(page)).toBeVisible()
    await page.screenshot({ path: `${SCREENS}/live-f19-1440.png`, fullPage: true })

    await page.setViewportSize({ width: 360, height: 780 })
    expect(await overflow(page)).toBeLessThanOrEqual(0)
    await page.screenshot({ path: `${SCREENS}/live-f19-360.png`, fullPage: true })
  })

  test('F24: трасса — «Компаньоны» с якорем-сервером и отклонённой CAL 2022', async ({ page }) => {
    await openSearch(page)
    await liveSearch(page, WS_QUERY)
    await page.getByRole('switch', { name: 'Трасса' }).click()
    const panel = tracePanel(page)
    const step = panel.getByRole('button', { name: /Компаньоны/ })
    await expect(step).toContainText('якорь найден')
    await step.click()
    await expect(panel.getByText(/Якорь: Microsoft Windows Server 2025 Standard/)).toBeVisible()
    await expect(
      panel.getByRole('list', { name: 'Отклонённые кандидаты' }).getByRole('listitem').first(),
    ).toBeVisible()
    await expect(panel.getByRole('list', { name: 'Отклонённые кандидаты' })).toContainText(
      'по версии',
    )
  })
})
