<script setup lang="ts">
// Строка-предложение над выдачей (SPEC §1, «Поиск + предложение»): не чат и не плавающая кнопка.
import { ListChecks } from 'lucide-vue-next'

defineProps<{ text?: string }>()
defineEmits<{ start: [] }>()
</script>

<template>
  <div class="ghint" data-testid="guide-hint">
    <ListChecks :size="16" aria-hidden="true" class="ghint__icon" />
    <p class="ghint__text">{{ text || 'Могу помочь выбрать редакцию и посчитать лицензии.' }}</p>
    <button
      type="button"
      class="btn btn--secondary btn--sm"
      data-testid="guide-start"
      @click="$emit('start')"
    >
      Помочь выбрать
    </button>
  </div>
</template>

<style scoped>
.ghint {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2) var(--space-3);
  padding: var(--space-2) var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
  background: var(--surface-2);
}
.ghint__icon {
  flex: none;
  color: var(--accent);
}
.ghint__text {
  flex: 1 1 14rem;
  margin: 0;
  font-size: var(--fs-body);
}
</style>
