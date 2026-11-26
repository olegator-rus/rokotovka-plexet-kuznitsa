<script setup lang="ts">
// «Доказательства» → «На данных заказчика» (контур эталонов): один экран — эталоны заказчика, закрытая часть
// набора (Success@1 у BM25, гибрида и полного продукта), доля ответов с ≥4 товарами, стоимость запроса при 15 тыс.
// в месяц с долями инфраструктуры и LLM, каталог. Все числа — из GET /api/evaluations/feed: прогоны `go run . eval`
// на фиде и трассы; фронт ничего не пересчитывает и чисел не вписывает. Дат на экране нет.
import { computed, onMounted, ref } from 'vue'
import { Database, RotateCcw } from 'lucide-vue-next'
import { getFeedSummary } from '@/api/evaluations'
import type { FeedCostRun, FeedRow } from '@/api/typesFeed'
import BlockError from '@/components/common/BlockError.vue'
import ExplainDialog, { type Explanation } from '@/components/common/ExplainDialog.vue'
import StatTile from '@/components/common/StatTile.vue'
import { useResource } from '@/composables/useResource'
import {
  closedCells,
  closedPending,
  configLabel,
  hasFeedRuns,
  headlineCost,
  modeLabel,
  outOf,
  setLabel,
  shareOrNull,
} from '@/lib/feedSummary'
import {
  formatCost,
  formatDecimal,
  formatInt,
  formatMoney,
  formatMs,
  formatPercent,
} from '@/lib/format'

const res = useResource((opts) => getFeedSummary(opts))
const data = computed(() => res.data.value)
onMounted(() => void res.load())

const open = ref(false)
const explanation = ref<Explanation | null>(null)
function explain(e: Explanation) {
  explanation.value = { source: 'GET /api/evaluations/feed', ...e }
  open.value = true
}

const num = (v: number | null | undefined): v is number =>
  typeof v === 'number' && Number.isFinite(v)
const paramValue = (v: number) => (Number.isInteger(v) ? formatInt(v) : formatDecimal(v, 2))

const catalogLine = computed(() => {
  const c = data.value?.catalog
  if (!c || c.source === 'unknown') return null
  return `${c.label}, ${formatInt(c.offers)} офферов`
})

const etalon = computed(() => data.value?.etalon ?? null)
const closed = computed(() => closedCells(data.value ?? null))
const pending = computed(() => closedPending(data.value ?? null))
const four = computed(() => data.value?.four ?? null)
const cost = computed(() => data.value?.cost ?? null)
const head = computed(() => headlineCost(data.value ?? null))
const costRuns = computed(() =>
  [cost.value?.cold, cost.value?.warm, cost.value?.journal].filter((r): r is FeedCostRun => !!r),
)

function explainEtalon() {
  const e = etalon.value
  if (!e) return
  explain({
    title: 'Эталоны заказчика',
    value: outOf(e.matched, e.total),
    what: 'Десять запросов из файла заказчика: первый ответ полного продукта ведёт на ту страницу, которую заказчик назвал целью.',
    how: 'Первая цель ответа — поле target, иначе первая карточка (URL семейства → оффера → вендора). URL сравниваются после нормализации: регистр хоста, завершающий слэш, URL-кодирование, filter[category][]= и [0]=.',
    rows: [
      { label: 'Конфигурация', value: configLabel(data.value ?? null, e.config) },
      { label: 'Цель в первых четырёх', value: shareOrNull(e.top4) ?? '—' },
      { label: 'Прогон', value: `…${e.run_tail}` },
    ],
  })
}

function explainClosed() {
  explain({
    title: 'Закрытая часть набора',
    value: pending.value
      ? 'Прогон не запускался'
      : closed.value
          .map((c) => `${c.label}: ${c.score ? (shareOrNull(c.score.success1) ?? '—') : '—'}`)
          .join(' · '),
    what: 'Новые формулировки, которых не было при настройке поиска: соседи эталонов по девяти вендорам (склонение, транслит, опечатка, раскладка, атрибут) и запросы по остальному каталогу (примеры ТЗ в словоформах, «сервер», «по запросу → альтернатива», сервисные и мусорные). Прогоняется один раз — при заморозке функций.',
    how: 'Success@1 — доля запросов, где первая цель ответа совпала с ожидаемой. Ожидаемые цели проверены по фиду и подтверждаются человеком.',
    rows: closed.value.map((c) => ({
      label: c.label,
      value: c.score ? outOf(c.score.matched, c.score.total) : 'прогона нет',
    })),
  })
}

