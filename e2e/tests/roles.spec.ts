// Вход по ролям: гость, покупатель, сотрудники. Демо-состояние не меняется: решения не принимаются —
// права проверяются неактивной кнопкой и ответом сервера на запрос с неверной причиной (403 — отказ по праву до
// разбора тела; 422 — право есть, но тело неверно), смена аккаунта и выход трогают только cookie этого браузера.
import {
  GUEST,
  ORIGIN,
  PERSONA,
  expect,
  loginAs,
  loginWith,
  resultCards,
  search,
  test,
} from './fixtures'
import type { Page } from '@playwright/test'

const header = (page: Page) => page.getByRole('banner')

/** Ждущее предложение правки связей роли product (сценарий «content не может принять relation_patch»). */
async function productRelationPatch(page: Page): Promise<string | null> {
  const resp = await page.request.get('/api/proposals?status=pending&role=product')
  expect(resp.status()).toBe(200)
  const items = ((await resp.json()) as { items: { id: string; action: string }[] }).items
  return items.find((p) => p.action === 'relation_patch')?.id ?? null
}

test.describe('гость', () => {
  test.use({ storageState: GUEST })

  test('витрина и «Войти» без входа; бэк-офиса нет; демо-вход в один клик ведёт на стартовый экран роли', async ({
    page,
  }) => {
    await page.goto('/search')
    await expect(header(page).getByRole('link', { name: 'Войти' })).toBeVisible()
    await expect(page.getByRole('combobox', { name: 'Клиент' })).toHaveCount(0)
    const body = await search(page, 'продлить антивирус')
    expect(body.items[0]?.purchased_before ?? false).toBe(false)

    await page.goto('/searchops?tab=queue')
    await expect(page).toHaveURL((u) => u.pathname === '/login' && u.searchParams.has('returnTo'))
    await page.getByRole('button', { name: /Войти как Дарья Лапина/ }).click()
    // returnTo — очередь SearchOps; категорийному и лицензионному менеджеру туда можно.
    await expect(page).toHaveURL(/\/searchops\?tab=queue/)
    await expect(header(page).getByRole('button', { name: /Аккаунт: Дарья Лапина/ })).toBeVisible()
  })

  test('запрос КП гостя — с контактом для ответа; без контакта не уходит', async ({ page }) => {
    const posted: { comment?: string; client_id?: string | null }[] = []
    // КП не пишем в базу: ответ сервера подменён.
    await page.route('**/api/quotes', (r) => {
      posted.push(r.request().postDataJSON() as { comment?: string })
      return r.fulfill({
        status: 201,
        json: { id: 'q-e2e', status: 'new', positions: 1, total_rub: 22900, on_request: 0 },
      })
    })
    await page.addInitScript(() =>
      window.localStorage.setItem(
        'plexet.spec',
        JSON.stringify([
          {
            product_id: 'eset-protect-entry-10-1y',
            title: 'ESET PROTECT Entry, 10 устройств, продление на 1 год',
            price_rub: 22900,
            price_mode: 'fixed',
            quantity: 1,
            origin: 'search',
          },
        ]),
      ),
    )
    await page.goto('/search')
    await page.getByRole('button', { name: /Спецификация, позиций: 1/ }).click()
    await page.getByRole('button', { name: 'Запросить КП' }).click()
    await expect(page.getByText('Оставьте почту или телефон — менеджер пришлёт КП')).toBeVisible()
    expect(posted).toHaveLength(0)
    await page.getByLabel('Контакт для ответа — почта или телефон').fill('buyer@example.ru')
    await page.getByRole('button', { name: 'Запросить КП' }).click()
    await expect.poll(() => posted.length).toBe(1)
    expect(posted[0]?.comment).toBe('Контакт: buyer@example.ru')
  })

  test('360 px: выход из бэк-офиса, затем «Назад» — бэк-офис не показывается, только вход', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 360, height: 780 })
    await page.goto('/search')
    await loginAs(page, PERSONA.head)
    // Два экрана бэк-офиса в истории: выход заменяет текущий, «Назад» ведёт на предыдущий — /searchops?tab=report.
    await page.goto('/searchops?tab=report')
    await page.goto('/searchops?tab=memory')
    await expect(page.getByRole('heading', { name: 'SearchOps', level: 1 })).toBeVisible()
    await header(page)
      .getByRole('button', { name: /Аккаунт: Сергей Орлов/ })
      .click()
    await page.getByRole('menuitem', { name: 'Выйти' }).click()
    await expect(page).toHaveURL((u) => u.pathname === '/search')
    await page.goBack()
    await expect(page).toHaveURL(loginWith('/searchops?tab=report'))
    await expect(page.getByRole('heading', { name: 'SearchOps', level: 1 })).toHaveCount(0)
    const de = await page.evaluate(() => [document.documentElement.scrollWidth, innerWidth])
    expect(de[0], 'экран входа без горизонтальной прокрутки').toBeLessThanOrEqual(de[1]!)
  })

  test('вход по паролю без аккаунтов — одна фраза об ошибке, пароль очищен', async ({ page }) => {
    await page.goto('/login')
    await page.getByLabel('Логин').fill('nobody')
    await page.getByLabel('Пароль', { exact: true }).fill('wrong-password')
    await page.getByRole('button', { name: 'Войти', exact: true }).click()
    await expect(page.getByRole('alert')).toContainText('Неверный логин или пароль')
    await expect(page.getByLabel('Пароль', { exact: true })).toHaveValue('')
  })
})

