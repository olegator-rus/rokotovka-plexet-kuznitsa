<script setup lang="ts">
// «До / После». Вкладки — reka Tabs, выбранная вкладка и запрос сравнения живут в адресе:
//   /compare?tab=metrics | one&q=…&left=B0&right=B3 | regressions&cfg=B3 | effect
// «Эффект агента» — реплей отложенной недели (набор holdout_week), только чтение.
// Прогон оценки — POST /api/evaluations/run, дальше опрос GET /api/evaluations, пока идёт; с экрана
// можно уйти и вернуться. Непосчитанные метрики не показываются, чисел-заглушек нет.
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { TabsContent, TabsList, TabsRoot, TabsTrigger } from 'reka-ui'
import { LoaderCircle, Play, RotateCcw } from 'lucide-vue-next'
import { getEvaluations, runEvaluation } from '@/api/evaluations'
import type { ConfigId, EvalJob } from '@/api/types'
import BlockError from '@/components/common/BlockError.vue'
import AgentEffect from '@/components/compare/AgentEffect.vue'
import JudgeAgreement from '@/components/compare/JudgeAgreement.vue'
import MetricsTable from '@/components/compare/MetricsTable.vue'
import OneQueryCompare from '@/components/compare/OneQueryCompare.vue'
import RegressionsList from '@/components/compare/RegressionsList.vue'
import { useToast } from '@/components/ui/useToast'
import { useResource } from '@/composables/useResource'
import { type ApiError, isApiError, MSG_UNKNOWN } from '@/lib/apiError'
import { evalColumns, latestRun, regressionCount, reproducibility, shortHash } from '@/lib/evalView'
import { formatDateTime, formatInt, formatMs, plural } from '@/lib/format'
import { isConfigId } from '@/lib/searchState'
import { useAppStore } from '@/stores/app'

const POLL_MS = 2000
const EVAL_SET = 'test'
const TABS = ['metrics', 'one', 'regressions', 'effect'] as const
type Tab = (typeof TABS)[number]

const route = useRoute()
const router = useRouter()
const app = useAppStore()
const { configs, defaultConfig } = storeToRefs(app)
const toast = useToast()

const evals = useResource((opts) => getEvaluations(opts))
const columns = computed(() => evalColumns(evals.data.value))
const repro = computed(() => reproducibility(columns.value))
const regressionTotal = computed(() => regressionCount(columns.value))
const latest = computed(() => latestRun(columns.value))
const judge = computed(() => evals.data.value?.judge_agreement ?? null)
const hasB4 = computed(() => columns.value.some((c) => c.config === 'B4'))
// Правки B4 из запусков агента до разреза журнала: на test B4 — не независимая оценка (GET /api/evaluations).
const b4PreHoldout = computed(() => evals.data.value?.agent_credit?.pre_holdout === true)

// ── Адрес ──
const str = (v: unknown) => (typeof v === 'string' ? v : '')
const tab = computed<Tab>({
  get: () => {
    const t = str(route.query.tab)
    return (TABS as readonly string[]).includes(t) ? (t as Tab) : 'metrics'
  },
  set: (t) => void router.replace({ query: { ...route.query, tab: t } }),
})
const oneQuery = computed(() => str(route.query.q))
const left = computed<ConfigId>(() => {
  const v = str(route.query.left)
  return isConfigId(v) ? v : 'B0'
})
const right = computed<ConfigId>(() => {
  const v = str(route.query.right)
  return isConfigId(v) ? v : defaultConfig.value
})
const regFilter = computed<string>({
  get: () => str(route.query.cfg) || 'all',
  set: (v) => void router.replace({ query: { ...route.query, cfg: v === 'all' ? undefined : v } }),
})

function onOneChange(q: string, l: ConfigId, r: ConfigId) {
  void router.replace({
    query: { ...route.query, tab: 'one', q: q || undefined, left: l, right: r },
  })
}
function onCompare(q: string, l: string, r: string) {
  void router.push({ query: { tab: 'one', q, left: l, right: r } })
}
function onRegressions(config: string) {
  void router.push({ query: { tab: 'regressions', cfg: config } })
}

// ── Прогон ──
// Идущий прогон, а без него — последний завершённый (удачный или с ошибкой).
const job = computed<EvalJob | null>(
  () => evals.data.value?.running ?? evals.data.value?.last_job ?? null,
)
const starting = ref(false)
const runError = ref<ApiError | null>(null)
const running = computed(() => starting.value || job.value?.status === 'running')
const isActive = (s: string | undefined) => s === 'running'
let timer: ReturnType<typeof setTimeout> | undefined

function schedulePoll() {
  clearTimeout(timer)
  if (!running.value) return
  timer = setTimeout(async () => {
    await evals.load({ quiet: true })
    schedulePoll()
  }, POLL_MS)
}

