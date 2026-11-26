// Контур доводки, на фиде store.softline.ru (контур фида, S3, S4): закупка, сервисный запрос и честное «нет».
// Демо-покупатели на фиде скрыты (их клиентов нет) — сценарии идут гостем. Демо-состояние не
// меняется: заявка КП и события перехватываются (тело проверяется, в базу не пишется). E2E_SCREENS=1 — снимки в
// artifacts/screens/p4/.
import type { Page, Request } from '@playwright/test'
import { GUEST, expect, search, test } from './fixtures'

const OUT = '../artifacts/screens/p4'
const SHOTS = process.env.E2E_SCREENS === '1'
const shot = async (page: Page, name: string) => {
  if (SHOTS) await page.screenshot({ path: `${OUT}/${name}.png`, fullPage: true })
}

test.describe('закупка и честное «нет»', () => {
  test.describe('гость', () => {
    test.use({ storageState: GUEST })

    test('S3: продление до КП с параметрами запроса', async ({ page }) => {
      await page.goto('/')
      const body = (await search(page, 'продлить касперский на 100 пк на 2 года')) as unknown as {
        purchase?: { deal: string; quantity: number; duration_months: number }
        items: { match_type?: string; license_line: string; title: string }[]
      }
      expect(body.purchase).toMatchObject({ deal: 'renewal', quantity: 100, duration_months: 24 })

      for (const label of ['«Лаборатория Касперского»', 'Продление', '100 устройств', 'Срок 2 года'])
        await expect(page.getByRole('button', { name: `Снять условие «${label}»` })).toBeVisible()

      // Жёсткое условие не ослабляется: все точные карточки — продления на 2 года.
      const exact = body.items.filter((i) => i.match_type === 'exact')
      expect(exact.length).toBeGreaterThan(0)
      for (const it of exact) expect(it.license_line, it.title).toMatch(/^Продление.*2 года/)

      const first = page.locator('[data-match-group="exact"]').getByRole('article').first()
      await expect(first).toContainText('Kaspersky')
      const offer = first.getByTestId('selected-offer')
      await expect(offer).toContainText('Продление · 2 года')
      await shot(page, 's2-renewal')

      // В спецификацию → «Запросить КП»: в заявке исходный запрос, тип покупки, количество и срок.
      await offer.getByTestId('offer-spec').click()
      await page.getByRole('button', { name: /^Спецификация, позиций: 1/ }).click()
      const dialog = page.getByRole('dialog', { name: 'Спецификация' })
      await expect(dialog.getByTestId('spec-purchase')).toContainText('продление')
      await expect(dialog.getByTestId('spec-purchase')).toContainText('2 года')
      let quote: Request | null = null
      await page.route('**/api/quotes', async (route) => {
        quote = route.request()
        await route.fulfill({
          status: 201,
          contentType: 'application/json',
          body: JSON.stringify({ id: 'e2e', status: 'new', positions: 1, total_rub: 1, on_request: 0 }),
        })
      })
      // Гостю нужен контакт для ответа (синтетический адрес).
      await dialog.getByLabel('Контакт для ответа — почта или телефон').fill('buyer@example.com')
      await dialog.getByRole('button', { name: 'Запросить КП' }).click()
      await expect.poll(() => quote !== null).toBe(true)
      const sent = (quote as unknown as Request).postDataJSON() as {
        items: { product_id: string; quantity: number; purchase?: Record<string, unknown> }[]
      }
      expect(sent.items[0]?.purchase).toMatchObject({
        query: 'продлить касперский на 100 пк на 2 года',
        deal: 'renewal',
        quantity: 100,
        duration_months: 24,
      })
      await shot(page, 's2-spec')
    })

    test('S4: «windows server» → первым по запросу с «Запросить цену», ниже «Можно купить сейчас»', async ({
      page,
    }) => {
      await page.goto('/')
      const body = (await search(page, 'windows server')) as unknown as {
        items: { match_type?: string; price_state?: string; title: string }[]
      }
      const first = page.locator('[data-match-group="exact"]').getByRole('article').first()
      await expect(first).toContainText('Windows Server')
      await expect(first.getByTestId('card-price')).toHaveText('Цена по запросу')
      await expect(first.getByRole('button', { name: 'Запросить цену' })).toBeVisible()
      // «Можно купить сейчас» — карточки alternative с ценой и подписью без «аналог».
      expect(
        body.items.filter((i) => i.match_type === 'alternative').length,
        'S4: в ответе нет карточек alternative («Можно купить сейчас»)',
      ).toBeGreaterThan(0)
      const alt = page.locator('[data-match-group="alternative"]')
      await expect(alt.getByRole('heading', { name: 'Можно купить сейчас' })).toBeVisible()
      await expect(alt.getByTestId('card-price').first()).not.toHaveText('Цена по запросу')
      await expect(alt.getByTestId('alternative-note').first()).toContainText(
        'Совместимость с вашими приложениями нужно проверить',
      )
      await expect(alt).not.toContainText('Аналог')
      await shot(page, 's4-buy-now')
    })

    test('сервисный запрос «доставка» → цель «Помощь менеджера», ниже «Популярное в наличии» отдельно', async ({
      page,
    }) => {
      await page.goto('/')
      const body = (await search(page, 'доставка')) as unknown as {
        target?: { type: string } | null
        items: { match_type?: string }[]
      }
      expect(body.target?.type).toBe('service')
      expect(body.items.every((i) => i.match_type === 'fallback')).toBe(true)
      await expect(page.getByRole('heading', { name: /«доставка» товаров нет/ })).toBeVisible()
      const fallback = page.locator('[data-match-group="fallback"]')
      await expect(fallback.getByRole('heading', { name: 'Популярное в наличии' })).toBeVisible()
      await expect(fallback).toContainText('Это не результат поиска по запросу')
      await expect(page.locator('[data-match-group]')).toHaveCount(1)
      await shot(page, 's3-service')
      await page.getByTestId('target-manager').click()
      await expect(page.getByRole('dialog', { name: 'Спросить менеджера' })).toBeVisible()
    })

    test('отсутствующий товар: честное «нет» и «Запросить поставку» с исходным запросом', async ({
      page,
    }) => {
      await page.goto('/')
      // «битрикс24» в фиде есть (1С-Битрикс24) — ищется, а не «нет в каталоге».
      await search(page, 'битрикс24')
      await expect(page.getByRole('article').filter({ hasText: 'Битрикс24' }).first()).toBeVisible()
      await expect(page.getByRole('heading', { name: /нет в каталоге/ })).toHaveCount(0)

      // «figma» в фиде нет: честное «нет», поставка и «Популярное в наличии» не выдаётся за найденное.
      const body = await search(page, 'figma')
      expect(body.total).toBe(0)
      await expect(page.getByRole('heading', { name: 'Товара «figma» нет в каталоге' })).toBeVisible()
      await expect(
        page.locator('[data-match-group="fallback"]').getByRole('heading', { name: 'Популярное в наличии' }),
      ).toBeVisible()
      const sent = page.waitForRequest(
        (r) =>
          r.url().endsWith('/api/events') &&
          (r.postDataJSON() as { type: string }).type === 'supply_request',
      )
      await page.getByTestId('supply-request').click()
      expect(((await sent).postDataJSON() as { payload: { query: string } }).payload.query).toBe('figma')
      await expect(page.getByTestId('supply-sent')).toBeVisible()
      await shot(page, 'absent-supply')
    })
  })

  test.describe('360 px', () => {
    test.use({ storageState: GUEST, viewport: { width: 360, height: 780 } })

    for (const q of [
      'продлить касперский на 100 пк на 2 года',
      'windows server',
      'доставка',
      'figma',
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
