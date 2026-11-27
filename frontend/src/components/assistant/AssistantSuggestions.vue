<script setup lang="ts">
// Подсказки следующего шага (CHAT.md §3): полноценные реплики; активны только под последним ответом.
defineProps<{ items: { label: string }[]; disabled?: boolean; testid?: string }>()
defineEmits<{ pick: [index: number] }>()
</script>

<template>
  <ul class="as-sug" :data-testid="testid ?? 'assistant-suggestions'">
    <li v-for="(s, i) in items" :key="s.label">
      <button type="button" class="as-sug__btn" :disabled="disabled" @click="$emit('pick', i)">
        {{ s.label }}
      </button>
    </li>
  </ul>
</template>

<style scoped>
.as-sug {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.as-sug__btn {
  min-height: 32px;
  padding: 0 var(--space-3);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-pill);
  background: var(--surface);
  color: var(--text);
  font: inherit;
  font-size: var(--fs-sm);
  cursor: pointer;
}
.as-sug__btn:hover:not(:disabled) {
  border-color: var(--accent);
  color: var(--accent);
}
.as-sug__btn:disabled {
  color: var(--text-3);
  cursor: default;
}
@media (max-width: 1023px) {
  .as-sug__btn {
    min-height: var(--touch-min);
  }
}
</style>
