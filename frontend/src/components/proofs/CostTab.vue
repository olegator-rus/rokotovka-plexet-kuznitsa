<script setup lang="ts">
// «Доказательства» → «Стоимость» (контур доводки S5: цена запроса по маршрутам и средняя): четыре плитки,
// таблица по маршрутам A–D, блок «Месяц» с допущениями, журнал вызовов модели (только сотруднику — в нём трассы
// поисков) и версия прайса. Вкладка «Эффект» удалена в контуре доводки (формула — в docs/EFFECT_MODEL.md). Всё из GET /api/costs (llm_usage + search_events); фронт
// ничего не пересчитывает, кроме сценариев месяца из замеров и допущений человека.
import { computed, onMounted, ref, shallowRef } from 'vue'
import { ExternalLink } from 'lucide-vue-next'
import { getRuns } from '@/api/agent'
import { getCosts, getCostsSummary } from '@/api/costs'
import type { CostRouteRow, LlmCall } from '@/api/types'
import BlockError from '@/components/common/BlockError.vue'
import ExplainDialog, { type Explanation } from '@/components/common/ExplainDialog.vue'
import StatTile from '@/components/common/StatTile.vue'
import CallsJournal from '@/components/costs/CallsJournal.vue'
import MonthBlock from '@/components/costs/MonthBlock.vue'
import RouteBadge from '@/components/trace/RouteBadge.vue'
import { useToast } from '@/components/ui/useToast'
import { useResource } from '@/composables/useResource'
import { isApiError } from '@/lib/apiError'
import { agentRunCost, runsInHint } from '@/lib/costMonth'
import { formatCost, formatInt, formatMs, formatPercent, plural } from '@/lib/format'
import { ROUTE_LABELS, isRoute } from '@/lib/trace'
import { useAuthStore } from '@/stores/auth'

// Журнал вызовов и список запусков агента — бэк-офис: гостю не запрашиваются. Гость получает открытую сводку
// GET /api/costs/summary (те же маршруты, средняя, доля без LLM и «Месяц», без журнала).
const auth = useAuthStore()

const res = useResource((opts) => (auth.staff ? getCosts(0, opts) : getCostsSummary(opts)))
const toast = useToast()
const extraCalls = shallowRef<LlmCall[]>([])
const loadingMore = ref(false)

const data = computed(() => res.data.value)
const summary = computed(() => data.value?.summary ?? null)
const pricing = computed(() => data.value?.pricing ?? null)
/** Прайс без версии: в версии дата, а дат на экранах нет (версия — в llm_usage.pricing_version). */
const pricingVersion = computed(() => (pricing.value ? 'Yandex AI Studio' : 'не указан'))
/** Страница тарифов провайдера; сервер присылает её в pricing.source_url. */
const PRICING_SOURCE_URL = 'https://aistudio.yandex.ru/ru/docs/ai-studio/pricing'
/**
 * Для чего модель используется в проекте. Сервер присылает это в prices[].use; словарь — запасной вариант
 * для сервера старой версии, чтобы таблица не оказалась пустой. Модели без назначения в проекте не
 * используются и в таблицу не попадают.
 */
const MODEL_USE: Record<string, string> = {
  'aliceai-llm-flash': 'Разбор сложных запросов (маршрут D) и агент SearchOps',
  'qwen3.6-35b-a3b': 'Резерв разбора и LLM-судья оценки',
}
const usedPrices = computed(() =>
  (pricing.value?.prices ?? [])
    .map((p) => ({ ...p, use: p.use || MODEL_USE[p.model] || '' }))
    .filter((p) => p.use),
)
const calls = computed(() => [...(data.value?.calls?.items ?? []), ...extraCalls.value])
const routes = computed<CostRouteRow[]>(() => data.value?.routes ?? [])

