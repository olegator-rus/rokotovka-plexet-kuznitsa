<script setup lang="ts">
// Блок «Месяц»: два сценария объёма. LLM и агент — из замеров сервера, сервер и число
// исследований агента — допущения, их можно поправить прямо здесь (пометка «допущение»).
// Без замера строка честно пустая, итог помечен «без …».
import { computed, reactive, watch } from 'vue'
import type { Explanation } from '@/components/common/ExplainDialog.vue'
import {
  AGENT_RUNS_MAX,
  SERVER_RUB_MAX,
  type AssumptionParse,
  monthScenario,
  parseAssumption,
} from '@/lib/costMonth'
import { formatCost, formatInt, formatRub, plural } from '@/lib/format'

const props = defineProps<{
  llmPerRequestRub: number | null
  agentRunRub: number | null
  /** Откуда стоимость запуска: «среднее 2 полных запусков» и т. п. */
  agentRunNote?: string
  pricingVersion: string
  /** Допущения сервера (monthly.params): те же сервер и число запусков, что в «На данных заказчика». */
  defaults?: { server20k: number; server200k: number; runs20k: number; runs200k: number } | null
}>()
const emit = defineEmits<{ explain: [e: Explanation] }>()

// Допущения по умолчанию приходят с сервера (monthly.params) — те же, что в расчёте «На данных заказчика»:
// сервер 4 vCPU / 16 ГБ / 50 ГБ SSD + IP ≈ 8 218 ₽ при 20 тыс. и 8 vCPU / 32 ГБ ≈ 15 000 ₽ при 200 тыс.;
// запусков агента 40 (ежедневно и по триггерам) и 120 (4 раза в день). Ниже — запасные значения на случай
// сервера без monthly. serverOk / runsOk — последнее верное значение поля: мусор в поле итог не пересчитывает.
const scenarios = reactive([
  {
    requests: 20_000,
    server: '8 218',
    runs: '40',
    serverOk: 8218,
    runsOk: 40,
    serverNote: '4 vCPU / 16 ГБ / 50 ГБ SSD + IP',
    runsNote: 'ежедневно и по триггерам',
  },
  {
    requests: 200_000,
    server: '15 000',
    runs: '120',
    serverOk: 15_000,
    runsOk: 120,
    serverNote: '8 vCPU / 32 ГБ',
    runsNote: '4 раза в день',
  },
])

// Пришли допущения сервера — подставляем, пока человек поле не правил.
const touched = [false, false]
watch(
  () => props.defaults,
  (d) => {
    if (!d) return
    const pairs: [number, number][] = [
      [d.server20k, d.runs20k],
      [d.server200k, d.runs200k],
    ]
    pairs.forEach(([server, runs], i) => {
      const s = scenarios[i]
      if (!s || touched[i]) return
      s.server = formatInt(server)
      s.runs = formatInt(runs)
      s.serverOk = server
      s.runsOk = runs
    })
  },
  { immediate: true },
)

const parsed = computed(() =>
  scenarios.map((s) => ({
    server: parseAssumption(s.server, { max: SERVER_RUB_MAX, unit: '₽' }),
    runs: parseAssumption(s.runs, { max: AGENT_RUNS_MAX, integer: true }),
  })),
)
watch(parsed, (list) =>
  list.forEach((p, i) => {
    const s = scenarios[i]
    if (!s) return
    if (p.server.ok) s.serverOk = p.server.value
    if (p.runs.ok) s.runsOk = p.runs.value
  }),
)
const valueOr = (p: AssumptionParse | undefined, last: number) => (p?.ok ? p.value : last)
const errorOf = (p: AssumptionParse | undefined) => (p && !p.ok ? p.error : null)

/** Значения, по которым считается сценарий: верный ввод или последнее верное. */
const used = computed(() =>
  scenarios.map((s, i) => ({
    server: valueOr(parsed.value[i]?.server, s.serverOk),
    runs: valueOr(parsed.value[i]?.runs, s.runsOk),
    stale: !parsed.value[i]?.server.ok || !parsed.value[i]?.runs.ok,
  })),
)

