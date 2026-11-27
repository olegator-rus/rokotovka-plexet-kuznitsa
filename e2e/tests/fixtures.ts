// Общая обвязка: страж демо-состояния и помощники по экранам.
//
// Обычный прогон не меняет демо: запросы, которые пишут состояние (запуск агента, решение по предложению,
// откат правил, прогон оценки, импорт), страж обрывает и валит тест. Клики и «Спросить менеджера» не уходят
// в аналитику (/api/events подменяется ответом 204), поиски помечаются source=compare — так их не видят
// аналитика, стоимость и агент. allowDestructive: true снимает запрет — только для @destructive и живого
// дубля видео. E2E_LOG_SEARCHES=1 — поиски пишутся как живые (для дубля, если нужно).
import { test as base, expect, type Page, type Response } from '@playwright/test'
import { BASE_URL } from './auth-state'

export { expect }

/** Origin витрины: изменяющие запросы API принимаются только с ним (и вход тоже). */
export const ORIGIN = new URL(BASE_URL).origin

/** Без входа: test.use({ storageState: GUEST }). */
export const GUEST = { cookies: [], origins: [] }

/** Демо-аккаунты. */
export const PERSONA = {
  buyerSmb: 'buyer-smb',
  buyerEnterprise: 'buyer-enterprise',
  buyerGov: 'buyer-gov',
  ecom: 'staff-ecom',
  product: 'staff-product',
  content: 'staff-content',
  head: 'staff-head',
} as const

/** Адрес — экран входа с возвратом на path (vue-router кодирует returnTo по-своему — сравниваем разобранным). */
export const loginWith = (path: string) => (u: URL) =>
  u.pathname === '/login' && u.searchParams.get('returnTo') === path

/** Войти в демо-аккаунт в контексте страницы (cookie общая с браузером). Демо-состояние базы не меняется. */
export async function loginAs(page: Page, persona: string) {
  const resp = await page.request.post('/api/auth/demo-login', {
    data: { persona },
    headers: { Origin: ORIGIN },
  })
  expect(resp.status(), `демо-вход ${persona}`).toBe(200)
  return ((await resp.json()) as { viewer: { role: string; name: string } }).viewer
}

const DESTRUCTIVE: { method: string; path: RegExp }[] = [
  { method: 'POST', path: /^\/api\/agent\/runs$/ },
  { method: 'POST', path: /^\/api\/agent\/runs\/[^/]+\/stop$/ },
  { method: 'POST', path: /^\/api\/proposals\/[^/]+\/(approve|reject)$/ },
  { method: 'POST', path: /^\/api\/configs\/rollback$/ },
  { method: 'POST', path: /^\/api\/evaluations\/run$/ },
  { method: 'POST', path: /^\/api\/catalog\/import$/ },
  // контур коммерции, бэк-офис: применение и откат политики маржи, откат справочника связей
  { method: 'POST', path: /^\/api\/margin\/policy(\/rollback)?$/ },
  { method: 'POST', path: /^\/api\/relations\/rollback$/ },
]

/** Главный запуск на фиде заказчика: разбор запроса «я360» — алиас → «Яндекс 360 для бизнеса», гипотеза прошла прогон,
 * предложение в очереди. Прежний mock-запуск («акад», зачёркнутые гипотезы) удалён из базы при чистке mock-эпохи. */
export const MAIN_RUN_ID = '01a217d0-7b25-79a0-b774-a89628a7b771'

type Fixtures = {
  allowDestructive: boolean
  blocked: string[]
}

export const test = base.extend<Fixtures>({
  allowDestructive: [false, { option: true }],
  blocked: [
    async ({ page, allowDestructive }, use, testInfo) => {
      const blocked: string[] = []
      const logSearches = process.env.E2E_LOG_SEARCHES === '1'
      await page.route('**/api/**', async (route) => {
        const req = route.request()
        const path = new URL(req.url()).pathname
        const method = req.method()
        if (!allowDestructive && DESTRUCTIVE.some((d) => d.method === method && d.path.test(path))) {
          blocked.push(`${method} ${path}`)
          return route.abort('blockedbyclient')
        }
        if (!allowDestructive && method === 'POST' && path === '/api/events') {
          return route.fulfill({ status: 204, body: '' })
        }
        if (!logSearches && method === 'POST' && path === '/api/search') {
          const body = req.postDataJSON() as Record<string, unknown>
          return route.continue({ postData: JSON.stringify({ ...body, source: 'compare' }) })
        }
        return route.fallback()
      })
      await use(blocked)
      if (blocked.length) {
        testInfo.annotations.push({ type: 'blocked', description: blocked.join(', ') })
      }
      expect(blocked, 'тест пытался изменить демо-состояние').toEqual([])
    },
    { auto: true },
  ],
})

/** Пауза для чтения в видео; VIDEO_PACE=0 — без пауз (быстрая проверка сценария). */
export const PACE = Number(process.env.VIDEO_PACE ?? '1')
export const pause = (page: Page, ms: number) =>
  PACE > 0 ? page.waitForTimeout(ms * PACE) : Promise.resolve()

export const searchBox = (page: Page) => page.getByRole('combobox', { name: 'Поиск по каталогу' })
export const results = (page: Page) => page.getByRole('list', { name: 'Результаты' })
export const resultCards = (page: Page) => page.getByRole('article')
export const tracePanel = (page: Page) => page.getByRole('complementary', { name: 'Трасса запроса' })
export const understanding = (page: Page) => page.getByRole('region', { name: 'Как понят запрос' })

const isSearch = (r: Response) =>
  r.url().endsWith('/api/search') && r.request().method() === 'POST'

/** Ответ POST /api/search, пришедший после действия. */
export async function searchResponse(page: Page, action: () => Promise<unknown>) {
  const [resp] = await Promise.all([page.waitForResponse(isSearch, { timeout: 30_000 }), action()])
  expect(resp.status(), 'POST /api/search').toBe(200)
  return (await resp.json()) as SearchBody
}

/** Набрать запрос и нажать Enter; typingDelay — видимый набор для видео. */
export async function search(page: Page, q: string, typingDelay = 0) {
  const box = searchBox(page)
  await box.click()
  await box.fill('')
  return searchResponse(page, async () => {
    if (typingDelay > 0) await box.pressSequentially(q, { delay: typingDelay })
    else await box.fill(q)
    await box.press('Enter')
  })
}

export interface SearchBody {
  query: string
  config: string
  total: number
  items: {
    product_id: string
    title: string
    group: string
    purchased_before?: boolean
    category?: string
    vendor?: string
  }[]
  chips: { field: string; value: string; label: string; strict: boolean }[]
  correction?: { from: string; to: string; applied: boolean }
  empty?: { message: string; show_manager: boolean }
  trace: {
    route: string
    parse_source: string
    cost_rub: number
    rules_version: number
    config?: string
    rejected?: { product_id: string; reason: string }[]
  }
}

/** Открыть «Поиск» и дождаться готовой страницы (строка в фокусе, мета загружена). */
export async function openSearch(page: Page, query = '') {
  await page.goto(`/search${query}`)
  await expect(searchBox(page)).toBeVisible()
}