watch(
  () => job.value?.status,
  (now, before) => {
    if (isActive(now)) schedulePoll()
    if (isActive(before) && now === 'done')
      toast.add({ tone: 'ok', title: 'Прогон завершён', detail: 'Таблица метрик обновлена' })
    if (isActive(before) && now === 'failed')
      toast.add({ tone: 'error', title: 'Прогон не выполнен', detail: job.value?.error })
  },
)

async function startRun() {
  starting.value = true
  runError.value = null
  try {
    const list = configs.value.filter((c) => c !== 'B4' || (app.meta?.rules_version ?? 1) > 1)
    await runEvaluation({ set: EVAL_SET, configs: [...list] })
    await evals.load({ quiet: true })
  } catch (e) {
    runError.value = isApiError(e) ? e : { status: 0, code: 'UNKNOWN', message: MSG_UNKNOWN }
  } finally {
    starting.value = false
    schedulePoll()
  }
}

const progress = computed(() => {
  const j = job.value
  if (!j || typeof j.total !== 'number' || j.total <= 0) return null
  return { done: j.done ?? 0, total: j.total }
})

// Сколько идёт прогон: тикает раз в секунду, пока он идёт.
const now = ref(Date.now())
let tick: ReturnType<typeof setInterval> | undefined
watch(
  running,
  (r) => {
    clearInterval(tick)
    if (r) tick = setInterval(() => (now.value = Date.now()), 1000)
  },
  { immediate: true },
)
const elapsed = computed(() => {
  const started = job.value?.started_at ? Date.parse(job.value.started_at) : NaN
  return Number.isFinite(started) ? Math.max(0, now.value - started) : null
})

onMounted(async () => {
  await evals.load()
  schedulePoll()
})
onBeforeUnmount(() => {
  clearTimeout(timer)
  clearInterval(tick)
})
</script>