test('покупатель client-a: своя выдача на «продлить антивирус», клиента не выбрать, бэк-офиса нет', async ({
  page,
}) => {
  // Демо-покупатель заведён, если его клиент есть в данных каталога (mock — clients.json, фид — clients.csv).
  const cfg = (await (await page.request.get('/api/auth/config')).json()) as {
    personas?: { id: string }[]
  }
  test.skip(
    !cfg.personas?.some((p) => p.id === PERSONA.buyerSmb),
    'на этом каталоге нет демо-покупателя client-a',
  )
  await loginAs(page, PERSONA.buyerSmb)
  // Чужой клиент в адресе заменяется своим; сервер всё равно берёт клиента из сессии.
  await page.goto('/search?client=client-b')
  await expect(page).toHaveURL(/client=client-a/)
  await expect(page.getByRole('combobox', { name: 'Клиент' })).toHaveCount(0)
  const body = await search(page, 'продлить антивирус')
  expect(body.items[0]?.purchased_before).toBe(true)
  await expect(resultCards(page).first()).toContainText('Kaspersky Small Office Security')
  // Пометка покупки: «Вы покупали этот продукт» (контур коммерции) или «Вы покупали» (страница результатов контура выдачи) — только
  // на полной витрине (VITE_SHOWCASE_FULL=1, frontend/src/lib/showcase.ts); узкая витрина контура фида её скрывает.
  if (process.env.VITE_SHOWCASE_FULL === '1') {
    await expect(resultCards(page).first()).toContainText(/Вы покупали/)
  }
  await expect(
    page.getByRole('navigation', { name: 'Разделы' }).getByRole('link', { name: 'SearchOps' }),
  ).toHaveCount(0)

  // Подмена client_id в теле не помогает: сервер берёт клиента из сессии.
  const resp = await page.request.post('/api/search', {
    headers: { Origin: ORIGIN },
    data: {
      query: 'продлить антивирус',
      client_id: 'client-b',
      config: 'live',
      source: 'compare',
      limit: 3,
    },
  })
  expect(
    ((await resp.json()) as { items: { purchased_before: boolean }[] }).items[0]?.purchased_before,
  ).toBe(true)
  // Свои КП — из меню аккаунта (клиент из сессии; пусто — подсказка собрать спецификацию).
  await page
    .getByRole('banner')
    .getByRole('button', { name: /Аккаунт: Ольга Северцева/ })
    .click()
  await page.getByRole('menuitem', { name: 'Мои запросы КП' }).click()
  const quotes = page.getByRole('dialog', { name: 'Мои запросы КП' })
  await expect(quotes).toContainText('Клиент А')
  await expect(
    quotes.getByRole('listitem').or(quotes.getByText('Запросов КП пока нет.')).first(),
  ).toBeVisible()
  await quotes.getByRole('button', { name: 'Закрыть' }).click()
  // Бэк-офис покупателю — 403 на сервере и вход сотрудником на фронте.
  expect((await page.request.get('/api/analytics?days=7')).status()).toBe(403)
  await page.goto('/searchops')
  await expect(page).toHaveURL(loginWith('/searchops'))
  await expect(page.getByText(/Этот раздел — для сотрудников магазина/)).toBeVisible()
})

