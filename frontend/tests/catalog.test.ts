// Витрина без DOM (src/lib/catalog.ts): временный поиск, фильтр категории, цена.
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  filterProducts,
  formatPrice,
  normalize,
  parseCategory,
  type Category,
  type Product,
} from '../src/lib/catalog.ts'

const categories: Category[] = [
  { id: 'home', title: 'Дом и кухня' },
  { id: 'sport', title: 'Спорт' },
]
const products: Product[] = [
  { id: 'a', title: 'Чайник электрический', category: 'home', price: 2290 },
  { id: 'b', title: 'Коврик для йоги', category: 'sport', price: 1290 },
  { id: 'c', title: 'Плед зелёный', category: 'home', price: 1490 },
]
const ids = (list: Product[]) => list.map((p) => p.id)

test('normalize: регистр, ё и пробелы', () => {
  assert.equal(normalize('  Зелёный   ПЛЕД '), 'зеленый плед')
})

test('filterProducts: пустой запрос — все товары', () => {
  assert.deepEqual(ids(filterProducts(products, categories, '   ')), ['a', 'b', 'c'])
})

test('filterProducts: каждое слово должно найтись в названии или категории', () => {
  assert.deepEqual(ids(filterProducts(products, categories, 'чайник')), ['a'])
  assert.deepEqual(ids(filterProducts(products, categories, 'кухня плед')), ['c'])
  assert.deepEqual(ids(filterProducts(products, categories, 'плед йоги')), [])
})

test('filterProducts: ё и е не различаются', () => {
  assert.deepEqual(ids(filterProducts(products, categories, 'зеленый')), ['c'])
})

test('filterProducts: категория сужает выдачу', () => {
  assert.deepEqual(ids(filterProducts(products, categories, '', 'home')), ['a', 'c'])
  assert.deepEqual(ids(filterProducts(products, categories, 'коврик', 'home')), [])
})

test('parseCategory: только известные категории', () => {
  assert.equal(parseCategory('sport', categories), 'sport')
  assert.equal(parseCategory('cars', categories), null)
  assert.equal(parseCategory(['sport'], categories), null)
  assert.equal(parseCategory(undefined, categories), null)
})

test('formatPrice: рубли без копеек, разряды через неразрывный пробел', () => {
  assert.equal(formatPrice(6990).replace(/\s/g, ' '), '6 990 ₽')
})
