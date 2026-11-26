<script setup lang="ts">
// Плитка показателя. Число посчитано сервером — плитка целиком кнопка и открывает объяснение.
// Нет значения — «нет данных» и причина вместо числа (непосчитанное не показывается).
defineProps<{
  label: string
  /** Готовая строка значения; null — значения нет. */
  value: string | null
  /** Подпись под числом: знаменатель, период. */
  sub?: string
  /** Почему нет значения. */
  missing?: string
}>()
defineEmits<{ explain: [] }>()
</script>

<template>
  <button
    v-if="value !== null"
    type="button"
    class="panel tile tile--action"
    :aria-label="`${label}: ${value}. Как посчитано`"
    @click="$emit('explain')"
  >
    <span class="tile__label">{{ label }}</span>
    <span class="tile__value t-num">{{ value }}</span>
    <span v-if="sub" class="tile__sub">{{ sub }}</span>
  </button>
  <div v-else class="panel tile">
    <span class="tile__label">{{ label }}</span>
    <span class="tile__value tile__value--missing">нет данных</span>
    <span v-if="missing" class="tile__sub">{{ missing }}</span>
  </div>
</template>

<style scoped>
.tile {
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--space-1);
  min-width: 0;
  padding: var(--space-3) var(--space-4);
  text-align: left;
  font: inherit;
  color: var(--text);
}
.tile--action {
  cursor: pointer;
  transition: border-color var(--dur-fast) var(--ease-standard);
}
.tile--action:hover {
  border-color: var(--border-control);
}
.tile__label {
  color: var(--text-2);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.tile__value {
  font-size: var(--fs-h1);
  line-height: var(--lh-h1);
  font-weight: var(--fw-semibold);
}
.tile__value--missing {
  color: var(--text-3);
  font-size: var(--fs-body);
  font-weight: var(--fw-medium);
}
.tile__sub {
  color: var(--text-3);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
}
</style>
