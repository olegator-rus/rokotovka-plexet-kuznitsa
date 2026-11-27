<script setup lang="ts">
// Вкладка «Запуски» («Экран «Запуски»»): сверху режим агента, расписание и пороги одной строкой;
// список — время, триггер иконкой и подписью, план одной строкой, итог «N предложений, M задач, K пропущено по
// памяти», стоимость, длительность; фильтр по триггеру и итогу. Строка ведёт в ленту запуска.
import { computed, ref, type Component } from 'vue'
import { RouterLink } from 'vue-router'
import {
  Activity,
  Bot,
  CalendarClock,
  Hand,
  LoaderCircle,
  PackageSearch,
  Play,
} from 'lucide-vue-next'
import type { AgentRun } from '@/api/types'
import type { AgentSettings } from '@/api/typesAgent'
import BlockError from '@/components/common/BlockError.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import type { ApiError } from '@/lib/apiError'
import { isRunActive, runStatusLabel, stopReasonLabel } from '@/lib/agentFeed'
import {
  OUTCOME_FILTERS,
  TRIGGERS,
  formatRunDuration,
  matchesOutcome,
  outcomeLine,
  planLine,
  runDurationMs,
  runWhen,
  triggerDetail,
  triggerLabel,
} from '@/lib/agentRuns'
import { formatDateTime, formatInt, formatMoney, plural } from '@/lib/format'
import type { ResourceStatus } from '@/composables/useResource'

const props = defineProps<{
  runs: AgentRun[] | null
  status: ResourceStatus
  error: ApiError | null
  starting: boolean
  settings: AgentSettings | null
}>()
defineEmits<{ start: []; retry: [] }>()

const ALL = 'all'
const TRIGGER_OPTIONS = [
  { value: ALL, label: 'Все триггеры' },
  ...TRIGGERS.map((t) => ({ value: t.id, label: t.label })),
]
const OUTCOME_OPTIONS = [{ value: ALL, label: 'Любой итог' }, ...OUTCOME_FILTERS]
const trigger = ref<string>(ALL)
const outcome = ref<string>(ALL)

const TRIGGER_ICONS: Record<string, Component> = {
  schedule: CalendarClock,
  spike: Activity,
  catalog_change: PackageSearch,
  manual: Hand,
}
const iconOf = (t: string) => TRIGGER_ICONS[t] ?? Hand

const sorted = computed(() =>
  [...(props.runs ?? [])].sort((a, b) => String(b.started_at).localeCompare(String(a.started_at))),
)
const filtered = computed(() =>
  sorted.value.filter(
    (r) =>
      (trigger.value === ALL || (r.trigger || 'manual') === trigger.value) &&
      (outcome.value === ALL || matchesOutcome(r, outcome.value)),
  ),
)
const filtering = computed(() => trigger.value !== ALL || outcome.value !== ALL)
const runHref = (r: AgentRun) => `/searchops/runs/${encodeURIComponent(r.id)}`
const now = new Date()

const nextSchedule = computed(() => {
  const s = props.settings?.schedule
  if (!s?.enabled) return ''
  if (s.next_at) return runWhen(s.next_at, now) || formatDateTime(s.next_at)
  return s.at
})
</script>

