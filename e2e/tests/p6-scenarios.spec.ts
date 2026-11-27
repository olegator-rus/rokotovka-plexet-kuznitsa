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
      // KES и Кибер Бэкап — A по документации вендора, ссылка на документ (S2 Кибер Бэкап с документом).
      for (const re of [/Kaspersky Endpoint Security/, /Кибер Бэкап/]) {
        const c = card(re).first()
        await expect(c).toHaveAttribute('data-evidence', 'A')
        await expect(c.getByTestId('evidence-doc')).toHaveAttribute('href', /^https:\/\//)
      }
      // У каждой карточки — бейдж уровня и строка причины; ссылка на документ — только у A.
      const all = block.getByTestId('companion')
      await expect(all).toHaveCount(Math.min(items.length, 6))
      for (const c of await all.all()) {
        await expect(c.getByTestId('evidence-badge')).toBeVisible()
        await expect(c.getByTestId('companion-reason').first()).not.toBeEmpty()
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
      await expect(offer).toContainText('Такая цена при заказе 100–149 устройств')
      await expect(offer).toContainText(/Лицензия на одно устройство —\s*3\s070,80\s₽/)
      await expect(offer.getByTestId('offer-total')).toContainText(/100 × 3\s070,80\s₽ =\s*307\s080\s₽/)

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

  test('«кибер бэкап»: вид class_logic — «по логике класса: <причина>», без ссылки и без «из каталога»', async ({
    page,
  }) => {
    await page.goto('/')
    const body = (await search(page, 'кибер бэкап')) as unknown as S6Body
    const logic = (body.companions?.items ?? []).find((c) => c.evidence?.kind === 'class_logic')
    expect(logic, 'в блоке есть связь из таблицы классов').toBeTruthy()
    const card = page.getByTestId('companion').filter({ hasText: logic?.title.slice(0, 30) ?? '' }).first()
    const badge = card.getByTestId('evidence-badge')
    await expect(badge).toContainText(`по логике класса: ${logic?.reason.charAt(0).toLowerCase()}`)
    await expect(badge).not.toContainText('из каталога')
    await expect(card.getByTestId('evidence-doc')).toHaveCount(0)
    // Бейдж скидки у карточки блока — как на выдаче.
    const discounted = (body.companions?.items ?? []).find((c) =>
      ((c as { badges?: string[] }).badges ?? []).some((x) => /Скидка/.test(x)),
    )
    if (discounted)
      await expect(
        page
          .getByTestId('companion')
          .filter({ hasText: discounted.title.slice(0, 30) })
          .getByTestId('companion-badge')
          .filter({ hasText: /^Скидка \d+%$/ }),
      ).toHaveCount(1)
    expect(await pageText(page)).not.toMatch(DATE)
  })

  for (const vp of [
    { width: 1440, height: 900 },
    { width: 360, height: 780 },
  ]) {
    test(`панель «аст» · ${vp.width}: «Вместе с этим» внизу панели, нажатие — как на товар`, async ({ page }) => {
      await page.setViewportSize(vp)
      await page.goto('/')
      const box = page.getByRole('combobox', { name: 'Поиск по каталогу' })
      await box.click()
      const [resp] = await Promise.all([
        page.waitForResponse((r) => r.url().includes('/api/suggest?') && r.url().includes('q=%D0%B0%D1%81%D1%82')),
        box.pressSequentially('аст', { delay: 30 }),
      ])
      const sug = (await resp.json()) as {
        products: { family_id: string; family_name: string }[]
        together: { anchor_family_id: string; family_name: string; title: string; price_rub: number; level: string }[]
      }
      expect(sug.products[0]?.family_name).toMatch(/Astra Linux/)
      expect(sug.together.length).toBeGreaterThan(0)
      expect(sug.together.every((t) => t.anchor_family_id === sug.products[0]?.family_id)).toBe(true)

      const list = page.getByRole('listbox', { name: 'Подсказки поиска' })
      const together = list.getByTestId('panel-together')
      await expect(together).toContainText('Вам может быть интересно')
      const rows = together.getByRole('option')
      await expect(rows).toHaveCount(Math.min(sug.together.length, 2))
      const first = sug.together[0]!
      await expect(rows.first()).toContainText(first.family_name)
      await expect(rows.first()).toContainText('₽')
      expect(await together.innerText()).not.toMatch(DATE)
      await noOverflow(page)

      // Нажатие — поиск по позиции, как у товара панели.
      const [req] = await Promise.all([
        page.waitForRequest((r) => r.url().endsWith('/api/search') && r.method() === 'POST'),
        rows.first().click(),
      ])
      expect((req.postDataJSON() as { query: string }).query).toBe(first.title || first.family_name)
    })
  }

  test('панель «rfcg»: раскладка исправлена — «Показаны варианты для «касп»»', async ({ page }) => {
    await page.goto('/')
    const box = page.getByRole('combobox', { name: 'Поиск по каталогу' })
    await box.click()
    await box.pressSequentially('rfcg', { delay: 30 })
    await expect(page.getByRole('listbox', { name: 'Подсказки поиска' }).getByText('Показаны варианты для «касп»')).toBeVisible()
    await expect(box).toHaveValue('rfcg')
  })
})
