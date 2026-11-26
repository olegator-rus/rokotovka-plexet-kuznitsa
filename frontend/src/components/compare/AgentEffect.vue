<script setup lang="ts">
// Вкладка «Эффект агента» (ревью B1): реплей отложенной недели журнала — набор holdout_week, B3 (без правок
// агента) против B4 (действующие правила агента). Агент эту неделю и тексты test не видит, поэтому эффект
// его правок меряется здесь, а не на test. Только чтение: GET /api/evaluations?set=holdout_week; оттуда же
// rescued (что идёт в вилку эффекта: до B3 или B4, вклад поиска, причина) и agent_credit.pre_holdout — правки B4
// из запусков до разреза журнала, тогда B4 подписан как не независимая оценка.
import { computed, onMounted } from 'vue'
import { RotateCcw } from 'lucide-vue-next'
import { getEvaluations } from '@/api/evaluations'
import type { EvalChange } from '@/api/types'
import BlockError from '@/components/common/BlockError.vue'
import DeltaMark from '@/components/common/DeltaMark.vue'
import { useResource } from '@/composables/useResource'
import { HOLDOUT_SET, holdoutVerdict, holdoutView, queryTypeLabel, shortHash } from '@/lib/evalView'
import { formatDateTime, formatDecimal, formatInt, formatPercent, plural } from '@/lib/format'
import { formatMetric, metricDef } from '@/lib/metrics'

const res = useResource((opts) => getEvaluations(opts, HOLDOUT_SET))
const view = computed(() => holdoutView(res.data.value))
// Доля спасённых для вилки эффекта (как в /api/effect) и честность правок B4 — из того же ответа.
const rescued = computed(() => res.data.value?.rescued ?? null)
const preHoldout = computed(() => res.data.value?.agent_credit?.pre_holdout === true)
const verdict = computed(() => (view.value ? holdoutVerdict(view.value) : ''))
const changes = computed(() => {
  const v = view.value
  if (!v) return []
  return [
    ...v.improvements.map((c) => ({ ...c, better: true })),
    ...v.regressions.map((c) => ({ ...c, better: false })),
  ]
})
const dec = (v: number | null | undefined) => (typeof v === 'number' ? formatDecimal(v, 3) : '—')
const ndcg = (c: EvalChange) => `${dec(c.ndcg_from)} → ${dec(c.ndcg_to)}`

const intro = computed(() => {
  const v = view.value
  if (!v) return ''
  let t = 'Реплей последней недели журнала'
  if (v.searches !== null) {
    t += ` — ${formatInt(v.searches)} ${plural(v.searches, ['поиск', 'поиска', 'поисков'])}`
    if (v.judgedSearches !== null) t += `, из них ${formatInt(v.judgedSearches)} размечены`
  }
  return `${t}; ${formatInt(v.queries)} ${plural(v.queries, ['пара', 'пары', 'пар'])} «запрос × клиент».`
})

const COMMAND = 'make eval SET=holdout_week CONFIGS=B3,B4'

onMounted(() => void res.load())
</script>

