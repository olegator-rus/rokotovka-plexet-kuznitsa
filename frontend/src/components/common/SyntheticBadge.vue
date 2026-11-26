<script setup lang="ts">
// Плашка «Синтетические данные» («Новые постоянные элементы»): стоит рядом с числами,
// посчитанными на придуманной марже или заказах. Общая для витрины и бэк-офиса.
import { FlaskConical } from 'lucide-vue-next'
import UiTooltip from '@/components/ui/UiTooltip.vue'

withDefaults(defineProps<{ label?: string; hint?: string }>(), {
  label: 'Синтетические данные',
  hint: 'Маржа и заказы в демо-данных придуманы. Числа показывают механику, не деньги магазина; плашка снимается, когда поле приходит из данных заказчика.',
})
</script>

<template>
  <UiTooltip :text="hint">
    <span class="badge synthetic" tabindex="0" :aria-label="`${label}. ${hint}`">
      <FlaskConical :size="12" aria-hidden="true" />
      {{ label }}
    </span>
  </UiTooltip>
</template>

<style scoped>
.synthetic {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  border-style: dashed;
  color: var(--text-2);
  white-space: nowrap;
}
</style>
