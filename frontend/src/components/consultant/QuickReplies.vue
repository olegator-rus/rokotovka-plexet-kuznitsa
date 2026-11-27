<script setup lang="ts">
// Быстрые ответы и стартовые подсказки: нажатие отправляет текст обычной репликой покупателя.
defineProps<{ items: string[]; testid?: string }>()
defineEmits<{ pick: [text: string] }>()
</script>

<template>
  <div class="qr" :data-testid="testid ?? 'consultant-quick'">
    <button
      v-for="t in items"
      :key="t"
      type="button"
      class="qr__btn"
      data-testid="consultant-quick-reply"
      @click="$emit('pick', t)"
    >
      {{ t }}
    </button>
  </div>
</template>

<style scoped>
.qr {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  padding-left: calc(1.75rem + var(--space-2));
}
.qr__btn {
  min-height: 2.25rem;
  padding: var(--space-1) var(--space-3);
  border: 1px solid var(--border-control);
  border-radius: var(--radius-pill);
  background: var(--surface);
  color: var(--text);
  font: inherit;
  font-size: var(--fs-sm);
  text-align: left;
  cursor: pointer;
}
.qr__btn:hover {
  background: var(--surface-2);
}
.qr__btn:focus-visible {
  outline: var(--focus-w) solid var(--focus-ring);
  outline-offset: 2px;
}
@media (max-width: 640px) {
  .qr {
    padding-left: 0;
  }
  .qr__btn {
    min-height: 2.75rem;
  }
}
</style>
