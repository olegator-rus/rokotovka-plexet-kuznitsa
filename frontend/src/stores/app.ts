// Общее состояние оболочки: /api/meta, клиент витрины, номер сессии вкладки.
// Клиент витрины: у покупателя — свой из сессии входа, у гостя — «Гость», выбирать клиента («Смотреть глазами
// клиента») может только сотрудник.
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { getProposals } from '@/api/agent'
import { getMeta } from '@/api/meta'
import type { ConfigId, Meta, MetaClient } from '@/api/types'
import type { ApiError } from '@/lib/apiError'
import { isApiError } from '@/lib/apiError'
import { SEARCH_CONFIGS_FALLBACK, withDefaultConfig } from '@/lib/configs'
import { isSessionToken } from '@/lib/events'
import { pendingCount } from '@/lib/proposals'
import { GUEST_CLIENT_ID } from '@/lib/searchState'
import { useAuthStore } from './auth'

const SESSION_KEY = 'plexet.session'
const CLIENT_KEY = 'plexet.client'

/** Без meta в переключателе только гость: других клиентов фронт не выдумывает. */
export const GUEST_CLIENT: MetaClient = { id: GUEST_CLIENT_ID, name: 'Гость', hint: 'без истории' }

function readStorage(key: string): string | null {
  try {
    return window.sessionStorage.getItem(key)
  } catch {
    return null
  }
}

function writeStorage(key: string, value: string) {
  try {
    window.sessionStorage.setItem(key, value)
  } catch {
    // Хранилище недоступно (приватный режим) — состояние живёт до перезагрузки.
  }
}

/** Сохранённый токен сессии вкладки; пусто — ждём новый из /api/meta. */
function initialSession(): string {
  const saved = readStorage(SESSION_KEY)
  return isSessionToken(saved) ? saved : ''
}

export const useAppStore = defineStore('app', () => {
  const auth = useAuthStore()
  const meta = ref<Meta | null>(null)
  const metaStatus = ref<'idle' | 'loading' | 'ready' | 'error'>('idle')
  const metaError = ref<ApiError | null>(null)
  /** Клиент, выбранный сотрудником («Смотреть глазами клиента»). */
  const pickedClient = ref<string>(readStorage(CLIENT_KEY) || GUEST_CLIENT_ID)
  /** Может ли смотрящий выбирать клиента витрины. */
  const canPickClient = computed(() => auth.staff)
  /** Клиент витрины: покупатель — свой из сессии, гость — «Гость», сотрудник — выбранный. */
  const clientId = computed<string>(() => {
    if (auth.buyer) return auth.viewer.client_id || GUEST_CLIENT_ID
    if (!auth.staff) return GUEST_CLIENT_ID
    return pickedClient.value
  })
  /** Токен сессии от сервера (GET /api/meta): до ответа meta — сохранённый или пустой. */
  const sessionId = ref<string>(initialSession())

  const clients = computed<MetaClient[]>(() => {
    const list = meta.value?.clients ?? []
    return list.some((c) => c.id === GUEST_CLIENT_ID) ? list : [GUEST_CLIENT, ...list]
  })
  const defaultConfig = computed<ConfigId>(() => meta.value?.default_config ?? 'B3')
  const configs = computed<readonly ConfigId[]>(() =>
    withDefaultConfig(
      meta.value?.configs?.length ? meta.value.configs : SEARCH_CONFIGS_FALLBACK,
      defaultConfig.value,
    ),
  )

  let inflight: Promise<void> | null = null
  function loadMeta(force = false): Promise<void> {
    if (inflight) return inflight
    if (metaStatus.value === 'ready' && !force) return Promise.resolve()
    metaStatus.value = 'loading'
    metaError.value = null
    inflight = getMeta(sessionId.value)
      .then((m) => {
        meta.value = m
        if (isSessionToken(m.session) && m.session !== sessionId.value) {
          sessionId.value = m.session
          writeStorage(SESSION_KEY, m.session)
        }
        metaStatus.value = 'ready'
        if (!clients.value.some((c) => c.id === pickedClient.value)) setClient(GUEST_CLIENT_ID)
      })
      .catch((e: unknown) => {
        metaStatus.value = 'error'
        metaError.value = isApiError(e)
          ? e
          : { status: 0, code: 'UNKNOWN', message: 'Не удалось получить настройки' }
      })
      .finally(() => {
        inflight = null
      })
    return inflight
  }

  /** Предложения агента, ждущие решения: счётчик у SearchOps. null — неизвестно (API не отвечает). */
  const pendingProposals = ref<number | null>(null)
  function loadPending(): Promise<void> {
    return getProposals({ status: 'pending' })
      .then((r) => {
        pendingProposals.value = pendingCount(r.items ?? [])
      })
      .catch(() => {
        pendingProposals.value = null
      })
  }

  /** Выбрать клиента витрины — только сотруднику; у покупателя и гостя клиент задан входом. */
  function setClient(id: string) {
    if (!canPickClient.value) return
    pickedClient.value = id || GUEST_CLIENT_ID
    writeStorage(CLIENT_KEY, pickedClient.value)
  }

  /** Смена входа: выбор клиента прежнего пользователя не переносится. */
  function resetForViewer() {
    pickedClient.value = GUEST_CLIENT_ID
    pendingProposals.value = null
    writeStorage(CLIENT_KEY, GUEST_CLIENT_ID)
  }

  return {
    meta,
    metaStatus,
    metaError,
    clientId,
    canPickClient,
    sessionId,
    clients,
    defaultConfig,
    configs,
    loadMeta,
    pendingProposals,
    loadPending,
    setClient,
    resetForViewer,
  }
})