<template>
  <section class="page compare" aria-labelledby="compare-title">
    <header class="compare__head">
      <div>
        <h1 id="compare-title" class="t-h1">До / После</h1>
        <p class="t-sm t-muted">
          Лестница конфигураций на замороженном наборе: от реконструкции исходного поиска (B0) до
          полной архитектуры.
        </p>
      </div>
      <div class="compare__run">
        <button
          type="button"
          class="btn btn--secondary"
          :disabled="running || evals.status.value === 'loading'"
          @click="startRun"
        >
          <LoaderCircle v-if="running" :size="16" aria-hidden="true" class="spin" />
          <Play v-else :size="16" aria-hidden="true" />
          {{ running ? 'Идёт прогон' : 'Запустить прогон' }}
        </button>
        <p v-if="running" class="t-xs t-muted compare__progress" role="status">
          <template v-if="progress">
            Выполнено {{ formatInt(progress.done) }} из {{ formatInt(progress.total) }}
            {{ plural(progress.total, ['поиска', 'поисков', 'поисков']) }}
            ({{ job?.configs.length }}
            {{
              plural(job?.configs.length ?? 0, ['конфигурация', 'конфигурации', 'конфигураций'])
            }})
            <template v-if="elapsed !== null"> · {{ formatMs(elapsed) }}</template>
          </template>
          <template v-else-if="job?.status === 'running'">
            {{ job.configs.join(', ') }} на наборе {{ job.set }}
            <template v-if="repro">
              · {{ formatInt(repro.queryCount) }}
              {{ plural(repro.queryCount, ['запрос', 'запроса', 'запросов']) }}
            </template>
            <template v-if="elapsed !== null"> · идёт {{ formatMs(elapsed) }}</template>
          </template>
          <template v-else>Запускаем прогон…</template>
        </p>
      </div>
    </header>

    <BlockError v-if="runError" :error="runError" title="Прогон не запустился" @retry="startRun" />
    <div v-if="job?.status === 'failed' && latest" class="panel compare__failed" role="alert">
      Новый прогон не выполнен{{ job.error ? `: ${job.error}` : '' }}. Ниже — последний удачный ({{
        formatDateTime(latest.created_at)
      }}).
    </div>

    <!-- Загрузка -->
    <div
      v-if="evals.status.value === 'loading' && !evals.data.value"
      class="compare__skeleton"
      aria-busy="true"
    >
      <span class="skeleton" style="width: 40%" />
      <div class="panel compare__skeleton-table">
        <span v-for="i in 6" :key="i" class="skeleton" :style="{ width: `${90 - i * 5}%` }" />
      </div>
    </div>

    <!-- Ошибка без данных -->
    <BlockError
      v-else-if="evals.status.value === 'error' && !evals.data.value"
      :error="evals.error.value!"
      title="Результаты прогонов не загрузились"
      @retry="evals.load()"
    />

    <TabsRoot v-else v-model="tab" class="compare__tabs">
      <TabsList class="tabs" aria-label="Разделы сравнения">
        <TabsTrigger value="metrics" class="tabs__trigger">Метрики</TabsTrigger>
        <TabsTrigger value="one" class="tabs__trigger">Один запрос</TabsTrigger>
        <TabsTrigger value="regressions" class="tabs__trigger">
          Ухудшения
          <span v-if="columns.length" class="badge t-num">{{ regressionTotal }}</span>
        </TabsTrigger>
        <TabsTrigger value="effect" class="tabs__trigger">Эффект агента</TabsTrigger>
      </TabsList>

      <TabsContent value="metrics" class="compare__panel">
        <div v-if="!columns.length" class="panel compare__empty">
          <p class="t-h2">Прогон ещё не запускался</p>
          <p class="t-muted">
            Метрики появятся после прогона конфигураций на тестовом наборе. Он идёт в фоне — с
            экрана можно уйти и вернуться.
          </p>
          <div>
            <button type="button" class="btn btn--primary" :disabled="running" @click="startRun">
              <Play :size="16" aria-hidden="true" />
              Запустить прогон
            </button>
          </div>
        </div>
        <template v-else>
          <p v-if="repro" class="t-sm">
            Набор <strong>{{ repro.set }}</strong>: {{ formatInt(repro.queryCount) }}
            {{ plural(repro.queryCount, ['запрос', 'запроса', 'запросов']) }}, заморожен — поиск
            B0–B3 на нём не настраивался.
            <span v-if="!repro.sameSet" class="t-muted">
              Внимание: столбцы посчитаны на разных версиях набора.
            </span>
          </p>
          <MetricsTable
            :columns="columns"
            :judge="judge?.set === repro?.set ? judge : null"
            :b4-pre-holdout="b4PreHoldout"
            @regressions="onRegressions"
          />
          <p v-if="repro" class="t-xs t-muted compare__repro">
            Воспроизводимость: хэш набора
            <span class="t-mono" :title="repro.setHash">{{ shortHash(repro.setHash) }}</span>
            · правила {{ repro.rulesVersions.map((v) => `v${v}`).join(', ') }} · каталог
            {{ repro.catalogVersions.join(', ') || '—' }} · коммит
            <span class="t-mono">{{
              repro.commits.map((c) => shortHash(c)).join(', ') || '—'
            }}</span>
            · <span class="t-mono">make eval</span> · последний прогон
            {{ latest ? formatDateTime(latest.created_at) : '—' }}
          </p>
          <p v-if="hasB4 && b4PreHoldout" class="t-sm">
            Столбец B4 — не независимая оценка: действующая правка агента предложена запуском до
            исправления оценки, когда агент ещё видел тексты test и отложенную неделю. Подробности —
            на вкладке
            <button type="button" class="link" @click="tab = 'effect'">«Эффект агента»</button>.
          </p>
          <p v-else-if="hasB4 && repro?.set === 'test'" class="t-sm t-muted">
            Столбец B4 на test — оценка обобщения правок агента на запросах, которых он не видит, а
            не их эффект. Эффект — на вкладке
            <button type="button" class="link" @click="tab = 'effect'">«Эффект агента»</button>
            (реплей отложенной недели).
          </p>
          <JudgeAgreement v-if="judge" :judge="judge" />
          <p
            v-if="evals.status.value === 'error' && evals.error.value"
            class="t-xs compare__stale"
            role="alert"
          >
            Не удалось обновить: {{ evals.error.value.message }}
            <button type="button" class="link" @click="evals.load()">
              <RotateCcw :size="12" aria-hidden="true" /> Повторить
            </button>
          </p>
        </template>
      </TabsContent>

      <TabsContent value="one" class="compare__panel">
        <OneQueryCompare :query="oneQuery" :left="left" :right="right" @change="onOneChange" />
      </TabsContent>

      <TabsContent value="effect" class="compare__panel">
        <AgentEffect />
      </TabsContent>

      <TabsContent value="regressions" class="compare__panel">
        <p v-if="!columns.length" class="panel compare__empty">
          Прогон ещё не запускался — ухудшения считаются по его результатам.
        </p>
        <RegressionsList
          v-else
          v-model:filter="regFilter"
          :columns="columns"
          @compare="onCompare"
        />
      </TabsContent>
    </TabsRoot>
  </section>
</template>

<style scoped>
.compare {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
.compare__head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-3);
}
.compare__head > div:first-child {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  max-width: 40rem;
}
.compare__run {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: var(--space-1);
}
.compare__progress {
  font-variant-numeric: tabular-nums;
  text-align: right;
}
.compare__failed {
  padding: var(--space-3) var(--space-4);
  border-color: var(--danger-bg);
  color: var(--text-2);
  font-size: var(--fs-sm);
}
.compare__skeleton {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.compare__skeleton-table {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
}
.compare__tabs {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
.compare__panel {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.compare__panel:focus-visible {
  outline-offset: 4px;
}
.compare__empty {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-5);
  color: var(--text-2);
}
.compare__repro {
  overflow-wrap: anywhere;
}
.compare__stale {
  color: var(--danger);
}
.spin {
  animation: spin 1s linear infinite;
}
@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
@media (max-width: 639px) {
  .compare__run {
    align-items: flex-start;
  }
  .compare__progress {
    text-align: left;
  }
}
</style>
