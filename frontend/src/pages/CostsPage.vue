<script setup lang="ts">
// «Экономика»: четыре плитки, таблица по маршрутам A–D, блок «Месяц» с допущениями,
// журнал вызовов модели и версия прайса. Всё из GET /api/costs (llm_usage + search_events); фронт
// ничего не пересчитывает, кроме сценариев месяца из замеров и допущений человека.
import { computed, onMounted, ref, shallowRef } from 'vue'
import { getCosts } from '@/api/costs'
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
import { formatCost, formatInt, formatMs, formatPercent, plural } from '@/lib/format'
import { ROUTE_LABELS, isRoute } from '@/lib/trace'

const res = useResource((opts) => getCosts(0, opts))
const toast = useToast()
const extraCalls = shallowRef<LlmCall[]>([])
const loadingMore = ref(false)

const data = computed(() => res.data.value)
const summary = computed(() => data.value?.summary ?? null)
const pricing = computed(() => data.value?.pricing ?? null)
const pricingVersion = computed(() => pricing.value?.version ?? 'не указан')
const calls = computed(() => [...(data.value?.calls?.items ?? []), ...extraCalls.value])
const routes = computed<CostRouteRow[]>(() => data.value?.routes ?? [])

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
onMounted(reload)

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
      sub: 'онлайн-LLM, без сервера',
      missing: 'запросов ещё не было',
      journal: true,
      explain: () =>
        explain({
          title: 'Средняя стоимость поискового запроса',
          value: num(s.avg_cost_rub) ? formatCost(s.avg_cost_rub) : '—',
          what: 'Сколько в среднем стоит онлайн-разбор запроса моделью. Сервер — в блоке «Месяц».',
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
      label: 'Одно исследование агента',
      value: num(s.agent_run_avg_cost_rub) ? formatCost(s.agent_run_avg_cost_rub) : null,
      sub: num(s.agent_runs)
        ? `по ${formatInt(s.agent_runs)} ${plural(s.agent_runs, ['запуску', 'запускам', 'запускам'])}`
        : undefined,
      missing: 'агент SearchOps ещё не запускался',
      journal: true,
      explain: () =>
        explain({
          title: 'Стоимость одного исследования агента',
          value: num(s.agent_run_avg_cost_rub) ? formatCost(s.agent_run_avg_cost_rub) : '—',
          how: 'Сумма calculated_cost вызовов модели с agent_run_id / число запусков агента.',
          rows: num(s.agent_runs) ? [{ label: 'Запусков', value: formatInt(s.agent_runs) }] : [],
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
  <section class="page costs" aria-labelledby="costs-title">
    <header class="costs__head">
      <h1 id="costs-title" class="t-h1">Экономика</h1>
      <p class="t-sm t-muted">Сколько стоит поиск и откуда берётся каждая цифра</p>
    </header>

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
        :llm-per-request-rub="summary?.avg_cost_rub ?? null"
        :agent-run-rub="summary?.agent_run_avg_cost_rub ?? null"
        :pricing-version="pricingVersion"
        @explain="explain"
      />

      <CallsJournal
        :calls="calls"
        :total="data.calls?.total ?? calls.length"
        :loading-more="loadingMore"
        :pricing-version="pricingVersion"
        @more="more"
        @explain="explain"
      />

      <footer class="costs__pricing t-xs t-muted">
        <span>
          Прайс <span class="t-mono">{{ pricingVersion }}</span>
          <template v-if="pricing?.date">, цены на {{ pricing.date }}</template>
        </span>
        <span v-for="p in pricing?.prices ?? []" :key="p.model">
          <span class="t-mono">{{ p.model }}</span>: {{ formatInt(p.input_rub_per_mtok) }} /
          {{ formatInt(p.output_rub_per_mtok) }} ₽ за 1 млн токенов (вход / выход)
        </span>
      </footer>
    </template>

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
.costs__pricing {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
@media (max-width: 1023px) {
  .costs__tiles {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
