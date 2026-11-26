// Медиа-запрос как ref: true, пока окно ему соответствует. Слушатель снимается с компонентом.
import { onBeforeUnmount, ref } from 'vue'

export const DESKTOP_QUERY = '(min-width: 1024px)'

export function useMediaQuery(query: string) {
  const mql = typeof window !== 'undefined' ? window.matchMedia(query) : null
  const matches = ref(mql?.matches ?? false)
  const onChange = (e: MediaQueryListEvent) => {
    matches.value = e.matches
  }
  mql?.addEventListener('change', onChange)
  onBeforeUnmount(() => mql?.removeEventListener('change', onChange))
  return matches
}
