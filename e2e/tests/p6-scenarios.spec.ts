// Контур фида на фиде store.softline.ru: сценарии защиты S2 и S3 (§6) и
// (допродажа с основанием A/B/C, акции, строка сравнения, ступень количества). Гостем: демо-покупатели
// на фиде скрыты. Демо-состояние не меняется: заявка КП перехватывается.
import type { Page, Request } from '@playwright/test'
import { GUEST, expect, search, test } from './fixtures'

interface Evidence {
  level: string
  kind: string
  title: string
  url: string
  doc_number: string
}
interface CompanionBody {
  product_id: string
  title: string
  vendor: string
  reason: string
  evidence?: Evidence
}
interface S6Body {
  target?: { type: string; title: string } | null
  companions?: { title?: string; items: CompanionBody[] } | null
  comparison?: { text: string; conditions: string; percent: number } | null
  items: {
    product_id: string
    title: string
    match_type?: string
    price_rub: number | null
    tier_min?: number
    tier_max?: number | null
    badges?: string[]
    promos?: { text: string; url: string }[]
  }[]
}

const DATE = /\b\d{1,2}\.\d{1,2}\.\d{4}\b|\b20\d\d-\d\d-\d\d\b/

async function noOverflow(page: Page) {
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
  expect(overflow, 'без горизонтальной прокрутки').toBeLessThanOrEqual(0)
}

async function pageText(page: Page) {
  return page.evaluate(() => document.body.innerText)
}

