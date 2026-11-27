<script setup lang="ts">
// Вход в помощника у строки поиска (CHAT.md §1): кнопка со значком сообщения и подписью; точка — есть новый ответ.
// Панель открывается только этим нажатием.
import { MessageSquareText } from 'lucide-vue-next'

defineProps<{ unread?: boolean; active?: boolean }>()
defineEmits<{ open: [] }>()
</script>

<template>
  <button
    type="button"
    class="as-entry"
    :aria-expanded="active"
    aria-controls="assistant-panel"
    data-testid="assistant-entry"
    @click="$emit('open')"
  >
    <MessageSquareText :size="18" aria-hidden="true" />
    <span class="as-entry__label">Помощник по выбору</span>
    <span v-if="unread && !active" class="as-entry__dot" data-testid="assistant-unread">
      <span class="sr-only">есть новый ответ</span>
    </span>
  </button>
</template>

<style scoped>
.as-entry {
  position: relative;
  display: inline-flex;
  flex: none;
  align-items: center;
  gap: var(--space-2);
  min-height: var(--search-h, var(--control-h));
  padding: 0 var(--space-4);
  border: var(--border-w) solid var(--hero-line, var(--border));
  border-radius: var(--radius-control);
  background: var(--hero-button, var(--surface));
  color: var(--on-hero-button, var(--text));
  font: inherit;
  font-weight: var(--fw-semibold);
  cursor: pointer;
  white-space: nowrap;
}
.as-entry:hover {
  background: var(--hero-button-hover, var(--surface-2));
}
.as-entry:focus-visible {
  outline: var(--focus-w) solid var(--focus-ring);
  outline-offset: 2px;
}
.as-entry__dot {
  position: absolute;
  top: 6px;
  right: 6px;
  width: 9px;
  height: 9px;
  border-radius: 50%;
  background: var(--accent);
  box-shadow: 0 0 0 2px var(--hero-button, var(--surface));
}
@media (max-width: 639px) {
  .as-entry {
    justify-content: center;
    min-height: var(--touch-min);
  }
}
</style>
