<script setup lang="ts">
// Круглая кнопка консультанта (CONSULTANT.md §3): та же позиция, что у кнопки помощника контура помощника, не двигается.
// Окно открывается только этим нажатием и раскрывается из неё: пока оно открыто, кнопка спрятана, свернуть — в шапке
// окна. Точка — ответ пришёл при закрытом окне, без анимации.
import { nextTick, ref, watch } from 'vue'
import { MessageSquareText } from 'lucide-vue-next'
import { useConsultant } from './composables/useConsultant'

const c = useConsultant()
const button = ref<HTMLButtonElement | null>(null)

// Свернули из окна — фокус возвращается на кнопку, а не теряется в body.
watch(
  () => c.open.value,
  async (open) => {
    if (open) return
    const inside = document.getElementById('consultant-window')?.contains(document.activeElement)
    await nextTick()
    if (inside || document.activeElement === document.body) button.value?.focus()
  },
)
</script>

<template>
  <button
    ref="button"
    type="button"
    class="cl"
    :class="{ 'cl--open': c.open.value }"
    aria-label="Открыть консультанта"
    title="Открыть консультанта"
    :aria-expanded="c.open.value"
    aria-controls="consultant-window"
    data-testid="consultant-launcher"
    @click="c.toggle()"
  >
    <MessageSquareText class="cl__icon" aria-hidden="true" />
    <span v-if="c.unread.value && !c.open.value" class="cl__dot" data-testid="consultant-unread">
      <span class="sr-only">есть новый ответ</span>
    </span>
  </button>
</template>

<style scoped>
.cl {
  position: fixed;
  right: calc(var(--fab-right) + env(safe-area-inset-right, 0px));
  bottom: calc(var(--fab-bottom) + env(safe-area-inset-bottom, 0px));
  z-index: 40;
  display: grid;
  place-items: center;
  width: var(--fab-size);
  height: var(--fab-size);
  padding: 0;
  border: var(--focus-w) solid var(--surface);
  border-radius: var(--radius-pill);
  background: var(--accent);
  box-shadow: var(--shadow-overlay);
  color: var(--on-accent);
  cursor: pointer;
  transition: background var(--dur-fast) var(--ease-standard);
}
/* Окно открыто — кнопка «стала окном»; появляется снова, когда окно свернулось в круг. */
.cl--open {
  visibility: hidden;
}
/* Проявляется под конец сворачивания. Скрываем только прозрачностью: фокус кнопка принимает сразу. */
.cl:not(.cl--open) {
  animation: cl-back 120ms var(--ease-standard) 240ms backwards;
}
@keyframes cl-back {
  from {
    opacity: 0;
  }
}
@media (prefers-reduced-motion: reduce) {
  .cl:not(.cl--open) {
    animation: none;
  }
}
.cl:hover {
  background: var(--accent-hover);
}
.cl:focus-visible {
  outline: var(--focus-w) solid var(--focus-ring);
  outline-offset: 2px;
}
.cl__icon {
  width: 1.875rem;
  height: 1.875rem;
}
.cl__dot {
  position: absolute;
  top: 0;
  right: 0;
  width: var(--space-4);
  height: var(--space-4);
  border: var(--focus-w) solid var(--surface);
  border-radius: var(--radius-pill);
  background: var(--text);
}
</style>

<style>
body:has(.cl) .shell__footer {
  padding-right: calc(var(--fab-right) + var(--fab-size) + var(--space-4));
}
</style>
