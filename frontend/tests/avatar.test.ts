// У каждой персоны со списком фото есть файл; без фото и без входа — инициалы.
import test from 'node:test'
import assert from 'node:assert/strict'
import { existsSync, readdirSync } from 'node:fs'
import { personaPhoto } from '../src/lib/avatar.ts'

test('файлы в public/avatars и список персон с фото совпадают', () => {
  const files = readdirSync(new URL('../public/avatars/', import.meta.url))
  assert.equal(files.length, 7)
  for (const f of files) {
    const src = personaPhoto(f.replace('.png', ''))
    assert.ok(src, `файл без персоны: ${f}`)
    assert.ok(existsSync(new URL(`../public${src}`, import.meta.url)))
  }
})

test('персона без фото и гость — без картинки', () => {
  assert.equal(personaPhoto('no-such-persona'), null)
  assert.equal(personaPhoto(''), null)
  assert.equal(personaPhoto(null), null)
})
