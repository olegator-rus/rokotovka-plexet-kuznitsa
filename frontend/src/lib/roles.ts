// Роли бэк-офиса («Постоянные элементы»): меняют набор блоков в Аналитике, не права доступа.
export type RoleId = 'ecom' | 'product' | 'content' | 'head'

export const ROLES: readonly { id: RoleId; label: string }[] = [
  { id: 'ecom', label: 'Менеджер e-com' },
  { id: 'product', label: 'Продукт-менеджер' },
  { id: 'content', label: 'Контент-менеджер' },
  { id: 'head', label: 'Руководитель e-com' },
]

export function isRoleId(v: unknown): v is RoleId {
  return ROLES.some((r) => r.id === v)
}
