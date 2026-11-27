// Витрина без входа (повторное ревью N1, вход по ролям —): гость ходит только в открытые пути — любой
// закрытый запрос с /search дал бы 401 до первого поиска. Бэк-офис гостю не показывается, а прямой адрес ведёт на
// /login с возвратом. Контур доводки: гость видит «Поиск» и «Доказательства», SearchOps — нет; старые адреса ведут на новые
// места. Сотрудник, у которого сессия кончилась посреди работы, тоже уходит на вход с возвратом.
import { GUEST, expect, loginWith, openSearch, search, test } from './fixtures'

/** Открытые пути витрины — как publicRoutes в backend/handlers/middleware_backoffice.go. */
const PUBLIC = new Set([
  'GET /api/health',
  'GET /api/meta',
  'GET /api/suggest',
  'GET /api/suggest/recommendations', // контур выдачи: рекомендации панели
  'GET /api/evaluations',
  'GET /api/evaluations/contribution', // контур доводки: «Доказательства» → «Вклад компонентов»
  'GET /api/evaluations/feed', // контур эталонов: «Доказательства» → «На данных заказчика»
  'POST /api/guide/route', // контур подбора: режим подбора — маршрутизатор строки, открыт гостю
  'POST /api/guide', // контур подбора: ход подбора, открыт гостю
  'POST /api/guide/spec', // контур подбора: позиция подбора в спецификацию
  'GET /api/costs/summary', // хвосты контура доводки: «Доказательства» → «Стоимость» без журнала вызовов
  'POST /api/search',
  'POST /api/events',
  'POST /api/quotes', // контур коммерции: запрос КП из спецификации (витрина без входа)
  'GET /api/companions', // «К этой позиции» в спецификации
  'GET /api/auth/me', // вход по ролям: кто смотрит
  'GET /api/auth/config', // вход по ролям: демо-аккаунты на /login
])

test.describe('гость', () => {
  test.use({ storageState: GUEST })

  test('N1: витрина и «Доказательства» не запрашивают закрытые пути; SearchOps в навигации нет; прямой адрес — вход с возвратом', async ({
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
    await expect(nav.getByRole('link', { name: 'SearchOps' })).toHaveCount(0)
    await expect(nav.getByRole('link')).toHaveText(['Поиск', 'Доказательства'])
    await expect(page.getByRole('link', { name: 'Войти' })).toBeVisible()
    // Гостю — без переключателя B0–B4 и плашки режима LLM.
    await expect(page.getByLabel('Конфигурация поиска')).toHaveCount(0)
    await expect(page.getByText(/LLM включена|Режим без LLM/)).toHaveCount(0)
    await nav.getByRole('link', { name: 'Доказательства' }).click()
    await expect(page.getByRole('heading', { name: 'Доказательства', level: 1 })).toBeVisible()
    // Контур эталонов (S6): экран по умолчанию — «На данных заказчика».
    await expect(page.getByRole('tab', { name: 'На данных заказчика' })).toHaveAttribute(
      'data-state',
      'active',
    )
    await expect(page.getByRole('button', { name: 'Запустить прогон' })).toHaveCount(0)
    await page.getByRole('tab', { name: 'Вклад компонентов' }).click()
    // Пусто — «Прогон не запускался»; после прогона — таблица вклада (трек доказательств).
    await expect(
      page.getByText('Прогон не запускался').or(page.getByRole('table').first()).first(),
    ).toBeVisible()
    // «Стоимость» гостю — открытая сводка (GET /api/costs/summary): плитки и маршруты, без журнала и без «нужен вход».
    await page.getByRole('tab', { name: 'Стоимость' }).click()
    await expect(page.getByText('Средняя стоимость запроса').first()).toBeVisible()
    await expect(page.getByText(/нужен вход/i)).toHaveCount(0)
    expect(closed, 'витрина запросила закрытые пути').toEqual([])

    // Старые адреса: /compare и /costs — в «Доказательства», /analytics — в SearchOps (вход с возвратом).
    await page.goto('/compare?tab=one&q=word')
    await expect(page).toHaveURL(/\/proofs\?tab=compare&view=one&q=word/)
    await page.goto('/analytics?days=7')
    await expect(page).toHaveURL(loginWith('/searchops?tab=failed'))
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
  await expect(page).toHaveURL(/\/searchops\?tab=changes/)
  await expect(page.getByRole('tab', { name: 'Изменения' })).toHaveAttribute('data-state', 'active')
  // Как после истечения cookie: бэк-офис отвечает 401.
  await page.route(
    /\/api\/(proposals|analytics|agent|configs|costs|traces|margin|relations)(\/|\?|$)/,
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
  await page.getByRole('tab', { name: 'Запросы без ответа' }).click()
  await expect(page).toHaveURL(
    (u) => loginWith('/searchops?tab=failed')(u) && u.searchParams.get('expired') === '1',
  )
  await expect(page.getByText('Сессия истекла — войдите снова.')).toBeVisible()
})
