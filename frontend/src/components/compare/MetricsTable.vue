<script setup lang="ts">
// Вкладка «Метрики»: строки — метрики, столбцы — посчитанные конфигурации лестницы. В ячейке значение
// и изменение к предыдущему столбцу. Ячейка — кнопка: открывает объяснение метрики и разбивку по типам.
import { computed, ref } from 'vue'
import type { EvaluationView, JudgeAgreement } from '@/api/types'
import DeltaMark from '@/components/common/DeltaMark.vue'
import ExplainDialog, { type Explanation } from '@/components/common/ExplainDialog.vue'
import { configHint } from '@/lib/configs'
import { type MetricCell, metricByType, metricRows, queryTypeLabel } from '@/lib/evalView'
import { formatDateTime, formatInt, formatPercent } from '@/lib/format'
import { formatMetric, metricDef } from '@/lib/metrics'

const props = defineProps<{
  columns: EvaluationView[]
  judge?: JudgeAgreement | null
  /** В B4 есть правки запусков агента до разреза журнала (agent_credit.pre_holdout): B4 видел тексты набора. */
  b4PreHoldout?: boolean
}>()
const emit = defineEmits<{ regressions: [config: string] }>()

const rows = computed(() => metricRows(props.columns))

const open = ref(false)
const explanation = ref<Explanation | null>(null)
const picked = ref<{ key: string; cell: MetricCell; run: EvaluationView } | null>(null)

const byType = computed(() => {
  const p = picked.value
  if (!p) return []
  const def = metricDef(p.key)
  return metricByType(p.run, p.key).map((r) => ({
    label: queryTypeLabel(r.type),
    value: formatMetric(r.value, def.format),
  }))
})

const regressionCount = computed(() => picked.value?.run.regressions?.length ?? 0)

function explain(key: string, cell: MetricCell, idx: number) {
  const def = metricDef(key)
  const run = props.columns[idx]
  picked.value = { key, cell, run }
  const rows = [
    { label: 'Конфигурация', value: `${cell.config} — ${configHint(cell.config)}` },
    { label: 'Набор', value: `${run.set}, ${formatInt(run.queries)} запросов` },
  ]
  if (cell.prevConfig && cell.delta) {
    rows.push({
      label: `${cell.prevConfig} (предыдущий столбец)`,
      value: formatMetric(cell.prevValue, def.format),
    })
    rows.push({ label: `Изменение к ${cell.prevConfig}`, value: cell.delta.label })
  }
  if (run.metrics.errors > 0)
    rows.push({ label: 'Запросов с ошибкой (не вошли)', value: formatInt(run.metrics.errors) })
  rows.push({ label: 'Прогон', value: formatDateTime(run.created_at) || run.run_id })
  explanation.value = {
    title: `${def.label} · ${cell.config}`,
    value: formatMetric(cell.value, def.format),
    what: def.what,
    how: def.how,
    rows,
    source: `GET /api/evaluations · прогон ${run.run_id}`,
  }
  open.value = true
}

function toRegressions() {
  if (!picked.value) return
  open.value = false
  emit('regressions', picked.value.run.config)
}
</script>

