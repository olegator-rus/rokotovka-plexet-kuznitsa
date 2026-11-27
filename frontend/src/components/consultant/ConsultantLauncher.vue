<script setup lang="ts">
// Круглая кнопка консультанта (CONSULTANT.md §3): та же позиция, что у кнопки помощника контура помощника, не двигается.
// Окно открывается только этим нажатием. Точка — ответ пришёл при закрытом окне, без анимации.
import { MessageSquareText, Minus } from 'lucide-vue-next'
import { useConsultant } from './composables/useConsultant'

const c = useConsultant()
</script>

<template>
  <button
    type="button"
    class="cl"
    :class="{ 'cl--open': c.open.value }"
    :aria-label="c.open.value ? 'Свернуть консультанта' : 'Открыть консультанта'"
    :title="c.open.value ? 'Свернуть консультанта' : 'Открыть консультанта'"
    :aria-expanded="c.open.value"
    aria-controls="consultant-window"
    data-testid="consultant-launcher"
    @click="c.toggle()"
  >
    <Minus v-if="c.open.value" class="cl__icon" aria-hidden="true" />
    <MessageSquareText v-else class="cl__icon" aria-hidden="true" />
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
/* Телефон: открытое окно — на весь экран, кнопка под ним не нужна (свернуть — в шапке). */
@media (max-width: 640px) {
  .cl--open {
    display: none;
  }
}
</style>

<style>
body:has(.cl) .shell__footer {
  padding-right: calc(var(--fab-right) + var(--fab-size) + var(--space-4));
}
</style>
