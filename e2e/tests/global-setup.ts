// Без API и эмбеддера сценарии бессмысленны: падаем сразу и понятно, а не таймаутами в каждом тесте.
// Затем вход: бэк-офис открыт только сотруднику, поэтому все сценарии идут под демо-аккаунтом
// руководителя (решает за любую роль) — cookie сессии сохраняется в .auth/head.json (storageState проекта).
// Гостевые и ролевые сценарии меняют вход сами (roles.spec.ts, loginAs в fixtures.ts).
import { mkdirSync, writeFileSync } from 'node:fs'
import { dirname } from 'node:path'
import { AUTH_STATE, BASE_URL, DEFAULT_PERSONA } from './auth-state'

const API = process.env.E2E_API_URL ?? 'http://127.0.0.1:8088'

export default async function globalSetup() {
  let body: Record<string, unknown>
  try {
    const resp = await fetch(`${API}/health`, {
      signal: AbortSignal.timeout(5000),
    })
    body = (await resp.json()) as Record<string, unknown>
  } catch (e) {
    throw new Error(`API ${API} недоступен (${String(e)}). Поднимите make dev и make ml.`)
  }
  const down = ['db', 'search', 'embedder'].filter((k) => body[k] !== 'ok')
  if (body.status !== 'ok' || down.length) {
    throw new Error(`API ${API}/health не ok: ${JSON.stringify(body)}`)
  }

  const origin = new URL(BASE_URL).origin
  const resp = await fetch(`${API}/api/auth/demo-login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Origin: origin },
    body: JSON.stringify({ persona: DEFAULT_PERSONA }),
    signal: AbortSignal.timeout(5000),
  })
  if (!resp.ok) {
    throw new Error(
      `Демо-вход ${DEFAULT_PERSONA} не удался: ${resp.status} ${await resp.text()}. ` +
        'Нужны DEMO_LOGIN_BUTTONS=on и PUBLIC_ORIGIN = адрес Vite.',
    )
  }
  const cookie = resp.headers.getSetCookie().find((c) => c.startsWith('plexet_auth='))
  if (!cookie) throw new Error('Демо-вход: сервер не выдал cookie plexet_auth')
  const value = cookie.split(';')[0]!.slice('plexet_auth='.length)
  const expires = Math.floor(Date.now() / 1000) + 6 * 3600
  mkdirSync(dirname(AUTH_STATE), { recursive: true })
  writeFileSync(
    AUTH_STATE,
    JSON.stringify({
      cookies: [
        {
          name: 'plexet_auth',
          value,
          domain: new URL(BASE_URL).hostname,
          path: '/api',
          expires,
          httpOnly: true,
          secure: false,
          sameSite: 'Lax',
        },
      ],
      origins: [],
    }),
  )
}
