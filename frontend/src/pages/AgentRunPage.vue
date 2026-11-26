<script setup lang="ts">
// Страница запуска агента: лента шагов по кластерам, новые снизу (SSE, при обрыве — опрос),
// справа колонка состояния. Завершённый запуск открывается в том же виде из истории.
// ?cluster=<id> — переход из предложения «Показать ход расследования»: прокрутка к шагам этого кластера.
// Контур автономии: в шапке — триггер и вход; блок «План» (последняя редакция), «Бюджет» — израсходовано из лимита; шаги
// памяти и проверки эффекта выделены в ленте (AgentStepRow); итог — «N предложений, M задач, K пропущено по памяти».
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { ArrowLeft, CircleAlert, LoaderCircle, Square } from 'lucide-vue-next'
import { stopRun } from '@/api/agent'
import BlockError from '@/components/common/BlockError.vue'
import AgentStepRow from '@/components/searchops/AgentStepRow.vue'
import RunStateColumn from '@/components/searchops/RunStateColumn.vue'
import { useToast } from '@/components/ui/useToast'
import { useAgentRun } from '@/composables/useAgentRun'
import { isApiError } from '@/lib/apiError'
import {
  clusterOutcomes,
  isRunActive,
  runOutcomeLine,
  runStatusLabel,
  segmentByCluster,
  stopReasonLabel,
} from '@/lib/agentFeed'
import { formatDateTime, formatInt, formatMoney, plural, russianQuotes } from '@/lib/format'
import { diagnosisLabel, effectLine } from '@/lib/proposals'
import { budgetRows, isLegacyRun, outcomeLine, triggerDetail, triggerLabel } from '@/lib/agentRuns'
import { useAppStore } from '@/stores/app'

const route = useRoute()
const app = useAppStore()
const toast = useToast()

const runId = computed(() => String(route.params.id ?? ''))
const { feed, status, error, transport, reload, refresh } = useAgentRun(runId)

const run = computed(() => feed.value.run)
const steps = computed(() => feed.value.steps)
const active = computed(() => isRunActive(run.value?.status))
const segments = computed(() => segmentByCluster(steps.value, run.value))
const lastStep = computed(() => steps.value[steps.value.length - 1] ?? null)
const focusCluster = computed(() =>
  typeof route.query.cluster === 'string' ? route.query.cluster : '',
)

const outcomes = computed(() => clusterOutcomes(run.value))
const effects = computed(() =>
  (run.value?.episode?.applied_effects ?? [])
    .map((e) => ({ label: e.cluster_label ?? '', line: effectLine(e) }))
    .filter((e) => e.line),
)
/** Серверная сводка повторяет строку итога — показываем её, только если в ней есть что-то ещё. */
const extraSummary = computed(() => {
  const t = run.value?.summary ?? ''
  return t && !t.startsWith('Разобрано кластеров') ? t : ''
})

function toCluster(id: string) {
  const all = document.querySelectorAll<HTMLElement>(`[data-cluster="${CSS.escape(id)}"]`)
  const el = all[0]
  el?.scrollIntoView({ block: 'start', behavior: 'smooth' })
  el?.focus({ preventScroll: true })
}

const stopReason = computed(() => stopReasonLabel(run.value?.stop_reason))
const legacy = computed(() => !!run.value && isLegacyRun(run.value))
const plan = computed(() => run.value?.plan ?? null)
const budget = computed(() => budgetRows(run.value?.budget, run.value?.stop_reason, run.value))
const unprocessed = computed(() => run.value?.unprocessed ?? [])
const title = computed(() => {
  const r = run.value
  if (!r) return 'Запуск агента'
  if (r.query) return `Разбор запроса «${r.query}»`
  if (r.trigger === 'schedule') return 'Ночной разбор неудачных запросов'
  if (r.trigger === 'spike') return 'Разбор всплеска неудачных поисков'
  if (r.trigger === 'catalog_change') return 'Проверка после обновления каталога'
  return 'Разбор неудачных запросов'
})
const why = computed(() => {
  const w = run.value?.input?.why
  return typeof w === 'string' ? w : ''
})
const failed = computed(() => run.value?.status === 'failed' || run.value?.status === 'stopped')
const noFailures = computed(
  () =>
    !!run.value &&
    !active.value &&
    (run.value.stop_reason === 'no_failed_queries' ||
      (run.value.clusters_total === 0 && run.value.status === 'done')),
)

