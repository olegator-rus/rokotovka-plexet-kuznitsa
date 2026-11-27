// Панель контура выдачи и страница результатов на ЖИВОМ API (без подмен): то, что отдаёт /api/suggest и POST /api/search
// на фиде store.softline.ru (контур фида). Скриншоты 1440/360 — artifacts/screens/p3/live-*.png. Демо-состояние не меняется
// (спецификация — в localStorage браузера теста).
import type { Page, Request } from '@playwright/test'
import { expect, openSearch, searchBox, test } from './fixtures'

const SCREENS = '../artifacts/screens/p3'
// Точный артикул позиции фида (Kaspersky Endpoint Security для бизнеса Стандартный, 413710): одна карточка.
const EXACT_MPN = 'KL4863RATFS'

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
      const [resp] = await Promise.all([
        page.waitForResponse((r) => r.url().includes('/api/suggest?') && r.url().includes('q=kasp')),
        typeQuery(page, 'kasp'),
      ])
      const sug = (await resp.json()) as {
        suggestions: { text: string }[]
        refinements: { label: string; query: string }[]
        vendors: { name: string }[]
        products: { family_name: string; vendor: string }[]
      }
      const list = listbox(page)
      // Фид: подсказки — запросы из журнала и словаря и семейства с ценой (коммит 1e8daea), производитель — своей
      // секцией, бейджи — уточнения сервера (вид, срок, количество). Проверяется суть, а не конкретная строка: всё
      // про Kaspersky, бейджи не дублируют подсказки.
      expect(sug.suggestions.length).toBeGreaterThan(0)
      for (const s of sug.suggestions) expect(s.text.toLowerCase()).toMatch(/^kasp/)
      await expect(list.getByRole('option', { name: sug.suggestions[0]!.text, exact: true })).toBeVisible()
      expect(sug.vendors.map((v) => v.name)).toContain('Лаборатория Касперского')
      await expect(list.getByRole('option', { name: 'Лаборатория Касперского', exact: true })).toBeVisible()
      // Семейства в панели — Kaspersky.
      expect(sug.products.length).toBeGreaterThan(0)
      for (const p of sug.products) expect(p.family_name).toMatch(/kaspersky/i)
      const families = list.locator('[id^="panel-opt-product-"]')
      await expect(families.first()).toBeVisible()
      for (const t of await families.allTextContents()) expect(t).toMatch(/Kaspersky/)

      expect(sug.refinements.length).toBeGreaterThan(0)
      const firstRef = sug.refinements[0]!.label
      await expect(badges(page).getByRole('button', { name: `Уточнить: ${firstRef}` })).toBeVisible()
      const labels = await badgeLabels(page)
      expect(labels).toEqual(expect.arrayContaining([firstRef]))
      expect(new Set(labels).size, 'бейджи без повторов').toBe(labels.length)
      // Бейдж не повторяет подсказку: запрос уточнения («<подсказка> <бейдж>») не стоит строкой в подсказках.
      const suggestions = (await list.locator('[id^="panel-opt-suggestion-"]').allTextContents()).map((t) =>
        t.trim().toLowerCase(),
      )
      for (const r of sug.refinements) {
        if (!labels.includes(r.label)) continue
        expect(suggestions.includes(r.query.toLowerCase()), `бейдж «${r.label}» дублирует подсказку`).toBe(false)
        expect(suggestions.includes(r.label.toLowerCase()), `бейдж «${r.label}» дублирует подсказку`).toBe(false)
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

    test('точный артикул → одна карточка, без бейджей, «В спецификацию» прямо из подсказки', async ({
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
      await expect(page.getByRole('button', { name: /^Спецификация, позиций: 1/ })).toBeVisible()
      // «К вашей спецификации» строится по справочнику связей; на фиде его нет — блок скрыт.
      await searchBox(page).fill('')
      await searchBox(page).click()
      await expect(listbox(page).getByText('К вашей спецификации')).toHaveCount(0)
      await noOverflow(page)
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
