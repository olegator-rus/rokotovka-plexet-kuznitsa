// Скриншоты входа по ролям в artifacts/screens/roles/, 1440 и 360 px: экран входа, меню и смена
// демо-аккаунта, витрина гостя и покупателя, стартовый экран каждой из четырёх ролей, отказ по праву. Только по
// E2E_SCREENS=1 (снимки — артефакт, не проверка); демо-состояние не меняется.
import type { Page } from '@playwright/test'
import { GUEST, PERSONA, expect, loginAs, resultCards, search, test } from './fixtures'

const OUT = '../artifacts/screens/roles'
const WIDTHS = [
  { w: 1440, h: 900 },
  { w: 360, h: 780 },
] as const

test.skip(process.env.E2E_SCREENS !== '1', 'снимки — только по E2E_SCREENS=1')
test.use({ storageState: GUEST })
test.setTimeout(120_000)

async function shot(page: Page, name: string, w: number, fullPage = false) {
  await page.waitForLoadState('networkidle').catch(() => undefined)
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth - window.innerWidth,
  )
  expect(overflow, `${name}-${w}: горизонтальная прокрутка`).toBeLessThanOrEqual(0)
  await page.screenshot({ path: `${OUT}/${name}-${w}.png`, fullPage })
}

for (const { w, h } of WIDTHS) {
  test.describe(`${w} px`, () => {
    test.use({ viewport: { width: w, height: h } })

    test(`вход и демо-аккаунты ${w}`, async ({ page }) => {
      await page.goto('/login')
      await expect(page.getByRole('button', { name: /Войти как Сергей Орлов/ })).toBeVisible()
      await shot(page, '01-login', w, true)
    })

    test(`витрина гостя ${w}`, async ({ page }) => {
      await page.goto('/search')
      await search(page, 'продлить антивирус')
      if (w < 1024) await resultCards(page).first().scrollIntoViewIfNeeded()
      await shot(page, '02-storefront-guest', w)
    })

    test(`витрина покупателя ${w}`, async ({ page }) => {
      await loginAs(page, PERSONA.buyerSmb)
      await page.goto('/search')
      await search(page, 'продлить антивирус')
      if (w < 1024) await resultCards(page).first().scrollIntoViewIfNeeded()
      await shot(page, '03-storefront-buyer-client-a', w)
    })

    for (const [persona, name, file] of [
      [PERSONA.ecom, 'Илья Ветров', '04-start-ecom'],
      [PERSONA.product, 'Дарья Лапина', '05-start-product'],
      [PERSONA.content, 'Елена Сорокина', '06-start-content'],
      [PERSONA.head, 'Сергей Орлов', '07-start-head'],
    ] as const) {
      test(`стартовый экран ${persona} ${w}`, async ({ page }) => {
        await page.goto('/login')
        await page.getByRole('button', { name: new RegExp(`Войти как ${name}`) }).click()
        await expect(page).not.toHaveURL(/\/login/)
        await expect(page.locator('main h1').first()).toBeVisible({ timeout: 30_000 })
        await page.waitForTimeout(800)
        await shot(page, file, w)
      })
    }

    test(`меню и смена демо-аккаунта ${w}`, async ({ page }) => {
      await loginAs(page, PERSONA.head)
      await page.goto('/analytics')
      await expect(page.locator('main h1').first()).toBeVisible()
      await page
        .getByRole('banner')
        .getByRole('button', { name: /Аккаунт: Сергей Орлов/ })
        .click()
      await expect(page.getByRole('menuitem', { name: 'Сменить демо-аккаунт' })).toBeVisible()
      await page.waitForTimeout(300)
      await shot(page, '08-account-menu', w)
      await page.getByRole('menuitem', { name: 'Сменить демо-аккаунт' }).click()
      const dialog = page.getByRole('dialog', { name: 'Сменить демо-аккаунт' })
      await expect(dialog.getByRole('button', { name: /Войти как Дарья Лапина/ })).toBeVisible()
      await page.waitForTimeout(600) // анимация появления диалога
      await shot(page, '09-switch-account', w)
    })

    test(`отказ по праву ${w}`, async ({ page }) => {
      await loginAs(page, PERSONA.content)
      const resp = await page.request.get('/api/proposals?status=pending&role=product')
      const items = ((await resp.json()) as { items: { id: string; action: string }[] }).items
      const id = items.find((p) => p.action === 'relation_patch')?.id
      test.skip(!id, 'нет ждущей правки связей роли product')
      await page.goto(`/searchops?tab=queue&status=pending&p=${id}`)
      const card = page.locator(`#proposal-${id}`)
      await expect(card.getByText('Это решение принимает продукт-менеджер')).toBeVisible({
        timeout: 15_000,
      })
      await card.getByRole('button', { name: 'Принять' }).scrollIntoViewIfNeeded()
      await page.mouse.wheel(0, 200)
      await shot(page, '10-deny-content', w)
    })
  })
}
