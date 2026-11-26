// Разбор конверта ошибки API (src/lib/apiError.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  MSG_AUTH,
  MSG_SERVER,
  isAuthError,
  MSG_UNKNOWN,
  isApiError,
  networkError,
  parseApiError,
} from '../src/lib/apiError.ts'

test('parseApiError: конверт сервера', () => {
  const e = parseApiError(400, {
    error: {
      code: 'VALIDATION',
      message: 'Пустой запрос',
      traceId: '01J',
      details: [{ field: 'query', message: 'обязательно' }, { bad: 1 }],
    },
  })
  assert.equal(e.code, 'VALIDATION')
  assert.equal(e.message, 'Пустой запрос')
  assert.equal(e.traceId, '01J')
  assert.deepEqual(e.fields, [{ field: 'query', message: 'обязательно' }])
})

test('parseApiError: тело не по контракту — текст по статусу', () => {
  assert.equal(parseApiError(502, '<html>').message, MSG_SERVER)
  assert.equal(parseApiError(404, undefined).message, MSG_UNKNOWN)
  assert.equal(parseApiError(404, undefined).code, 'HTTP_404')
})

test('networkError и isApiError', () => {
  const e = networkError()
  assert.equal(e.status, 0)
  assert.equal(isApiError(e), true)
  assert.equal(isApiError(new Error('x')), false)
})

test('parseApiError: 401 — понятное «нужен вход менеджера» вместо текста про токен', () => {
  const e = parseApiError(401, {
    error: {
      code: 'UNAUTHORIZED',
      message: 'Нужен вход в бэк-офис: … BACKOFFICE_TOKEN',
      traceId: 't1',
    },
  })
  assert.equal(e.message, MSG_AUTH)
  assert.equal(e.code, 'UNAUTHORIZED')
  assert.equal(e.traceId, 't1')
  assert.ok(isAuthError(e))
  assert.ok(isAuthError(parseApiError(401, '<html>')))
  assert.ok(!isAuthError(parseApiError(403, null)))
})