const results = computed(() =>
  scenarios.map((s, i) =>
    monthScenario({
      requests: s.requests,
      serverRub: used.value[i]?.server ?? 0,
      llmPerRequestRub: props.llmPerRequestRub,
      agentRunRub: props.agentRunRub,
      agentRuns: used.value[i]?.runs ?? 0,
    }),
  ),
)

const runsLabel = (n: number) => `${formatInt(n)} ${plural(n, ['запуск', 'запуска', 'запусков'])}`

function explainTotal(i: number) {
  const s = scenarios[i]
  const r = results.value[i]
  const u = used.value[i]
  if (!s || !r || !u) return
  emit('explain', {
    title: `Месяц при ${formatInt(s.requests)} запросов`,
    value: formatRub(r.total),
    how: 'Сервер (допущение) + онлайн-LLM (на запрос = измеренные доля маршрута D × (1 − доля попаданий в кэш) × стоимость разбора; × число запросов) + агент (измеренная стоимость запуска × число запусков в месяц, допущение). Те же числа — в GET /api/costs (monthly) и CSV-экспорте.',
    rows: [
      { label: `Сервер, ${s.serverNote} (допущение)`, value: formatRub(r.server) },
      {
        label: 'Онлайн-LLM',
        value:
          r.llm === null
            ? 'нет замера'
            : `${formatCost(props.llmPerRequestRub ?? 0)} × ${formatInt(s.requests)} = ${formatRub(r.llm)}`,
      },
      {
        label: `Агент, ${runsLabel(u.runs)} (допущение: ${s.runsNote})`,
        value:
          r.agent === null
            ? 'нет замера'
            : `${formatCost(props.agentRunRub ?? 0)} × ${formatInt(u.runs)} = ${formatRub(r.agent)}`,
      },
      ...(props.agentRunNote ? [{ label: 'Стоимость запуска', value: props.agentRunNote }] : []),
      { label: 'На запрос', value: r.perRequest === null ? '—' : formatCost(r.perRequest) },
      ...(u.stale
        ? [{ label: 'В поле ошибка', value: 'посчитано по последнему верному значению' }]
        : []),
    ],
    source: `GET /api/costs · прайс ${props.pricingVersion}`,
  })
}
</script>

