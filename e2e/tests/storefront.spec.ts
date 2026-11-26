// Витрина без входа (повторное ревью N1, вход по ролям —): гость ходит только в открытые пути — любой
// закрытый запрос с /search дал бы 401 до первого поиска. Бэк-офис гостю не показывается, а прямой адрес ведёт на
// /login с возвратом. Сотрудник, у которого сессия кончилась посреди работы, тоже уходит на вход с возвратом.
import { GUEST, expect, loginWith, openSearch, search, test } from './fixtures'

/** Открытые пути витрины — как publicRoutes в backend/handlers/middleware_backoffice.go. */
const PUBLIC = new Set([
  'GET /api/health',
  'GET /api/meta',
  'GET /api/suggest',
  'GET /api/suggest/recommendations', // контур выдачи: рекомендации панели
  'GET /api/evaluations',
  'POST /api/search',
  'POST /api/events',
  'POST /api/quotes', // контур коммерции: запрос КП из спецификации (витрина без входа)
  'GET /api/companions', // «К этой позиции» в спецификации
  'GET /api/auth/me', // вход по ролям: кто смотрит
  'GET /api/auth/config', // вход по ролям: демо-аккаунты на /login
])

test.describe('гость', () => {
  test.use({ storageState: GUEST })

  test('N1: витрина не запрашивает закрытые пути; бэк-офиса в навигации нет; прямой адрес — вход с возвратом', async ({
    page,
  }) => {
    const closed: string[] = []
    page.on('request', (req) => {
      const url = new URL(req.url())
      if (!url.pathname.startsWith('/api/')) return
      const key = `${req.method()} ${url.pathname.replace(/\/$/, '')}`
      if (!PUBLIC.has(key)) closed.push(key)
    })

    await openSearch(page)
    await search(page, 'касперский')
    const nav = page.getByRole('navigation', { name: 'Разделы' })
    await expect(nav.getByRole('link', { name: 'Поиск' })).toBeVisible()
    await expect(nav.getByRole('link', { name: 'Аналитика' })).toHaveCount(0)
    await expect(page.getByRole('link', { name: 'Войти' })).toBeVisible()
    await nav.getByRole('link', { name: 'До / После' }).click()
    await page.getByRole('tab', { name: 'Эффект агента' }).click()
    await expect(
      page.getByRole('region', { name: 'Эффект агента на отложенной неделе' }),
    ).toBeVisible()
    expect(closed, 'витрина запросила закрытые пути').toEqual([])

    await page.goto('/analytics?days=7')
    await expect(page).toHaveURL(loginWith('/analytics?days=7'))
    await expect(
      page.getByText('Раздел бэк-офиса — войдите под аккаунтом сотрудника.'),
    ).toBeVisible()
    expect(closed, 'вход запросил закрытые пути').toEqual([])
  })
})

test('сессия кончилась посреди работы: 401 бэк-офиса — на вход с возвратом и пометкой', async ({
  page,
}) => {
  await page.goto('/changes')
  await expect(page.getByRole('heading', { name: 'Изменения', level: 1 })).toBeVisible()
  // Как после истечения cookie: бэк-офис отвечает 401.
  await page.route(
    /\/api\/(proposals|analytics|agent|configs|costs|traces|effect)(\/|\?|$)/,
    (route) =>
      route.fulfill({
        status: 401,
        headers: { 'WWW-Authenticate': 'Bearer realm="PLEXeT SearchOps"' },
        json: {
          error: {
            code: 'UNAUTHENTICATED',
            message: 'Нужен вход сотрудника',
            traceId: 'e2e',
          },
        },
      }),
  )
  await page
    .getByRole('navigation', { name: 'Разделы' })
    .getByRole('link', { name: 'Аналитика' })
    .click()
  await expect(page).toHaveURL(
    (u) => loginWith('/analytics')(u) && u.searchParams.get('expired') === '1',
  )
  await expect(page.getByText('Сессия истекла — войдите снова.')).toBeVisible()
})
