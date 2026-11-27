// Прогон помощника по выбору через интерфейс (витрина localhost:5177): тупики, повторы ответа, ошибки, ожидание.
// node scripts/eval/assistant_ui_sweep.mjs scripts/eval/assistant_ui_dialogs.json <папка отчёта> [ширина]
// Флаги хода: REPEAT — ответ слово в слово как прошлый; DEAD_END — ни карточек, ни кнопок; STUCK_WAIT — ответа нет 25 с.
import { createRequire } from 'node:module'
import fs from 'node:fs'
const require = createRequire(new URL('../../e2e/package.json', import.meta.url))
const { chromium } = require('@playwright/test')
const dialogs = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'))
const outDir = process.argv[3]
const width = Number(process.argv[4] || 1440)
const browser = await chromium.launch()
const ctx = await browser.newContext({ viewport: { width, height: 900 }, extraHTTPHeaders: { 'X-Test-Run': 'e2e' } })
const report = []
for (const [di, d] of dialogs.entries()) {
  const page = await ctx.newPage()
  await page.goto('http://localhost:5177/search' + (d.q ? '?q=' + encodeURIComponent(d.q) : ''))
  await page.evaluate(() => sessionStorage.clear())
  await page.reload()
  await page.waitForTimeout(1200)
  await page.getByRole('button', { name: /Помощник по выбору/ }).first().click()
  const panel = page.getByTestId('assistant-panel')
  await panel.waitFor()
  let prev = ''
  const turns = []
  for (const [ti, step] of d.steps.entries()) {
    const before = await panel.getByTestId('assistant-message').count()
    const errBefore = await panel.getByTestId('assistant-error').count()
    let label = step
    try {
      if (typeof step === 'string') {
        await panel.getByTestId('assistant-input').fill(step)
        await panel.getByTestId('assistant-send').click()
      } else if (step.sugg !== undefined) {
        const s = panel.getByTestId('assistant-message').last().locator('[data-testid="assistant-suggestions"] button, [data-testid="assistant-question"] button')
        const all = (await s.count()) ? s : panel.locator('[data-testid="assistant-suggestions"] button')
        const n = await all.count()
        if (!n) { turns.push({ step: 'sugg#' + step.sugg, flags: ['NO_SUGGESTION_TO_CLICK'] }); continue }
        const b = all.nth(Math.min(step.sugg, n - 1)); label = 'кнопка: ' + (await b.innerText())
        await b.click()
      } else if (step.more) {
        const m = panel.getByTestId('assistant-more').last()
        if (!(await m.count())) { turns.push({ step: 'more', flags: ['NO_MORE_BUTTON'] }); continue }
        label = 'кнопка: ' + (await m.innerText()); await m.click(); await page.waitForTimeout(1500)
        turns.push({ step: label, flags: [], url: decodeURIComponent(page.url()), panelOpen: await panel.isVisible() }); continue
      } else if (step.spec) {
        const a = panel.getByTestId('assistant-message').last().getByTestId('assistant-action').first()
        if (!(await a.count())) { turns.push({ step: 'spec', flags: ['NO_SPEC_BUTTON'] }); continue }
        label = 'кнопка: ' + (await a.innerText()); await a.click(); await page.waitForTimeout(1500)
        turns.push({ step: label, flags: [], after: (await panel.getByTestId('assistant-feed').innerText()).slice(-200) }); continue
      }
    } catch (e) { turns.push({ step: label, flags: ['ACTION_FAILED ' + String(e).slice(0, 120)] }); continue }
    const t0 = Date.now(); let flags = []
    while (true) {
      const n = await panel.getByTestId('assistant-message').count()
      const e = await panel.getByTestId('assistant-error').count()
      const w = await panel.getByTestId('assistant-wait').count()
      if ((n > before || e > errBefore) && !w) break
      if (Date.now() - t0 > 25000) { flags.push('STUCK_WAIT'); break }
      await page.waitForTimeout(250)
    }
    const ms = Date.now() - t0
    if (ms > 8000) flags.push('SLOW ' + ms + 'ms')
    if ((await panel.getByTestId('assistant-error').count()) > errBefore) flags.push('ERROR ' + (await panel.getByTestId('assistant-error').last().innerText()).slice(0, 80))
    const m = panel.getByTestId('assistant-message').last()
    const text = (await m.count()) ? (await m.innerText()) : ''
    const status = (await m.count()) ? await m.getAttribute('data-status') : ''
    const cards = await m.getByTestId('assistant-card').count()
    const sugg = await m.locator('[data-testid="assistant-suggestions"] button').count()
    const more = await m.getByTestId('assistant-more').count()
    const q = await m.getByTestId('assistant-question').count()
    const head = text.split('\n').filter(Boolean).slice(0, 3).join(' | ')
    if (head && head === prev) flags.push('REPEAT')
    if (!cards && !sugg && !more && !q) flags.push('DEAD_END')
    if (/слева|справа|выше|ниже выдачи/.test(text)) flags.push('LOCATION_WORD')
    if (/Не нашёл лицензию/.test(text)) flags.push('NO_OFFERS')
    prev = head
    turns.push({ step: label, ms, status, cards, sugg, more, q, head: head.slice(0, 220), flags })
  }
  await page.screenshot({ path: `${outDir}/d${String(di).padStart(2, '0')}.png`, fullPage: false })
  report.push({ id: d.id, q: d.q || '', turns })
  console.log(`${d.id}: ${turns.map(t => t.flags.length ? '✗' : '✓').join('')}`)
  await page.close()
}
fs.writeFileSync(`${outDir}/report.json`, JSON.stringify(report, null, 1))
await browser.close()
