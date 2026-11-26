// Бэк-офис контура коммерции: «Маржа», «Изменения» → «Связи», «До / После» → «Коммерция»,
// карточка relation_patch в очереди SearchOps, 360 px. API контура коммерции подменяется контрактными ответами
// (backoffice-p2-data.ts); E2E_P2_LIVE=1 — живой API. E2E_SCREENS=1 — скриншоты в artifacts/screens/p2/.
import type { Page } from '@playwright/test'
import { mockBackofficeP2 } from './backoffice-p2-data'
import { expect, test } from './fixtures'

const SHOTS = process.env.E2E_SCREENS === '1'
const shot = async (page: Page, name: string) => {
  if (SHOTS)
    await page.screenshot({ path: `../artifacts/screens/p2/${name}.png`, fullPage: true })
}

test.describe('Бэк-офис П2', () => {
  test('F23: рычаг маржи — кривая, упор, «было / стало», применение через диалог', async ({
    page,
  }) => {
    const posts = await mockBackofficeP2(page)
    await page.goto('/margin')
    await expect(page.getByRole('heading', { name: 'Маржа', level: 1 })).toBeVisible()
    await expect(page.getByRole('link', { name: 'Маржа' })).toBeVisible()
    await expect(page.getByText('Синтетические данные').first()).toBeVisible()

    const lever = page.getByRole('group', { name: 'Приоритет маржи' })
    await expect(lever.getByRole('button', { name: /Вес 0,05, действует/ })).toBeVisible()
    await expect(lever.getByRole('button', { name: /0,12, за упором/ })).toBeDisabled()
    await expect(page.getByRole('img', { name: /Кривая/ })).toBeVisible()
    await expect(page.getByText('Точные запросы защищены')).toBeVisible()
    await expect(page.getByText(/Оценка на 120 размеченных запросах/)).toBeVisible()
    await expect(page.getByRole('button', { name: /^Применить вес/ })).toBeDisabled()
    await shot(page, 'backoffice-margin-1440')

    await lever.getByRole('button', { name: /Вес 0,08/ }).click()
    await expect(page).toHaveURL(/w=0\.08/)
    await expect(page.getByText(/Упор\./)).toBeVisible()
    await expect(
      page.getByRole('button', { name: /Соответствие в первой десятке: В среднем на 0,2/ }),
    ).toBeVisible()
    await expect(page.getByRole('button', { name: /Изменился первый результат: 3 запроса/ })).toBeVisible()

    await page.getByRole('button', { name: /антивирус для бизнеса на 50 пк/ }).click()
    await expect(page.getByText('Было (вес 0,05)')).toBeVisible()
    await expect(page.getByText('поднялась с 2')).toBeVisible()

    await page.getByRole('button', { name: 'Применить вес 0,08' }).click()
    const dialog = page.getByRole('dialog', { name: 'Применить вес 0,08?' })
    await expect(dialog).toContainText('Будет создана политика v2')
    await dialog.getByRole('button', { name: 'Применить' }).click()
    await expect(dialog).toBeHidden()
    if (process.env.E2E_P2_LIVE !== '1')
      expect(posts).toContainEqual({ path: '/api/margin/policy', body: { weight: 0.08, comment: '' } })
  })

  test('F29: справочник связей — фильтр, «проверить», карточка с историей, откат через диалог', async ({
    page,
  }) => {
    const posts = await mockBackofficeP2(page)
    await page.goto('/changes?tab=relations')
    await expect(page.getByRole('tab', { name: 'Связи' })).toHaveAttribute('data-state', 'active')
    const table = page.getByRole('table', { name: 'Справочник связей' })
    await expect(table.getByRole('row')).toHaveCount(3)
    await shot(page, 'backoffice-relations-1440')

    await page.getByRole('searchbox', { name: 'Фильтр по товару' }).fill('R740')
    await expect(table.getByRole('row')).toHaveCount(2)
    await page.getByRole('searchbox', { name: 'Фильтр по товару' }).fill('')

    await expect(page.getByText('стало: дополнение · условно обязательное')).toBeVisible()
    await expect(page.getByText(/было: дополнение · условно обязательное · совместимость не подтверждена/)).toBeVisible()

    await table.getByRole('button', { name: 'Windows Server 2025 Standard' }).click()
    const card = page.getByRole('dialog', { name: /Windows Server 2025 Standard → Windows Server CAL 2025/ })
    await expect(card).toContainText('E2 — документ вендора')
    await expect(card).toContainText('утвердил: Агент + человек')
    await card.getByRole('button', { name: 'Откатить к v1' }).click()

    const rb = page.getByRole('dialog', { name: 'Откатить версию 2?' })
    await expect(rb).toContainText('копия версии 1')
    await rb.getByRole('button', { name: 'Откатить' }).click()
    await expect(rb).toBeHidden()
    if (process.env.E2E_P2_LIVE !== '1')
      expect(posts).toContainEqual({ path: '/api/relations/rollback', body: { to_version: 1, comment: '' } })
  })

  test('F25: «Коммерция» — лестница C0–C3, плитки блока, пары совместимости, ухудшения', async ({
    page,
  }) => {
    await mockBackofficeP2(page)
    await page.goto('/compare?tab=commerce')
    const table = page.getByRole('table', { name: 'Коммерческая лестница конфигураций' })
    await expect(table.getByRole('row')).toHaveCount(5)
    await expect(table.getByRole('row', { name: /C3 · \+ политика маржи не готово/ })).toBeVisible()
    await expect(page.getByText(/Воспроизводимость: .*hash=a1b2c3d4/)).toBeVisible()
    await expect(page.getByText('Один компонент за раз')).toBeVisible()
    await expect(page.getByRole('table', { name: 'Запросы с ухудшением основной выдачи' })).toContainText(
      'сервер dell r740',
    )
    await shot(page, 'backoffice-commerce-1440')

    await page.getByRole('button', { name: /^Нарушения совместимости: 0/ }).click()
    const ex = page.getByRole('dialog', { name: 'Нарушения совместимости' })
    await expect(ex).toContainText('Проверенные пары')
    await expect(ex).toContainText('40 размеченных')
    await expect(ex).toContainText('Ни одна пара с разметкой «несовместимо» не попала в блок')
  })

  test('F26: карточка relation_patch — было/стало, доказательства, два статуса, диалог без принятия', async ({
    page,
  }) => {
    await mockBackofficeP2(page)
    await page.goto('/searchops?tab=queue')
    const card = page.getByRole('article').filter({ hasText: 'Сломанная связь' })
    await expect(card).toBeVisible()
    await expect(card).toContainText('было: дополнение · условно обязательное · совместимость не подтверждена')
    await expect(card).toContainText('Уровень: E2 — документ вендора · сильное')
    await expect(card).toContainText('Техническая корректность: подтверждена')
    await expect(card).toContainText('Эффект: неизвестен, нужен замер')
    await expect(card).toContainText('Полнота обязательных: было 50')
    await expect(card).toContainText('Ошибки справочника: было 1, стало 0 — находка снята')
    await expect(card).toContainText('Правило вендора lr-ws-cal')
    await shot(page, 'backoffice-searchops-relation-1440')

    await card.getByRole('button', { name: 'Принять' }).click()
    const dialog = page.getByRole('dialog', { name: 'Принять правку?' })
    await expect(dialog).toContainText('версия справочника связей')
    await dialog.getByRole('button', { name: 'Отмена' }).click()
  })

  test('360 px: «Маржа» и «Связи» без горизонтальной прокрутки страницы', async ({ page }) => {
    await mockBackofficeP2(page)
    await page.setViewportSize({ width: 360, height: 780 })
    for (const [url, name] of [
      ['/margin', 'backoffice-margin-360'],
      ['/changes?tab=relations', 'backoffice-relations-360'],
      ['/compare?tab=commerce', 'backoffice-commerce-360'],
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

  test('Состояния: ошибка симуляции с повтором, пустой справочник', async ({ page }) => {
    await mockBackofficeP2(page)
    await page.route(/\/api\/margin\/simulate$/, (r) =>
      r.fulfill({
        status: 500,
        contentType: 'application/json',
        body: JSON.stringify({ error: { code: 'UNAVAILABLE', message: 'Прогон не удался', traceId: 't1' } }),
      }),
    )
    await page.addInitScript(() => window.localStorage.removeItem('plexet.margin.lastSim'))
    await page.goto('/margin')
    await expect(page.getByRole('alert').filter({ hasText: 'Симуляция не выполнена' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Повторить' })).toBeVisible()

    // 503 — базовая выдача собирается: прогресс вместо ошибки.
    await page.route(/\/api\/margin\/simulate$/, (r) =>
      r.fulfill({
        status: 503,
        contentType: 'application/json',
        body: JSON.stringify({ error: { code: 'UNAVAILABLE', message: 'Готовим базу', traceId: 't2' } }),
      }),
    )
    await page.route(/\/api\/margin\/simulate\/status$/, (r) =>
      r.fulfill({ contentType: 'application/json', body: JSON.stringify({ running: true, done: 40, total: 120 }) }),
    )
    await page.goto('/margin')
    await expect(page.getByRole('status').filter({ hasText: 'Собираем базовую выдачу' })).toContainText(
      '40 из 120 запросов',
    )

    await page.route(/\/api\/relations(\?.*)?$/, (r) =>
      r.fulfill({ contentType: 'application/json', body: JSON.stringify({ version: 1, items: [] }) }),
    )
    await page.goto('/changes?tab=relations')
    await expect(page.getByText('Связей нет')).toBeVisible()
  })
})
