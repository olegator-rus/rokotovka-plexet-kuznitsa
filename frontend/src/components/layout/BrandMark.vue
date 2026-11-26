<script setup lang="ts">
import { PRODUCT_NAME, PRODUCT_SHORT } from '@/lib/brand'

defineProps<{ compact?: boolean }>()
</script>

<template>
  <span class="brand" :aria-label="PRODUCT_NAME">
    <span class="brand__name">{{ PRODUCT_SHORT }}</span>
    <span v-if="!compact" class="brand__sub">SearchOps</span>
  </span>
</template>

<style scoped>
.brand {
  display: inline-flex;
  align-items: baseline;
  gap: var(--space-2);
  white-space: nowrap;
}
.brand__name {
  font-weight: var(--fw-semibold);
  font-size: var(--fs-h2);
  letter-spacing: -0.01em;
}
.brand__sub {
  color: var(--text-2);
  font-size: var(--fs-sm);
}
</style>