<template>
  <section class="month" aria-labelledby="month-title">
    <div class="month__head">
      <h2 id="month-title" class="t-h2">Месяц</h2>
      <span class="t-xs t-muted">поля с пометкой «допущение» можно править</span>
    </div>
    <p class="t-xs t-muted month__note">
      Полная стоимость запроса: сервер, онлайн-LLM и агент. Вкладка «На данных заказчика» берёт те
      же сервер и агента, но 15 000 запросов в месяц, поэтому на запрос там выходит больше.
    </p>
    <div class="data-scroll">
      <table class="data-table">
        <thead>
          <tr>
            <th scope="col">Статья</th>
            <th v-for="s in scenarios" :key="s.requests" scope="col" class="num">
              {{ formatInt(s.requests) }} запросов
            </th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <th scope="row">
              <span class="month__row">Сервер <span class="badge">допущение</span></span>
            </th>
            <td v-for="(s, i) in scenarios" :key="i" class="num">
              <div class="month__cell">
                <span class="month__field">
                  <input
                    v-model="s.server"
                    inputmode="decimal"
                    class="month__input"
                    :aria-label="`Сервер в месяц, рублей, при ${formatInt(s.requests)} запросов`"
                    :aria-invalid="errorOf(parsed[i]?.server) ? 'true' : undefined"
                    :aria-describedby="
                      errorOf(parsed[i]?.server) ? `month-server-err-${i}` : undefined
                    "
                    @input="touched[i] = true"
                  />
                  <span aria-hidden="true">₽</span>
                </span>
                <span
                  v-if="errorOf(parsed[i]?.server)"
                  :id="`month-server-err-${i}`"
                  class="month__error t-xs"
                >
                  {{ errorOf(parsed[i]?.server) }}
                </span>
                <span class="t-xs t-muted">{{ s.serverNote }}</span>
              </div>
            </td>
          </tr>
          <tr>
            <th scope="row">
              <span class="month__row">
                Онлайн-LLM <span class="t-xs t-muted">замер</span>
                <span v-if="llmPerRequestRub !== null" class="t-xs t-muted">
                  {{ formatCost(llmPerRequestRub) }} на запрос по формуле (доли D и кэша), в плитке
                  — фактическое среднее
                </span>
              </span>
            </th>
            <td v-for="(r, i) in results" :key="i" class="num">
              <span :class="{ 't-muted': r.llm === null }">
                {{ r.llm === null ? 'нет замера' : formatRub(r.llm) }}
              </span>
            </td>
          </tr>
          <tr>
            <th scope="row">
              <span class="month__row">
                Агент <span class="badge">допущение</span>
                <span class="t-xs t-muted">
                  <template v-if="agentRunRub !== null"
                    >{{ formatCost(agentRunRub) }} за запуск (замер) × запусков</template
                  >
                  <template v-else>замер × запусков</template>
                </span>
              </span>
            </th>
            <td v-for="(r, i) in results" :key="i" class="num">
              <div class="month__cell">
                <span :class="{ 't-muted': r.agent === null }">
                  {{ r.agent === null ? 'нет замера' : formatRub(r.agent) }}
                </span>
                <span class="month__field">
                  <input
                    v-model="scenarios[i].runs"
                    inputmode="numeric"
                    class="month__input month__input--sm"
                    :aria-label="`Запусков агента в месяц при ${formatInt(scenarios[i].requests)} запросов, допущение`"
                    :aria-invalid="errorOf(parsed[i]?.runs) ? 'true' : undefined"
                    :aria-describedby="errorOf(parsed[i]?.runs) ? `month-runs-err-${i}` : undefined"
                    @input="touched[i] = true"
                  />
                  <span class="t-xs t-muted" aria-hidden="true">запусков</span>
                </span>
                <span
                  v-if="errorOf(parsed[i]?.runs)"
                  :id="`month-runs-err-${i}`"
                  class="month__error t-xs"
                >
                  {{ errorOf(parsed[i]?.runs) }}
                </span>
                <span class="t-xs t-muted">{{ scenarios[i].runsNote }}</span>
              </div>
            </td>
          </tr>
          <tr class="month__total">
            <th scope="row">Итого</th>
            <td v-for="(r, i) in results" :key="i" class="num">
              <div class="month__cell">
                <button
                  type="button"
                  class="num-btn"
                  :aria-label="`Итого ${formatRub(r.total)}. Как посчитано`"
                  @click="explainTotal(i)"
                >
                  {{ formatRub(r.total) }}
                </button>
                <span v-if="r.partial" class="t-xs t-muted">без строк «нет замера»</span>
                <span v-if="used[i]?.stale" class="t-xs t-muted"> по прежним значениям </span>
              </div>
            </td>
          </tr>
          <tr>
            <th scope="row">На запрос</th>
            <td v-for="(r, i) in results" :key="i" class="num">
              {{ r.perRequest === null ? '—' : formatCost(r.perRequest) }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<style scoped>
/* Секция без своей рамки, как «По маршрутам»: рамку даёт только таблица,
   иначе таблица выглядела вложенной в ещё одну карточку. */
.month {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.month__note {
  margin: 0;
}
.month__head {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--space-2);
}
.month th[scope='row'] {
  font-weight: var(--fw-regular);
}
.month__row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1) var(--space-2);
}
.month__cell {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: var(--space-1);
}
.month__field {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
}
.month__input {
  box-sizing: border-box;
  width: 6.5rem;
  min-height: var(--control-h-sm);
  padding: 0 var(--space-2);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-control);
  background: var(--surface-2);
  color: var(--text);
  font: inherit;
  font-variant-numeric: tabular-nums;
  text-align: right;
}
.month__input--sm {
  width: 4.5rem;
}
.month__input[aria-invalid='true'] {
  border-color: var(--danger);
  box-shadow: inset 0 0 0 1px var(--danger);
}
.month__error {
  max-width: 11rem;
  color: var(--danger);
  text-align: right;
}
.month__total th,
.month__total td {
  font-weight: var(--fw-semibold);
}
</style>
