<script setup lang="ts">
// Сводка трассы одной карточкой: маршрут, общее время, стоимость, источник разбора.
import { Coins, Info, ScanText, Timer } from 'lucide-vue-next'
import type { Trace } from '@/api/types'
import { formatCost, formatMs } from '@/lib/format'
import { intentFallback, parseSourceLabel } from '@/lib/trace'
import RouteBadge from './RouteBadge.vue'

defineProps<{ trace: Trace }>()
</script>

<template>
  <div class="tsum">
    <dl class="tsum__list">
      <div class="tsum__route">
        <dt>Маршрут</dt>
        <dd><RouteBadge :route="trace.route" /></dd>
      </div>
      <div class="tsum__cell">
        <dt><Timer :size="14" aria-hidden="true" /> Время</dt>
        <dd class="t-num">{{ formatMs(trace.total_ms) }}</dd>
      </div>
      <div class="tsum__cell">
        <dt><Coins :size="14" aria-hidden="true" /> Стоимость</dt>
        <dd class="t-num">{{ formatCost(trace.cost_rub) }}</dd>
      </div>
      <div class="tsum__cell">
        <dt><ScanText :size="14" aria-hidden="true" /> Разбор</dt>
        <dd>{{ parseSourceLabel(trace.parse_source) }}</dd>
      </div>
    </dl>
    <p v-if="intentFallback(trace)" class="tsum__why t-xs">
      <Info :size="14" aria-hidden="true" /> {{ intentFallback(trace) }}
    </p>
    <p class="tsum__ids t-xs">
      <span class="tsum__tag">
        {{ trace.config }}
        <template v-if="trace.effective_config && trace.effective_config !== trace.config">
          (отработала как {{ trace.effective_config }})
        </template>
      </span>
      <span class="tsum__tag">правила v{{ trace.rules_version }}</span>
      <span class="t-mono tsum__id" title="Номер запроса">{{ trace.request_id }}</span>
    </p>
  </div>
</template>

<style scoped>
.tsum {
  container-type: inline-size;
  overflow: hidden;
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
}
.tsum__list {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  margin: 0;
}
.tsum dt {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  color: var(--text-3);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
}
.tsum dd {
  min-width: 0;
  margin: 0;
}
.tsum__route {
  display: flex;
  grid-column: 1 / -1;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
  border-bottom: var(--border-w) solid var(--border);
  background: var(--surface-2);
}
.tsum__route :deep(.badge) {
  gap: var(--space-2);
  min-height: var(--control-h-sm);
  padding-inline: var(--space-3);
  border-radius: var(--radius-pill);
  font-size: var(--fs-sm);
}
.tsum__cell {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 0;
  padding: var(--space-3) var(--space-4);
}
.tsum__cell + .tsum__cell {
  border-left: var(--border-w) solid var(--border);
}
.tsum__cell dd {
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
  font-weight: var(--fw-bold);
  overflow-wrap: anywhere;
}
.tsum__why {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
  margin: 0;
  padding: var(--space-2) var(--space-4);
  border-top: var(--border-w) solid var(--border);
  background: var(--status-pending-bg);
  color: var(--status-pending);
}
.tsum__why svg {
  flex: none;
  margin-top: 1px;
}
.tsum__ids {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
  margin: 0;
  padding: var(--space-2) var(--space-4);
  border-top: var(--border-w) solid var(--border);
  color: var(--text-3);
}
.tsum__tag {
  padding: 2px var(--space-2);
  border-radius: var(--radius-pill);
  background: var(--surface-2);
  color: var(--text-2);
}
.tsum__id {
  min-width: 0;
  overflow-wrap: anywhere;
}
@container (max-width: 20rem) {
  .tsum__list {
    grid-template-columns: minmax(0, 1fr);
  }
  .tsum__cell + .tsum__cell {
    border-top: var(--border-w) solid var(--border);
    border-left: 0;
  }
}
</style>
