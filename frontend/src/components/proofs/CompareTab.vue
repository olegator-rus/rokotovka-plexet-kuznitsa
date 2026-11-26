<script setup lang="ts">
// «Доказательства» → «До / После» (контур доводки — вкладка раздела «Доказательства»). Виды — reka Tabs,
// выбранный вид и запрос сравнения живут в адресе:
//   /proofs?tab=compare&view=metrics | one&q=…&left=B0&right=B3 | regressions&cfg=B3 | commerce | panel
// «Коммерция» (контур коммерции) — лестница C0–C3 на наборе p2; «Панель» (контур выдачи) — прогон по префиксам.
// Вкладка «Эффект агента» удалена в контуре доводки.
// Прогон оценки — POST /api/evaluations/run, дальше опрос GET /api/evaluations, пока идёт; с экрана
// можно уйти и вернуться. Непосчитанные метрики не показываются, чисел-заглушек нет.
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { LoaderCircle, Play, RotateCcw } from 'lucide-vue-next'
import { getEvaluations, runEvaluation } from '@/api/evaluations'
import type { ConfigId, EvalJob } from '@/api/types'
import BlockError from '@/components/common/BlockError.vue'
import CommerceTab from '@/components/compare/CommerceTab.vue'
import JudgeAgreement from '@/components/compare/JudgeAgreement.vue'
import MetricsTable from '@/components/compare/MetricsTable.vue'
import OneQueryCompare from '@/components/compare/OneQueryCompare.vue'
import PanelTab from '@/components/compare/PanelTab.vue'
import RegressionsList from '@/components/compare/RegressionsList.vue'
import { useToast } from '@/components/ui/useToast'
import { useResource } from '@/composables/useResource'
import { type ApiError, isApiError, MSG_UNKNOWN } from '@/lib/apiError'
import { evalColumns, latestRun, regressionCount, reproducibility, shortHash } from '@/lib/evalView'
import { formatInt, formatMs, plural } from '@/lib/format'
import { MOCK_RUN_LABEL, catalogLabel, isMockCatalog } from '@/lib/feedSummary'
import { isConfigId } from '@/lib/searchState'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'

const POLL_MS = 2000
const EVAL_SET = 'test'
const TABS = ['metrics', 'one', 'regressions', 'commerce', 'panel'] as const
type Tab = (typeof TABS)[number]

const emit = defineEmits<{ regressionsTotal: [total: number | null] }>()

const route = useRoute()
const router = useRouter()
const app = useAppStore()
// Прогон оценки — бэк-офис (POST /api/evaluations/run): гостю кнопки нет, таблица последнего прогона открыта.
const auth = useAuthStore()
const { configs, defaultConfig } = storeToRefs(app)
const toast = useToast()

const evals = useResource((opts) => getEvaluations(opts))
const columns = computed(() => evalColumns(evals.data.value))
const repro = computed(() => reproducibility(columns.value))
const regressionTotal = computed(() => regressionCount(columns.value))
const latest = computed(() => latestRun(columns.value))
/** Все столбцы посчитаны на mock-каталоге — до данных заказчика (подпись над таблицей). */
const mockOnly = computed(
  () => columns.value.length > 0 && columns.value.every((c) => isMockCatalog(c.catalog_version)),
)
const judge = computed(() => evals.data.value?.judge_agreement ?? null)
const hasB4 = computed(() => columns.value.some((c) => c.config === 'B4'))
// Правки B4 из запусков агента до разреза журнала: на test B4 — не независимая оценка (GET /api/evaluations).
const b4PreHoldout = computed(() => evals.data.value?.agent_credit?.pre_holdout === true)

// ── Адрес ──
const str = (v: unknown) => (typeof v === 'string' ? v : '')
/** Экран выбирает меню раздела (ProofsPage); здесь — только показ выбранного. */
const tab = computed<Tab>(() => {
  const t = str(route.query.view)
  return (TABS as readonly string[]).includes(t) ? (t as Tab) : 'metrics'
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
    query: { ...route.query, view: 'one', q: q || undefined, left: l, right: r },
  })
}
function onCompare(q: string, l: string, r: string) {
  void router.push({ query: { tab: 'compare', view: 'one', q, left: l, right: r } })
}
function onRegressions(config: string) {
  void router.push({ query: { tab: 'compare', view: 'regressions', cfg: config } })
}

watch(
  () => (columns.value.length ? regressionTotal.value : null),
  (total) => emit('regressionsTotal', total),
  { immediate: true },
)

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
  <section class="compare" aria-labelledby="compare-title">
    <h2 id="compare-title" class="sr-only">До / После</h2>
    <!-- Прогон оценки нужен там, где показываются его числа. -->
    <header v-if="auth.staff && (tab === 'metrics' || tab === 'regressions')" class="compare__head">
      <div v-if="auth.staff" class="compare__run">
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
      Новый прогон не выполнен{{ job.error ? `: ${job.error}` : '' }}. Ниже — последний удачный.
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

    <div v-else class="compare__tabs">
      <div v-if="tab === 'metrics'" class="compare__panel">
        <div v-if="!columns.length" class="panel compare__empty">
          <p class="t-h2">Прогон ещё не запускался</p>
          <p class="t-muted">
            Метрики появятся после прогона конфигураций на тестовом наборе. Он идёт в фоне — с
            экрана можно уйти и вернуться.
          </p>
          <div v-if="auth.staff">
            <button type="button" class="btn btn--primary" :disabled="running" @click="startRun">
              <Play :size="16" aria-hidden="true" />
              Запустить прогон
            </button>
          </div>
        </div>
        <template v-else>
          <p v-if="mockOnly" class="panel t-sm compare__mock">
            Прогон на каталоге «{{ MOCK_RUN_LABEL }}». Числа на фиде магазина —
            <RouterLink :to="{ query: { tab: 'feed' } }">«На данных заказчика»</RouterLink>.
          </p>
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
            {{ repro.catalogVersions.map(catalogLabel).join(', ') || '—' }} · коммит
            <span class="t-mono">{{
              repro.commits.map((c) => shortHash(c)).join(', ') || '—'
            }}</span>
            · <span class="t-mono">make eval</span>
          </p>
          <p v-if="hasB4 && b4PreHoldout" class="t-sm">
            Столбец B4 — не независимая оценка: действующая правка агента предложена запуском до
            исправления оценки, когда агент ещё видел тексты test и отложенную неделю.
          </p>
          <p v-else-if="hasB4 && repro?.set === 'test'" class="t-sm t-muted">
            Столбец B4 на test — оценка обобщения правок агента на запросах, которых он не видит, а
            не их эффект.
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
      </div>

      <div v-else-if="tab === 'one'" class="compare__panel">
        <OneQueryCompare :query="oneQuery" :left="left" :right="right" @change="onOneChange" />
      </div>

      <div v-else-if="tab === 'commerce'" class="compare__panel">
        <CommerceTab />
      </div>

      <div v-else-if="tab === 'panel'" class="compare__panel">
        <PanelTab />
      </div>

      <div v-else-if="tab === 'regressions'" class="compare__panel">
        <p v-if="!columns.length" class="panel compare__empty">
          Прогон ещё не запускался — ухудшения считаются по его результатам.
        </p>
        <RegressionsList
          v-else
          v-model:filter="regFilter"
          :columns="columns"
          @compare="onCompare"
        />
      </div>
    </div>
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
  justify-content: flex-end;
  gap: var(--space-3);
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
.compare__mock {
  padding: var(--space-3) var(--space-4);
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
