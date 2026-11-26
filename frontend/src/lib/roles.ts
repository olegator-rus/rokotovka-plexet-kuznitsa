// Роли бэк-офиса («Постоянные элементы»): роль приходит из сессии входа, задаёт набор блоков
// «Аналитики» и зону решений (lib/auth.ts). Список — для «Смотреть как» у руководителя.
import type { StaffRole } from './auth.ts'

export type RoleId = StaffRole

export const ROLES: readonly { id: RoleId; label: string }[] = [
  { id: 'ecom', label: 'Менеджер e-com' },
  { id: 'product', label: 'Продукт-менеджер' },
  { id: 'content', label: 'Контент-менеджер' },
  { id: 'head', label: 'Руководитель e-com' },
]

export function isRoleId(v: unknown): v is RoleId {
  return ROLES.some((r) => r.id === v)
}
