// Тосты: модульное состояние + хост UiToast (подключён в App.vue). Виден один тост: новый заменяет старый.
import { reactive } from 'vue'

export type ToastTone = 'ok' | 'error' | 'info'

export interface ToastAction {
  label: string
  run: () => void
}

export interface ToastMessage {
  tone: ToastTone
  title: string
  detail?: string
  /** Кнопка в тосте («Проверить в поиске»); такой тост живёт дольше. */
  action?: ToastAction
}

export interface ToastItem extends ToastMessage {
  id: number
  open: boolean
}

export const TOAST_LIFE_MS = 4000
export const TOAST_ACTION_LIFE_MS = 12000

let seq = 0
export const toastState = reactive<{ items: ToastItem[] }>({ items: [] })

function add(message: ToastMessage): number {
  const id = ++seq
  for (const t of toastState.items) t.open = false
  toastState.items.push({ ...message, id, open: true })
  return id
}

function remove(id: number) {
  const i = toastState.items.findIndex((t) => t.id === id)
  if (i >= 0) toastState.items.splice(i, 1)
}

/** Закрыть все видимые тосты (открылась спецификация — «Добавлено…» с «Открыть» больше не нужно). */
function dismissAll() {
  for (const t of toastState.items) t.open = false
  const ids = toastState.items.map((t) => t.id)
  setTimeout(() => ids.forEach(remove), 300)
}

export function useToast() {
  return { add, remove, dismissAll }
}
