<script setup lang="ts">
// Вкладка «Контроль улучшений»: сначала сводка по парам конфигураций — сколько запросов стало лучше, осталось
// без изменений и стало хуже (счёт сервера, change_counts), затем запросы с ухудшением: разница и причина.
// Клик по запросу — поиск в конфигурации пары с открытой трассой; «Сравнить» — вкладка «Один запрос»
// с двумя конфигурациями.
import { computed, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { Columns2, Equal, TrendingDown, TrendingUp } from 'lucide-vue-next'
import type { EvaluationView } from '@/api/types'
import DeltaMark from '@/components/common/DeltaMark.vue'
import ExplainDialog, { type Explanation } from '@/components/common/ExplainDialog.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import {
  NDCG_CHANGE_THRESHOLD,
  type PairSummary,
  type RegressionRow,
  agentLine,
  attentionLine,
  changeReasonLabel,
  comparedLine,
  pairSummaries,
  queryTypeLabel,
  regressionRows,
} from '@/lib/evalView'
import { catalogLabel, runTail } from '@/lib/feedSummary'
import { formatDecimal, formatInt } from '@/lib/format'
import { formatMetric, metricDef, metricDelta } from '@/lib/metrics'

const props = defineProps<{ columns: EvaluationView[] }>()
const filter = defineModel<string>('filter', { default: 'all' })
defineEmits<{ compare: [query: string, left: string, right: string] }>()

const NDCG = metricDef('ndcg10')
const pairs = computed(() => pairSummaries(props.columns))
/** Фактическая строка пары с правками агента; без такой пары или её счёта — строки нет. */
const agentFact = computed(() => agentLine(pairs.value.find((p) => p.agent)?.counts ?? null))
const all = computed(() => regressionRows(props.columns))
const rows = computed(() =>
  filter.value === 'all' ? all.value : all.value.filter((r) => r.config === filter.value),
)
const options = computed(() => [
  { value: 'all', label: 'Все конфигурации' },
  ...pairs.value.map((p) => ({ value: p.config, label: p.title })),
])

/** Три числа пары в порядке показа; те же подписи — в окне «Как посчитано». */
function groups(p: PairSummary) {
  const c = p.counts
  if (!c) return []
  return [
    { id: 'better', label: 'Стало лучше', value: formatInt(c.improved_n), icon: TrendingUp },
    { id: 'same', label: 'Без изменений', value: formatInt(c.unchanged_n), icon: Equal },
    { id: 'worse', label: 'Стало хуже', value: formatInt(c.regressed_n), icon: TrendingDown },
  ]
}
function pairLabel(p: PairSummary) {
  const c = p.counts
  if (!c) return p.title
  const nums = groups(p)
    .map((g) => `${g.label.toLowerCase()} — ${g.value}`)
    .join(', ')
  return `${p.title}: ${nums}. ${comparedLine(c.compared_n)}. Как посчитано`
}

const explainOpen = ref(false)
const explanation = ref<Explanation | null>(null)
function explain(p: PairSummary) {
  const c = p.counts
  if (!c) return
  explanation.value = {
    title: p.title,
    // Коротко — помещается в окно на 360 px; подписи трёх чисел — строками ниже.
    value: [c.improved_n, c.unchanged_n, c.regressed_n].map(formatInt).join(' / '),
    what: `Одни и те же запросы набора прошли через конфигурации ${p.against} и ${p.config}. Каждый запрос входит ровно в одну группу, поэтому три числа в сумме дают число проверенных запросов.`,
    how: `Запрос изменился, если NDCG@10 сдвинулся на ${formatDecimal(NDCG_CHANGE_THRESHOLD)} и больше, появился или пропал подходящий товар, выдача стала пустой или перестала быть пустой, точный товар занял или потерял первое место, изменилось число нарушений условий лицензии, вместо честного «нет» показано лишнее или наоборот. Запрос, у которого одно стало лучше, а другое хуже, считается ухудшением. Запросы с ошибкой поиска не сравниваются.`,
    rows: [
      ...groups(p).map((g) => ({ label: g.label, value: g.value })),
      { label: 'Всего проверено', value: formatInt(c.compared_n) },
      { label: 'Набор', value: `${p.run.set}, ${formatInt(p.run.queries)} запросов` },
      { label: 'Прогон', value: runTail(p.run.run_id) },
      { label: 'Каталог', value: catalogLabel(p.run.catalog_version) },
    ],
    source: `GET /api/evaluations · прогон ${p.run.run_id}`,
  }
  explainOpen.value = true
}

