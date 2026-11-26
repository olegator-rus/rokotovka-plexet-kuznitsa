// Панель контура выдачи и страница результатов на ЖИВОМ API (без подмен): то, что отдаёт /api/suggest и POST /api/search
// на демо-каталоге mock. Скриншоты 1440/360 — artifacts/screens/p3/live-*.png. Демо-состояние не меняется
// (спецификация — в localStorage браузера теста).
import type { Page, Request } from '@playwright/test'
import { expect, openSearch, searchBox, test } from './fixtures'

const SCREENS = '../artifacts/screens/p3'
// Точный парт-номер сервера Dell R740 из демо-каталога: к нему есть проверенные связи (память, диски, сервис).
const EXACT_MPN = 'PER740-2S4210R-64G'

const listbox = (page: Page) => page.getByRole('listbox', { name: 'Подсказки поиска' })
const badges = (page: Page) => page.getByRole('group', { name: 'Уточнения запроса' })
const isSearchReq = (r: Request) => r.url().endsWith('/api/search') && r.method() === 'POST'

async function typeQuery(page: Page, q: string, delay = 40) {
  const box = searchBox(page)
  await box.click()
  await box.pressSequentially(q, { delay })
}

async function noOverflow(page: Page) {
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
  expect(overflow, 'без горизонтальной прокрутки').toBeLessThanOrEqual(0)
}

async function shot(page: Page, name: string) {
  await page.waitForTimeout(250)
  await page.screenshot({ path: `${SCREENS}/${name}.png`, fullPage: false })
}

/** Подписи бейджей и тексты подсказок панели. */
async function badgeLabels(page: Page): Promise<string[]> {
  const names = await badges(page).getByRole('button').allTextContents()
  return names.map((n) => n.replace(/^\s*\+\s*/, '').trim())
}

for (const vp of [
  { width: 1440, height: 900, tag: '1440' },
  { width: 360, height: 780, tag: '360' },
]) {
  test.describe(`Живая панель и выдача контура выдачи · ${vp.tag}`, () => {
    test.use({ viewport: { width: vp.width, height: vp.height } })

    test('«kasp»: структурированные бейджи без дублей подсказок, семейства', async ({ page }) => {
      await openSearch(page)
      await typeQuery(page, 'kasp')
      const list = listbox(page)
      await expect(list.getByRole('option', { name: 'kaspersky', exact: true })).toBeVisible()
      await expect(badges(page).getByRole('button', { name: 'Уточнить: для дома' })).toBeVisible()
      const labels = await badgeLabels(page)
      expect(labels).toEqual(expect.arrayContaining(['для дома', 'продление']))
      const suggestions = (await list.getByRole('option').allTextContents()).map((t) => t.trim().toLowerCase())
      for (const l of labels) {
        expect(
          suggestions.some((s) => s === `kaspersky ${l.toLowerCase()}`),
          `бейдж «${l}» дублирует подсказку`,
        ).toBe(false)
      }
      await noOverflow(page)
      await shot(page, `live-kasp-${vp.tag}`)
    })

    test('«лфызукылн»: раскладка — строка как набрана, варианты для «kaspersky»', async ({ page }) => {
      await openSearch(page)
      await typeQuery(page, 'лфызукылн')
      await expect(listbox(page).getByText('Показаны варианты для «kaspersky»')).toBeVisible()
      await expect(searchBox(page)).toHaveValue('лфызукылн')
      await noOverflow(page)
      await shot(page, `live-layout-${vp.tag}`)
    })

    test('точный артикул → одна карточка, без бейджей; пустая строка — рекомендации от спецификации', async ({
      page,
    }) => {
      await openSearch(page)
      await typeQuery(page, EXACT_MPN, 20)
      const list = listbox(page)
      await expect(list.locator('[id^="panel-opt-product-"]')).toHaveCount(1)
      await expect(badges(page)).toHaveCount(0)
      await noOverflow(page)
      await shot(page, `live-exact-${vp.tag}`)

      await list.getByRole('option', { name: /^В спецификацию:/ }).click()
      await expect(page.getByText('Добавлено в спецификацию').first()).toBeVisible()
      await searchBox(page).fill('')
      await searchBox(page).click()
      await expect(listbox(page).getByText('К вашей спецификации')).toBeVisible()
      await noOverflow(page)
      await shot(page, `live-empty-recs-${vp.tag}`)
    })

    test('«антивирус» → страница результатов: заголовок со счётчиками, чипы-уточнения, фасеты', async ({
      page,
    }) => {
      await openSearch(page)
      await typeQuery(page, 'антивирус')
      await expect(listbox(page)).toBeVisible()
      const [req] = await Promise.all([
        page.waitForRequest(isSearchReq),
        searchBox(page).press('Enter'),
      ])
      expect((req.postDataJSON() as { query: string }).query).toBe('антивирус')
      await expect(page.getByRole('status').filter({ hasText: /\d+ продукт/ }).first()).toBeVisible()
      await page.keyboard.press('Escape')
      await noOverflow(page)
      await shot(page, `live-results-${vp.tag}`)
    })
  })
}
