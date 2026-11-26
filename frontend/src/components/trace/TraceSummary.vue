<script setup lang="ts">
// Сводка трассы одной строкой: маршрут, общее время, стоимость, источник разбора.
import type { Trace } from '@/api/types'
import { formatCost, formatMs } from '@/lib/format'
import { intentFallback, parseSourceLabel } from '@/lib/trace'
import RouteBadge from './RouteBadge.vue'

defineProps<{ trace: Trace }>()
</script>

<template>
  <dl class="trace-summary">
    <div>
      <dt>Маршрут</dt>
      <dd><RouteBadge :route="trace.route" /></dd>
    </div>
    <div>
      <dt>Время</dt>
      <dd class="t-num">{{ formatMs(trace.total_ms) }}</dd>
    </div>
    <div>
      <dt>Стоимость</dt>
      <dd class="t-num">{{ formatCost(trace.cost_rub) }}</dd>
    </div>
    <div>
      <dt>Разбор</dt>
      <dd>
        {{ parseSourceLabel(trace.parse_source) }}
        <span class="t-mono t-muted">{{ trace.parse_source }}</span>
        <span v-if="intentFallback(trace)" class="t-xs t-muted trace-summary__why">
          {{ intentFallback(trace) }}
        </span>
      </dd>
    </div>
  </dl>
  <p class="trace-summary__ids t-xs t-muted">
    {{ trace.config }}
    <template v-if="trace.effective_config && trace.effective_config !== trace.config">
      (отработала как {{ trace.effective_config }})
    </template>
    · правила v{{ trace.rules_version }} ·
    <span class="t-mono">{{ trace.request_id }}</span>
  </p>
</template>

<style scoped>
.trace-summary__why {
  display: block;
}
.trace-summary {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-2);
  margin: 0;
}
.trace-summary > div {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--space-2) var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
  background: var(--surface-2);
}
.trace-summary dt {
  color: var(--text-3);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
}
.trace-summary dd {
  margin: 0;
  font-weight: var(--fw-medium);
}
.trace-summary__ids {
  overflow-wrap: anywhere;
}
@media (min-width: 1280px) {
  .trace-summary {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
}
</style>
