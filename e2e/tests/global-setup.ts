// Без API и эмбеддера сценарии бессмысленны: падаем сразу и понятно, а не таймаутами в каждом тесте.
const API = process.env.E2E_API_URL ?? 'http://127.0.0.1:8088'

export default async function globalSetup() {
  let body: Record<string, unknown>
  try {
    const resp = await fetch(`${API}/health`, { signal: AbortSignal.timeout(5000) })
    body = (await resp.json()) as Record<string, unknown>
  } catch (e) {
    throw new Error(`API ${API} недоступен (${String(e)}). Поднимите make dev и make ml.`)
  }
  const down = ['db', 'search', 'embedder'].filter((k) => body[k] !== 'ok')
  if (body.status !== 'ok' || down.length) {
    throw new Error(`API ${API}/health не ok: ${JSON.stringify(body)}`)
  }
}
