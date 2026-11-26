// Панель поиска контура выдачи. Ответы /api/suggest и
// /api/suggest/recommendations — фикстуры по контракту (p3-panel-data.ts); полный поиск по Enter — живой API.
// Скриншоты 1440/360 — в artifacts/screens/p3/.
import type { Page, Request } from '@playwright/test'
import { expect, openSearch, searchBox, test } from './fixtures'
import {
  BROAD_PRODUCTS,
  EXACT_MPN,
  POPULAR,
  RECS_CATEGORY,
  RECS_EXACT,
  broad,
  exact,
  layout,
  tagged,
} from './p3-panel-data'

const SCREENS = '../artifacts/screens/p3'

interface MockOptions {
  products?: boolean
  delays?: Record<string, number>
  recsFail?: boolean
}

function pick(q: string, opts: MockOptions) {
  if (q === '') return POPULAR
  if (q.toUpperCase().startsWith('SRV')) return exact(q)
  if (q.startsWith('rfcg')) return layout(q)
  if ('антивирус'.startsWith(q) || q.startsWith('антивирус')) return broad(q, opts)
  return tagged(q)
}

/** Подменить /api/suggest и рекомендации фикстурами; возвращает список запрошенных префиксов. */
async function mockPanel(page: Page, opts: MockOptions = {}) {
  const asked: string[] = []
  await page.route(/\/api\/suggest\/recommendations/, async (route) => {
    const u = new URL(route.request().url())
    if (opts.recsFail)
      return route.fulfill({
        status: 500,
        json: { error: { code: 'X', message: 'сбой' } },
      })
    let body: unknown = null
    if (u.searchParams.get('exact')) body = RECS_EXACT
    else if (u.searchParams.get('category')) body = RECS_CATEGORY
    return route.fulfill({ json: body })
  })
  await page.route(/\/api\/suggest\?/, async (route) => {
    const q = new URL(route.request().url()).searchParams.get('q') ?? ''
    asked.push(q)
    const delay = opts.delays?.[q] ?? 0
    if (delay) await new Promise((r) => setTimeout(r, delay))
    try {
      await route.fulfill({ json: pick(q, opts) })
    } catch {
      // Браузер уже отменил запрос (AbortController) — так и задумано.
    }
  })
  return asked
}

const listbox = (page: Page) => page.getByRole('listbox', { name: 'Подсказки поиска' })
const isSearchReq = (r: Request) => r.url().endsWith('/api/search') && r.method() === 'POST'

async function typeQuery(page: Page, q: string, delay = 40) {
  const box = searchBox(page)
  await box.click()
  await box.pressSequentially(q, { delay })
}

async function shot(page: Page, name: string) {
  await page.waitForTimeout(250)
  await page.screenshot({ path: `${SCREENS}/${name}.png`, fullPage: false })
}