/** Дельту NDCG красим, только если она и есть потеря; иначе ухудшение — по причине (бейдж). */
function delta(r: RegressionRow) {
  const d = metricDelta(r.ndcg_to, r.ndcg_from, NDCG)
  return d && d.verdict === 'worse' ? d : null
}
function searchLink(query: string, config: string) {
  return { path: '/search', query: { q: query, config, trace: '1' } }
}
</script>

<template>
  <div class="regs">
    <section v-if="pairs.length" class="regs__summary" aria-labelledby="regs-summary-title">
      <h3 id="regs-summary-title" class="sr-only">Сводка по парам конфигураций</h3>
      <ul class="regs__pairs">
        <li v-for="p in pairs" :key="p.config">
          <button
            v-if="p.counts"
            type="button"
            :class="['panel', 'pair', 'pair--action', { 'pair--picked': filter === p.config }]"
            :aria-label="pairLabel(p)"
            @click="explain(p)"
          >
            <span class="pair__title">
              {{ p.title }}
              <span v-if="p.agent" class="badge">правки агента</span>
            </span>
            <span v-for="g in groups(p)" :key="g.id" :class="['pair__num', `pair__num--${g.id}`]">
              <span class="pair__value t-num">
                <component :is="g.icon" :size="16" aria-hidden="true" />
                {{ g.value }}
              </span>
              <span class="pair__label">{{ g.label }}</span>
            </span>
            <span class="pair__total">{{ comparedLine(p.counts.compared_n) }}</span>
          </button>
          <div v-else :class="['panel', 'pair', { 'pair--picked': filter === p.config }]">
            <span class="pair__title">
              {{ p.title }}
              <span v-if="p.agent" class="badge">правки агента</span>
            </span>
            <span class="pair__missing">
              нет данных: счёт пары не посчитан — в прогоне нет запросов обеих конфигураций
            </span>
          </div>
        </li>
      </ul>
      <p v-if="agentFact" class="t-sm regs__agent">{{ agentFact }}</p>
    </section>

    <section class="regs__attention" aria-labelledby="regs-attention-title">
      <div class="regs__head">
        <div class="regs__heading">
          <h3 id="regs-attention-title" class="t-h2">Запросы, требующие внимания</h3>
          <p v-if="rows.length" class="t-sm t-muted">
            {{ attentionLine(rows.length) }}. У каждого — причина и сравнение двух выдач.
          </p>
        </div>
        <UiSelect
          v-if="options.length > 2 && all.length > 0"
          v-model="filter"
          :options="options"
          label="Конфигурация"
        />
      </div>

      <p v-if="all.length === 0" class="panel regs__empty">
        Ухудшений не обнаружено. На всех проверенных запросах качество поиска сохранилось или
        выросло.
      </p>
      <p v-else-if="rows.length === 0" class="panel regs__empty">
        У этой пары конфигураций ухудшений нет.
      </p>

      <ul v-else class="regs__list">
        <li v-for="r in rows" :key="`${r.config}:${r.query_id}`" class="panel regs__row">
          <div class="regs__main">
            <RouterLink :to="searchLink(r.text, r.config)" class="link regs__query">
              {{ r.text }}
            </RouterLink>
            <span class="t-xs t-muted">
              <template v-if="r.type">{{ queryTypeLabel(r.type) }} · </template>
              <span class="t-num">{{ formatInt(r.freq) }} в месяц</span> · {{ r.query_id }}
            </span>
            <span v-if="r.reasons?.length" class="regs__reasons">
              <span v-for="why in r.reasons" :key="why" class="badge">{{
                changeReasonLabel(why)
              }}</span>
            </span>
          </div>
          <div class="regs__metric">
            <span class="t-xs t-muted">{{ NDCG.label }}</span>
            <span class="t-num">
              {{ r.against || '—' }} {{ formatMetric(r.ndcg_from, NDCG.format) }} → {{ r.config }}
              {{ formatMetric(r.ndcg_to, NDCG.format) }}
            </span>
            <DeltaMark v-if="delta(r)" :delta="delta(r)!" />
            <span v-else class="t-xs t-muted">NDCG не упал — см. причину</span>
          </div>
          <button
            v-if="r.against"
            type="button"
            class="btn btn--ghost btn--sm"
            :aria-label="`Сравнить «${r.text}» в ${r.against} и ${r.config}`"
            @click="$emit('compare', r.text, r.against, r.config)"
          >
            <Columns2 :size="14" aria-hidden="true" />
            Сравнить
          </button>
        </li>
      </ul>
    </section>
    <ExplainDialog v-model:open="explainOpen" :explanation="explanation" />
  </div>
