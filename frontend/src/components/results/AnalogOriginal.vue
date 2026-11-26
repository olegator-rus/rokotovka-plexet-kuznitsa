<script setup lang="ts">
// «Вы искали: …» (контур доводки): исходный продукт запроса замены — отдельной строкой,
// не среди аналогов; со статусом реестра.
import type { AnalogOriginal } from '@/api/types'
import { formatPrice } from '@/lib/format'

defineProps<{ original: AnalogOriginal }>()
</script>

<template>
  <p class="analog-original t-sm" data-testid="analog-original">
    <span class="t-muted">Вы искали:</span>
    <strong>{{ original.family_name }}</strong>
    <span class="t-muted">· {{ original.vendor }}</span>
    <span v-if="original.price_from_rub != null" class="t-muted t-num">
      · от {{ formatPrice(original.price_from_rub) }}
    </span>
    <span v-if="original.reestr" class="badge">{{ original.reestr.label }}</span>
    <span class="t-muted">— ниже аналоги того же класса.</span>
  </p>
</template>

<style scoped>
.analog-original {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: var(--space-1) var(--space-2);
  margin: 0;
  padding: var(--space-2) var(--space-3);
  border: var(--border-w) dashed var(--border);
  border-radius: var(--radius-card);
}
</style>
