<script setup lang="ts">
// Страница запуска агента: лента шагов по кластерам, новые снизу (SSE, при обрыве — опрос),
// справа колонка состояния. Завершённый запуск открывается в том же виде из истории.
// ?cluster=<id> — переход из предложения «Показать ход расследования»: прокрутка к шагам этого кластера.
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
  isRunActive,
  runOutcomeLine,
  runStatusLabel,
  segmentByCluster,
  stopReasonLabel,
} from '@/lib/agentFeed'
import { formatDateTime, formatRub, plural } from '@/lib/format'
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

const stopReason = computed(() => stopReasonLabel(run.value?.stop_reason))
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
  <section class="page run" aria-labelledby="run-title">
    <RouterLink to="/searchops?tab=runs" class="run__back link t-sm">
      <ArrowLeft :size="14" aria-hidden="true" /> Все запуски
    </RouterLink>

    <header class="run__head">
      <div>
        <h1 id="run-title" class="t-h1">Разбор неудачных запросов</h1>
        <p class="t-sm t-muted">
          <template v-if="run">
            {{ formatDateTime(run.started_at) }} ·
            {{ run.trigger === 'schedule' ? 'по расписанию' : 'запущен вручную' }} ·
          </template>
          <span class="t-mono">{{ runId }}</span>
        </p>
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
      @retry="reload()"
    />

    <div v-else-if="run" class="run__grid">
      <div class="run__main">
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

        <!-- Итог завершённого запуска -->
        <div v-if="!active && run.status === 'done' && !noFailures" class="panel run__outcome">
          <p class="t-h2">Итог</p>
          <p>{{ runOutcomeLine(run) }}</p>
          <p v-if="run.summary" class="t-sm t-muted">{{ run.summary }}</p>
          <p class="t-sm">
            Стоимость запуска:
            <RouterLink to="/costs" class="num-btn">{{ formatRub(run.cost_rub) }}</RouterLink>
            <template v-if="run.proposals_created">
              ·
              <RouterLink to="/searchops?tab=queue" class="link">Открыть очередь</RouterLink>
            </template>
          </p>
        </div>

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
.run__outcome {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-4);
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
