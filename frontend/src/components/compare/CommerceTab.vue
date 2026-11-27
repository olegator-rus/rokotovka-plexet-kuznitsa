<script setup lang="ts">
// «До / После» → «Коммерция»: лестница C0–C3 на наборе p2, плитки метрик блока, прогоны с
// одним изменённым компонентом (улучшения рядом с ухудшениями), запросы, где основная выдача ухудшилась.
// Числа — GET /api/evaluations/commerce; непосчитанное не показывается. Грузится при открытии вкладки.
import { computed, ref } from 'vue'
import { CircleAlert, RotateCcw } from 'lucide-vue-next'
import { getCommerceEval } from '@/api/relations'
import type { CommerceMetricKey } from '@/api/typesCommerce'
import BlockError from '@/components/common/BlockError.vue'
import ExplainDialog, { type Explanation } from '@/components/common/ExplainDialog.vue'
import StatTile from '@/components/common/StatTile.vue'
import SyntheticBadge from '@/components/common/SyntheticBadge.vue'
import { useResource } from '@/composables/useResource'
import {
  COMMERCE_METRICS,
  configLabel,
  ladderColumns,
  ladderReady,
  ladderRegressions,
  metricDef,
  metricValue,
  singleChangeDeltas,
  singleChangeLabel,
  sortLadder,
} from '@/lib/commerce'
import { shortHash } from '@/lib/evalView'
import { formatDecimal, formatInt, formatPercent, plural } from '@/lib/format'
import { MOCK_RUN_LABEL, runTail } from '@/lib/feedSummary'
import { compatibilityLabel } from '@/lib/relations'

const res = useResource((opts) => getCommerceEval(opts))
void res.load()

const data = computed(() => res.data.value)
/** «: 120 запросов» сразу за именем набора, без пробела перед двоеточием. */
const setSize = computed(() => {
  const n = data.value?.queries
  return n ? `: ${formatInt(n)} ${plural(n, ['запрос', 'запроса', 'запросов'])}` : ''
})
const ladder = computed(() => sortLadder(data.value?.ladder ?? []))
const cols = computed(() => ladderColumns(ladder.value))
const ready = (r: (typeof ladder.value)[number]) => ladderReady(r, data.value?.ready)
// Плитки блока — с последней готовой ступени, где блок посчитан.
const top = computed(
  () =>
    [...ladder.value]
      .reverse()
      .find((r) => ready(r) && typeof r.metrics?.compat_violations === 'number') ??
    [...ladder.value].reverse().find(ready) ??
    null,
)
const singles = computed(() =>
  (data.value?.single_changes ?? []).map((c) => ({ c, d: singleChangeDeltas(c, ladder.value) })),
)
// Таблица «Один компонент за раз»: строки — метрики, которые хоть где-то изменились,
// столбцы — прогоны с одним изменением. Пустая ячейка — метрика в этом прогоне не сдвинулась.
const singleRows = computed(() =>
  COMMERCE_METRICS.filter((def) =>
    singles.value.some(({ d }) => [...d.better, ...d.worse].some((x) => x.def.key === def.key)),
  ).map((def) => ({
    def,
    cells: singles.value.map(
      ({ d }) => [...d.better, ...d.worse, ...d.same].find((x) => x.def.key === def.key) ?? null,
    ),
  })),
)
const singlesHaveQueries = computed(() => singles.value.some(({ c }) => c.regressions?.length))
const worse = computed(() => ladderRegressions(ladder.value))
const pairs = computed(() => top.value?.block?.violations ?? [])
const notes = computed(() => (data.value?.notes ?? []).filter(Boolean))
const judge = computed(() => data.value?.judge_agreement ?? null)

const BLOCK_TILES: CommerceMetricKey[] = [
  'compat_violations',
  'required_recall',
  'coverage',
  'block_precision_judge',
]

const explain = ref<Explanation | null>(null)
const pairsOpen = ref(false)
const explainOpen = computed({
  get: () => explain.value !== null,
  set: (v) => {
    if (!v) {
      explain.value = null
      pairsOpen.value = false
    }
  },
})
function explainMetric(key: CommerceMetricKey) {
  const def = metricDef(key)
  if (!def || !top.value) return
  explain.value = {
    title: def.label,
    value: metricValue(key, top.value.metrics[key]) ?? '—',
    what: def.what,
    rows: ladder.value.map((r) => ({
      label: configLabel(r.config),
      value: metricValue(key, r.metrics?.[key]) ?? 'не посчитано',
    })),
    source: `make eval SET=${data.value?.set ?? 'p2'} · GET /api/evaluations/commerce${data.value?.synthetic ? ' · данные синтетические' : ''}`,
  }
  pairsOpen.value = key === 'compat_violations'
}
</script>

