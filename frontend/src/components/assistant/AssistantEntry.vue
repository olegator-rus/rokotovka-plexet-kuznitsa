<script setup lang="ts">
// Вход в помощника (CHAT.md §1; по решению владельца — круглая кнопка в правом нижнем углу вместо кнопки у
// строки поиска): значок сообщения, подпись — для скринридера и подсказкой при наведении; точка — есть новый ответ.
// Панель открывается только этим нажатием.
import { MessageSquareText } from 'lucide-vue-next'

defineProps<{ unread?: boolean }>()
defineEmits<{ open: [] }>()
</script>

<template>
  <button
    type="button"
    class="as-entry"
    aria-label="Помощник по выбору"
    aria-haspopup="dialog"
    data-testid="assistant-entry"
    @click="$emit('open')"
  >
    <MessageSquareText class="as-entry__icon" aria-hidden="true" />
    <span class="as-entry__label" aria-hidden="true">Помощник по выбору</span>
    <span v-if="unread" class="as-entry__dot" data-testid="assistant-unread">
      <span class="sr-only">есть новый ответ</span>
    </span>
  </button>
</template>

<style scoped>
/* Подвал оставляет под кнопку место справа (стиль ниже): переключатель «Трасса» она не закрывает. */
.as-entry {
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
.as-entry:hover {
  background: var(--accent-hover);
}
.as-entry:focus-visible {
  outline: var(--focus-w) solid var(--focus-ring);
  outline-offset: 2px;
}
.as-entry__icon {
  width: 1.875rem;
  height: 1.875rem;
}
.as-entry__label {
  position: absolute;
  top: 50%;
  right: calc(100% + var(--space-3));
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-control);
  background: var(--text);
  color: var(--surface);
  font-size: var(--fs-sm);
  font-weight: var(--fw-semibold);
  white-space: nowrap;
  opacity: 0;
  pointer-events: none;
  transform: translateY(-50%);
  transition: opacity var(--dur-fast) var(--ease-standard);
}
.as-entry:hover .as-entry__label,
.as-entry:focus-visible .as-entry__label {
  opacity: 1;
}
.as-entry__dot {
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
body:has(.as-entry) .shell__footer {
  padding-right: calc(var(--fab-right) + var(--fab-size) + var(--space-4));
}
</style>