function explainFour() {
  const f = four.value
  if (!f) return
  explain({
    title: 'Ответы с ≥4 товарами',
    value: shareOrNull(f.families4) ?? '—',
    what: 'Требование ТЗ: в ответе не меньше четырёх товаров. Считаются разные семейства среди карточек ответа полного продукта, включая «Можно купить сейчас» и «Популярное в наличии».',
    how: 'Ответы с ≥4 разными семействами / все ответы последних прогонов наборов.',
    rows: [
      { label: 'Ответов', value: formatInt(f.answers) },
      { label: 'С ≥4 семействами', value: formatInt(f.families4_n) },
      { label: 'С ≥4 SKU', value: `${formatInt(f.skus4_n)} (${shareOrNull(f.skus4) ?? '—'})` },
      { label: 'Наборы', value: f.sets.map(setLabel).join(', ') },
    ],
  })
}

function explainCost(r: FeedCostRun) {
  const c = cost.value
  explain({
    title:
      r.mode === 'journal'
        ? 'Стоимость запроса по журналу витрины на фиде'
        : `Стоимость запроса, ${modeLabel(r.mode)} прогон`,
    value: formatMoney(r.per_request_rub),
    what: `При ${formatInt(c?.requests ?? 0)} запросах в месяц. Постоянные расходы — инфраструктура стенда и агент SearchOps; LLM — разбор трудных запросов.`,
    how: c?.formula,
    rows: [
      { label: 'Поисков в прогоне', value: formatInt(r.searches) },
      { label: 'p — с разбором моделью', value: formatPercent(r.p) },
      { label: 'h — разбор из кэша', value: formatPercent(r.h) },
      { label: 'c_llm — поиск с вызовом модели', value: num(r.c_llm) ? formatCost(r.c_llm) : '—' },
      { label: 'LLM на запрос', value: formatCost(r.llm_per_request_rub) },
      { label: 'Постоянные на запрос', value: formatCost(r.fixed_per_request_rub) },
      { label: 'Доля инфраструктуры', value: formatPercent(r.infra_share) },
      { label: 'Доля агента', value: formatPercent(r.agent_share) },
      { label: 'Доля LLM', value: formatPercent(r.llm_share, 2) },
    ],
  })
}

const costSub = computed(() => {
  const r = head.value
  if (!r) return undefined
  return `${formatInt(cost.value?.requests ?? 0)} запросов в месяц · инфраструктура ${formatPercent(r.infra_share, 0)} · LLM ${formatPercent(r.llm_share, 2)}`
})

/** Таблица: подпись набора — только в первой строке группы. */
const rows = computed(() =>
  (data.value?.table ?? []).map((r, i, all) => ({
    ...r,
    first: i === 0 || all[i - 1]!.set !== r.set,
  })),
)
function explainRow(r: FeedRow) {
  const m = r.metrics
  explain({
    title: `${setLabel(r.set)} — ${configLabel(data.value ?? null, r.config)}`,
    value: m.success1 !== null ? outOf(m.success1_n, m.judged) : '—',
    what: data.value?.configs.find((c) => c.id === r.config)?.note,
    rows: [
      { label: 'Запросов', value: formatInt(r.queries) },
      { label: 'Success@1', value: shareOrNull(m.success1) ?? '—' },
      { label: 'В первых четырёх', value: shareOrNull(m.top4) ?? '—' },
      { label: 'MRR@10', value: num(m.mrr10) ? formatDecimal(m.mrr10, 3) : '—' },
      { label: '≥4 семейств', value: shareOrNull(m.families4) ?? '—' },
      { label: '≥4 SKU', value: shareOrNull(m.skus4) ?? '—' },
      {
        label: 'Нарушения жёстких условий',
        value: m.hard_queries
          ? `${formatInt(m.hard_violated)} из ${formatInt(m.hard_queries)}`
          : '—',
      },
      { label: 'p50 / p95', value: `${formatMs(r.p50_ms)} / ${formatMs(r.p95_ms)}` },
      { label: 'Режим', value: modeLabel(r.mode) },
      { label: 'Прогон', value: `…${r.run_tail}` },
    ],
  })
}
</script>

