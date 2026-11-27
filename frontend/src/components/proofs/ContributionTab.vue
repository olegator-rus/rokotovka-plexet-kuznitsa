<script setup lang="ts">
// «Доказательства → Вклад компонентов» (контур доводки, ): лестница от точного совпадения
// до полного пути и прогоны полного пути без одного компонента — качество, пустые, нарушения условий, задержка,
// стоимость; блокирующие проверки; предварительные решения по реранкеру, векторам и LLM; строка воспроизводимости.
// Числа — GET /api/evaluations/contribution; непосчитанное не показывается. Сотруднику — прогон и режим «без моделей».
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { CircleCheck, CircleX, Play, PowerOff, Power, RotateCcw } from 'lucide-vue-next'
import {
  type ContributionSet,
  getContribution,
  runContribution,
  setOfflineMode,
} from '@/api/contribution'
import BlockError from '@/components/common/BlockError.vue'
import { useResource } from '@/composables/useResource'
import { isApiError } from '@/lib/apiError'
import { formatDecimal, formatInt, formatPercent, plural } from '@/lib/format'
import { MOCK_RUN_LABEL, isMockCatalog, runTail } from '@/lib/feedSummary'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'

const SETS: { id: ContributionSet; label: string }[] = [
  { id: 'test', label: 'test — замороженный' },
  { id: 'tune', label: 'tune' },
  { id: 'regression', label: 'regression' },
]

const auth = useAuthStore()
const app = useAppStore()
const set = ref<ContributionSet>('test')
const res = useResource((opts) => getContribution(set.value, opts))
void res.load()
watch(set, () => void res.load())

const data = computed(() => res.data.value)
const run = computed(() => data.value?.run ?? null)
/** Прогон с метриками по семействам (после их появления): в таблице — они, иначе прежние по артикулу. */
const famCols = computed(() =>
  (run.value?.rows ?? []).some((r) => typeof r.metrics.exact_accuracy_family === 'number'),
)
const ladder = computed(() => run.value?.rows.filter((r) => r.kind === 'ladder') ?? [])
const singles = computed(() => run.value?.rows.filter((r) => r.kind === 'single_change') ?? [])
const blockingFailed = computed(
  () => run.value?.checks.filter((c) => c.blocking && !c.passed).length ?? 0,
)
const running = computed(() => data.value?.running ?? null)
const modes = computed(() => data.value?.modes ?? null)
const offline = computed(() => modes.value?.vectors.offline ?? false)

// Прогон идёт — опрос раз в 2 с, пока не закончится.
let timer: ReturnType<typeof setTimeout> | null = null
watch(running, (r) => {
  if (timer) clearTimeout(timer)
  timer = r ? setTimeout(() => void res.load({ quiet: true }), 2000) : null
})
onBeforeUnmount(() => timer && clearTimeout(timer))

const actionError = ref<string | null>(null)
const busy = ref(false)
async function startRun() {
  actionError.value = null
  busy.value = true
  try {
    await runContribution(set.value)
    await res.load({ quiet: true })
  } catch (e) {
    actionError.value = isApiError(e) ? e.message : 'Прогон не запустился'
  } finally {
    busy.value = false
  }
}
async function toggleOffline() {
  actionError.value = null
  busy.value = true
  try {
    await setOfflineMode(offline.value ? 'off' : 'on')
    await Promise.all([res.load({ quiet: true }), app.loadMeta(true)])
  } catch (e) {
    actionError.value = isApiError(e) ? e.message : 'Режим не переключился'
  } finally {
    busy.value = false
  }
}

const pct = (v: number | null | undefined) => (typeof v === 'number' ? formatPercent(v) : '—')
const num3 = (v: number | null | undefined) => (typeof v === 'number' ? formatDecimal(v, 3) : '—')
const ms = (v: number) => formatDecimal(v, 1)
const rub = (v: number | null) => (v === null ? 'LLM выкл.' : formatDecimal(v, 2))
const verdictText = { keep: 'оставить', remove: 'прироста нет', borderline: 'в пределах шума' }
</script>