// Новые шаги снизу: если человек у конца ленты, лента едет за ними; если читает выше — не дёргаем.
const feedEl = ref<HTMLElement | null>(null)
function nearBottom(): boolean {
  const doc = document.documentElement
  return window.innerHeight + window.scrollY >= doc.scrollHeight - 160
}
watch(
  () => steps.value.length,
  async (n, prev) => {
    if (!active.value || !prev || n <= prev || focusCluster.value) return
    const follow = nearBottom()
    await nextTick()
    if (follow) feedEl.value?.lastElementChild?.scrollIntoView({ block: 'end' })
  },
)

// Переход из предложения: к последнему сегменту кластера, один раз после загрузки.
let focused = ''
watch(
  [segments, focusCluster],
  async () => {
    const id = focusCluster.value
    if (!id || focused === `${runId.value}:${id}` || !segments.value.length) return
    focused = `${runId.value}:${id}`
    await nextTick()
    const all = document.querySelectorAll<HTMLElement>(`[data-cluster="${CSS.escape(id)}"]`)
    const el = all[all.length - 1]
    if (el) {
      el.scrollIntoView({ block: 'start' })
      el.focus({ preventScroll: true })
    }
  },
  { immediate: true },
)

const stopping = ref(false)
async function onStop() {
  if (!run.value) return
  stopping.value = true
  try {
    await stopRun(run.value.id)
    toast.add({
      tone: 'info',
      title: 'Запуск останавливается',
      detail: 'Агент закончит текущий шаг. Готовые предложения останутся в очереди.',
    })
    void refresh()
  } catch (e) {
    toast.add({
      tone: 'error',
      title: 'Не удалось остановить',
      detail: isApiError(e) ? e.message : undefined,
    })
  } finally {
    stopping.value = false
  }
}

// Запуск закончился — обновляем счётчик очереди в левой панели.
watch(
  () => feed.value.done,
  (d, was) => {
    if (d && was === false) void app.loadPending()
  },
)

// Время идёт, пока запуск идёт.
const now = ref(Date.now())
const clock = window.setInterval(() => {
  if (active.value) now.value = Date.now()
}, 1000)
onBeforeUnmount(() => window.clearInterval(clock))
</script>

