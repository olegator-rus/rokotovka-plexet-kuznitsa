// Роли и права во фронте (src/lib/auth.ts): охрана маршрутов, куда вести после входа, матрица решений, Retry-After.
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  afterLogin,
  canDecide,
  canViewAs,
  decideGuard,
  denyMessage,
  homeFor,
  isBackofficePath,
  isStaff,
  retryAfterSeconds,
  safeReturnTo,
} from '../src/lib/auth.ts'

test('isStaff: четыре роли и служебный доступ; гость и покупатель — нет', () => {
  for (const r of ['ecom', 'product', 'content', 'head', 'system']) assert.ok(isStaff(r), r)
  for (const r of ['guest', 'buyer', '', null, undefined, 'admin'])
    assert.ok(!isStaff(r), String(r))
})

test('canDecide: руководитель — всё, роль — только своё, гость и покупатель — ничего (как сервер)', () => {
  const owners = ['ecom', 'product', 'content']
  for (const actor of ['guest', 'buyer', 'ecom', 'product', 'content', 'head', 'system']) {
    for (const owner of owners) {
      const want = actor === 'head' || actor === 'system' || actor === owner
      assert.equal(canDecide(actor, owner), want, `${actor} → ${owner}`)
    }
  }
  assert.equal(denyMessage('product'), 'Это решение принимает продукт-менеджер')
  assert.equal(denyMessage('ecom'), 'Это решение принимает менеджер e-com')
  assert.equal(denyMessage('content'), 'Это решение принимает контент-менеджер')
  assert.equal(denyMessage('x'), 'Это решение принимает руководитель e-com')
  assert.ok(canViewAs('head') && !canViewAs('ecom') && !canViewAs('buyer'))
})

test('homeFor: стартовый экран роли', () => {
  assert.equal(homeFor('guest'), '/search')
  assert.equal(homeFor('buyer'), '/search')
  assert.equal(homeFor('ecom'), '/analytics')
  assert.equal(homeFor('content'), '/analytics')
  assert.equal(homeFor('head'), '/analytics')
  assert.equal(homeFor('product'), '/searchops?tab=queue&role=product')
})

test('safeReturnTo: только свой путь, не экран входа', () => {
  assert.equal(safeReturnTo('/analytics?days=7'), '/analytics?days=7')
  for (const bad of [
    'https://evil.ru',
    '//evil.ru',
    '/\\evil.ru',
    'analytics',
    '/login',
    '/login?x=1',
    '',
    5,
    '/a\u0000',
  ])
    assert.equal(safeReturnTo(bad), null, String(bad))
  assert.equal(safeReturnTo('/' + 'a'.repeat(600)), null)
})

test('afterLogin: returnTo, если роли туда можно; иначе стартовый экран', () => {
  assert.equal(afterLogin('ecom', '/changes'), '/changes')
  assert.equal(afterLogin('buyer', '/analytics'), '/search')
  assert.equal(afterLogin('buyer', '/search?q=1c'), '/search?q=1c')
  assert.equal(afterLogin('head', 'https://evil.ru'), '/analytics')
  assert.equal(afterLogin('product', undefined), '/searchops?tab=queue&role=product')
  assert.ok(isBackofficePath('/searchops/runs/1') && isBackofficePath('/margin?w=1'))
  assert.ok(
    !isBackofficePath('/search') && !isBackofficePath('/compare') && !isBackofficePath('/rules'),
  )
})

test('decideGuard: бэк-офис без сотрудника → вход с returnTo; вошедший на /login → свой экран', () => {
  const base = { fullPath: '/analytics?days=7', isLogin: false, backoffice: true }
  assert.deepEqual(decideGuard({ ...base, role: 'guest' }), {
    kind: 'redirect',
    to: '/login?returnTo=%2Fanalytics%3Fdays%3D7',
  })
  assert.equal(decideGuard({ ...base, role: 'buyer' }).kind, 'redirect')
  assert.deepEqual(decideGuard({ ...base, role: 'content' }), { kind: 'allow' })
  assert.deepEqual(decideGuard({ ...base, role: 'system' }), { kind: 'allow' })
  assert.deepEqual(
    decideGuard({ fullPath: '/search', isLogin: false, backoffice: false, role: 'guest' }),
    {
      kind: 'allow',
    },
  )
  const login = { fullPath: '/login', isLogin: true, backoffice: false }
  assert.deepEqual(decideGuard({ ...login, role: 'guest' }), { kind: 'allow' })
  assert.deepEqual(decideGuard({ ...login, role: 'head' }), { kind: 'redirect', to: '/analytics' })
  assert.deepEqual(decideGuard({ ...login, role: 'buyer' }), { kind: 'redirect', to: '/search' })
  // Покупателя вернули из бэк-офиса — может войти сотрудником.
  assert.deepEqual(decideGuard({ ...login, role: 'buyer', returnTo: '/analytics' }), {
    kind: 'allow',
  })
  assert.deepEqual(decideGuard({ ...login, role: 'ecom', returnTo: '/changes' }), {
    kind: 'redirect',
    to: '/changes',
  })
})

test('retryAfterSeconds: секунды, дата, мусор', () => {
  assert.equal(retryAfterSeconds('42'), 42)
  assert.equal(retryAfterSeconds('0'), 1)
  assert.equal(retryAfterSeconds(null), null)
  assert.equal(retryAfterSeconds('soon'), null)
  const now = Date.parse('2026-05-27T10:00:00Z')
  assert.equal(retryAfterSeconds('Wed, 27 May 2026 10:00:30 GMT', now), 30)
})

test('parseApiError: 401 входа — текст сервера; 429 — секунды ожидания', async () => {
  const { MSG_AUTH, parseApiError } = await import('../src/lib/apiError.ts')
  const body = {
    error: { code: 'UNAUTHENTICATED', message: 'Неверный логин или пароль', traceId: 't' },
  }
  assert.equal(parseApiError(401, body).message, MSG_AUTH)
  assert.equal(parseApiError(401, body, null, true).message, 'Неверный логин или пароль')
  const tooMany = { error: { code: 'TOO_MANY_REQUESTS', message: 'Подождите', traceId: 't' } }
  assert.equal(parseApiError(429, tooMany, 17).retryAfter, 17)
  assert.equal(parseApiError(500, {}, 17).retryAfter, undefined)
})

test('contactProblem: почта или телефон гостя для ответа на КП', async () => {
  const { contactProblem } = await import('../src/lib/auth.ts')
  assert.equal(contactProblem('buyer@example.ru'), null)
  assert.equal(contactProblem('+7 (900) 000-00-00'), null)
  assert.match(contactProblem('') ?? '', /почту или телефон/)
  assert.ok(contactProblem('позвоните мне'))
  assert.ok(contactProblem('12-34'))
  assert.ok(contactProblem('a'.repeat(121) + '@x.ru'))
})
