// Контраст пар текст/фон темы «Ночь» (src/styles/tokens.css): текст ≥ 4,5:1, контуры контролов ≥ 3:1.
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const css = readFileSync(new URL('../src/styles/tokens.css', import.meta.url), 'utf8')
const tokens = new Map<string, string>()
for (const m of css.matchAll(/--([\w-]+):\s*(#[0-9a-fA-F]{6})\s*;/g)) tokens.set(m[1], m[2])

function hex(name: string): string {
  const v = tokens.get(name)
  assert.ok(v, `нет токена --${name}`)
  return v
}

function luminance(color: string): number {
  const n = parseInt(color.slice(1), 16)
  const [r, g, b] = [n >> 16, (n >> 8) & 255, n & 255].map((c) => {
    const s = c / 255
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

function ratio(a: string, b: string): number {
  const [x, y] = [luminance(hex(a)), luminance(hex(b))]
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05)
}

const surfaces = ['bg', 'surface', 'surface-2', 'surface-3']

test('основной, вторичный и третичный текст на всех поверхностях ≥ 4,5:1', () => {
  for (const fg of ['text', 'text-2', 'text-3']) {
    for (const bg of surfaces) {
      assert.ok(ratio(fg, bg) >= 4.5, `--${fg} на --${bg}: ${ratio(fg, bg).toFixed(2)}`)
    }
  }
})

test('текст на акценте и смысловые цвета на своих подложках ≥ 4,5:1', () => {
  assert.ok(ratio('on-accent', 'accent') >= 4.5)
  assert.ok(ratio('on-accent', 'accent-hover') >= 4.5)
  const pairs: [string, string][] = [
    ['route-a', 'route-a-bg'],
    ['route-b', 'route-b-bg'],
    ['route-c', 'route-c-bg'],
    ['route-d', 'route-d-bg'],
    ['status-pending', 'status-pending-bg'],
    ['status-accepted', 'status-accepted-bg'],
    ['status-rejected', 'status-rejected-bg'],
    ['status-rolled-back', 'status-rolled-back-bg'],
    ['danger', 'danger-bg'],
  ]
  for (const [fg, bg] of pairs) assert.ok(ratio(fg, bg) >= 4.5, `--${fg} на --${bg}`)
  for (const fg of ['delta-up', 'delta-down', 'danger']) {
    for (const bg of surfaces) assert.ok(ratio(fg, bg) >= 4.5, `--${fg} на --${bg}`)
  }
})

test('контур контролов и фокус ≥ 3:1 к фону и панели', () => {
  for (const bg of ['bg', 'surface', 'surface-2']) {
    assert.ok(ratio('border-control', bg) >= 3, `--border-control на --${bg}`)
    assert.ok(ratio('focus-ring', bg) >= 3, `--focus-ring на --${bg}`)
  }
})