<template>
  <div class="runs">
    <section class="panel runs__mode" aria-label="Режим агента">
      <Bot :size="18" aria-hidden="true" class="runs__mode-icon" />
      <p v-if="settings" class="runs__mode-line">
        <strong data-testid="agent-mode">{{ settings.mode_label }}</strong>
        <span class="t-muted"> · </span>
        <span class="t-sm" data-testid="agent-line">{{ settings.line }}</span>
        <template v-if="nextSchedule">
          <span class="t-muted"> · </span>
          <span class="t-sm">следующий по расписанию {{ nextSchedule }}</span>
        </template>
      </p>
      <p v-else class="t-sm t-muted runs__mode-line">Режим агента не загрузился</p>
    </section>

    <div v-if="sorted.length" class="runs__filters">
      <UiSelect v-model="trigger" :options="TRIGGER_OPTIONS" label="Триггер" prefix="Триггер" />
      <UiSelect v-model="outcome" :options="OUTCOME_OPTIONS" label="Итог" prefix="Итог" />
      <span class="t-sm t-muted t-num">
        {{ formatInt(filtered.length) }}
        {{ plural(filtered.length, ['запуск', 'запуска', 'запусков']) }}
        <template v-if="filtering"> из {{ formatInt(sorted.length) }}</template>
      </span>
    </div>

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
        <p class="t-h2">
          Запусков ещё не было<template v-if="nextSchedule"
            >; следующий по расписанию {{ nextSchedule }}</template
          >
        </p>
        <p class="t-muted">
          Агент сам запускается по расписанию, на всплеск неудачных поисков и на обновление
          каталога. Разбор можно запустить и вручную.
        </p>
        <button type="button" class="btn btn--primary" :disabled="starting" @click="$emit('start')">
          <Play :size="14" aria-hidden="true" /> Запустить разбор
        </button>
      </div>
    </div>

    <div v-else-if="!filtered.length" class="panel runs__empty">
      <div>
        <p class="t-h2">По этому фильтру запусков нет</p>
        <button
          type="button"
          class="btn btn--secondary"
          @click="((trigger = ALL), (outcome = ALL))"
        >
          Сбросить фильтр
        </button>
      </div>
    </div>

    <div v-else class="data-scroll" role="region" aria-label="Запуски агента" tabindex="0">
      <table class="data-table runs__table">
        <thead>
          <tr>
            <th scope="col">Время</th>
            <th scope="col">Триггер</th>
            <th scope="col">План</th>
            <th scope="col">Итог</th>
            <th scope="col" class="num">Стоимость</th>
            <th scope="col" class="num">Длительность</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in filtered" :key="r.id" :data-trigger="r.trigger || 'manual'">
            <th scope="row">
              <RouterLink :to="runHref(r)" class="link" :title="formatDateTime(r.started_at)">{{
                runWhen(r.started_at, now)
              }}</RouterLink>
              <span class="t-xs t-muted runs__sub runs__status">
                <LoaderCircle
                  v-if="isRunActive(r.status)"
                  :size="12"
                  aria-hidden="true"
                  class="runs__spin"
                />
                {{ runStatusLabel(r.status) }}
                <template v-if="stopReasonLabel(r.stop_reason)">
                  · {{ stopReasonLabel(r.stop_reason) }}</template
                >
              </span>
            </th>
            <td>
              <span class="runs__trigger" data-testid="run-trigger">
                <component :is="iconOf(r.trigger || 'manual')" :size="14" aria-hidden="true" />
                {{ triggerLabel(r.trigger) }}
              </span>
              <span
                v-if="triggerDetail(r) !== triggerLabel(r.trigger)"
                class="t-xs t-muted runs__sub"
                >{{ triggerDetail(r) }}</span
              >
            </td>
            <td class="runs__plan">
              <span v-if="planLine(r)" :title="planLine(r)">{{ planLine(r) }}</span>
              <span v-else class="t-muted" title="Ранний запуск: план не записывался">—</span>
            </td>
            <td class="runs__outcome" data-testid="run-outcome">{{ outcomeLine(r) }}</td>
            <td class="num">{{ formatMoney(r.cost_rub) }}</td>
            <td class="num">{{ formatRunDuration(runDurationMs(r)) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.runs {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.runs__mode {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
}
.runs__mode-icon {
  flex: none;
  color: var(--text-2);
}
.runs__mode-line {
  min-width: 0;
  overflow-wrap: anywhere;
}
.runs__filters {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2) var(--space-3);
}
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
.runs__sub {
  display: block;
  font-weight: var(--fw-regular);
}
.runs__status {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  white-space: nowrap;
}
.runs__trigger {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  white-space: nowrap;
}
.runs__plan {
  max-width: 22rem;
}
.runs__plan > span {
  display: -webkit-box;
  overflow: hidden;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  line-clamp: 2;
}
.runs__outcome {
  min-width: 14rem;
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
