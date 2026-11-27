// Помощник по выбору (src/lib/assistant.ts): как открыли панель, что уходит серверу, стартовые подсказки,
// строка после добавления, хранение разговора.
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  addedLine,
  isDetailedQuery,
  lastAssistantId,
  loadAssistant,
  openPlan,
  saveAssistant,
  starterPrompts,
  turnBody,
  type ChatEntry,
} from '../src/lib/assistant.ts'

test('открытие: короткий запрос — приветствие, развёрнутый — в поле ввода, приглашение — отправить сразу', () => {
  assert.deepEqual(openPlan('button', 'антивирус', false), { send: '', prefill: '' })
  assert.deepEqual(openPlan('button', 'нужен антивирус на 20 компьютеров', false), {
    send: '',
    prefill: 'нужен антивирус на 20 компьютеров',
  })
  // Разговор уже идёт — кнопка просто показывает его, ничего не подставляет.
  assert.deepEqual(openPlan('button', 'нужен антивирус на 20 компьютеров', true), {
    send: '',
    prefill: '',
  })
  assert.deepEqual(openPlan('invite', 'антивирус', false), { send: 'антивирус', prefill: '' })
  assert.deepEqual(openPlan('product', 'Astra Linux Special Edition', false), {
    send: '',
    prefill: '',
  })
  assert.equal(isDetailedQuery('kes 50'), true)
  assert.equal(isDetailedQuery(''), false)
})

test('ход: первая реплика — запрос без состояния, дальше — реплика или чип с состоянием', () => {
  assert.deepEqual(turnBody({ text: ' самый дешёвый криптопро ' }, null), {
    query: 'самый дешёвый криптопро',
  })
  const st = { selection_id: 's1' }
  assert.deepEqual(turnBody({ text: 'на 50' }, st, 'kes на 30'), {
    query: 'kes на 30',
    message: 'на 50',
    state: st,
  })
  assert.deepEqual(turnBody({ chip: { key: 'variant', value: 'Орел' } }, st), {
    query: '',
    chip: { key: 'variant', value: 'Орел' },
    state: st,
  })
})

test('стартовые подсказки по контексту', () => {
  assert.equal(starterPrompts('').length, 4)
  assert.equal(starterPrompts('антивирус')[0]?.label, 'Подобрать антивирус для офиса')
  assert.equal(
    starterPrompts('Astra Linux Special Edition')[0]?.label,
    'Сравнить уровни защищённости',
  )
  assert.match(starterPrompts('КриптоПро CSP 5.0')[0]?.message ?? '', /КриптоПро/)
})

test('строка после добавления: позиции и сумма; без суммы — только позиции', () => {
  assert.equal(
    addedLine(1, 197010).replace(/\s/g, ' '),
    'Добавлено в спецификацию · 1 позиция · 197 010 ₽',
  )
  assert.equal(addedLine(3, null), 'Добавлено в спецификацию · 3 позиции')
})

test('хранение: битое — пустой разговор; сохранённое читается; последнее сообщение помощника', () => {
  const box = new Map<string, string>()
  const storage = {
    getItem: (k: string) => box.get(k) ?? null,
    setItem: (k: string, v: string) => void box.set(k, v),
  }
  assert.deepEqual(loadAssistant(storage).entries, [])
  box.set('plexet.assistant.v1', '{')
  assert.deepEqual(loadAssistant(storage).entries, [])
  const entries: ChatEntry[] = [
    { id: 1, kind: 'user', text: 'привет' },
    { id: 2, kind: 'system', text: 'Добавлено' },
  ]
  saveAssistant(storage, { entries, state: { selection_id: 'x' }, product: '' })
  const back = loadAssistant(storage)
  assert.equal(back.entries.length, 2)
  assert.deepEqual(back.state, { selection_id: 'x' })
  assert.equal(lastAssistantId(back.entries), -1)
  assert.equal(loadAssistant(null).entries.length, 0)
})
