// Киллер-фичи контура коммерции на живом API (без подмен, ничего не меняется): очередь по оценке приоритета с
// доказательствами и комплектом, лента запуска с судьёй и отклонёнными гипотезами, песочница предложения по марже
// (только до диалога), пересчёт модели эффекта, «Допродажи» для четырёх ролей. Нет методов — пропуск.
// E2E_SCREENS=1 — скриншоты live-killer-* в artifacts/screens/p2/.
import type { Page } from '@playwright/test'
import { expect, test } from './fixtures'

const API = process.env.E2E_API_URL ?? 'http://127.0.0.1:8088'
const SHOTS = process.env.E2E_SCREENS === '1'
const shot = async (page: Page, name: string) => {
  if (SHOTS)
    await page.screenshot({ path: `../artifacts/screens/p2/${name}.png`, fullPage: true })
}

type P = { id: string; action: string; diagnosis: string; run_id?: string; priority_score?: number | null }

async function pending(): Promise<P[] | null> {
  try {
    const eff = await fetch(`${API}/api/effect/commerce`, { signal: AbortSignal.timeout(10_000) })
    if (!eff.ok) return null
    const r = await fetch(`${API}/api/proposals?status=pending`, { signal: AbortSignal.timeout(10_000) })
    return r.ok ? ((await r.json()) as { items: P[] }).items : null
  } catch {
    return null
  }
}

test.describe('Киллер-фичи П2 — живой API', () => {
  let items: P[] = []
  test.beforeEach(async () => {
    const list = await pending()
    test.skip(list === null, 'методов на API нет')
    items = list ?? []
  })

  test('F27 live: очередь по оценке приоритета, доказательства и комплект', async ({ page }) => {
    const scored = items.filter((p) => typeof p.priority_score === 'number')
    test.skip(!scored.length, 'в очереди нет предложений по покупкам')
    await page.goto('/searchops?tab=queue')
    const first = page.getByRole('article').first()
    await expect(first.getByTestId('priority')).toContainText('оценка приоритета, не прогноз')
    const rel = page.getByRole('article').filter({ hasText: 'Покупают вместе, связи нет' }).first()
    if (await rel.count()) {
      await expect(rel.getByRole('table', { name: 'Таблица доказательств' })).toContainText('Аккаунтов')
      await expect(rel.getByTestId('strength')).toBeVisible()
      await expect(rel).toContainText('Судья:')
    }
    const bundle = page.getByRole('article').filter({ hasText: 'Возможен комплект' }).first()
    if (await bundle.count()) {
      await expect(bundle.getByRole('list', { name: 'Позиции комплекта' })).toBeVisible()
      await expect(bundle.getByRole('button', { name: 'Взять в работу' })).toBeVisible()
    }
    await shot(page, 'live-killer-f27-queue-1440')
  })

  test('F27 live: лента запуска — судья и отклонённые гипотезы', async ({ page }) => {
    const p = items.find((x) => x.diagnosis === 'MISSING_RELATION' && x.run_id)
    test.skip(!p, 'нет предложения MISSING_RELATION с запуском')
    await page.goto(`/searchops/runs/${p!.run_id}`)
    await expect(page.getByText('Судья другого семейства').first()).toBeVisible({ timeout: 30_000 })
    await expect(page.getByText(/Гипотеза \d+:/).first()).toBeVisible()
    await shot(page, 'live-killer-f27-run-1440')
  })

  test('F28 live: «Открыть в песочнице» → рычаг категории и числа предложения, без применения', async ({
    page,
  }) => {
    test.setTimeout(120_000)
    const p = items.find((x) => x.action === 'margin_policy')
    test.skip(!p, 'нет ждущего предложения по марже')
    await page.goto(`/searchops?tab=queue&p=${p!.id}`)
    const card = page.locator(`#proposal-${p!.id}`)
    await expect(card).toContainText('Индекс показанной маржи')
    await card.getByRole('link', { name: 'Открыть в песочнице' }).click()
    await expect(page).toHaveURL(/cat=/)
    await expect(page.getByText(/Рычаг категории «/)).toBeVisible()
    await expect(page.getByTestId('margin-proposal')).toContainText('Предложение агента:')
    await expect(page.getByRole('img', { name: /Кривая/ })).toBeVisible({ timeout: 90_000 })
    await expect(page.getByTestId('margin-proposal').getByRole('status')).toBeVisible()
    await shot(page, 'live-killer-f28-sandbox-1440')
    const apply = page.getByRole('button', { name: /^Применить вес/ })
    if (await apply.isEnabled()) {
      await apply.click()
      const dlg = page.getByRole('dialog', { name: /Применить вес/ })
      await expect(dlg).toContainText('общий вес не меняется')
      await dlg.getByRole('button', { name: 'Отмена' }).click()
    }
  })

  test('F31 live: модель эффекта пересчитывается при смене входа', async ({ page }) => {
    await page.goto('/costs?tab=effect')
    await expect(page.getByRole('heading', { name: 'Модель чувствительности, не прогноз' })).toBeVisible()
    await expect(page.getByRole('list', { name: 'Сценарии' }).getByRole('listitem')).toHaveCount(3)
    await shot(page, 'live-killer-f31-effect-1440')
    const before = await page.getByTestId('scenario-base').textContent()
    const inc = page.getByLabel('Коэффициент инкрементальности')
    await inc.fill('60')
    await inc.press('Enter')
    await expect(page.locator('.ef__inlist').getByText('изменено вами')).toBeVisible()
    await expect(page.getByTestId('scenario-base')).not.toHaveText(before ?? '')
    await expect(page.getByText('тождество сходится').first()).toBeVisible()
  })

  for (const role of ['ecom', 'product', 'content', 'head'] as const) {
    test(`F30 live: «Допродажи» — ${role}`, async ({ page }) => {
      await page.addInitScript((r) => window.localStorage.setItem('plexet.role', r), role)
      await page.goto('/analytics?days=30')
      const block = page.getByRole('region', { name: 'Допродажи' })
      await expect(block).toBeVisible({ timeout: 30_000 })
      await expect(block.locator('.cb__tile').first()).toBeVisible()
      await shot(page, `live-killer-f30-${role}-1440`)
    })
  }
})
