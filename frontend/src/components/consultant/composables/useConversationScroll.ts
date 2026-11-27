// Прокрутка ленты (CONSULTANT.md §5, «Прокрутка»): у нижнего края (ближе 80 px) новые сообщения остаются в поле
// зрения; прокрутил вверх — вниз не возвращаем, показываем «Новый ответ»; после своей реплики — всегда вниз.
import { nextTick, ref, type Ref } from 'vue'

export const NEAR_BOTTOM_PX = 80

export function useConversationScroll(feed: Ref<HTMLElement | null>) {
  const atBottom = ref(true)
  const hasNew = ref(false)

  function measure() {
    const el = feed.value
    if (!el) return
    atBottom.value = el.scrollHeight - el.scrollTop - el.clientHeight <= NEAR_BOTTOM_PX
    if (atBottom.value) hasNew.value = false
  }

  async function toBottom(smooth = false) {
    await nextTick()
    const el = feed.value
    if (!el) return
    el.scrollTo({ top: el.scrollHeight, behavior: smooth ? 'smooth' : 'auto' })
    atBottom.value = true
    hasNew.value = false
  }

  /** Пришло новое содержимое: у края — следуем, иначе — кнопка «Новый ответ». */
  async function onContent() {
    if (atBottom.value) await toBottom()
    else hasNew.value = true
  }

  return { atBottom, hasNew, measure, toBottom, onContent }
}