<template>
  <section class="feed" aria-labelledby="feed-title">
    <h2 id="feed-title" class="sr-only">На данных заказчика</h2>

    <template v-if="res.status.value === 'loading' && !data">
      <div class="feed__tiles" aria-busy="true">
        <div v-for="i in 4" :key="i" class="panel feed__skel">
          <span class="skeleton" style="width: 60%" />
          <span class="skeleton" style="width: 40%; height: 1.5rem" />
        </div>
      </div>
    </template>

    <BlockError
      v-else-if="res.status.value === 'error' && !data"
      :error="res.error.value!"
      title="Сводка на данных заказчика не загрузилась"
      @retry="res.load()"
    />

    <template v-else-if="data">
      <p v-if="catalogLine" class="feed__catalog">
        <Database :size="16" aria-hidden="true" />
        Каталог: <strong>{{ catalogLine }}</strong>
      </p>

      <div v-if="!hasFeedRuns(data)" class="panel feed__empty">
        <p class="t-h2">Прогонов на фиде ещё не было</p>
        <p class="t-muted">
          Числа появятся после прогона наборов «по цели»:
          <span class="t-mono">go run . eval --set etalon,tz,neighbors</span>.
        </p>
        <div>
          <button type="button" class="btn btn--secondary" @click="res.load()">
            <RotateCcw :size="16" aria-hidden="true" /> Обновить
          </button>
        </div>
      </div>

      <template v-else>
        <div class="feed__tiles">
          <StatTile
            label="Эталоны заказчика"
            :value="etalon ? outOf(etalon.matched, etalon.total) : null"
            sub="первый ответ ведёт на цель заказчика"
            missing="прогон эталонов не запускался"
            @explain="explainEtalon"
          />

          <button
            type="button"
            class="panel feed__closed"
            :aria-label="
              pending
                ? 'Закрытая часть: прогон не запускался. Что это'
                : 'Закрытая часть, Success@1. Как посчитано'
            "
            @click="explainClosed"
          >
            <span class="feed__label">Закрытая часть, Success@1</span>
            <span v-if="pending" class="feed__pending">Прогон не запускался</span>
            <span v-else class="feed__triple">
              <span v-for="c in closed" :key="c.config" class="feed__cell">
                <span class="feed__cell-value t-num">{{
                  c.score ? (shareOrNull(c.score.success1) ?? '—') : '—'
                }}</span>
                <span class="feed__cell-label">{{ c.label }}</span>
              </span>
            </span>
            <span class="feed__sub">{{
              pending ? 'один прогон при заморозке функций' : 'новые формулировки, без настройки'
            }}</span>
          </button>

          <StatTile
            label="Ответы с ≥4 товарами"
            :value="four ? shareOrNull(four.families4) : null"
            :sub="four ? `${outOf(four.families4_n, four.answers)} · разных семейств` : undefined"
            missing="прогонов полного продукта нет"
            @explain="explainFour"
          />

          <StatTile
            label="Стоимость запроса"
            :value="head ? formatMoney(head.per_request_rub) : null"
            :sub="costSub"
            missing="прогонов полного продукта нет"
            @explain="head && explainCost(head)"
          />
        </div>

        <section v-if="cost && costRuns.length" class="feed__block" aria-labelledby="feed-cost">
          <h3 id="feed-cost" class="t-h2">Стоимость запроса</h3>
          <p class="t-sm">
            <span class="t-mono">{{ cost.formula }}</span>, N = {{ formatInt(cost.requests) }}
            запросов в месяц. Доли p и h — по трассам прогона полного продукта; c_llm — по llm_usage
            прогона, а если модель в нём не вызывалась — по llm_usage поисков на этом каталоге.
            Строка журнала — для сравнения: поиски витрины и трафика на фиде.
          </p>
          <div class="data-scroll" role="region" aria-label="Стоимость запроса" tabindex="0">
            <table class="data-table">
              <thead>
                <tr>
                  <th scope="col">Прогон</th>
                  <th scope="col" class="num">Поисков</th>
                  <th scope="col" class="num">p</th>
                  <th scope="col" class="num">h</th>
                  <th scope="col" class="num">c_llm</th>
                  <th scope="col" class="num">LLM на запрос</th>
                  <th scope="col" class="num">C на запрос</th>
                  <th scope="col" class="num">Инфраструктура</th>
                  <th scope="col" class="num">LLM</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="r in costRuns" :key="r.mode">
                  <th scope="row">
                    {{
                      r.mode === 'journal'
                        ? 'журнал витрины на фиде'
                        : `${modeLabel(r.mode)} прогон`
                    }}
                  </th>
                  <td class="num">{{ formatInt(r.searches) }}</td>
                  <td class="num">{{ formatPercent(r.p) }}</td>
                  <td class="num">{{ formatPercent(r.h) }}</td>
                  <td class="num">{{ num(r.c_llm) ? formatCost(r.c_llm) : '—' }}</td>
                  <td class="num">{{ formatCost(r.llm_per_request_rub) }}</td>
                  <td class="num">
                    <button type="button" class="num-btn" @click="explainCost(r)">
                      <strong>{{ formatMoney(r.per_request_rub) }}</strong>
                    </button>
                  </td>
                  <td class="num">{{ formatPercent(r.infra_share) }}</td>
                  <td class="num">{{ formatPercent(r.llm_share, 2) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <ul class="feed__params t-sm">
            <li v-for="p in cost.params" :key="p.key">
              {{ p.label }}: <strong class="t-num">{{ paramValue(p.value) }} {{ p.unit }}</strong>
              <span class="t-muted">
                — {{ p.source === 'measured' ? 'замер' : 'допущение' }}: {{ p.hint }}</span
              >
            </li>
          </ul>
          <p v-for="n in cost.notes" :key="n" class="t-xs t-muted">{{ n }}</p>
        </section>

        <section v-if="rows.length" class="feed__block" aria-labelledby="feed-sets">
          <h3 id="feed-sets" class="t-h2">Наборы и конфигурации</h3>
          <div class="data-scroll" role="region" aria-label="Наборы и конфигурации" tabindex="0">
            <table class="data-table">
              <thead>
                <tr>
                  <th scope="col">Набор</th>
                  <th scope="col">Конфигурация</th>
                  <th scope="col" class="num">Success@1</th>
                  <th scope="col" class="num">В первых 4</th>
                  <th scope="col" class="num">MRR@10</th>
                  <th scope="col" class="num">≥4 семейств</th>
                  <th scope="col" class="num">p50 / p95</th>
                  <th scope="col" class="num">₽ LLM на запрос</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="r in rows"
                  :key="`${r.set}-${r.config}`"
                  :class="{ 'feed__group-start': r.first }"
                >
                  <th scope="row">{{ r.first ? setLabel(r.set) : '' }}</th>
                  <td>{{ configLabel(data, r.config) }}</td>
                  <td class="num">
                    <button type="button" class="num-btn" @click="explainRow(r)">
                      {{
                        r.metrics.success1 !== null
                          ? outOf(r.metrics.success1_n, r.metrics.judged)
                          : '—'
                      }}
                    </button>
                  </td>
                  <td class="num">{{ shareOrNull(r.metrics.top4) ?? '—' }}</td>
                  <td class="num">
                    {{ num(r.metrics.mrr10) ? formatDecimal(r.metrics.mrr10, 3) : '—' }}
                  </td>
                  <td class="num">{{ shareOrNull(r.metrics.families4) ?? '—' }}</td>
                  <td class="num">{{ formatMs(r.p50_ms) }} / {{ formatMs(r.p95_ms) }}</td>
                  <td class="num">
                    {{ num(r.rub_per_query) ? formatCost(r.rub_per_query) : '—' }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <p class="t-xs t-muted">
            BM25 — текстовый поиск по запросу как набран; BM25 + нормализатор — плюс раскладка,
            опечатки, транслит и словари; гибрид — плюс векторы и слияние RRF; полный продукт —
            понимание запроса, цель ответа, лестница «4 товара» и модель на трудных запросах.
          </p>
        </section>
      </template>

      <p v-if="data.mock_runs" class="t-xs t-muted">
        Прогоны на mock-каталоге, до данных заказчика, остались во вкладках «Метрики» и «Вклад
        компонентов» и подписаны там.
      </p>
      <p
        v-if="res.status.value === 'error' && res.error.value"
        class="t-xs feed__stale"
        role="alert"
      >
        Не удалось обновить: {{ res.error.value.message }}
        <button type="button" class="link" @click="res.load()">
          <RotateCcw :size="12" aria-hidden="true" /> Повторить
        </button>
      </p>
    </template>

    <ExplainDialog v-model:open="open" :explanation="explanation" />
  </section>
</template>

<style scoped>
.feed {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  min-width: 0;
}
.feed__catalog {
  display: inline-flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
  margin: 0;
  color: var(--text-2);
}
.feed__catalog strong {
  color: var(--text);
  font-weight: var(--fw-semibold);
}
.feed__tiles {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--space-3);
}
.feed__skel {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
}
.feed__empty {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-5);
  color: var(--text-2);
}
/* Плитка закрытой части — как StatTile, но с тремя числами. */
.feed__closed {
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--space-1);
  min-width: 0;
  padding: var(--space-3) var(--space-4);
  text-align: left;
  font: inherit;
  color: var(--text);
  cursor: pointer;
  transition: border-color var(--dur-fast) var(--ease-standard);
}
.feed__closed:hover {
  border-color: var(--border-control);
}
.feed__label {
  color: var(--text-2);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.feed__pending {
  color: var(--text-3);
  font-size: var(--fs-body);
  font-weight: var(--fw-medium);
  line-height: var(--lh-h1);
}
.feed__triple {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-4);
}
.feed__cell {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.feed__cell-value {
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
  font-weight: var(--fw-semibold);
}
.feed__cell-label,
.feed__sub {
  color: var(--text-3);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
}
.feed__block {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}
.feed__block p {
  margin: 0;
}
.feed__params {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin: 0;
  padding-left: var(--space-4);
}
.feed__group-start th,
.feed__group-start td {
  border-top: var(--border-w) solid var(--border);
}
.feed__stale {
  color: var(--danger);
}
@media (max-width: 1023px) {
  .feed__tiles {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
@media (max-width: 639px) {
  .feed__tiles {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
