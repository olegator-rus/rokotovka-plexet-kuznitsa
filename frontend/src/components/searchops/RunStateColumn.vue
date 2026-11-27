<script setup lang="ts">
// Колонка состояния запуска: текущий кластер, шаг N из лимита, кластеры, токены и рубли.
// Стоимость ведёт в «Доказательства» → «Стоимость» — там журнал вызовов модели с run_id запуска.
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import type { AgentRun, AgentStep } from '@/api/types'
import type { FeedTransport } from '@/composables/useAgentRun'
import {
  DEFAULT_TOOL_LIMIT,
  currentClusterCalls,
  formatDuration,
  isRunActive,
  runStatusLabel,
} from '@/lib/agentFeed'
import { formatInt, formatMoney } from '@/lib/format'

const props = defineProps<{
  run: AgentRun
  steps: AgentStep[]
  transport: FeedTransport
  now: number
}>()

const active = computed(() => isRunActive(props.run.status))
const limit = computed(() => props.run.max_tool_calls ?? DEFAULT_TOOL_LIMIT)
const calls = computed(() => currentClusterCalls(props.run, props.steps))
const transportLabel = computed(() =>
  props.transport === 'sse'
    ? 'поток событий'
    : props.transport === 'poll'
      ? 'опрос раз в 2 с'
      : 'запуск завершён',
)
</script>

<template>
  <aside class="panel state" aria-label="Состояние запуска">
    <dl class="state__list">
      <div>
        <dt>Статус</dt>
        <dd>{{ runStatusLabel(run.status) }}</dd>
      </div>
      <template v-if="active">
        <div class="state__wide">
          <dt>Текущая группа</dt>
          <dd>{{ run.current_cluster ? `«${run.current_cluster.label}»` : '—' }}</dd>
        </div>
        <div class="state__wide">
          <dt>Шаг в группе</dt>
          <dd class="t-num">
            {{ formatInt(calls) }} из {{ formatInt(limit) }}
            <span class="state__bar" aria-hidden="true">
              <span :style="{ width: `${Math.min(100, (calls / limit) * 100)}%` }" />
            </span>
          </dd>
        </div>
      </template>
      <div>
        <dt>Группы запросов</dt>
        <dd class="t-num">
          {{ formatInt(run.clusters_done) }} из {{ formatInt(run.clusters_total) }}
        </dd>
      </div>
      <div>
        <dt>Предложений</dt>
        <dd class="t-num">{{ formatInt(run.proposals_created) }}</dd>
      </div>
      <div>
        <dt>Вызовов инструментов</dt>
        <dd class="t-num">{{ formatInt(run.tool_calls) }}</dd>
      </div>
      <div>
        <dt>Токены, вход / выход</dt>
        <dd class="t-num">
          {{ formatInt(run.input_tokens) }} / {{ formatInt(run.output_tokens) }}
        </dd>
      </div>
      <div>
        <dt>Стоимость</dt>
        <dd class="t-num">
          <RouterLink
            to="/proofs?tab=cost"
            class="num-btn"
            :aria-label="`Стоимость ${formatMoney(run.cost_rub)}: журнал вызовов в «Доказательствах»`"
            >{{ formatMoney(run.cost_rub) }}</RouterLink
          >
        </dd>
      </div>
      <div>
        <dt>Длительность</dt>
        <dd class="t-num">{{ formatDuration(run.started_at, run.finished_at, now) }}</dd>
      </div>
      <div>
        <dt>Обновление</dt>
        <dd class="t-muted">{{ transportLabel }}</dd>
      </div>
    </dl>
  </aside>
</template>

<style scoped>
.state {
  position: sticky;
  top: calc(var(--header-h) + var(--space-4));
  padding: var(--space-3) var(--space-4);
}
.state__list {
  display: grid;
  grid-template-columns: 1fr;
  gap: var(--space-2) var(--space-4);
  margin: 0;
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.state__list dt {
  color: var(--text-2);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
}
.state__list dd {
  margin: 0;
  font-weight: var(--fw-medium);
}
.state__bar {
  display: block;
  height: 4px;
  margin-top: var(--space-1);
  border-radius: var(--radius-pill);
  background: var(--surface-3);
  overflow: hidden;
}
.state__bar > span {
  display: block;
  height: 100%;
  background: var(--text-2);
}
@media (max-width: 1023px) {
  .state {
    position: static;
  }
  .state__list {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .state__wide {
    grid-column: 1 / -1;
  }
}
</style>
