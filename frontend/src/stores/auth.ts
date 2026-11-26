// Кто смотрит сайт: гость, покупатель или сотрудник. Сессия — HttpOnly-cookie сервера, стор её не
// видит; источник правды — GET /api/auth/me при старте и ответы входа. Здесь же — способы входа и демо-аккаунты.
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  type AuthConfig,
  type Viewer,
  getAuthConfig,
  getMe,
  postDemoLogin,
  postDemoSwitch,
  postLogin,
  postLogout,
} from '@/api/auth'
import { onUnauthorized } from '@/api/http'
import { type StaffRole, canDecide, canViewAs, isStaff } from '@/lib/auth'

export const GUEST_VIEWER: Viewer = {
  role: 'guest',
  role_label: 'Гость',
  name: '',
  client_id: null,
  client_name: null,
  persona: null,
  expires_at: null,
}

export const useAuthStore = defineStore('auth', () => {
  const viewer = ref<Viewer>(GUEST_VIEWER)
  /** unknown — сервер ещё не спрашивали; ready — ответ есть (или сервер не ответил — тогда гость). */
  const status = ref<'unknown' | 'ready'>('unknown')
  /** Сессия кончилась посреди работы (401 от бэк-офиса) — роутер ведёт на вход. */
  const expired = ref(false)
  const config = ref<AuthConfig | null>(null)
  const configStatus = ref<'idle' | 'loading' | 'ready' | 'error'>('idle')
  /** Номер смены входа: экраны, зависящие от роли, перезагружают данные. */
  const generation = ref(0)

  const role = computed(() => viewer.value.role)
  const staff = computed(() => isStaff(role.value))
  const buyer = computed(() => role.value === 'buyer')
  /** Вошёл по-настоящему (есть сессия), а не гость и не служебный доступ без входа. */
  const signedIn = computed(() => role.value !== 'guest' && role.value !== 'system')
  const demo = computed(() => !!viewer.value.persona)
  const viewAs = computed(() => canViewAs(role.value))
  const can = (owner: string | null | undefined) => canDecide(role.value, owner)
  /** Роль блоков «Аналитики» по умолчанию: своя; у служебного доступа — руководитель. */
  const ownView = computed<StaffRole>(() => {
    const r = role.value
    return r === 'ecom' || r === 'product' || r === 'content' ? r : 'head'
  })

  let pending: Promise<void> | null = null
  /** Спросить сервер один раз за загрузку страницы; охрана маршрутов ждёт ответа. */
  function ensure(): Promise<void> {
    if (status.value === 'ready') return Promise.resolve()
    pending ??= getMe()
      .then((v) => set(v, false))
      .catch(() => set(GUEST_VIEWER, false))
      .finally(() => {
        pending = null
      })
    return pending
  }

  /** Новый смотрящий; bump — это смена входа (не первый ответ сервера), экраны сбрасывают выбор прежнего. */
  function set(v: Viewer, bump = true) {
    const changed = v.role !== viewer.value.role || v.persona !== viewer.value.persona
    viewer.value = v
    status.value = 'ready'
    if (v.role !== 'guest') expired.value = false
    if (changed && bump) generation.value++
  }

  function loadConfig(force = false): Promise<void> {
    if (configStatus.value === 'loading' || (configStatus.value === 'ready' && !force))
      return Promise.resolve()
    configStatus.value = 'loading'
    return getAuthConfig()
      .then((c) => {
        config.value = c
        configStatus.value = 'ready'
      })
      .catch(() => {
        configStatus.value = 'error'
      })
  }

  async function login(name: string, password: string) {
    set(await postLogin(name, password))
  }
  async function demoLogin(persona: string) {
    set(await postDemoLogin(persona))
  }
  async function demoSwitch(persona: string) {
    set(await postDemoSwitch(persona))
  }
  async function logout() {
    try {
      await postLogout()
    } finally {
      // Без сессии сервер всё равно считает гостем; в открытом режиме development — служебный доступ.
      set(await getMe().catch(() => GUEST_VIEWER))
    }
  }

  onUnauthorized(() => {
    if (signedIn.value) {
      expired.value = true
      set(GUEST_VIEWER)
    }
  })

  return {
    viewer,
    status,
    expired,
    config,
    configStatus,
    generation,
    role,
    staff,
    buyer,
    signedIn,
    demo,
    viewAs,
    ownView,
    can,
    ensure,
    loadConfig,
    login,
    demoLogin,
    demoSwitch,
    logout,
  }
})
