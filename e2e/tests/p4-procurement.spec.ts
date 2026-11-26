// Контур доводки, («Поиск → ответ справки / отсутствующий товар / аналоги»): закупка и честное
// «нет». Нужны данные контура доводки (make import после gen_mock.py). Демо-состояние не меняется: заявка КП и события
// перехватываются (тело проверяется, в базу не пишется). E2E_SCREENS=1 — снимки в artifacts/screens/p4/.
import type { Page, Request } from '@playwright/test'
import { GUEST, PERSONA, expect, loginAs, search, test } from './fixtures'

const OUT = '../artifacts/screens/p4'
const SHOTS = process.env.E2E_SCREENS === '1'
const shot = async (page: Page, name: string) => {
  if (SHOTS) await page.screenshot({ path: `${OUT}/${name}.png`, fullPage: true })
}

test.describe(' закупка и честное «нет»', () => {
  test('S2: продление до КП с параметрами запроса', async ({ page }) => {
    await loginAs(page, PERSONA.buyerSmb)
    await page.goto('/')
    const body = await search(page, 'продлить касперский на 100 пк на 2 года')
    expect((body as unknown as { purchase?: { deal: string } }).purchase?.deal).toBe('renewal')

    for (const label of ['Kaspersky', 'Продление', '100 устройств', 'Срок 2 года'])
      await expect(page.getByRole('button', { name: `Снять условие «${label}»` })).toBeVisible()

    const first = page.getByRole('article').first()
    await expect(first).toContainText('Kaspersky Endpoint Security')
    const offer = first.getByTestId('selected-offer')
    await expect(offer).toContainText('Продление · 2 года · Стандартный')
    await expect(offer).toContainText('× 100 устройств')
    await expect(
      page.getByText('Какая редакция у вас сейчас: Стандартный или Расширенный?'),
    ).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Старшая редакция' })).toBeVisible()
    await expect(page.getByTestId('help-hint')).toContainText('Как продлить в кабинете')
    await shot(page, 's2-renewal')

    // Ответ на вопрос меняет вариант.
    await page.getByRole('button', { name: 'Расширенный', exact: true }).click()
    await expect(page.getByRole('article').first().getByTestId('selected-offer')).toContainText(
      'Продление · 2 года · Расширенный',
    )

    // В спецификацию → «Запросить КП»: в заявке исходный запрос, тип покупки, количество, срок, редакция.
    await page.getByRole('article').first().getByTestId('offer-spec').click()
    await page.getByRole('button', { name: /^Спецификация, позиций: 1/ }).click()
    const dialog = page.getByRole('dialog', { name: 'Спецификация' })
    await expect(dialog.getByTestId('spec-purchase')).toContainText(
      'продление · редакция Расширенный · 2 года',
    )
    let quote: Request | null = null
    await page.route('**/api/quotes', async (route) => {
      quote = route.request()
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        body: JSON.stringify({
          id: 'e2e',
          status: 'new',
          positions: 1,
          total_rub: 609000,
          on_request: 0,
        }),
      })
    })
    await dialog.getByRole('button', { name: 'Запросить КП' }).click()
    await expect.poll(() => quote !== null).toBe(true)
    const sent = (quote as unknown as Request).postDataJSON() as {
      items: {
        product_id: string
        quantity: number
        purchase?: Record<string, unknown>
      }[]
    }
    expect(sent.items[0]?.quantity).toBe(100)
    expect(sent.items[0]?.purchase).toMatchObject({
      query: 'продлить касперский на 100 пк на 2 года',
      deal: 'renewal',
      quantity: 100,
      duration_months: 24,
      edition: 'Расширенный',
    })
    await shot(page, 's2-spec')
  })

  test.describe('гость', () => {
    test.use({ storageState: GUEST })

    test('S3: аналог microsoft office — «Вы искали», реестр, «из реестра»', async ({ page }) => {
      await page.goto('/')
      await search(page, 'аналог microsoft office')
      const original = page.getByTestId('analog-original')
      await expect(original).toContainText('Вы искали:')
      await expect(original).toContainText('Microsoft Office 2024')
      const analogs = page
        .getByRole('region', { name: 'Аналоги' })
        .or(page.locator('section[aria-labelledby="analogs-title"]'))
      await expect(analogs.getByRole('article')).toHaveCount(2)
      await expect(analogs).not.toContainText('Microsoft Office 2024 для')
      await expect(
        analogs.getByTestId('reestr').filter({ hasText: 'В реестре российского ПО, №' }),
      ).toHaveCount(1)
      await expect(
        analogs.getByTestId('reestr').filter({ hasText: 'Статус в реестре не установлен' }),
      ).toHaveCount(1)
      await shot(page, 's3-analogs')

      await search(page, 'аналог microsoft office из реестра')
      const unknown = page.getByTestId('reestr-unconfirmed')
      await expect(unknown.getByRole('heading', { name: 'Статус не установлен' })).toBeVisible()
      await expect(unknown.getByRole('article')).toHaveCount(1)
      await shot(page, 's3-reestr')
    })

    test('S3: «как продлить лицензию» — ответ справки без товаров', async ({ page }) => {
      await page.goto('/')
      await search(page, 'как продлить лицензию')
      const help = page.getByTestId('help-answer')
      await expect(help.getByRole('heading', { name: 'Как продлить лицензию' })).toBeVisible()
      await expect(help.getByRole('link', { name: 'Как продлить в кабинете' })).toHaveAttribute(
        'href',
        '/help#renewal',
      )
      await expect(page.getByRole('article')).toHaveCount(0)
      await shot(page, 's3-help')
      await help.getByRole('link', { name: 'Как продлить в кабинете' }).click()
      await expect(page.getByRole('heading', { name: 'Помощь покупателю' })).toBeVisible()
      await expect(page.locator('#help-renewal')).toBeVisible()
    })

    test('отсутствующий товар: честное «нет» и «Запросить поставку» с исходным запросом', async ({
      page,
    }) => {
      await page.goto('/')
      await search(page, 'битрикс24')
      await expect(
        page.getByRole('heading', {
          name: 'Товара «битрикс24» нет в каталоге',
        }),
      ).toBeVisible()
      const sent = page.waitForRequest(
        (r) =>
          r.url().endsWith('/api/events') &&
          (r.postDataJSON() as { type: string }).type === 'supply_request',
      )
      await page.getByTestId('supply-request').click()
      expect(((await sent).postDataJSON() as { payload: { query: string } }).payload.query).toBe(
        'битрикс24',
      )
      await expect(page.getByTestId('supply-sent')).toBeVisible()
      await shot(page, 'absent-supply')
    })
  })

  test.describe('360 px', () => {
    test.use({ storageState: GUEST, viewport: { width: 360, height: 780 } })

    for (const q of [
      'продлить касперский на 100 пк на 2 года',
      'аналог microsoft office',
      'счёт',
      'битрикс24',
    ])
      test(`без горизонтальной прокрутки: ${q}`, async ({ page }) => {
        await page.goto('/')
        await search(page, q)
        const overflow = await page.evaluate(
          () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
        )
        expect(overflow).toBeLessThanOrEqual(0)
        if (q === 'продлить касперский на 100 пк на 2 года') await shot(page, 's2-360')
      })
  })
})
