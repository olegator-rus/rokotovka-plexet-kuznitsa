<script setup lang="ts">
// Вкладка «Запуски»: прошлые запуски агента, новые сверху. Строка ведёт на страницу запуска,
// где лента открывается в том же виде, что и вживую.
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import { Bot, LoaderCircle, Play } from 'lucide-vue-next'
import type { AgentRun } from '@/api/types'
import BlockError from '@/components/common/BlockError.vue'
import type { ApiError } from '@/lib/apiError'
import { formatDuration, isRunActive, runStatusLabel, stopReasonLabel } from '@/lib/agentFeed'
import { formatCost, formatDateTime, formatInt } from '@/lib/format'
import type { ResourceStatus } from '@/composables/useResource'

const props = defineProps<{
  runs: AgentRun[] | null
  status: ResourceStatus
  error: ApiError | null
  starting: boolean
}>()
defineEmits<{ start: []; retry: [] }>()

const sorted = computed(() =>
  [...(props.runs ?? [])].sort((a, b) => String(b.started_at).localeCompare(String(a.started_at))),
)
const runHref = (r: AgentRun) => `/searchops/runs/${encodeURIComponent(r.id)}`
</script>

<template>
  <div v-if="status === 'loading' && !runs" class="panel runs__skel" aria-busy="true">
    <span v-for="i in 4" :key="i" class="skeleton" :style="{ width: `${94 - i * 7}%` }" />
  </div>

  <BlockError
    v-else-if="status === 'error' && error"
    :error="error"
    title="Запуски не загрузились"
    @retry="$emit('retry')"
  />

  <div v-else-if="!sorted.length" class="panel runs__empty">
    <Bot :size="20" aria-hidden="true" class="runs__empty-icon" />
    <div>
      <p class="t-h2">Агент ещё не запускался</p>
      <p class="t-muted">
        Агент берёт неудачные поиски за 30 дней, группирует их в кластеры, ищет причину, проверяет
        правку прогоном и кладёт в очередь только прошедшие проверку.
      </p>
      <button type="button" class="btn btn--primary" :disabled="starting" @click="$emit('start')">
        <Play :size="14" aria-hidden="true" /> Запустить разбор
      </button>
    </div>
  </div>

  <div v-else class="data-scroll" role="region" aria-label="Запуски агента" tabindex="0">
    <table class="data-table">
      <thead>
        <tr>
          <th scope="col">Запуск</th>
          <th scope="col">Статус</th>
          <th scope="col" class="num">Кластеры</th>
          <th scope="col" class="num">Предложений</th>
          <th scope="col" class="num">Вызовов</th>
          <th scope="col" class="num">Стоимость</th>
          <th scope="col" class="num">Длительность</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="r in sorted" :key="r.id">
          <th scope="row">
            <RouterLink :to="runHref(r)" class="link">{{
              formatDateTime(r.started_at)
            }}</RouterLink>
            <span class="t-xs t-muted runs__trigger">
              {{ r.trigger === 'schedule' ? 'по расписанию' : 'вручную' }}
            </span>
          </th>
          <td>
            <span class="runs__status">
              <LoaderCircle
                v-if="isRunActive(r.status)"
                :size="12"
                aria-hidden="true"
                class="runs__spin"
              />
              {{ runStatusLabel(r.status) }}
            </span>
            <span v-if="stopReasonLabel(r.stop_reason)" class="t-xs t-muted runs__trigger">
              {{ stopReasonLabel(r.stop_reason) }}
            </span>
          </td>
          <td class="num">{{ formatInt(r.clusters_done) }} из {{ formatInt(r.clusters_total) }}</td>
          <td class="num">{{ formatInt(r.proposals_created) }}</td>
          <td class="num">{{ formatInt(r.tool_calls) }}</td>
          <td class="num">{{ formatCost(r.cost_rub) }}</td>
          <td class="num">{{ formatDuration(r.started_at, r.finished_at) }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<style scoped>
.runs__skel {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
}
.runs__empty {
  display: flex;
  gap: var(--space-3);
  padding: var(--space-5);
}
.runs__empty > div {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--space-2);
  max-width: 40rem;
}
.runs__empty-icon {
  flex: none;
  color: var(--text-2);
}
.runs__trigger {
  display: block;
  font-weight: var(--fw-regular);
}
.runs__status {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  white-space: nowrap;
}
.runs__spin {
  animation: runs-spin 1s linear infinite;
}
@keyframes runs-spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
