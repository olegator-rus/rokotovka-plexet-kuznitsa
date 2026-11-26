// Киллер-фичи контура коммерции в интерфейсе на подменах: ответы API — снимки живого
// стенда (killer-front-data.ts). Ничего не меняется в демо: решения и применение — подменённые POST.
// E2E_SCREENS=1 — скриншоты в artifacts/screens/p2/.
import type { Page, Route } from '@playwright/test'
import { expect, test } from './fixtures'
import {
  analyticsCommerce,
  effect,
  effectIncrementality,
  marginPolicy,
  marginSimCategory,
  proposals,
} from './killer-front-data'

const SHOTS = process.env.E2E_SCREENS === '1'
const shot = async (page: Page, name: string) => {
  if (SHOTS)
    await page.screenshot({ path: `../artifacts/screens/p2/${name}.png`, fullPage: true })
}
const json = (route: Route, body: unknown, status = 200) =>
  route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })

const bundle = proposals.find((p: { action: string }) => p.action === 'bundle')
const relation = proposals.find((p: { action: string }) => p.action === 'relation_patch')
const margin = proposals.find((p: { action: string }) => p.action === 'margin_policy')

/** Очередь: сервер присылает в обратном порядке — фронт сам ставит по оценке приоритета. */
async function mockQueue(page: Page) {
  await page.route(/\/api\/proposals(\?.*)?$/, (r) => json(r, { items: [margin, relation, bundle] }))
}

async function noOverflow(page: Page, what: string) {
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
  )
  expect(overflow, `${what}: горизонтальная прокрутка`).toBeLessThanOrEqual(0)
}