<template>
  <div
    v-if="res.status.value === 'loading' && !res.data.value"
    class="effect__skeleton"
    aria-busy="true"
  >
    <span class="skeleton" style="width: 60%" />
    <div class="panel effect__skeleton-table">
      <span v-for="i in 4" :key="i" class="skeleton" :style="{ width: `${85 - i * 8}%` }" />
    </div>
  </div>

  <BlockError
    v-else-if="res.status.value === 'error' && !res.data.value"
    :error="res.error.value!"
    title="Реплей отложенной недели не загрузился"
    @retry="res.load()"
  />

  <div v-else-if="!view" class="panel effect__empty">
    <p class="t-h2">Реплей отложенной недели ещё не считался</p>
    <p class="t-muted">
      Эффект правок агента меряется на последней неделе журнала, которую агент не видит: её поиски
      прогоняются через B3 и B4. Прогон запускается на сервере командой
      <span class="t-mono">{{ COMMAND }}</span>.
    </p>
    <div>
      <button type="button" class="btn btn--secondary btn--sm" @click="res.load()">
        <RotateCcw :size="14" aria-hidden="true" /> Обновить
      </button>
    </div>
  </div>

  <template v-else>
    <p class="t-sm effect__intro">
      {{ intro }}
      <template v-if="preHoldout">
        <strong>Столбец {{ view.agent.config }} — не независимая оценка:</strong> действующая правка
        предложена запуском до исправления оценки, когда агент ещё видел эту неделю и тексты test.
      </template>
      <template v-else>
        <strong>Агент не видел эту неделю и тексты test</strong> — поэтому эффект его правок
        меряется здесь.
      </template>
    </p>

    <section v-if="rescued" class="panel effect__fork" aria-labelledby="effect-fork-title">
      <h2 id="effect-fork-title" class="t-sm effect__fork-title">Что идёт в вилку эффекта</h2>
      <dl class="effect__fork-list">
        <div>
          <dt>Доля спасённых считается до</dt>
          <dd>
            <strong class="t-num">{{ rescued.config }}</strong>
            <span class="effect__hint">{{
              rescued.config === 'B3'
                ? 'только поиск: вклад правок агента не засчитан'
                : 'поиск и правки агента из честных запусков'
            }}</span>
          </dd>
        </div>
        <div>
          <dt>Вклад поиска, {{ rescued.baseline }} → B3</dt>
          <dd>
            <strong class="t-num">{{ formatPercent(rescued.search_share) }}</strong>
            <span class="effect__hint"
              >спасено среди поисков недели, где у {{ rescued.baseline }} подходящего в топ-10 не
              было</span
            >
          </dd>
        </div>
        <div>
          <dt>Вклад агента, B3 → B4</dt>
          <dd>
            <strong class="t-num">{{ formatPercent(rescued.agent_share) }}</strong>
            <span class="effect__hint">{{
              rescued.agent_counted ? 'засчитан' : 'не засчитан'
            }}</span>
          </dd>
        </div>
      </dl>
      <p class="t-xs t-muted effect__note">{{ rescued.agent_note }}</p>
    </section>

    <div
      class="data-scroll"
      role="region"
      aria-label="Эффект агента на отложенной неделе"
      tabindex="0"
    >
      <table class="data-table effect__table">
        <thead>
          <tr>
            <th scope="col">Метрика</th>
            <th scope="col" class="num">
              {{ view.base.config }}
              <span class="effect__hint"
                >без правок агента · правила v{{ view.base.rules_version }}</span
              >
            </th>
            <th scope="col" class="num">
              {{ view.agent.config }}
              <span class="effect__hint"
                >с правками агента · правила v{{ view.agent.rules_version }}</span
              >
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in view.rows" :key="row.key">
            <th scope="row">
              {{ metricDef(row.key).label }}
              <span class="effect__hint">{{
                metricDef(row.key).better === 'up' ? 'больше — лучше' : 'меньше — лучше'
              }}</span>
            </th>
            <td class="num">{{ formatMetric(row.base, metricDef(row.key).format) }}</td>
            <td class="num">
              <span class="effect__value">{{
                formatMetric(row.agent, metricDef(row.key).format)
              }}</span>
              <DeltaMark v-if="row.delta" :delta="row.delta" />
            </td>
          </tr>
          <tr v-if="view.rescued !== null">
            <th scope="row">
              Спасено поисков
              <span class="effect__hint"
                >у {{ view.base.config }} в топ-10 подходящего не было, у
                {{ view.agent.config }} есть</span
              >
            </th>
            <td class="num t-muted">—</td>
            <td class="num">{{ formatInt(view.rescued) }}</td>
          </tr>
          <tr v-if="view.lost !== null">
            <th scope="row">
              Потеряно поисков
              <span class="effect__hint"
                >у {{ view.base.config }} подходящий был, у {{ view.agent.config }} пропал</span
              >
            </th>
            <td class="num t-muted">—</td>
            <td class="num">{{ formatInt(view.lost) }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <p class="effect__verdict">{{ verdict }}</p>

    <div v-if="changes.length" class="data-scroll">
      <table class="data-table">
        <caption class="effect__caption t-xs t-muted">
          Запросы недели, где выдача изменилась
        </caption>
        <thead>
          <tr>
            <th scope="col">Запрос</th>
            <th scope="col">Тип</th>
            <th scope="col" class="num">Поисков</th>
            <th scope="col" class="num">NDCG@10</th>
            <th scope="col">Итог</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="c in changes" :key="c.query_id">
            <th scope="row">{{ c.text }}</th>
            <td class="t-muted">{{ queryTypeLabel(c.type) }}</td>
            <td class="num">{{ formatInt(c.freq) }}</td>
            <td class="num">{{ ndcg(c) }}</td>
            <td :class="['effect__tone', c.better ? 'delta-better' : 'delta-worse']">
              {{ c.better ? 'лучше' : 'хуже' }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <p v-if="!rescued" class="t-xs t-muted">
      Правки, принятые до того, как агенту закрыли эту неделю и тексты test, могли их видеть — для
      них результат здесь верхняя оценка.
    </p>
    <p class="t-xs t-muted effect__repro">
      Воспроизводимость: набор {{ view.agent.set }} · хэш
      <span class="t-mono" :title="view.agent.set_hash">{{ shortHash(view.agent.set_hash) }}</span>
      · прогон <span class="t-mono">{{ view.agent.run_id }}</span> ·
      {{ formatDateTime(view.agent.created_at) }} ·
      <span class="t-mono">{{ COMMAND }}</span>
    </p>
  </template>
</template>

<style scoped>
.effect__skeleton {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.effect__skeleton-table {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
}
.effect__empty {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-5);
  color: var(--text-2);
}
.effect__intro {
  max-width: 52rem;
}
.effect__table th[scope='row'] {
  font-weight: var(--fw-regular);
}
.effect__hint {
  display: block;
  color: var(--text-3);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
  font-weight: var(--fw-regular);
  white-space: normal;
}
.effect__value {
  display: block;
  font-weight: var(--fw-medium);
}
.effect__fork {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
}
.effect__fork-title {
  font-weight: var(--fw-medium);
}
.effect__fork-list {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(12rem, 1fr));
  gap: var(--space-3);
}
.effect__fork-list dt {
  color: var(--text-2);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
}
.effect__fork-list dd {
  margin: 0;
}
.effect__note {
  overflow-wrap: anywhere;
}
.effect__verdict {
  font-weight: var(--fw-medium);
}
.effect__caption {
  padding: var(--space-2) var(--space-3);
  text-align: left;
  caption-side: top;
}
.effect__tone.delta-better {
  color: var(--delta-up);
}
.effect__tone.delta-worse {
  color: var(--delta-down);
}
.effect__repro {
  overflow-wrap: anywhere;
}
</style>
