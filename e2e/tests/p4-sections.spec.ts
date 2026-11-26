// Контур доводки («Переделанные экраны»): три раздела — «Поиск», «SearchOps», «Доказательства»;
// SearchOps — шесть вкладок (контур автономии вернул «Память»), «Запросы без ответа» с фильтрами по причине и владельцу и кнопкой «Разобрать»;
// «Доказательства» — три вкладки, «Вклад компонентов» пока пуст. Старые адреса ведут на новые места.
// Демо-состояние не меняется. E2E_SCREENS=1 — снимки разделов 1440/360 в artifacts/screens/p4/.
import type { Page } from '@playwright/test'
import { MAIN_RUN_ID, expect, test } from './fixtures'

const OUT = '../artifacts/screens/p4'
const SHOTS = process.env.E2E_SCREENS === '1'

async function noOverflow(page: Page, name: string) {
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
  )
  expect(overflow, `${name}: горизонтальная прокрутка`).toBeLessThanOrEqual(0)
}

test.describe('Контур доводки: три раздела', () => {
  test('сотрудник: в навигации ровно три раздела; SearchOps — шесть вкладок по порядку (контур автономии вернул «Память»)', async ({
    page,
  }) => {
    await page.goto('/searchops')
    const nav = page.getByRole('navigation', { name: 'Разделы' })
    await expect(nav.getByRole('link')).toHaveText(['Поиск', /^SearchOps/, 'Доказательства'])
    await expect(page.getByRole('tablist', { name: 'Разделы SearchOps' }).getByRole('tab')).toHaveText([
      'Запросы без ответа',
      'Запуски',
      /Очередь/,
      'Память',
      'Изменения',
      'Политика ранжирования',
    ])
    await expect(page.getByRole('tab', { name: 'Запросы без ответа' })).toHaveAttribute(
      'data-state',
      'active',
    )
    // Удалённого нет: «Аналитика», «Экономика», «Маржа», «Эффект».
    for (const name of ['Аналитика', 'Экономика', 'Маржа', 'Изменения', 'До / После'])
      await expect(nav.getByRole('link', { name })).toHaveCount(0)
  })

  test('«Запросы без ответа»: колонки, фильтры по причине и владельцу, «Разобрать» — запуск на одном запросе', async ({
    page,
  }) => {
    await page.goto('/searchops?tab=failed')
    const table = page.getByRole('region', { name: 'Таблица запросов без ответа' })
    const empty = page.getByText('Неудачных запросов за период нет.')
    await expect(table.or(empty).first()).toBeVisible({ timeout: 20_000 })
    test.skip(await empty.isVisible(), 'в демо-базе нет неудачных запросов за период')
    for (const h of ['Запрос', 'Частота', 'Исход', 'Причина', 'Владелец', 'Статус'])
      await expect(table.getByRole('columnheader', { name: h })).toBeVisible()
    await expect(page.getByRole('combobox', { name: 'Причина' })).toBeVisible()
    await expect(page.getByRole('combobox', { name: 'Владелец' })).toBeVisible()

    const first = table.getByRole('row').nth(1)
    const query = ((await first.getByRole('link').first().textContent()) ?? '').trim()
    // Контур автономии: «Разобрать» — POST /api/agent/runs {"query"} и переход в ленту. Здесь запуск подменён ответом с
    // готовым запуском: демо-состояние не меняется (живой запуск — в s4-agent.spec.ts по E2E_LIVE_AGENT=1).
    let body: unknown = null
    await page.route('**/api/agent/runs', async (route) => {
      if (route.request().method() !== 'POST') return route.fallback()
      body = route.request().postDataJSON()
      await route.fulfill({ status: 202, json: { run_id: MAIN_RUN_ID } })
    })
    await first.getByRole('button', { name: /^Разобрать/ }).click()
    await expect(page).toHaveURL(new RegExp(`/searchops/runs/${MAIN_RUN_ID}`))
    expect(body).toEqual({ query })

    // Фильтр по владельцу пишется в адрес и сужает таблицу.
    await page.goto('/searchops?tab=failed&owner=' + encodeURIComponent('Администратор поиска'))
    await expect(page.getByRole('combobox', { name: 'Владелец' })).toContainText('Администратор поиска')
    const rows = page.getByRole('region', { name: 'Таблица запросов без ответа' }).getByRole('row')
    const none = page.getByText('По выбранной причине и владельцу запросов нет.')
    await expect(rows.first().or(none).first()).toBeVisible()
    if (await none.isVisible()) return
    const owners = await rows.locator('td:nth-child(5)').allTextContents()
    expect(owners.every((o) => o.trim() === 'Администратор поиска')).toBe(true)
  })

  test('«Доказательства»: три вкладки, «Вклад компонентов» — «Прогон не запускался» и кнопка', async ({
    page,
  }) => {
    await page.goto('/proofs')
    await expect(page.getByRole('tablist', { name: 'Разделы доказательств' }).getByRole('tab')).toHaveText([
      'До / После',
      'Вклад компонентов',
      'Стоимость',
    ])
    await expect(page.getByRole('tab', { name: 'Эффект агента' })).toHaveCount(0)
    await page.getByRole('tab', { name: 'Вклад компонентов' }).click()
    await expect(page).toHaveURL(/tab=contribution/)
    const pending = page.getByText('Прогон не запускался')
    const table = page.getByRole('table').first()
    await expect(pending.or(table).first()).toBeVisible()
    await page.getByRole('tab', { name: 'Стоимость' }).click()
    await expect(page.getByRole('heading', { name: 'По маршрутам' })).toBeVisible()
  })

  test('старые адреса — на новые места', async ({ page }) => {
    const cases: [string, RegExp][] = [
      ['/analytics', /\/searchops\?tab=failed$/],
      ['/changes?tab=relations', /\/searchops\?tab=changes&view=relations$/],
      ['/margin?w=0.08', /\/searchops\?tab=policy&w=0\.08$/],
      ['/costs', /\/proofs\?tab=cost$/],
      ['/costs?tab=effect', /\/proofs\?tab=cost$/],
      ['/compare?tab=effect', /\/proofs\?tab=compare$/],
      ['/compare?tab=regressions&cfg=B3', /\/proofs\?tab=compare&view=regressions&cfg=B3$/],
    ]
    for (const [from, to] of cases) {
      await page.goto(from)
      await expect(page, from).toHaveURL(to)
    }
  })
})