test.describe('Киллер-фичи П2 (подмены)', () => {
  test('F27: очередь по оценке приоритета, доказательства, ступени, судья, комплект', async ({ page }) => {
    await mockQueue(page)
    await page.goto('/searchops?tab=queue')
    const cards = page.getByRole('article')
    await expect(cards).toHaveCount(3)
    await expect(page.getByText(/по оценке приоритета \(не прогноз\)/)).toBeVisible()
    // 281 918 ₽ (комплект) > 69 942 ₽ (связь) > без оценки (маржа).
    await expect(cards.nth(0)).toContainText('Комплект: Dell PowerEdge R750')
    await expect(cards.nth(1)).toContainText('Покупают вместе: Dell PowerEdge R750 → Ippon Innova RT II')
    await expect(cards.nth(2)).toContainText('Маржу можно поднять')
    await expect(cards.nth(0).getByTestId('priority')).toContainText('281 918')
    await expect(cards.nth(0).getByTestId('priority')).toContainText('оценка приоритета, не прогноз')

    const rel = cards.nth(1)
    await expect(rel).toContainText('Покупают вместе, связи нет')
    await expect(rel.getByTestId('strength')).toContainText('сильное')
    const table = rel.getByRole('table', { name: 'Таблица доказательств' })
    await expect(table).toContainText('6 из 41')
    await expect(table).toContainText('значимо на уровне 0,99')
    await expect(table).toContainText('Повторилось в позднем периоде')
    await expect(rel).toContainText('Судья: возражений нет')
    await rel.getByRole('button', { name: /Ступени проверки · 7/ }).click()
    const ladder = rel.getByRole('list', { name: 'Ступени проверки' })
    await expect(ladder.getByRole('listitem')).toHaveCount(7)
    await expect(ladder).toContainText('Совместимость — пройдена')
    await expect(ladder).toContainText('Судья — пройдена')

    const b = cards.nth(0)
    await expect(b.getByRole('list', { name: 'Позиции комплекта' }).getByRole('listitem')).toHaveCount(3)
    await expect(b).toContainText('ИБП Ippon Innova RT II 3000')
    await expect(b).toContainText('совместимо при условии')
    await expect(b).toContainText('Маржа комплекта: 8,9 %')
    await b.getByRole('button', { name: /Ступени проверки/ }).click()
    await expect(b).toContainText('Регрессия поиска — не применялась')
    await shot(page, 'killer-f27-queue-1440')

    await b.getByRole('button', { name: 'Взять в работу' }).click()
    const dlg = page.getByRole('dialog', { name: 'Взять в работу?' })
    await expect(dlg).toContainText('Справочник связей и выдача не меняются')
    await dlg.getByRole('button', { name: 'Отмена' }).click()
    await expect(dlg).toBeHidden()
  })

  test('F28: карточка маржи → песочница категории с теми же входами → применение с proposal_id', async ({
    page,
  }) => {
    await mockQueue(page)
    const posts: unknown[] = []
    await page.route(/\/api\/margin\/policy$/, (r) => {
      if (r.request().method() === 'POST') {
        posts.push(r.request().postDataJSON())
        return json(r, { version: 1, weight: 0.05, version_before: 0, weight_before: 0.05 }, 201)
      }
      return json(r, marginPolicy)
    })
    await page.route(/\/api\/margin\/simulate$/, (r) => {
      posts.push({ simulate: r.request().postDataJSON() })
      return json(r, marginSimCategory)
    })
    await page.route(new RegExp(`/api/proposals/${margin.id}$`), (r) => json(r, margin))

    await page.goto('/searchops?tab=queue')
    const card = page.getByRole('article').filter({ hasText: 'Маржу можно поднять' })
    await expect(card).toContainText('0,05 → 0,08')
    await expect(card).toContainText('0,4791 → 0,4793')
    await expect(card).toContainText('проверено на 125 размеченных запросах набора tune')
    await expect(card.getByText('Синтетические данные')).toBeVisible()
    await expect(card.getByRole('button', { name: 'Принять' })).toHaveCount(0)
    await shot(page, 'killer-f28-card-1440')

    await card.getByRole('link', { name: 'Открыть в песочнице' }).click()
    await expect(page).toHaveURL(/\/margin\?.*w=0\.08/)
    await expect(page).toHaveURL(/cat=ide/)
    await expect(page.getByText(/Рычаг категории «Среды разработки»/)).toBeVisible()
    const prop = page.getByTestId('margin-proposal')
    await expect(prop).toContainText('Предложение агента: 0,05 → 0,08')
    // Снимок симуляции с другим базовым прогоном: экран говорит о расхождении, а не прячет его.
    await expect(prop.getByRole('status')).toContainText('Числа разошлись с предложением')
    expect(posts).toContainEqual({
      simulate: { set: 'tune', positions: [0, 0.03, 0.05, 0.08, 0.12, 0.15], category: 'ide' },
    })
    await expect(page.getByRole('button', { name: /Индекс показанной маржи: 0,4788/ })).toBeVisible()
    await shot(page, 'killer-f28-sandbox-1440')

    await page.getByRole('button', { name: 'Применить вес 0,08' }).click()
    const dlg = page.getByRole('dialog', { name: 'Применить вес 0,08?' })
    await expect(dlg).toContainText('для категории «Среды разработки» (общий вес не меняется)')
    await expect(dlg).toContainText('Предложение агента станет принятым')
    await dlg.getByRole('button', { name: 'Применить' }).click()
    await expect(dlg).toBeHidden()
    expect(posts).toContainEqual({ categories: { ide: 0.08 }, comment: '', proposal_id: margin.id })
  })

  test('F31: «Эффект» — сценарии, член за членом, тождество, источники, пересчёт входа', async ({
    page,
  }) => {
    const asked: string[] = []
    await page.route(/\/api\/effect\/commerce(\?.*)?$/, (r) => {
      const u = new URL(r.request().url())
      asked.push(u.search)
      return json(r, u.searchParams.get('incrementality') === '0.5' ? effectIncrementality : effect)
    })
    await page.goto('/costs?tab=effect')
    await expect(page.getByRole('tab', { name: 'Эффект' })).toHaveAttribute('data-state', 'active')
    await expect(page.getByRole('heading', { name: 'Модель чувствительности, не прогноз' })).toBeVisible()
    await expect(page.getByText('Синтетические данные').first()).toBeVisible()
    const scen = page.getByRole('list', { name: 'Сценарии' })
    await expect(scen.getByRole('listitem')).toHaveCount(3)
    await expect(scen).toContainText('тождество сходится')
    const t = page.getByRole('table', { name: /Разложение эффекта/ })
    for (const term of ['Возврат поиска: спасённые заказы', 'Допродажа', 'Сдвиг микса', 'Потеря конверсии якоря', 'Возвраты из-за несовместимости'])
      await expect(t).toContainText(term)
    await expect(page.getByText(/Проверочное тождество: ΔGP = ΔO·G₀ \+ O₀·ΔG \+ ΔO·ΔG/)).toBeVisible()
    for (const src of ['литература', 'допущение', 'замер стенда', 'синтетические данные'])
      await expect(page.locator('.ef__inlist').getByText(src, { exact: true }).first()).toBeVisible()
    await expect(page.getByText(/Пилот 4–8 недель/)).toBeVisible()
    await shot(page, 'killer-f31-effect-1440')

    const before = await page.getByTestId('scenario-base').textContent()
    const inc = page.getByLabel('Коэффициент инкрементальности')
    await inc.fill('150')
    await inc.press('Enter')
    await expect(page.getByText('Доля — от 0 до 100 %')).toBeVisible()
    await inc.fill('50')
    await inc.press('Enter')
    await expect.poll(() => asked.some((q) => q.includes('incrementality=0.5'))).toBe(true)
    await expect(page.getByTestId('scenario-base')).not.toHaveText(before ?? '')
    await expect(page.locator('.ef__inlist').getByText('изменено вами')).toBeVisible()
  })

  test('F31: ошибка модели — блок с повтором', async ({ page }) => {
    await page.route(/\/api\/effect\/commerce(\?.*)?$/, (r) =>
      json(r, { error: { code: 'INTERNAL', message: 'Сбой расчёта', traceId: 't1' } }, 500),
    )
    await page.goto('/costs?tab=effect')
    await expect(page.getByRole('alert').filter({ hasText: 'Модель эффекта не посчитана' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Повторить' })).toBeVisible()
  })

  for (const role of ['ecom', 'product', 'content', 'head'] as const) {
    test(`F30: «Допродажи» для роли ${role}`, async ({ page }) => {
      await page.addInitScript((r) => window.localStorage.setItem('plexet.role', r), role)
      await page.route(/\/api\/analytics\?.*/, async (r) => {
        const resp = await r.fetch()
        const body = await resp.json()
        return json(r, { ...body, commerce: analyticsCommerce[role] })
      })
      await page.goto('/analytics?days=30')
      const block = page.getByRole('region', { name: 'Допродажи' })
      await expect(block).toBeVisible({ timeout: 30_000 })
      await expect(block).toContainText('события стенда, не трафик магазина')
      const tiles = analyticsCommerce[role].tiles as { key: string; label: string; list?: string; link?: string }[]
      await expect(block.locator('.cb__tile')).toHaveCount(tiles.length)
      const withList = tiles.find((t) => t.list)
      if (withList) {
        await block.getByTestId(`tile-${withList.key}`).click()
        const dlg = page.getByRole('dialog', { name: withList.label })
        await expect(dlg).toBeVisible()
        if (role === 'product') await expect(dlg.getByRole('link').first()).toBeVisible()
        await dlg.getByRole('button', { name: 'Закрыть' }).last().click()
      }
      const withLink = tiles.find((t) => t.link && !t.list)
      if (withLink) {
        await block.getByTestId(`tile-${withLink.key}`).click()
        await expect(page).toHaveURL(/\/costs\?tab=effect/)
      }
      await shot(page, `killer-f30-${role}-1440`)
    })
  }

  test('360 px: очередь, песочница категории, «Эффект», «Допродажи» без горизонтальной прокрутки', async ({
    page,
  }) => {
    await mockQueue(page)
    await page.route(/\/api\/effect\/commerce(\?.*)?$/, (r) => json(r, effect))
    await page.route(/\/api\/margin\/policy$/, (r) => json(r, marginPolicy))
    await page.route(/\/api\/margin\/simulate$/, (r) => json(r, marginSimCategory))
    await page.route(new RegExp(`/api/proposals/${margin.id}$`), (r) => json(r, margin))
    await page.addInitScript(() => window.localStorage.setItem('plexet.role', 'product'))
    await page.setViewportSize({ width: 360, height: 780 })
    for (const [url, name] of [
      ['/searchops?tab=queue', 'killer-f27-queue-360'],
      [`/margin?w=0.08&cat=ide&p=${margin.id}`, 'killer-f28-sandbox-360'],
      ['/costs?tab=effect', 'killer-f31-effect-360'],
      ['/analytics?days=30', 'killer-f30-product-360'],
    ]) {
      await page.goto(url)
      await expect(page.locator('main h1')).toBeVisible()
      await page.waitForLoadState('networkidle')
      await noOverflow(page, url)
      await shot(page, name)
    }
  })
})