<template>
  <div class="metrics-scroll" role="region" aria-label="Таблица метрик" tabindex="0">
    <table class="metrics">
      <thead>
        <tr>
          <th scope="col" class="metrics__name">Метрика</th>
          <th v-for="run in columns" :key="run.config" scope="col" class="metrics__col">
            <span class="metrics__cfg">{{ run.config }}</span>
            <span class="metrics__cfg-hint">{{ configHint(run.config) }}</span>
            <span
              v-if="run.config === 'B4' && b4PreHoldout"
              class="metrics__cfg-note metrics__cfg-note--warn"
            >
              не независимая оценка: действующая правка предложена до исправления оценки
            </span>
            <span v-else-if="run.config === 'B4' && run.set === 'test'" class="metrics__cfg-note">
              на test — оценка обобщения
            </span>
          </th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in rows" :key="row.key">
          <th scope="row" class="metrics__name">
            <span>{{ metricDef(row.key).label }}</span>
            <span class="metrics__dir">
              {{ metricDef(row.key).better === 'up' ? 'больше — лучше' : 'меньше — лучше' }}
            </span>
            <a v-if="row.key === 'ndcg10' && judge" href="#judge" class="link metrics__judge">
              LLM-судья: согласие {{ formatPercent(judge.within1, 0) }} (±1 балл)
            </a>
          </th>
          <td v-for="(cell, i) in row.cells" :key="cell.config" class="metrics__cell">
            <button
              v-if="cell.value !== null"
              type="button"
              class="metrics__btn"
              :aria-label="`${metricDef(row.key).label}, ${cell.config}: ${formatMetric(cell.value, metricDef(row.key).format)}${cell.delta ? ', ' + cell.delta.label : ''}. Как посчитано`"
              @click="explain(row.key, cell, i)"
            >
              <span class="metrics__value t-num">{{
                formatMetric(cell.value, metricDef(row.key).format)
              }}</span>
              <DeltaMark v-if="cell.delta" :delta="cell.delta" />
            </button>
            <span v-else class="metrics__none" title="Не посчитано в этом прогоне"
              >не считалось</span
            >
          </td>
        </tr>
      </tbody>
    </table>
  </div>

  <ExplainDialog v-model:open="open" :explanation="explanation">
    <div v-if="byType.length" class="by-type">
      <p class="t-xs t-muted">По типам запросов</p>
      <dl class="by-type__list">
        <template v-for="r in byType" :key="r.label">
          <dt>{{ r.label }}</dt>
          <dd class="t-num">{{ r.value }}</dd>
        </template>
      </dl>
    </div>
    <button
      v-if="regressionCount > 0"
      type="button"
      class="btn btn--secondary btn--sm"
      @click="toRegressions"
    >
      Запросы, где {{ picked?.run.config }} проиграла: {{ regressionCount }}
    </button>
  </ExplainDialog>
</template>

<style scoped>
.metrics-scroll {
  overflow-x: auto;
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
}
.metrics {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.metrics th,
.metrics td {
  padding: var(--space-2) var(--space-3);
  border-bottom: var(--border-w) solid var(--border);
  text-align: right;
  vertical-align: top;
}
.metrics tbody tr:last-child th,
.metrics tbody tr:last-child td {
  border-bottom: 0;
}
.metrics thead th {
  font-weight: var(--fw-medium);
  color: var(--text-2);
  background: var(--surface-2);
}
.metrics__name {
  position: sticky;
  left: 0;
  z-index: 1;
  min-width: 11rem;
  text-align: left !important;
  font-weight: var(--fw-regular);
  background: var(--surface);
}
thead .metrics__name {
  background: var(--surface-2);
}
.metrics__name span {
  display: block;
}
.metrics__judge {
  display: block;
  margin-top: 2px;
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
  color: var(--text-2);
}
.metrics__dir {
  color: var(--text-3);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
}
.metrics__col {
  min-width: 7.5rem;
}
.metrics__cfg {
  display: block;
  color: var(--text);
  font-weight: var(--fw-semibold);
}
.metrics__cfg-hint {
  display: block;
  max-width: 10rem;
  margin-left: auto;
  color: var(--text-3);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
  font-weight: var(--fw-regular);
  white-space: normal;
}
.metrics__cfg-note.metrics__cfg-note--warn {
  color: var(--text);
  font-weight: var(--fw-medium);
}
.metrics__cfg-note {
  display: block;
  max-width: 10rem;
  margin-left: auto;
  color: var(--text-2);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
  font-weight: var(--fw-regular);
  white-space: normal;
}
.metrics__btn {
  display: inline-flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 2px;
  padding: var(--space-1) var(--space-2);
  margin: calc(-1 * var(--space-1)) calc(-1 * var(--space-2));
  border: var(--border-w) solid transparent;
  border-radius: var(--radius-control);
  background: none;
  cursor: pointer;
}
.metrics__btn:hover {
  border-color: var(--border-control);
  background: var(--surface-2);
}
.metrics__value {
  font-weight: var(--fw-medium);
}
.metrics__none {
  color: var(--text-3);
  font-size: var(--fs-xs);
}
@media (max-width: 639px) {
  .metrics__name {
    min-width: 8.5rem;
  }
  .metrics__col {
    min-width: 5.5rem;
  }
  .metrics__cfg-hint {
    display: none;
  }
  /* Пометка у B4 в узкой колонке растягивала шапку всей таблицы до ~180 px пустоты. */
  .metrics__cfg-note {
    min-width: 11rem;
  }
}
.by-type {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}
.by-type__list {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 2px var(--space-3);
  margin: 0;
  font-size: var(--fs-sm);
}
.by-type__list dd {
  margin: 0;
  text-align: right;
}
</style>