<template>
  <div class="ctb">
    <div class="ctb__bar">
      <div class="ctb__sets" role="group" aria-label="Набор запросов">
        <button
          v-for="s in SETS"
          :key="s.id"
          type="button"
          class="btn btn--sm"
          :class="set === s.id ? 'btn--primary' : 'btn--secondary'"
          :aria-pressed="set === s.id"
          @click="set = s.id"
        >
          {{ s.label }}
        </button>
      </div>
      <div v-if="auth.staff" class="ctb__actions">
        <button
          type="button"
          class="btn btn--secondary btn--sm"
          :disabled="busy || !!running"
          @click="startRun"
        >
          <Play :size="14" aria-hidden="true" />
          {{ running ? `Идёт прогон: ${running.done} из ${running.total}` : 'Запустить прогон' }}
        </button>
        <button
          v-if="modes"
          type="button"
          class="btn btn--sm"
          :class="offline ? 'btn--primary' : 'btn--secondary'"
          :disabled="busy"
          :aria-pressed="offline"
          @click="toggleOffline"
        >
          <component :is="offline ? Power : PowerOff" :size="14" aria-hidden="true" />
          {{ offline ? 'Включить модели' : 'Без моделей' }}
        </button>
      </div>
    </div>
    <p v-if="actionError" class="t-sm ctb__err" role="alert">{{ actionError }}</p>
    <p v-if="modes && (modes.vectors.banner || modes.llm.banner)" class="panel t-sm ctb__mode">
      {{ modes.vectors.banner ?? modes.llm.banner }} — поиск отвечает; качество этого режима —
      строка «B3-M» ниже.
    </p>

    <div v-if="res.status.value === 'loading' && !data" class="panel ctb__empty" aria-busy="true">
      <p class="t-sm t-muted" role="status">Загружаем последний прогон…</p>
      <span v-for="i in 5" :key="i" class="skeleton" :style="{ width: `${92 - i * 8}%` }" />
    </div>
    <BlockError
      v-else-if="res.status.value === 'error' && !data"
      :error="res.error.value!"
      title="Таблица вклада не загрузилась"
      @retry="res.load()"
    />
    <div v-else-if="data && !run" class="panel ctb__empty">
      <p class="t-h2">Прогон не запускался</p>
      <p class="t-muted">
        Таблица появится после прогона набора {{ set }}:
        <span class="t-mono">make eval SET=contribution CONTRIB_SET={{ set }}</span
        ><template v-if="auth.staff"> или кнопка «Запустить прогон»</template>.
      </p>
      <button type="button" class="btn btn--secondary btn--sm ctb__retry" @click="res.load()">
        <RotateCcw :size="14" aria-hidden="true" /> Обновить
      </button>
    </div>

    <template v-else-if="run">
      <p v-if="res.status.value === 'error'" class="t-sm ctb__err" role="alert">
        Не обновилось: {{ res.error.value?.message }}. Показан последний прогон
        {{ runTail(run.run_id) }}.
      </p>
      <p v-if="isMockCatalog(run.catalog_version)" class="t-sm ctb__mock">
        Прогон на каталоге «{{ MOCK_RUN_LABEL }}». Числа на фиде магазина — в разделе «На данных
        заказчика».
      </p>
      <p v-if="famCols && !isMockCatalog(run.catalog_version)" class="t-sm ctb__mock">
        Данные заказчика: разметка — клики на боевой выдаче и LLM-судья, счёт по семействам. Это
        оценка с указанием, откуда она, а не ручная разметка.
      </p>
      <p class="t-sm ctb__lead">
        Набор <strong>{{ run.set }}</strong>: {{ formatInt(run.queries) }}
        {{ plural(run.queries, ['запрос', 'запроса', 'запросов']) }}. Каждая строка лестницы
        добавляет один компонент; ниже — полный путь без одного компонента.
      </p>

      <section class="ctb__section" aria-labelledby="ctb-ladder">
        <h3 id="ctb-ladder" class="t-h2">Лестница</h3>
        <div class="data-scroll">
          <table class="data-table">
            <caption class="sr-only">
              Вклад компонентов: лестница и прогоны без одного компонента
            </caption>
            <thead>
              <tr>
                <th scope="col">Строка</th>
                <th scope="col" class="num">{{ famCols ? 'NDCG@10 по семействам' : 'NDCG@10' }}</th>
                <th scope="col" class="num">Пустые</th>
                <th scope="col" class="num">Нарушения условий</th>
                <th scope="col" class="num">
                  {{ famCols ? 'Hit@1 по семейству' : 'Точные на 1-м' }}
                </th>
                <th scope="col" class="num">Ложное спасение</th>
                <th scope="col" class="num">P95, мс</th>
                <th scope="col" class="num">₽ LLM / 1000</th>
                <th scope="col" class="num">Лучше / хуже</th>
              </tr>
            </thead>
            <tbody>
              <template v-for="group in [ladder, singles]" :key="group[0]?.id">
                <tr v-if="group === singles && singles.length" class="ctb__group">
                  <th scope="rowgroup" colspan="9">Полный путь без одного компонента</th>
                </tr>
                <tr v-for="r in group" :key="r.id">
                  <th scope="row" class="ctb__row">
                    <span class="t-mono">{{ r.id }}</span> {{ r.label }}
                  </th>
                  <td class="num">
                    {{ num3(famCols ? r.metrics.ndcg10_family : r.metrics.ndcg10) }}
                  </td>
                  <td class="num">{{ pct(r.metrics.empty_rate) }}</td>
                  <td class="num">
                    {{ pct(r.metrics.violation_rate) }}
                    <span class="t-xs t-muted ctb__sub">{{ r.metrics.violation_items }} поз.</span>
                  </td>
                  <td class="num">
                    {{ pct(famCols ? r.metrics.exact_accuracy_family : r.metrics.exact_accuracy) }}
                  </td>
                  <td class="num">{{ pct(r.metrics.false_rescue_rate) }}</td>
                  <td class="num">{{ ms(r.metrics.latency.p95) }}</td>
                  <td
                    class="num"
                    :title="`по трассировкам; без кэша — ${rub(r.cost.cold_llm_rub_per_1k)}`"
                  >
                    {{ formatDecimal(r.cost.llm_rub_per_1k, 2) }}
                    <span class="t-xs t-muted ctb__sub"
                      >без кэша {{ rub(r.cost.cold_llm_rub_per_1k) }}</span
                    >
                  </td>
                  <td class="num">
                    <template v-if="r.against">
                      +{{ r.improvements }} / −{{ r.regressions }}
                      <span class="t-xs t-muted ctb__sub">к {{ r.against }}</span>
                    </template>
                    <template v-else>—</template>
                  </td>
                </tr>
              </template>
            </tbody>
          </table>
        </div>
        <p class="t-xs t-muted">
          ₽ LLM — по трассировкам прогона (кэш разборов как на стенде); «без кэша» — доля маршрута D
          × стоимость разбора {{ formatDecimal(run.parse_cost_rub, 3) }} ₽ ({{
            run.parse_cost_source === 'measured' ? 'замер' : 'допущение'
          }}) × 1000. Эмбеддер и реранкер — свой сервер, в ₽ за запрос не входят. Время — локальная
          машина, поиски по одному, кэш моделей выключен.
        </p>
      </section>

      <section class="ctb__section" aria-labelledby="ctb-checks">
        <h3 id="ctb-checks" class="t-h2">
          Проверки
          <span v-if="blockingFailed" class="t-sm ctb__bad">
            · не пройдено блокирующих: {{ blockingFailed }}</span
          >
        </h3>
        <ul class="ctb__list">
          <li v-for="c in run.checks" :key="c.id" class="ctb__check">
            <component
              :is="c.passed ? CircleCheck : CircleX"
              :size="16"
              aria-hidden="true"
              :class="c.passed ? 'ctb__ok' : 'ctb__bad'"
            />
            <span>
              {{ c.label
              }}<template v-if="c.blocking"> <em class="t-xs">(блокирующая)</em></template
              >: <strong>{{ c.value }}</strong>
              <span class="sr-only">{{ c.passed ? 'пройдена' : 'не пройдена' }}</span>
            </span>
          </li>
        </ul>
      </section>

      <section class="ctb__section" aria-labelledby="ctb-dec">
        <h3 id="ctb-dec" class="t-h2">Предварительные решения</h3>
        <ul class="ctb__list">
          <li v-for="d in run.decisions" :key="d.component" class="panel ctb__dec">
            <p>
              <strong>{{ d.label }}</strong> —
              <span :class="`ctb__v ctb__v--${d.verdict}`">{{ verdictText[d.verdict] }}</span>
            </p>
            <p class="t-sm t-muted">{{ d.text }}</p>
          </li>
        </ul>
      </section>

      <ul v-if="run.notes.length" class="t-xs t-muted ctb__notes">
        <li v-for="n in run.notes" :key="n">{{ n }}</li>
      </ul>
      <p class="t-xs t-mono t-muted ctb__repro">
        прогон {{ runTail(run.run_id) }} · {{ run.repro }}
      </p>
    </template>
  </div>
</template>

<style scoped>
.ctb {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.ctb__bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
}
.ctb__sets,
.ctb__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}
.ctb__empty {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-4);
}
.ctb__retry {
  align-self: flex-start;
}
.ctb__mode {
  padding: var(--space-2) var(--space-3);
}
.ctb__lead {
  max-width: 48rem;
}
.ctb__section {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.ctb__sub {
  display: block;
  white-space: nowrap;
}
.ctb__row {
  min-width: 12rem;
  font-weight: var(--fw-regular);
}
.ctb__group th {
  padding-top: var(--space-3);
  font-weight: var(--fw-medium);
  text-align: left;
}
.ctb__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.ctb__check {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
}
.ctb__dec {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--space-3) var(--space-4);
}
.ctb__ok,
.ctb__v--keep {
  color: var(--delta-up);
}
.ctb__bad,
.ctb__err,
.ctb__v--remove {
  color: var(--danger);
}
.ctb__v--borderline {
  color: var(--status-pending);
}
.ctb__notes {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  padding-left: var(--space-4);
}
.ctb__repro {
  overflow-wrap: anywhere;
}
</style>