// Стоимость запуска агента для «Месяца» — по полным запускам; список запусков не обязателен.
const runs = useResource((opts) => getRuns(opts))
const runCost = computed(() =>
  agentRunCost(runs.data.value?.items ?? [], summary.value?.agent_run_avg_cost_rub ?? null),
)
// «Месяц» считается от тех же замеров, что и monthly в GET /api/costs (и CSV-экспорт): онлайн-LLM на запрос =
// доля D × (1 − доля кэша) × стоимость разбора, запуск агента — среднее завершённых запусков. Нет блока — запасной путь.
const monthly = computed(() => data.value?.monthly ?? null)
const monthParam = (key: string) => monthly.value?.params.find((p) => p.key === key) ?? null
const monthLlmPerRequest = computed(
  () => monthly.value?.scenarios[0]?.llm_per_request_rub ?? summary.value?.avg_cost_rub ?? null,
)
const monthAgentRun = computed(() => {
  const p = monthParam('agent_run_cost_rub')
  return p && p.source === 'measured' ? p.value : runCost.value.rub
})
const monthAgentNote = computed(() => {
  const p = monthParam('agent_run_cost_rub')
  return p && p.source === 'measured' ? p.hint.replace(/^Замер — /, '') : runCostNote.value
})
/** Плитка «Запуск агента» и строка «Агент» в «Месяце» — одна база: замер сервера по завершённым запускам. */
const agentTileSub = computed(() => {
  const p = monthParam('agent_run_cost_rub')
  if (p && p.source === 'measured') {
    const n = runsInHint(p.hint)
    return n
      ? `по ${formatInt(n)} ${plural(n, ['завершённому запуску', 'завершённым запускам', 'завершённым запускам'])}`
      : 'по завершённым запускам'
  }
  if (runCost.value.full) return runCostNote.value
  const all = summary.value?.agent_runs
  return num(all)
    ? `по ${formatInt(all)} ${plural(all, ['запуску', 'запускам', 'запускам'])}`
    : undefined
})
const runCostNote = computed(() =>
  runCost.value.full
    ? `среднее по ${formatInt(runCost.value.full)} ${plural(runCost.value.full, ['полному запуску', 'полным запускам', 'полным запускам'])}`
    : 'среднее по всем запускам из журнала',
)

async function reload() {
  extraCalls.value = []
  await res.load()
}
async function more() {
  loadingMore.value = true
  try {
    const next = await getCosts(calls.value.length)
    extraCalls.value = [...extraCalls.value, ...(next.calls?.items ?? [])]
  } catch (e) {
    toast.add({
      tone: 'error',
      title: 'Не удалось загрузить вызовы',
      detail: isApiError(e) ? e.message : undefined,
    })
  } finally {
    loadingMore.value = false
  }
}
onMounted(() => {
  void reload()
  if (auth.staff) void runs.load().catch(() => undefined)
})

