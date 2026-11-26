// Вход по ролям: кто смотрит сайт, демо-аккаунты, вход и выход. Сессия — HttpOnly-cookie сервера:
// фронт её не видит и не хранит, источник правды — ответы этих методов.
import { http } from './http'
import type { ViewerRole } from '@/lib/auth'

export interface Viewer {
  role: ViewerRole
  role_label: string
  /** Имя вошедшего; у гостя пусто. */
  name: string
  /** Клиент витрины покупателя из сессии; у остальных null. */
  client_id: string | null
  client_name: string | null
  /** Демо-аккаунт; null — вход по паролю или гость. */
  persona: string | null
  expires_at: string | null
}

export interface Persona {
  id: string
  name: string
  initials: string
  role: ViewerRole
  role_label: string
  title: string
  note: string
  client_id: string | null
  client_name: string | null
  client_hint: string | null
  /** Предложения, ждущие решения этой роли (у руководителя — все); null — очередь недоступна. */
  pending: number | null
}

export interface AuthConfig {
  demo_login: boolean
  auth_required: boolean
  password_login: boolean
  personas: Persona[]
}

interface AuthMe {
  viewer: Viewer
}

/** GET /api/auth/config — способы входа и демо-аккаунты. */
export const getAuthConfig = () => http.get<AuthConfig>('/auth/config')

/** GET /api/auth/me — кто смотрит (гость — role guest). */
export const getMe = () => http.get<AuthMe>('/auth/me').then((r) => r.viewer)

/** POST /api/auth/login — вход по логину и паролю. */
export const postLogin = (login: string, password: string) =>
  http.post<AuthMe>('/auth/login', { login, password }).then((r) => r.viewer)

/** POST /api/auth/demo-login — вход в демо-аккаунт одной кнопкой, пароль в браузер не уходит. */
export const postDemoLogin = (persona: string) =>
  http.post<AuthMe>('/auth/demo-login', { persona }).then((r) => r.viewer)

/** POST /api/auth/demo-switch — сменить демо-аккаунт, не выходя. */
export const postDemoSwitch = (persona: string) =>
  http.post<AuthMe>('/auth/demo-switch', { persona }).then((r) => r.viewer)

/** POST /api/auth/logout — выход (204). */
export const postLogout = () => http.post<void>('/auth/logout')
