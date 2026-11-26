// У каждого производителя mock-каталога есть значок, и файл значка лежит в public/vendors.
import test from 'node:test'
import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'
import { vendorLogo } from '../src/lib/vendorLogo.ts'

const vendors: { name: string }[] = JSON.parse(
  readFileSync(new URL('../../data/mock/vendors.json', import.meta.url), 'utf8'),
)

test('значок есть у каждого производителя каталога', () => {
  for (const v of vendors) {
    const src = vendorLogo(v.name)
    assert.ok(src, `нет значка: ${v.name}`)
    assert.ok(existsSync(new URL(`../public${src}`, import.meta.url)), `нет файла ${src}`)
  }
})

test('незнакомый производитель и пустое имя — без значка', () => {
  assert.equal(vendorLogo('Неизвестный'), null)
  assert.equal(vendorLogo(''), null)
  assert.equal(vendorLogo(null), null)
})