</template>

<style scoped>
.regs {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}
.regs__summary,
.regs__attention {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.regs__pairs {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
/* Пара — строка: название, три числа с подписями, «всего проверено». На узком экране — карточка. */
.pair {
  box-sizing: border-box;
  display: grid;
  grid-template-columns: minmax(0, 1.3fr) repeat(3, minmax(0, 1fr)) minmax(0, 1.4fr);
  align-items: center;
  gap: var(--space-2) var(--space-3);
  width: 100%;
  padding: var(--space-3) var(--space-4);
  color: var(--text);
  font: inherit;
  text-align: left;
}
.pair--action {
  cursor: pointer;
  transition: border-color var(--dur-fast) var(--ease-standard);
}
.pair--action:hover {
  border-color: var(--border-control);
}
.pair--picked,
.pair--picked:hover {
  border-color: var(--accent);
  background: var(--accent-soft);
}
.pair__title {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1) var(--space-2);
  min-width: 0;
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
  font-weight: var(--fw-semibold);
}
.pair__num {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.pair__value {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  font-size: var(--fs-h1);
  line-height: var(--lh-h1);
  font-weight: var(--fw-semibold);
}
/* Значок группы — рядом с числом; цвет только у «лучше» и «хуже», подпись стоит всегда. */
.pair__value svg {
  flex: none;
  color: var(--text-3);
}
.pair__num--better svg {
  color: var(--delta-up);
}
.pair__num--worse svg {
  color: var(--delta-down);
}
.pair__label {
  color: var(--text-2);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
}
.pair__total,
.pair__missing {
  color: var(--text-3);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
}
.pair__total {
  text-align: right;
}
.pair__missing {
  grid-column: 2 / -1;
}
.regs__agent {
  margin: 0;
  color: var(--text-2);
}
.regs__head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--space-2);
}
.regs__heading {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 0;
}
.regs__heading h3,
.regs__heading p {
  margin: 0;
}
.regs__empty {
  padding: var(--space-4);
  color: var(--text-2);
}
.regs__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.regs__row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto auto;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-3);
}
.regs__main,
.regs__metric {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.regs__metric {
  align-items: flex-end;
  font-size: var(--fs-sm);
}
.regs__query {
  overflow-wrap: anywhere;
}
.regs__reasons {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1);
  margin-top: 2px;
}
@media (max-width: 639px) {
  .pair {
    grid-template-columns: repeat(3, minmax(0, 1fr));
    align-items: start;
    gap: var(--space-2);
  }
  .pair__title,
  .pair__total,
  .pair__missing {
    grid-column: 1 / -1;
  }
  .pair__total {
    text-align: left;
  }
  .regs__row {
    grid-template-columns: minmax(0, 1fr) auto;
  }
  .regs__metric {
    grid-column: 1 / -1;
    grid-row: 2;
    align-items: flex-start;
  }
}
</style>