test.describe('сценарии S2 и S3 на фиде', () => {
  test.use({ storageState: GUEST })

  for (const vp of [
    { width: 1440, height: 900 },
    { width: 360, height: 780 },
  ]) {
    test(`S2 · ${vp.width}: «астра линукс» — цель, акция, «Дополните решение» с основанием A/B`, async ({
      page,
    }) => {
      await page.setViewportSize(vp)
      await page.goto('/')
      const body = (await search(page, 'астра линукс')) as unknown as S6Body

      // Цель — семейство Astra Linux Special Edition (эталон заказчика), карточек не меньше четырёх.
      expect(body.target).toMatchObject({ type: 'family', title: 'Astra Linux Special Edition' })
      await expect(page.getByRole('article').nth(3)).toBeVisible()

      // Первая карточка: бейдж скидки из фида и отдельной строкой — акция со ссылкой на условия.
      const first = page.locator('[data-match-group="exact"]').getByRole('article').first()
      await expect(first.getByTestId('discount-badge')).toHaveText(/^Скидка \d+%$/)
      const promo = first.getByTestId('promo')
      await expect(promo).toContainText('Акция:')
      await expect(promo).not.toContainText('условия по ссылке')
      const terms = promo.getByRole('link', { name: /условия/ })
      await expect(terms).toHaveAttribute('target', '_blank')
      await expect(terms).toHaveAttribute('href', /^https:\/\//)

      // Блок: заголовок из ответа, «Часто покупают вместе» нигде нет.
      const items = body.companions?.items ?? []
      const title = body.companions?.title || 'Дополните решение'
      const block = page.getByRole('region', { name: title })
      await expect(block).toBeVisible()
      await expect(page.getByText(/часто покупают/i)).toHaveCount(0)

      const card = (re: RegExp) => block.getByTestId('companion').filter({ hasText: re })
      // Техподдержка — B, из каталога, «к этой лицензии».
      const support = card(/Техподдержка к этой лицензии/)
      await expect(support).toHaveAttribute('data-evidence', 'B')
      await expect(support.getByTestId('evidence-badge')).toContainText('из каталога')
      // КриптоПро — A, сертификат Ready for Astra с номером, документ открывается в новой вкладке.
      const crypto = items.find((c) => /КриптоПро/.test(c.title))
      expect(crypto?.evidence).toMatchObject({ level: 'A', kind: 'compat_document' })
      expect(crypto?.evidence?.doc_number).toBeTruthy()
      const cp = card(/КриптоПро/)
      await expect(cp).toHaveAttribute('data-evidence', 'A')
      await expect(cp.getByTestId('evidence-badge')).toContainText('по документу вендора')
      await expect(cp.getByTestId('companion-reason')).toContainText(
        `сертификат Ready for Astra № ${crypto?.evidence?.doc_number}`,
      )
      const doc = cp.getByTestId('evidence-doc')
      await expect(doc).toHaveAttribute('href', crypto?.evidence?.url ?? '')
      await expect(doc).toHaveAttribute('target', '_blank')
      await expect(doc).toHaveAttribute('rel', /noopener/)
      // Kaspersky Endpoint Security и Кибер Бэкап — A по документации вендора, ссылка на документ.
      for (const re of [/Kaspersky Endpoint Security/, /Кибер Бэкап/]) {
        const c = card(re)
        await expect(c).toHaveAttribute('data-evidence', 'A')
        await expect(c.getByTestId('evidence-doc')).toHaveAttribute('href', /^https:\/\//)
      }
      // Экосистема вендора — B.
      await expect(card(/Из экосистемы/).first()).toHaveAttribute('data-evidence', 'B')
      // У каждой карточки — бейдж уровня и строка причины; ссылка на документ — только у A.
      const all = block.getByTestId('companion')
      await expect(all).toHaveCount(Math.min(items.length, 6))
      for (const c of await all.all()) {
        await expect(c.getByTestId('evidence-badge')).toBeVisible()
        await expect(c.getByTestId('companion-reason')).not.toBeEmpty()
        if ((await c.getAttribute('data-evidence')) !== 'A')
          await expect(c.getByTestId('evidence-doc')).toHaveCount(0)
      }

      const text = await pageText(page)
      expect(text).not.toMatch(DATE)
      expect(text).not.toMatch(/(^|[^\d\s ])[\s ]0[\s ]₽/)
      await noOverflow(page)
    })

    test(`S3 · ${vp.width}: продление на 100 пк — строка сравнения и итог «за узел × 100»`, async ({
      page,
    }) => {
      await page.setViewportSize(vp)
      await page.goto('/')
      const body = (await search(page, 'продлить касперский на 100 пк на 2 года')) as unknown as S6Body

      // Строка сравнения: текст и условия ступени, без оценок и дат.
      expect(body.comparison?.percent).toBe(31)
      const cmp = page.getByTestId('comparison')
      await expect(cmp).toContainText('Стандартный на 31% дешевле, чем Расширенный, за узел')
      await expect(cmp.getByTestId('comparison-conditions')).toContainText('100–149 устройств')
      await expect(cmp).not.toContainText(/лучше|выгодн/i)

      // Первая точная карточка — KES Стандартный, ступень 100–149: цена за узел × 100 мест, а не один пакет.
      const top = body.items.find((i) => i.match_type === 'exact')
      expect(top).toMatchObject({ tier_min: 100, tier_max: 149, price_rub: 3070.8 })
      const first = page.locator('[data-match-group="exact"]').getByRole('article').first()
      await expect(first).toContainText('Стандартный')
      await expect(first.getByTestId('per-place')).toHaveText('за узел')
      const offer = first.getByTestId('selected-offer')
      await expect(offer).toContainText('ступень 100–149 устройств')
      await expect(offer).toContainText(/3\s070,80\s₽ за узел × 100 устройств = 307\s080\s₽/)

      // «Запросить КП» в одно нажатие из спецификации: в заявке 100 мест.
      await offer.getByTestId('offer-spec').click()
      await page.getByRole('button', { name: /^Спецификация, позиций: 1/ }).click()
      const dialog = page.getByRole('dialog', { name: 'Спецификация' })
      let quote: Request | null = null
      await page.route('**/api/quotes', async (route) => {
        quote = route.request()
        await route.fulfill({
          status: 201,
          contentType: 'application/json',
          body: JSON.stringify({ id: 'e2e', status: 'new', positions: 1, total_rub: 1, on_request: 0 }),
        })
      })
      await dialog.getByLabel('Контакт для ответа — почта или телефон').fill('buyer@example.com')
      await dialog.getByRole('button', { name: 'Запросить КП' }).click()
      await expect.poll(() => quote !== null).toBe(true)
      const sent = (quote as unknown as Request).postDataJSON() as {
        items: { product_id: string; quantity: number }[]
      }
      expect(sent.items[0]).toMatchObject({ product_id: top?.product_id, quantity: 100 })

      expect(await pageText(page)).not.toMatch(DATE)
      await noOverflow(page)
    })
  }

  test('Dr.Web: акция «Акция: −50% …» со ссылкой «условия» отдельно от бейджа скидки', async ({ page }) => {
    await page.goto('/')
    const body = (await search(page, 'dr.web')) as unknown as S6Body
    const withPromo = body.items.find((i) => (i.promos ?? []).some((p) => /50%/.test(p.text)))
    expect(withPromo, 'у Dr.Web в выдаче есть акция −50%').toBeTruthy()
    const card = page.locator(`article[data-product="${withPromo?.product_id}"]`)
    const promo = card.getByTestId('promo')
    await expect(promo).toContainText(/Акция: −50%/)
    await expect(promo.getByRole('link', { name: /условия/ })).toHaveAttribute('target', '_blank')
    // Акция — не бейдж: в строке бейджей её нет.
    await expect(card.getByTestId('discount-badge').filter({ hasText: /Акция/ })).toHaveCount(0)
    expect(await pageText(page)).not.toMatch(DATE)
  })
})
