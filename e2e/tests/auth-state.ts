// Общее для global-setup, конфига и фикстур: где лежит сессия демо-аккаунта и под кем идут сценарии.
import { fileURLToPath } from 'node:url'

export const BASE_URL = process.env.E2E_BASE_URL ?? 'http://localhost:5177'
/** Руководитель e-com: видит весь бэк-офис и решает за любую роль — прежние сценарии идут без изменений. */
export const DEFAULT_PERSONA = 'staff-head'
export const AUTH_STATE = fileURLToPath(new URL('../.auth/head.json', import.meta.url))
