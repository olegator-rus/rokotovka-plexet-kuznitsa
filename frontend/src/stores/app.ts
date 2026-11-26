// Общее состояние оболочки: /api/meta, выбранный клиент, роль бэк-офиса, номер сессии вкладки.
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { getMeta } from '@/api/meta'
import type { ConfigId, Meta, MetaClient } from '@/api/types'
import type { ApiError } from '@/lib/apiError'
import { isApiError } from '@/lib/apiError'
import { SEARCH_CONFIGS_FALLBACK } from '@/lib/configs'
import { newSessionId } from '@/lib/events'
import { type RoleId, isRoleId } from '@/lib/roles'
import { GUEST_CLIENT_ID } from '@/lib/searchState'

const SESSION_KEY = 'plexet.session'
const ROLE_KEY = 'plexet.role'
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

function initialSession(): string {
  const saved = readStorage(SESSION_KEY)
  if (saved && /^s-[0-9a-f]{16}$/.test(saved)) return saved
  const id = newSessionId()
  writeStorage(SESSION_KEY, id)
  return id
}

export const useAppStore = defineStore('app', () => {
  const meta = ref<Meta | null>(null)
  const metaStatus = ref<'idle' | 'loading' | 'ready' | 'error'>('idle')
  const metaError = ref<ApiError | null>(null)
  const clientId = ref<string>(readStorage(CLIENT_KEY) || GUEST_CLIENT_ID)
  const savedRole = readStorage(ROLE_KEY)
  const role = ref<RoleId>(isRoleId(savedRole) ? savedRole : 'ecom')
  const sessionId = initialSession()

  const clients = computed<MetaClient[]>(() => {
    const list = meta.value?.clients ?? []
    return list.some((c) => c.id === GUEST_CLIENT_ID) ? list : [GUEST_CLIENT, ...list]
  })
  const defaultConfig = computed<ConfigId>(() => meta.value?.default_config ?? 'B3')
  const configs = computed<readonly ConfigId[]>(() =>
    meta.value?.configs?.length ? meta.value.configs : SEARCH_CONFIGS_FALLBACK,
  )

  let inflight: Promise<void> | null = null
  function loadMeta(force = false): Promise<void> {
    if (inflight) return inflight
    if (metaStatus.value === 'ready' && !force) return Promise.resolve()
    metaStatus.value = 'loading'
    metaError.value = null
    inflight = getMeta()
      .then((m) => {
        meta.value = m
        metaStatus.value = 'ready'
        if (!clients.value.some((c) => c.id === clientId.value)) setClient(GUEST_CLIENT_ID)
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

  function setClient(id: string) {
    clientId.value = id || GUEST_CLIENT_ID
    writeStorage(CLIENT_KEY, clientId.value)
  }

  function setRole(id: RoleId) {
    role.value = id
    writeStorage(ROLE_KEY, id)
  }

  return {
    meta,
    metaStatus,
    metaError,
    clientId,
    role,
    sessionId,
    clients,
    defaultConfig,
    configs,
    loadMeta,
    setClient,
    setRole,
  }
})