<template>
  <div class="com">
    <div v-if="res.status.value === 'loading' && !data" class="panel com__loading" aria-busy="true">
      <p class="t-sm t-muted" role="status">Загружаем последний прогон набора p2…</p>
      <span v-for="i in 5" :key="i" class="skeleton" :style="{ width: `${92 - i * 8}%` }" />
    </div>
    <BlockError
      v-else-if="res.status.value === 'error' && !data"
      :error="res.error.value!"
      title="Коммерческая лестница не загрузилась"
      @retry="res.load()"
    />
    <div v-else-if="data && !ladder.length" class="panel com__empty">
      <p class="t-h2">Прогон не запускался</p>
      <p class="t-muted">
        Лестница C0–C3 появится после прогона набора p2:
        <span class="t-mono">make eval SET=p2</span>.
      </p>
      <button type="button" class="btn btn--secondary btn--sm com__retry" @click="res.load()">
        <RotateCcw :size="14" aria-hidden="true" /> Обновить
      </button>
    </div>

    <template v-else-if="data">
      <p class="panel t-sm com__mock">
        Прогон на каталоге «{{ MOCK_RUN_LABEL }}». На фиде заказчика пары «товар → дополнение» не
        размечены, поэтому лестница C0–C3 считается на mock-каталоге и качество на фиде не
        показывает.
      </p>
      <div class="com__lead">
        <p class="t-sm">
          Набор <strong>{{ data.set ?? 'p2' }}</strong>{{ setSize }}
          — лицензии, парт-номера, характеристики, совместимость, комплекты и негативные примеры.
          Каждая ступень добавляет один компонент.
        </p>
        <SyntheticBadge v-if="data.synthetic" />
      </div>

      <div v-if="top" class="com__tiles">
        <StatTile
          v-for="k in BLOCK_TILES"
          :key="k"
          :label="metricDef(k)!.label"
          :value="metricValue(k, top.metrics[k])"
          :sub="
            k === 'block_precision_judge' && judge
              ? `согласие судьи с людьми ${formatPercent(judge.agreement, 0)} на ${formatInt(judge.n)}`
              : configLabel(top.config)
          "
          missing="не посчитано в этом прогоне"
          @explain="explainMetric(k)"
        />
      </div>

      <div class="data-scroll">
        <table class="data-table com__table">
          <caption class="sr-only">
            Коммерческая лестница конфигураций
          </caption>
          <thead>
            <tr>
              <th scope="col">Конфигурация</th>
              <th v-for="m in cols" :key="m.key" scope="col" class="num">
                <button type="button" class="num-btn" @click="top && explainMetric(m.key)">
                  {{ m.label }}
                </button>
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="r in ladder" :key="r.config">
              <th scope="row">
                {{ configLabel(r.config) }}
                <span v-if="!ready(r)" class="badge">не готово</span>
              </th>
              <td v-for="m in cols" :key="m.key" class="num">
                {{ metricValue(m.key, r.metrics?.[m.key]) ?? '—' }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <ul v-if="notes.length" class="com__notes t-xs t-muted">
        <li v-for="n in notes" :key="n">{{ n }}</li>
      </ul>
      <p class="t-xs t-muted com__repro">
        Воспроизводимость:
        <template v-if="data.repro"
          ><span class="t-mono">{{ data.repro }}</span></template
        ><template v-else
          >набор {{ data.set ?? 'p2'
          }}<template v-if="data.set_hash">
            · хэш <span class="t-mono">{{ shortHash(data.set_hash) }}</span></template
          ><template v-if="data.commit">
            · коммит <span class="t-mono">{{ shortHash(data.commit) }}</span></template
          ></template
        ><template v-if="data.run_id"> · прогон {{ runTail(data.run_id) }}</template> ·
        <span class="t-mono">make eval SET=p2</span>
      </p>

      <section v-if="singles.length" class="com__singles" aria-labelledby="sc-title">
        <h2 id="sc-title" class="t-h2">Один компонент за раз</h2>
        <p class="t-sm t-muted">
          Прогон с одним изменённым компонентом против своей базы: значение метрики и сдвиг. Прочерк
          — метрика не изменилась.
        </p>
        <div class="data-scroll">
          <table class="data-table com__sc-table">
            <caption class="sr-only">
              Изменения метрик в прогонах с одним изменённым компонентом
            </caption>
            <thead>
              <tr>
                <th scope="col">Метрика</th>
                <th v-for="{ c } in singles" :key="c.config" scope="col" class="num">
                  <span class="com__sc-name">{{ singleChangeLabel(c) }}</span>
                  <span class="com__sc-base">к {{ configLabel(c.against ?? c.base ?? '') }}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="row in singleRows" :key="row.def.key">
                <th scope="row">{{ row.def.label }}</th>
                <td v-for="(x, i) in row.cells" :key="i" class="num">
                  <template v-if="x && x.delta.tone !== 'same'">
                    <span class="com__sc-value">{{ x.value }}</span>
                    <span
                      class="com__delta"
                      :class="x.delta.tone === 'better' ? 'com__delta--up' : 'com__delta--down'"
                      >{{ x.delta.text }}</span
                    >
                  </template>
                  <span v-else class="com__sc-same" title="Не изменилось">—</span>
                </td>
              </tr>
              <tr v-if="!singleRows.length">
                <td :colspan="singles.length + 1" class="t-muted">
                  Ни один прогон не сдвинул метрики.
                </td>
              </tr>
              <tr v-if="singlesHaveQueries">
                <th scope="row">Запросы хуже</th>
                <td v-for="{ c } in singles" :key="c.config" class="num com__sc-queries">
                  <template v-if="c.regressions?.length">
                    {{ c.regressions.map((q) => `«${q.text}»`).join(', ') }}
                  </template>
                  <span v-else class="com__sc-same">—</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section class="com__worse" aria-labelledby="cw-title">
        <h2 id="cw-title" class="t-h2">Где основная выдача ухудшилась</h2>
        <p v-if="!worse.length" class="panel com__none t-sm t-muted">
          Ни один запрос набора не стал хуже по NDCG@10 от добавленных компонентов.
        </p>
        <div v-else class="data-scroll">
          <table class="data-table">
            <caption class="sr-only">
              Запросы с ухудшением основной выдачи
            </caption>
            <thead>
              <tr>
                <th scope="col">Запрос</th>
                <th scope="col">Ступень</th>
                <th scope="col" class="num">NDCG@10 было</th>
                <th scope="col" class="num">стало</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="w in worse" :key="w.query + w.to">
                <th scope="row">{{ w.query }}</th>
                <td>{{ [w.from, w.to].filter(Boolean).join(' → ') || '—' }}</td>
                <td class="num">
                  {{ w.before !== null ? formatDecimal(w.before, 3) : '—' }}
                </td>
                <td class="num com__down">
                  {{ w.after !== null ? formatDecimal(w.after, 3) : '—' }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <p v-if="res.status.value === 'error'" class="t-xs com__stale" role="alert">
        <CircleAlert :size="12" aria-hidden="true" />
        Не удалось обновить: {{ res.error.value?.message }}. Показан последний удачный прогон.
        <button type="button" class="link" @click="res.load()">Повторить</button>
      </p>
    </template>

    <ExplainDialog v-model:open="explainOpen" :explanation="explain">
      <div v-if="pairsOpen" class="com__pairs">
        <p class="t-xs t-muted">
          Проверенные пары<template v-if="data?.pairs"
            >: {{ formatInt(data.pairs) }} размеченных</template
          >
        </p>
        <p v-if="!pairs.length" class="t-sm">
          Ни одна пара с разметкой «несовместимо» не попала в блок.
        </p>
        <ul v-else class="com__pairs-ul">
          <li v-for="(p, i) in pairs" :key="i" class="t-sm">
            {{ p.anchor_product }} → {{ p.candidate_product }}
            <span class="t-muted">· {{ compatibilityLabel(p.class) }}</span>
          </li>
        </ul>
      </div>
    </ExplainDialog>
  </div>
</template>

<style scoped>
.com__mock {
  padding: var(--space-3) var(--space-4);
  color: var(--text-2);
}
.com {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.com__loading,
.com__empty {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-4);
}
.com__retry {
  align-self: flex-start;
}
.com__lead {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-2);
}
.com__lead p {
  max-width: 48rem;
}
.com__tiles {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 11rem), 1fr));
  gap: var(--space-3);
}
.com__table th[scope='row'] {
  white-space: nowrap;
}
.com__notes {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  padding-left: var(--space-4);
}
.com__repro {
  overflow-wrap: anywhere;
}
.com__singles,
.com__worse {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.com__sc-table th[scope='col'] {
  vertical-align: bottom;
  white-space: normal;
}
.com__sc-name {
  display: block;
  color: var(--text);
  font-weight: var(--fw-semibold);
  white-space: nowrap;
}
.com__sc-base {
  display: block;
  color: var(--text-3);
  font-size: var(--fs-xs);
  font-weight: var(--fw-regular);
}
.com__sc-table tbody th {
  color: var(--text-2);
  font-weight: var(--fw-regular);
  white-space: nowrap;
}
.com__sc-value {
  font-weight: var(--fw-medium);
}
.com__sc-same {
  color: var(--text-3);
}
.com__sc-queries {
  min-width: 10rem;
  white-space: normal;
  color: var(--text-2);
}
/* Изменение — цветная плашка; остальная строка нейтральная */
.com__delta {
  display: inline-block;
  min-width: 4.5rem;
  margin-left: var(--space-2);
  padding: 0 0.375rem;
  border-radius: var(--radius-sm);
  font-size: var(--fs-xs);
  font-weight: var(--fw-medium);
  font-variant-numeric: tabular-nums;
  text-align: right;
  white-space: nowrap;
}
.com__delta--up {
  background: color-mix(in srgb, var(--delta-up) 10%, transparent);
  color: var(--delta-up);
}
.com__delta--down {
  background: color-mix(in srgb, var(--delta-down) 10%, transparent);
  color: var(--delta-down);
}
.com__up {
  color: var(--delta-up);
}
.com__down {
  color: var(--delta-down);
}
.com__none {
  padding: var(--space-3) var(--space-4);
}
.com__stale {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  color: var(--danger);
}
.com__pairs {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}
.com__pairs-ul {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: 14rem;
  overflow: auto;
  margin: 0;
  padding: 0;
  list-style: none;
}
</style>