<template>
  <section :class="['page', 'run', { 'run--live': active }]" aria-labelledby="run-title">
    <RouterLink to="/searchops?tab=runs" class="run__back link t-sm">
      <ArrowLeft :size="14" aria-hidden="true" /> Все запуски
    </RouterLink>

    <header class="run__head">
      <div>
        <h1 id="run-title" class="t-h1">{{ title }}</h1>
        <p class="t-sm t-muted">
          <template v-if="run">
            {{ formatDateTime(run.started_at) }} ·
            <span data-testid="run-trigger">триггер: {{ triggerLabel(run.trigger) }}</span
            ><template v-if="triggerDetail(run) !== triggerLabel(run.trigger)">
              ({{ triggerDetail(run) }})</template
            >
            ·
            <template v-if="run.read_only">только чтение · </template>
          </template>
          <span class="t-mono">{{ runId }}</span>
        </p>
        <p v-if="why" class="t-sm run__why">{{ why }}</p>
      </div>
      <div class="run__actions">
        <span v-if="run" class="badge badge--strong">
          <LoaderCircle v-if="active" :size="12" class="run__spin" aria-hidden="true" />
          {{ runStatusLabel(run.status) }}
        </span>
        <button
          v-if="active"
          type="button"
          class="btn btn--secondary"
          :disabled="stopping"
          @click="onStop"
        >
          <Square :size="14" aria-hidden="true" />
          Остановить
        </button>
      </div>
    </header>

    <!-- Загрузка -->
    <div v-if="status === 'loading' && !run" class="run__grid" aria-busy="true">
      <div class="panel run__skel">
        <span v-for="i in 6" :key="i" class="skeleton" :style="{ width: `${90 - i * 8}%` }" />
      </div>
      <div class="panel run__skel">
        <span v-for="i in 4" :key="i" class="skeleton" />
      </div>
    </div>

    <BlockError
      v-else-if="status === 'error' && error"
      :error="error"
      :title="error.status === 404 ? 'Запуск не найден' : 'Запуск не загрузился'"
      :back="{ label: 'Все запуски', to: { path: '/searchops', query: { tab: 'runs' } } }"
      @retry="reload()"
    />

    <div v-else-if="run" class="run__grid">
      <div class="run__main">
        <!-- План: последняя редакция (модель или по приоритету) -->
        <section v-if="plan" class="panel run__plan" aria-label="План запуска">
          <p class="run__block-title">
            План<template v-if="plan.revision > 1"> · редакция {{ plan.revision }}</template>
            <span class="t-xs t-muted">
              ·
              {{
                plan.source === 'model'
                  ? 'построила модель'
                  : 'по приоритету (модель не ответила по схеме)'
              }}</span
            >
          </p>
          <p>{{ russianQuotes(plan.summary) }}</p>
          <p v-if="plan.change" class="t-sm t-muted">Изменение: {{ russianQuotes(plan.change) }}</p>
          <ol v-if="plan.clusters?.length" class="run__plan-list t-sm">
            <li v-for="c in plan.clusters" :key="c.cluster_id">
              <button type="button" class="link" @click="toCluster(c.cluster_id)">
                {{ c.label }}
              </button>
              <span v-if="c.why" class="t-muted"> — {{ russianQuotes(c.why) }}</span>
            </li>
          </ol>
          <p v-if="plan.dropped?.length" class="t-xs t-muted">
            Выпало из плана: {{ plan.dropped.map((d) => `«${d.label}»`).join(', ') }}
          </p>
        </section>

        <!-- Бюджет: израсходовано из лимита -->
        <section v-if="budget.length" class="panel run__budget" aria-label="Бюджет запуска">
          <p class="run__block-title">
            Бюджет запуска · израсходовано из лимита{{ active ? ' на сейчас' : '' }}
          </p>
          <ul class="run__budget-list">
            <li v-for="b in budget" :key="b.key" :class="{ 'run__budget--hit': b.exhausted }">
              <span class="t-sm">{{ b.label }}</span>
              <span class="t-num t-sm">{{ b.used }} из {{ b.limit }}</span>
              <span class="run__bar" aria-hidden="true"
                ><span :style="{ width: `${Math.round(b.share * 100)}%` }"
              /></span>
            </li>
          </ul>
          <p v-if="unprocessed.length" class="t-sm t-muted">
            Не разобрано: {{ unprocessed.map((u) => `«${u.label}»`).join(', ') }}
          </p>
        </section>

        <!-- Остановлен или упал: причина, готовые предложения сохранены -->
        <div v-if="failed" class="panel run__stopped" role="status">
          <CircleAlert :size="18" aria-hidden="true" class="run__stopped-icon" />
          <div>
            <p class="run__stopped-title">
              Запуск {{ run.status === 'failed' ? 'прерван ошибкой' : 'остановлен'
              }}<template v-if="stopReason">: {{ stopReason }}</template>
            </p>
            <p class="t-sm t-muted">
              Уже готовые предложения сохранены
              <template v-if="run.proposals_created">
                ({{ run.proposals_created }}) —
                <RouterLink to="/searchops?tab=queue" class="link">открыть очередь</RouterLink>
              </template>
              <template v-else>; этот запуск предложений не успел создать</template>.
            </p>
          </div>
        </div>

        <!-- Итог завершённого запуска -->
        <div v-if="!active && run.status === 'done' && !noFailures" class="panel run__outcome">
          <p class="t-h2">Итог</p>
          <p data-testid="run-outcome">{{ legacy ? runOutcomeLine(run) : outcomeLine(run) }}</p>
          <p v-if="!legacy && stopReason" class="t-sm t-muted">Остановка: {{ stopReason }}</p>
          <p v-if="run.episode?.metrics?.failed_queries" class="t-sm t-muted run__metrics">
            За {{ run.episode.metrics.days ?? run.days ?? 30 }} дней:
            {{ formatInt(run.episode.metrics.failed_queries) }} неудачных запросов из
            {{ formatInt(run.episode.metrics.distinct_queries ?? 0) }} различных ·
            {{ formatInt(run.input_tokens) }} / {{ formatInt(run.output_tokens) }} токенов
          </p>
          <p v-if="extraSummary" class="t-sm t-muted">{{ extraSummary }}</p>
          <ul v-if="effects.length" class="run__effects t-sm">
            <li v-for="e in effects" :key="e.label">
              Эффект принятой правки «{{ e.label }}»: {{ e.line!.text.toLowerCase() }} —
              {{ e.line!.holds ? 'держится' : 'не держится' }}
            </li>
          </ul>
          <div v-if="outcomes.length" class="data-scroll">
            <table class="data-table run__outcomes">
              <caption class="sr-only">
                Итог по кластерам
              </caption>
              <thead>
                <tr>
                  <th scope="col">Кластер</th>
                  <th scope="col">Диагноз</th>
                  <th scope="col" class="num">Гипотез</th>
                  <th scope="col">Итог</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="o in outcomes" :key="o.id">
                  <th scope="row">
                    <button type="button" class="link" @click="toCluster(o.id)">
                      {{ o.label }}
                    </button>
                  </th>
                  <td>{{ diagnosisLabel(o.diagnosis) }}</td>
                  <td class="num">
                    <template v-if="o.hypotheses">
                      {{ o.hypotheses
                      }}<template v-if="o.rejected">, отклонено {{ o.rejected }}</template>
                    </template>
                    <span v-else class="t-muted">—</span>
                  </td>
                  <td>
                    <RouterLink
                      v-if="o.proposalId"
                      :to="{
                        path: '/searchops',
                        query: { tab: 'queue', status: 'all', p: o.proposalId },
                      }"
                      class="link"
                      >{{ o.outcome }}</RouterLink
                    >
                    <span v-else class="t-muted">{{ o.outcome }}</span>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <p class="t-sm">
            Стоимость запуска:
            <RouterLink to="/proofs?tab=cost" class="num-btn">{{
              formatMoney(run.cost_rub)
            }}</RouterLink>
            <template v-if="run.proposals_created">
              ·
              <RouterLink to="/searchops?tab=queue" class="link">Открыть очередь</RouterLink>
            </template>
          </p>
        </div>

        <div v-if="noFailures" class="panel run__empty">
          <p class="t-h2">Неудачных запросов за период нет</p>
          <p class="t-muted">
            Агенту нечего разбирать: пустых выдач, поисков без клика и переформулировок не было.
          </p>
        </div>

        <p v-else-if="!steps.length" class="panel run__empty t-muted">
          <LoaderCircle :size="16" class="run__spin" aria-hidden="true" />
          Агент начинает: снимает метрики и собирает неудачные запросы…
        </p>

        <ol v-else ref="feedEl" class="run__feed" aria-label="Шаги агента">
          <li
            v-for="(seg, i) in segments"
            :key="`${seg.clusterId}-${i}`"
            class="panel run__segment"
            :class="{ 'run__segment--focus': seg.clusterId && seg.clusterId === focusCluster }"
            :data-cluster="seg.clusterId ?? undefined"
            tabindex="-1"
          >
            <h2 class="run__segment-title">
              <template v-if="seg.clusterId">Кластер «{{ seg.label }}»</template>
              <template v-else>{{ seg.label }}</template>
              <span class="t-xs t-muted t-num"
                >· {{ seg.steps.length }}
                {{ plural(seg.steps.length, ['шаг', 'шага', 'шагов']) }}</span
              >
            </h2>
            <ol class="run__steps">
              <AgentStepRow v-for="v in seg.steps" :key="v.step.id || v.step.seq" :view="v" />
            </ol>
          </li>
        </ol>

        <p v-if="active && steps.length" class="run__live t-sm t-muted">
          <LoaderCircle :size="14" class="run__spin" aria-hidden="true" />
          Агент работает — шаги появляются по мере выполнения
        </p>

        <p class="sr-only" aria-live="polite">
          <template v-if="lastStep">{{ lastStep.title }}</template>
        </p>
      </div>

      <RunStateColumn :run="run" :steps="steps" :transport="transport" :now="now" />
    </div>
  </section>
