<script setup lang="ts">
// «Печатает…» и смена на этап, который сервер действительно выполняет (событие status).
import { computed } from 'vue'
import { stepLabel } from '@/lib/consultant'

const props = defineProps<{ step: string | null }>()
const label = computed(() => stepLabel(props.step))
</script>

<template>
  <div class="ti" role="status" aria-live="polite" data-testid="consultant-typing">
    <span class="ti__dots" aria-hidden="true"><i /><i /><i /></span>
    <span class="ti__label">{{ label }}</span>
  </div>
</template>

<style scoped>
.ti {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-3);
  border-radius: 0.875rem;
  background: var(--surface-2);
  color: var(--text-2);
  font-size: var(--fs-sm);
}
.ti__dots {
  display: inline-flex;
  gap: 3px;
}
.ti__dots i {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--text-3);
  animation: ti-blink 1.2s infinite ease-in-out;
}
.ti__dots i:nth-child(2) {
  animation-delay: 0.2s;
}
.ti__dots i:nth-child(3) {
  animation-delay: 0.4s;
}
@keyframes ti-blink {
  0%,
  80%,
  100% {
    opacity: 0.3;
  }
  40% {
    opacity: 1;
  }
}
@media (prefers-reduced-motion: reduce) {
  .ti__dots i {
    animation: none;
  }
}
</style>