const open = ref(false)
const explanation = ref<Explanation | null>(null)
/** Объяснение стоимости ведёт дальше — в журнал вызовов, из которых она сложилась. */
const journalLink = ref(false)
function explain(e: Explanation) {
  explanation.value = { source: 'GET /api/costs', ...e }
  journalLink.value = false
  open.value = true
}
function openTile(t: { explain: () => void; journal: boolean }) {
  t.explain()
  journalLink.value = t.journal && calls.value.length > 0
}
function toJournal() {
  open.value = false
  document.getElementById('calls')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

const num = (v: number | null | undefined): v is number =>
  typeof v === 'number' && Number.isFinite(v)
const requestsRow = () => ({
  label: 'Поисковых запросов в расчёте',
  value: formatInt(summary.value?.searches ?? 0),
})

const tiles = computed(() => {
  const s = summary.value
  if (!s) return []
  return [
    {
      key: 'avg',
      label: 'Средняя стоимость запроса',
      value: num(s.avg_cost_rub) ? formatCost(s.avg_cost_rub) : null,
      sub: 'фактическое среднее, без сервера',
      missing: 'запросов ещё не было',
      journal: true,
      explain: () =>
        explain({
          title: 'Средняя стоимость поискового запроса',
          value: num(s.avg_cost_rub) ? formatCost(s.avg_cost_rub) : '—',
          what: 'Сколько в среднем стоит онлайн-разбор запроса моделью. Сервер — в блоке «Месяц». В «Месяце» LLM на запрос — прогноз по формуле (доля маршрута D × (1 − доля кэша) × стоимость разбора), поэтому число там чуть другое.',
          how: 'Сумма calculated_cost вызовов модели для поисков / число поисков. Запросы без LLM и из кэша входят с нулём.',
          rows: [requestsRow()],
        }),
    },
    {
      key: 'nollm',
      label: 'Запросы без LLM',
      value: num(s.share_without_llm) ? formatPercent(s.share_without_llm) : null,
      sub: 'маршруты A–C и кэш',
      missing: 'запросов ещё не было',
      journal: false,
      explain: () =>
        explain({
          title: 'Доля запросов без LLM',
          value: num(s.share_without_llm) ? formatPercent(s.share_without_llm) : '—',
          what: 'Запросы, на которые поиск ответил без вызова внешней модели.',
          how: 'Поиски с источником разбора rules или cache / все поиски.',
          rows: [requestsRow()],
        }),
    },
    {
      key: 'cache',
      label: 'Попадания в кэш',
      value: num(s.cache_hit_share) ? formatPercent(s.cache_hit_share) : null,
      sub: 'среди запросов маршрута D',
      missing: 'маршрут D ещё не срабатывал',
      journal: false,
      explain: () =>
        explain({
          title: 'Доля попаданий в кэш разборов',
          value: num(s.cache_hit_share) ? formatPercent(s.cache_hit_share) : '—',
          what: 'Повторный сложный запрос берёт разбор из кэша — модель не вызывается, стоимость 0 ₽.',
          how: 'Поиски маршрута D с источником cache / все поиски маршрута D.',
        }),
    },
    {
      key: 'agent',
      label: 'Запуск агента',
      value: num(monthAgentRun.value) ? formatCost(monthAgentRun.value) : null,
      sub: agentTileSub.value,
      missing: 'агент SearchOps ещё не запускался',
      journal: true,
      explain: () =>
        explain({
          title: 'Стоимость одного запуска агента',
          value: num(monthAgentRun.value) ? formatCost(monthAgentRun.value) : '—',
          what: 'То же число стоит в «Месяце» в строке «Агент».',
          how: 'Сумма стоимости вызовов модели с agent_run_id по завершённым запускам / число таких запусков. Отладочные и прерванные запуски цифру не занижают.',
          rows: [
            ...(monthAgentNote.value ? [{ label: 'База', value: monthAgentNote.value }] : []),
            ...(num(s.agent_runs)
              ? [
                  {
                    label: `Все запуски в журнале (${formatInt(s.agent_runs)}), с прерванными`,
                    value: num(s.agent_run_avg_cost_rub)
                      ? `${formatCost(s.agent_run_avg_cost_rub)} в среднем`
                      : '—',
                  },
                ]
              : []),
          ],
        }),
    },
  ]
})

function explainRoute(r: CostRouteRow, field: 'share' | 'avg_ms' | 'avg_cost_rub') {
  const name = isRoute(r.route) ? `${r.route} · ${ROUTE_LABELS[r.route]}` : r.route
  const map = {
    share: {
      title: `Доля маршрута ${name}`,
      value: num(r.share) ? formatPercent(r.share) : '—',
      how: 'Поиски, ушедшие на этот маршрут / все поиски (прогоны оценки не входят).',
    },
    avg_ms: {
      title: `Среднее время, маршрут ${name}`,
      value: num(r.avg_ms) ? formatMs(r.avg_ms) : '—',
      how: 'Среднее total_ms трасс поисков этого маршрута.',
    },
    avg_cost_rub: {
      title: `Средняя стоимость, маршрут ${name}`,
      value: num(r.avg_cost_rub) ? formatCost(r.avg_cost_rub) : '—',
      how: 'Сумма стоимости вызовов модели для поисков маршрута / число поисков маршрута.',
    },
  }[field]
  explain({
    ...map,
    rows: [
      { label: 'Поисков на маршруте', value: formatInt(r.count) },
      { label: 'Из них разобрано LLM', value: formatInt(r.llm_parses) },
      { label: 'Из них взято из кэша', value: formatInt(r.cache_hits) },
    ],
  })
}
</script>

<template>
  <section class="costs" aria-labelledby="costs-title">
    <header class="costs__head">
      <h2 id="costs-title" class="sr-only">Стоимость</h2>
    </header>

    <div class="costs">
      <template v-if="res.status.value === 'loading' && !data">
        <div class="costs__tiles" aria-busy="true">
          <div v-for="i in 4" :key="i" class="panel costs__tile-skel">
            <span class="skeleton" style="width: 60%" />
            <span class="skeleton" style="width: 40%; height: 1.25rem" />
          </div>
        </div>
        <div class="panel costs__table-skel" aria-hidden="true">
          <span v-for="i in 4" :key="i" class="skeleton" :style="{ width: `${90 - i * 8}%` }" />
        </div>
      </template>

      <BlockError
        v-else-if="res.status.value === 'error'"
        :error="res.error.value!"
        title="Сводка стоимости не загрузилась"
        @retry="reload"
      />

      <template v-else-if="data">
        <div class="costs__tiles">
          <StatTile
            v-for="t in tiles"
            :key="t.key"
            :label="t.label"
            :value="t.value"
            :sub="t.sub"
            :missing="t.missing"
            @explain="openTile(t)"
          />
        </div>

        <section class="costs__routes" aria-labelledby="routes-title">
          <h2 id="routes-title" class="t-h2">По маршрутам</h2>
          <p v-if="routes.length === 0" class="panel costs__empty">
            Поисков ещё не было — распределения по маршрутам нет.
          </p>
          <div v-else class="data-scroll" role="region" aria-label="Маршруты" tabindex="0">
            <table class="data-table">
              <thead>
                <tr>
                  <th scope="col">Маршрут</th>
                  <th scope="col" class="num">Поисков</th>
                  <th scope="col" class="num">Доля</th>
                  <th scope="col" class="num">Среднее время</th>
                  <th scope="col" class="num">Средняя стоимость</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="r in routes" :key="r.route">
                  <td><RouteBadge :route="r.route" /></td>
                  <td class="num">{{ formatInt(r.count) }}</td>
                  <td class="num">
                    <button
                      v-if="num(r.share)"
                      type="button"
                      class="num-btn"
                      @click="explainRoute(r, 'share')"
                    >
                      {{ formatPercent(r.share) }}
                    </button>
                    <span v-else class="t-muted">—</span>
                  </td>
                  <td class="num">
                    <button
                      v-if="num(r.avg_ms)"
                      type="button"
                      class="num-btn"
                      @click="explainRoute(r, 'avg_ms')"
                    >
                      {{ formatMs(r.avg_ms) }}
                    </button>
                    <span v-else class="t-muted">—</span>
                  </td>
                  <td class="num">
                    <button
                      v-if="num(r.avg_cost_rub)"
                      type="button"
                      class="num-btn"
                      @click="explainRoute(r, 'avg_cost_rub')"
                    >
                      {{ formatCost(r.avg_cost_rub) }}
                    </button>
                    <span v-else class="t-muted">—</span>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>

        <p v-if="summary && summary.eval_calls > 0" class="t-xs t-muted">
          Прогоны оценки вызывали модель {{ formatInt(summary.eval_calls) }}
          {{ plural(summary.eval_calls, ['раз', 'раза', 'раз']) }} на
          {{ formatCost(summary.eval_cost_rub) }} — в стоимость запроса покупателя не входят.
          Исторические логи без маршрута в таблицу не попадают.
        </p>

        <MonthBlock
          :llm-per-request-rub="monthLlmPerRequest"
          :agent-run-rub="monthAgentRun"
          :agent-run-note="monthAgentNote"
          :pricing-version="pricingVersion"
          @explain="explain"
        />

        <CallsJournal
          v-if="auth.staff"
          :calls="calls"
          :total="data.calls?.total ?? calls.length"
          :loading-more="loadingMore"
          :pricing-version="pricingVersion"
          @more="more"
          @explain="explain"
        />

        <!-- Модели в проекте: по этим ценам посчитана каждая сумма на вкладке -->
        <section class="costs__models" aria-labelledby="models-title">
          <div class="costs__models-head">
            <h2 id="models-title" class="t-h2">Модели в проекте</h2>
            <a
              :href="pricing?.source_url || PRICING_SOURCE_URL"
              target="_blank"
              rel="noopener noreferrer"
              class="costs__models-link"
            >
              Тарифы Yandex AI Studio
              <ExternalLink :size="12" aria-hidden="true" />
            </a>
          </div>
          <div class="data-scroll">
            <table class="data-table">
              <caption class="sr-only">
                Модели Yandex AI Studio, которые используются в проекте, и их цены
              </caption>
              <thead>
                <tr>
                  <th scope="col">Модель</th>
                  <th scope="col">Для чего</th>
                  <th scope="col" class="num">Вход</th>
                  <th scope="col" class="num">Выход</th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="!usedPrices.length">
                  <td colspan="4" class="t-muted">Сервер не прислал цены моделей.</td>
                </tr>
                <tr v-for="p in usedPrices" :key="p.model">
                  <th scope="row" class="t-mono costs__model">{{ p.model }}</th>
                  <td>{{ p.use }}</td>
                  <td class="num">{{ formatInt(p.input_rub_per_mtok) }} ₽</td>
                  <td class="num">{{ formatInt(p.output_rub_per_mtok) }} ₽</td>
                </tr>
              </tbody>
            </table>
          </div>
          <p class="t-xs t-muted">
            Цены за 1 млн токенов из прайса Yandex AI Studio. По ним сервер считает стоимость
            каждого вызова модели.
          </p>
        </section>
      </template>
    </div>

    <ExplainDialog v-model:open="open" :explanation="explanation">
      <button
        v-if="journalLink"
        type="button"
        class="btn btn--secondary btn--sm"
        @click="toJournal"
      >
        К журналу вызовов
      </button>
    </ExplainDialog>
  </section>
</template>

<style scoped>
.costs {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
.costs__head {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}
.costs__tiles {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--space-3);
}
.costs__tile-skel,
.costs__table-skel {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
}
.costs__routes {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.costs__empty {
  padding: var(--space-4);
  color: var(--text-2);
}
.costs__models {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.costs__models-head {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--space-2);
}
.costs__models-link {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  color: var(--text-2);
  font-size: var(--fs-sm);
  text-decoration: underline;
  text-underline-offset: 3px;
}
.costs__models-link:hover {
  color: var(--text);
}
.costs__model {
  white-space: nowrap;
}
@media (max-width: 1023px) {
  .costs__tiles {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