test.describe('Контур доводки: снимки разделов', () => {
  test.skip(!SHOTS, 'снимки — только по E2E_SCREENS=1')
  test.setTimeout(180_000)
  const screens: [string, string, string][] = [
    ['/searchops?tab=failed', 'searchops-failed', 'Запросы без ответа'],
    ['/searchops?tab=runs', 'searchops-runs', 'Запуски'],
    ['/searchops?tab=queue', 'searchops-queue', 'Очередь'],
    ['/searchops?tab=changes', 'searchops-changes', 'Изменения'],
    ['/searchops?tab=policy', 'searchops-policy', 'Политика ранжирования'],
    ['/proofs?tab=compare', 'proofs-compare', 'До / После'],
    ['/proofs?tab=contribution', 'proofs-contribution', 'Вклад компонентов'],
    ['/proofs?tab=cost', 'proofs-cost', 'Стоимость'],
  ]
  for (const { w, h } of [
    { w: 1440, h: 900 },
    { w: 360, h: 780 },
  ]) {
    test(`разделы ${w} px`, async ({ page }) => {
      await page.setViewportSize({ width: w, height: h })
      for (const [url, name, tab] of screens) {
        await page.goto(url)
        await expect(page.getByRole('tab', { name: new RegExp(`^${tab}`) })).toHaveAttribute(
          'data-state',
          'active',
        )
        await page.waitForLoadState('networkidle').catch(() => undefined)
        await page.waitForTimeout(600)
        await noOverflow(page, `${name}-${w}`)
        await page.screenshot({ path: `${OUT}/${name}-${w}.png`, fullPage: true })
      }
    })
  }
})