test('«Каталог» не может утвердить правку связей: кнопки неактивны с подсказкой, сервер — 403', async ({
  page,
}) => {
  const viewer = await loginAs(page, PERSONA.content)
  expect(viewer.role).toBe('content')
  const id = await productRelationPatch(page)
  test.skip(!id, 'в очереди нет ждущей правки связей роли product')
  await page.goto(`/searchops?tab=queue&status=pending&p=${id}`)
  const card = page.locator(`#proposal-${id}`)
  await expect(card).toBeVisible({ timeout: 15_000 })
  await expect(card.getByRole('button', { name: 'Утвердить' })).toBeDisabled()
  await expect(card.getByRole('button', { name: 'Отклонить с причиной' })).toBeDisabled()
  const deny = card.getByText(/Это решение принимает /)
  await expect(deny).toBeVisible()

  // Только reject с неверной причиной: при отказе по праву — 403, а если бы право было — 422 без изменения
  // состояния (approve того же пути проверки в e2e не шлём — он изменил бы демо при ошибке).
  const resp = await page.request.post(`/api/proposals/${id}/reject`, {
    headers: { Origin: ORIGIN },
    data: { reason: 'not-a-reason' },
  })
  expect(resp.status()).toBe(403)
  // Подсказка у кнопки — та же фраза, что отказ сервера.
  expect(((await resp.json()) as { error: { message: string } }).error.message).toBe(
    (await deny.textContent())?.trim(),
  )
  // Откат правил — тоже не его зона.
  const rb = await page.request.post('/api/configs/rollback', {
    headers: { Origin: ORIGIN },
    data: {},
  })
  expect(rb.status()).toBe(403)
  // Фраза отказа — services.DenyMessage, та же, что lib/auth.ts denyMessage у кнопок.
  expect(((await rb.json()) as { error: { message: string } }).error.message).toBe(
    'Это решение принимает администратор поиска',
  )
})

test('руководитель может всё: кнопки активны, сервер пропускает к проверке тела (422, не 403)', async ({
  page,
}) => {
  const id = await productRelationPatch(page)
  test.skip(!id, 'в очереди нет ждущей правки связей роли product')
  await page.goto(`/searchops?tab=queue&status=pending&p=${id}`)
  const card = page.locator(`#proposal-${id}`)
  await expect(card).toBeVisible({ timeout: 15_000 })
  await expect(card.getByRole('button', { name: 'Утвердить' })).toBeEnabled()
  await expect(card.getByText(/Это решение принимает/)).toHaveCount(0)
  // Неверная причина отказа — 422 до изменения состояния: право есть.
  const resp = await page.request.post(`/api/proposals/${id}/reject`, {
    headers: { Origin: ORIGIN },
    data: { reason: 'not-a-reason' },
  })
  expect(resp.status()).toBe(422)
  for (const path of [
    '/api/configs/rollback',
    '/api/relations/rollback',
    '/api/margin/policy/rollback',
  ]) {
    const r = await page.request.post(path, {
      headers: { Origin: ORIGIN },
      data: { to_version: -1 },
    })
    expect(r.status(), path).not.toBe(403)
    expect(r.status(), path).toBeLessThan(500)
  }
})

test('смена демо-аккаунта из шапки и выход', async ({ page }) => {
  await page.goto('/searchops')
  await header(page)
    .getByRole('button', { name: /Аккаунт: Сергей Орлов/ })
    .click()
  await page.getByRole('menuitem', { name: 'Сменить демо-аккаунт' }).click()
  const dialog = page.getByRole('dialog', { name: 'Сменить демо-аккаунт' })
  await expect(dialog.getByRole('button', { name: /Войти как Сергей Орлов/ })).toHaveAttribute(
    'aria-current',
    'true',
  )
  await dialog.getByRole('button', { name: /Войти как Илья Ветров/ }).click()
  await expect(dialog).toBeHidden()
  await expect(page).toHaveURL(/\/searchops$/)
  await expect(page.getByRole('tab', { name: 'Отчёт' })).toHaveAttribute('data-state', 'active')
  await expect(header(page).getByRole('button', { name: /Аккаунт: Илья Ветров/ })).toBeVisible()

  await header(page)
    .getByRole('button', { name: /Аккаунт: Илья Ветров/ })
    .click()
  await page.getByRole('menuitem', { name: 'Выйти' }).click()
  await expect(page).toHaveURL(/\/search/)
  await expect(header(page).getByRole('link', { name: 'Войти' })).toBeVisible()
  expect((await page.request.get('/api/auth/me').then((r) => r.json())).viewer.role).toBe('guest')
  // Старый адрес «Маржи» — SearchOps: гостю вход с возвратом.
  await page.goto('/margin')
  await expect(page).toHaveURL(loginWith('/searchops?tab=policy'))
})
