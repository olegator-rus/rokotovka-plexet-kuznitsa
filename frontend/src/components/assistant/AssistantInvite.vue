<script setup lang="ts">
// Строка-приглашение над выдачей при общем запросе (CHAT.md §1): текст с кнопкой, а не всплывающее окно.
defineEmits<{ start: [] }>()
</script>

<template>
  <div class="as-invite" data-testid="assistant-invite">
    <span class="as-invite__text">Нужна помощь с выбором лицензии?</span>
    <button type="button" class="btn btn--secondary btn--sm" @click="$emit('start')">
      Подобрать под мою задачу
    </button>
  </div>
</template>

<style scoped>
.as-invite {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2) var(--space-3);
  padding: var(--space-2) var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
}
.as-invite__text {
  color: var(--text-2);
  font-size: var(--fs-sm);
}
</style>