test.describe('Панель поиска (контур выдачи)', () => {
  test('F38: печатаю → стрелками до товара → Enter, без мыши', async ({ page }) => {
    await mockPanel(page)
    await openSearch(page)
    await searchBox(page).focus()
    await page.keyboard.type('антивирус', { delay: 40 })
    const list = listbox(page)
    await expect(
      list.getByRole('option', {
        name: /^Kaspersky Endpoint Security для бизнеса Лаборатория/,
      }),
    ).toBeVisible()
    // Первый элемент не выделен автоматически.
    await expect(searchBox(page)).not.toHaveAttribute('aria-activedescendant', /.+/)

    let reached = false
    for (let i = 0; i < 20 && !reached; i++) {
      await page.keyboard.press('ArrowDown')
      const id = (await searchBox(page).getAttribute('aria-activedescendant')) ?? ''
      reached = id.includes('-product-')
    }
    expect(reached, 'стрелки дошли до товаров').toBe(true)
    // Обход линейный: левая колонка (5 подсказок + 2 категории) — до товаров.
    await expect(searchBox(page)).toHaveAttribute('aria-activedescendant', 'panel-opt-product-0')

    const [req] = await Promise.all([
      page.waitForRequest(isSearchReq),
      page.keyboard.press('Enter'),
    ])
    const body = req.postDataJSON() as { query: string; intent_id?: string }
    expect(body.query).toBe(BROAD_PRODUCTS[0]!.title)
    expect(body.intent_id).toBeTruthy()
    await expect(list).toBeHidden()
  })

  test('F38: печатаю → Enter ведёт в выдачу по набранному; Esc закрывает и сохраняет текст', async ({
    page,
  }) => {
    await mockPanel(page)
    await openSearch(page)
    await searchBox(page).focus()
    await page.keyboard.type('антивирус', { delay: 40 })
    await expect(listbox(page)).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(listbox(page)).toBeHidden()
    await expect(searchBox(page)).toHaveValue('антивирус')

    await page.keyboard.press('ArrowDown')
    await expect(listbox(page)).toBeVisible()
    await page.keyboard.press('ArrowUp')
    await page.keyboard.press('ArrowDown')
    // Выход за край возвращает фокус в строку: выделения нет — Enter ищет набранное.
    await expect(searchBox(page)).not.toHaveAttribute('aria-activedescendant', /.+/)
    const [req] = await Promise.all([
      page.waitForRequest(isSearchReq),
      page.keyboard.press('Enter'),
    ])
    expect((req.postDataJSON() as { query: string }).query).toBe('антивирус')
    await expect(page).toHaveURL(/q=%D0%B0%D0%BD%D1%82%D0%B8%D0%B2%D0%B8%D1%80%D1%83%D1%81/)
  })

  test('быстрый набор: ответ на устаревший префикс не показывается', async ({ page }) => {
    // «ze» и «zet» отвечают медленно, «zeta» — быстро: без отмены и номера ответа старое легло бы поверх.
    const asked = await mockPanel(page, {
      delays: { ze: 900, zet: 600, zeta: 30 },
    })
    await openSearch(page)
    await typeQuery(page, 'zeta', 200)
    const list = listbox(page)
    await expect(list.getByRole('option', { name: 'zeta ответ' })).toBeVisible()
    await page.waitForTimeout(1200)
    await expect(list.getByRole('option', { name: 'zeta ответ' })).toBeVisible()
    await expect(list.getByRole('option', { name: /^zet? ответ$/ })).toHaveCount(0)
    await expect(list.getByText('Товар для «zeta»')).toBeVisible()
    await expect(list.getByText(/Товар для «zet?»/)).toHaveCount(0)
    expect(asked).toEqual(expect.arrayContaining(['ze', 'zet', 'zeta']))
  })

  test('товарная секция выключена или упала — подсказки и Enter работают', async ({ page }) => {
    await mockPanel(page, { products: false, recsFail: true })
    await openSearch(page)
    await typeQuery(page, 'антивирус')
    const list = listbox(page)
    await expect(list.getByText('Подсказки', { exact: true })).toBeVisible()
    await expect(list.getByText('Товары', { exact: true })).toHaveCount(0)
    await expect(list.getByText(/^Популярное в категории/)).toHaveCount(0)
    await expect(list.getByRole('option', { name: 'Все результаты (37)' })).toBeVisible()
    const [req] = await Promise.all([
      page.waitForRequest(isSearchReq),
      searchBox(page).press('Enter'),
    ])
    expect((req.postDataJSON() as { query: string }).query).toBe('антивирус')
  })

  test('F34: бейдж с атрибутом — условие без правки строки; категория — фильтр', async ({
    page,
  }) => {
    const asked = await mockPanel(page)
    await openSearch(page)
    await typeQuery(page, 'антивирус')
    const badges = page.getByRole('group', { name: 'Уточнения запроса' })
    await badges.getByRole('button', { name: 'Уточнить: 1 год' }).click()
    await expect(badges.getByRole('button', { name: 'Снять уточнение: 1 год' })).toBeVisible()
    await expect(searchBox(page)).toHaveValue('антивирус')
    await expect.poll(() => asked.includes('антивирус 1 год')).toBe(true)
    await shot(page, 'panel-refinement-1440')

    const [req] = await Promise.all([
      page.waitForRequest(isSearchReq),
      searchBox(page).press('Enter'),
    ])
    const body = req.postDataJSON() as { query: string; filters?: unknown[] }
    expect(body.query).toBe('антивирус')
    expect(body.filters).toEqual([{ key: 'duration_months', op: 'eq', value: 12 }])

    await searchBox(page).click()
    await searchBox(page).fill('')
    await typeQuery(page, 'антивирус')
    const [req2] = await Promise.all([
      page.waitForRequest(isSearchReq),
      listbox(page).getByRole('option', { name: 'Защита конечных точек' }).click(),
    ])
    expect((req2.postDataJSON() as { filters?: unknown[] }).filters).toEqual([
      { key: 'category', op: 'eq', value: 'endpoint' },
    ])
  })

  test('F35: раскладка — строка не переписывается, «Показаны варианты для …»', async ({ page }) => {
    await mockPanel(page)
    await openSearch(page)
    await typeQuery(page, 'rfcgth')
    await expect(listbox(page).getByText('Показаны варианты для «каспер»')).toBeVisible()
    await expect(searchBox(page)).toHaveValue('rfcgth')
    await shot(page, 'panel-layout-1440')
  })

  test('F36 + F37: точный артикул — одна карточка, «В спецификацию», блок «Дополните …»', async ({
    page,
  }) => {
    await mockPanel(page)
    await openSearch(page)
    await typeQuery(page, EXACT_MPN, 20)
    const list = listbox(page)
    await expect(
      list.getByRole('option', {
        name: `В спецификацию: ${exact(EXACT_MPN).products[0]!.title}`,
      }),
    ).toBeVisible()
    await expect(list.locator('[id^="panel-opt-product-"]')).toHaveCount(1)
    await expect(list.getByText(RECS_EXACT.title)).toBeVisible()
    await expect(list.getByRole('option', { name: /^Память Dell 32 ГБ DDR4 RDIMM/ })).toBeVisible()
    await expect(page.getByRole('group', { name: 'Уточнения запроса' })).toHaveCount(0)
    await shot(page, 'panel-exact-1440')
    await list.getByRole('option', { name: /^В спецификацию:/ }).click()
    await expect(page.getByText('Добавлено в спецификацию').first()).toBeVisible()
  })

  test('F32: пустая строка — недавние с удалением и «Очистить», популярные', async ({ page }) => {
    await mockPanel(page)
    await openSearch(page)
    await typeQuery(page, 'антивирус')
    await Promise.all([page.waitForRequest(isSearchReq), searchBox(page).press('Enter')])
    await page.getByRole('button', { name: 'Очистить строку' }).click()
    const list = listbox(page)
    await expect(list.getByText('Недавние запросы')).toBeVisible()
    await expect(list.getByRole('option', { name: 'антивирус' })).toBeVisible()
    await expect(list.getByText('Популярные запросы')).toBeVisible()
    await expect(list.getByRole('option', { name: 'windows 11 pro' })).toBeVisible()
    await shot(page, 'panel-empty-1440')
    await list.getByRole('button', { name: 'Удалить из недавних: антивирус' }).click()
    await expect(list.getByText('Недавние запросы')).toHaveCount(0)
  })

  test('широкий запрос на 1440: две колонки, категории, 6 семейств, популярное в категории', async ({
    page,
  }) => {
    await mockPanel(page)
    await openSearch(page)
    await typeQuery(page, 'антивирус')
    const list = listbox(page)
    await expect(list.locator('[id^="panel-opt-product-"]')).toHaveCount(6)
    await expect(list.getByText(RECS_CATEGORY.title)).toBeVisible()
    await expect(list.getByRole('link', { name: /Почему показано/ })).toHaveAttribute(
      'href',
      '/rules',
    )
    await shot(page, 'panel-broad-1440')
  })
})

test.describe('Панель поиска на 360 px', () => {
  test.use({ viewport: { width: 360, height: 780 } })

  test('одна колонка, без горизонтальной прокрутки; скриншоты', async ({ page }) => {
    await mockPanel(page)
    await openSearch(page)
    await typeQuery(page, 'антивирус')
    const list = listbox(page)
    await expect(list.locator('[id^="panel-opt-product-"]')).toHaveCount(3)
    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth - window.innerWidth,
    )
    expect(overflow).toBeLessThanOrEqual(0)
    await shot(page, 'panel-broad-360')

    await searchBox(page).fill('')
    await typeQuery(page, EXACT_MPN, 20)
    await expect(list.getByText(RECS_EXACT.title)).toBeVisible()
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth),
    ).toBeLessThanOrEqual(0)
    await shot(page, 'panel-exact-360')

    await page.getByRole('button', { name: 'Очистить строку' }).click()
    await expect(list.getByText('Популярные запросы')).toBeVisible()
    await shot(page, 'panel-empty-360')

    await typeQuery(page, 'rfcgth')
    await expect(list.getByText('Показаны варианты для «каспер»')).toBeVisible()
    await shot(page, 'panel-layout-360')
  })
})