</template>

<style scoped>
.run {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  max-width: 80rem;
}
.run__back {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  align-self: flex-start;
}
.run__head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-3);
}
.run__head > div:first-child {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 0;
}
.run__actions {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}
.run__grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 17rem;
  align-items: start;
  gap: var(--space-4);
}
.run__main {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  min-width: 0;
}
.run__skel {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
}
.run__feed {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  margin: 0;
  padding: 0;
  list-style: none;
}
.run__segment {
  overflow: hidden;
  scroll-margin-top: calc(var(--header-h) + var(--space-4));
}
.run__segment:focus-visible {
  outline-offset: 0;
}
.run__segment--focus {
  border-color: var(--border-control);
}
.run__segment-title {
  display: flex;
  align-items: baseline;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  border-bottom: var(--border-w) solid var(--border);
  background: var(--surface-2);
  font-size: var(--fs-body);
  font-weight: var(--fw-semibold);
}
.run__steps {
  margin: 0;
  padding: 0;
  list-style: none;
}
.run__steps > :last-child {
  border-bottom: 0;
}
.run__live,
.run__empty {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}
.run__empty {
  flex-direction: column;
  align-items: flex-start;
  padding: var(--space-4);
}
p.run__empty {
  flex-direction: row;
  align-items: center;
}
.run__stopped {
  display: flex;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-4);
  border-color: var(--danger-bg);
}
.run__stopped-icon {
  flex: none;
  color: var(--danger);
}
.run__stopped-title {
  font-weight: var(--fw-medium);
}
.run__why {
  color: var(--text-2);
}
.run__plan,
.run__budget {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
}
.run__block-title {
  font-weight: var(--fw-semibold);
}
.run__plan-list {
  margin: 0;
  padding-left: var(--space-5);
}
.run__budget-list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(11rem, 1fr));
  gap: var(--space-2) var(--space-4);
  margin: 0;
  padding: 0;
  list-style: none;
}
.run__budget-list > li {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 2px var(--space-2);
}
.run__bar {
  grid-column: 1 / -1;
  height: 4px;
  overflow: hidden;
  border-radius: var(--radius-sm);
  background: var(--surface-3);
}
.run__bar > span {
  display: block;
  height: 100%;
  background: var(--text-2);
}
.run__budget--hit .run__bar > span {
  background: var(--status-pending);
}
.run__budget--hit > span:nth-child(2) {
  color: var(--status-pending);
}
.run__outcome {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-4);
}
.run__metrics {
  font-variant-numeric: tabular-nums;
}
.run__effects {
  margin: 0;
  padding: 0;
  list-style: none;
}
.run__outcomes th[scope='row'] {
  font-weight: var(--fw-regular);
}
.run__spin {
  animation: run-spin 1s linear infinite;
}
@keyframes run-spin {
  to {
    transform: rotate(360deg);
  }
}
@media (max-width: 1023px) {
  .run__grid {
    grid-template-columns: minmax(0, 1fr);
  }
  /* На узком экране состояние — над лентой: шаг и стоимость видны сразу. */
  .run__grid > :last-child {
    order: -1;
  }
}
</style>
