// Витрина без входа (повторное ревью N1): на стенде API бэк-офиса закрыто basic_auth, и любой запрос к нему с
// /search показал бы жюри окно пароля до первого поиска. Витрина (/search, /compare) ходит только в открытые
// пути; закрытые запрашиваются после перехода в бэк-офис.
import { expect, openSearch, search, test } from './fixtures'

/** Открытые пути витрины — как publicRoutes в backend/handlers/middleware_backoffice.go. */
const PUBLIC = new Set([
  'GET /api/health',
  'GET /api/meta',
  'GET /api/suggest',
  'GET /api/evaluations',
  'POST /api/search',
  'POST /api/events',
])

test('N1: витрина не запрашивает закрытые пути до перехода в бэк-офис; 401 — понятное сообщение', async ({
  page,
}) => {
  const closed: string[] = []
  page.on('request', (req) => {
    const url = new URL(req.url())
    if (!url.pathname.startsWith('/api/')) return
    const key = `${req.method()} ${url.pathname.replace(/\/$/, '')}`
    if (!PUBLIC.has(key)) closed.push(key)
  })
  // Как на стенде без входа: бэк-офис отвечает 401 с вызовом basic-auth.
  const backoffice = (url: URL) =>
    /^\/api\/(proposals|analytics|agent|configs|costs|traces|effect)(\/|$)/.test(url.pathname)
  await page.route(backoffice, (route) =>
    route.fulfill({
      status: 401,
      headers: { 'WWW-Authenticate': 'Basic realm="PLEXeT SearchOps"' },
      json: { error: { code: 'UNAUTHORIZED', message: 'Нужен вход в бэк-офис', traceId: 'e2e' } },
    }),
  )

  await openSearch(page)
  await search(page, 'касперский')
  await page.getByRole('navigation', { name: 'Разделы' }).getByRole('link', { name: 'До / После' }).click()
  await page.getByRole('tab', { name: 'Эффект агента' }).click()
  await expect(page.getByRole('region', { name: 'Эффект агента на отложенной неделе' })).toBeVisible()
  expect(closed, 'витрина запросила закрытые пути').toEqual([])

  await page.getByRole('navigation', { name: 'Разделы' }).getByRole('link', { name: 'Аналитика' }).click()
  await expect(page.getByRole('button', { name: 'Войти' }).first()).toBeVisible()
  await expect(page.getByText(/Нужен вход менеджера/).first()).toBeVisible()
  expect(closed.some((k) => k.startsWith('GET /api/proposals'))).toBe(true)
})
