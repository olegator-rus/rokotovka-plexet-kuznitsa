<script setup lang="ts">
// «До / После» → «Панель»: прогон по префиксам для прежних подсказок (контур коммерции) и панели контура выдачи,
// задержки, блокирующие проверки, ухудшения рядом с улучшениями, две конверсии по событиям стенда.
// Числа — GET /api/evaluations?set=panel; непосчитанное не показывается. Грузится при открытии вкладки.
import { computed, ref } from 'vue'
import {
  CircleAlert,
  CircleCheck,
  CircleX,
  RotateCcw,
  TrendingDown,
  TrendingUp,
} from 'lucide-vue-next'
import { getPanelEvaluation } from '@/api/evalPanel'
import type { IntentRate } from '@/api/typesEvalPanel'
import BlockError from '@/components/common/BlockError.vue'
import ExplainDialog, { type Explanation } from '@/components/common/ExplainDialog.vue'
import StatTile from '@/components/common/StatTile.vue'
import { useResource } from '@/composables/useResource'
import {
  blockingChecks,
  PANEL_METRICS,
  type PanelMetricDef,
  panelDelta,
  panelMode,
  panelValue,
  PREFIX_TYPE_LABELS,
  rankLabel,
  rateSub,
  rateValue,
  SECTION_LABELS,
  splitChanges,
} from '@/lib/evalPanel'
import { shortHash } from '@/lib/evalView'
import { formatDateTime, formatInt, plural } from '@/lib/format'

const res = useResource((opts) => getPanelEvaluation(opts))
void res.load()

const data = computed(() => res.data.value)
const run = computed(() => data.value?.run ?? null)
const oldMode = computed(() => panelMode(run.value, 'old'))
const newMode = computed(() => panelMode(run.value, 'new'))
const modes = computed(() => [oldMode.value, newMode.value].filter((m) => m !== null))
const checks = computed(() =>
  newMode.value?.available ? blockingChecks(newMode.value.metrics) : [],
)
const changes = computed(() => splitChanges(run.value?.changes ?? []))
const intents = computed(() => data.value?.intents ?? null)
const SHOWN = 12

const explain = ref<Explanation | null>(null)
const explainOpen = computed({
  get: () => explain.value !== null,
  set: (v) => {
    if (!v) explain.value = null
  },
})

function explainMetric(def: PanelMetricDef) {
  if (!run.value) return
  explain.value = {
    title: def.label,
    value:
      panelValue(def, newMode.value?.metrics) ?? panelValue(def, oldMode.value?.metrics) ?? '—',
    what: def.what,
    rows: modes.value.map((m) => ({
      label: m.label,
      value: m.available ? (panelValue(def, m.metrics) ?? 'не посчитано') : 'не прогнан',
    })),
    source: `make eval SET=panel · ${run.value.repro}`,
  }
}

function explainRate(title: string, r: IntentRate | null | undefined, what: string, how: string) {
  if (!r) return
  explain.value = {
    title,
    value: rateValue(r) ?? '—',
    what,
    how,
    rows: [
      { label: 'Числитель', value: formatInt(r.num) },
      { label: 'Знаменатель', value: formatInt(r.den) },
    ],
    source: `события стенда за ${data.value?.intents_days ?? 30} дней · GET /api/evaluations?set=panel`,
  }
}

const CONV_LEGACY = {
  title: 'Конверсия поиска, прежняя',
  what: 'Только отправленные запросы и выбранные подсказки: доля, после которых было действие с товаром.',
  how: 'Намерения с исходом «отправлен запрос» или «выбрана подсказка», где в том же намерении был клик, корзина, заказ или запрос цены, делённые на все такие намерения.',
}
const CONV_UNIFIED = {
  title: 'Конверсия поиска, единая',
  what: 'Все намерения: клик по товару в панели считается исходом, а не пропавшим поиском.',
  how: 'Намерения (кроме отказов) с действием с товаром или кликом по товару в панели, делённые на все намерения с исходом, включая отказ.',
}
const EMPTY_RESULTS = {
  title: 'Пустые выдачи',
  what: 'Доля отправленных запросов без результатов. Клики в панели в знаменатель не входят.',
  how: 'search_commit с числом результатов 0, делённые на все search_commit с известным числом результатов.',
}
const EMPTY_SUGGEST = {
  title: 'Пустые подсказки',
  what: 'Доля показов панели без единого элемента — отдельно от пустых выдач.',
  how: 'suggest_impression с items = 0 на префиксе от 2 символов, делённые на все такие показы.',
}

function sourcesLabel(src: Record<string, number> | undefined) {
  const parts = Object.entries(src ?? {}).map(
    ([k, v]) =>
      `${k === 'live' ? 'живые' : k === 'mock_log' ? 'журнал стенда' : k} ${formatInt(v)}`,
  )
  return parts.join(' · ')
}
</script>

<template>
  <div class="pt">
    <div v-if="res.status.value === 'loading' && !data" class="panel pt__box" aria-busy="true">
      <p class="t-sm t-muted" role="status">Загружаем прогон по префиксам…</p>
      <span v-for="i in 5" :key="i" class="skeleton" :style="{ width: `${92 - i * 8}%` }" />
    </div>
    <BlockError
      v-else-if="res.status.value === 'error' && !data"
      :error="res.error.value!"
      title="Прогон панели не загрузился"
      @retry="res.load()"
    />

    <template v-else-if="data">
      <div v-if="!run" class="panel pt__box">
        <p class="t-h2">Прогон по префиксам не запускался</p>
        <p class="t-muted">
          Метрики подсказок и товаров панели появятся после прогона набора префиксов:
          <span class="t-mono">make eval SET=panel</span>.
        </p>
        <button type="button" class="btn btn--secondary btn--sm pt__retry" @click="res.load()">
          <RotateCcw :size="14" aria-hidden="true" /> Обновить
        </button>
      </div>

      <template v-else>
        <p class="t-sm pt__lead">
          Набор <strong>panel</strong>: {{ formatInt(run.prefixes) }}
          {{ plural(run.prefixes, ['префикс', 'префикса', 'префиксов']) }} — широкие, семейства,
          артикулы, раскладка, опечатки, дубли лицензий и запросы без результатов. Индекс подсказок
          построен на раннем периоде журнала, проверка — на позднем<template
            v-if="run.split.late_from && run.split.late_to"
          >
            ({{ formatDateTime(run.split.late_from) }} — {{ formatDateTime(run.split.late_to) }},
            {{ formatInt(run.split.late_targets) }}
            {{ plural(run.split.late_targets, ['цель', 'цели', 'целей']) }})</template
          >. Прежние подсказки собраны из всего журнала, как работают на витрине.
        </p>

        <div class="data-scroll">
          <table class="data-table pt__table">
            <caption class="sr-only">
              Метрики панели: прежние подсказки и панель контура выдачи
            </caption>
            <thead>
              <tr>
                <th scope="col">Метрика</th>
                <th v-for="m in modes" :key="m.mode" scope="col" class="num">{{ m.label }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="def in PANEL_METRICS" :key="def.key">
                <th scope="row">
                  <button type="button" class="num-btn" @click="explainMetric(def)">
                    {{ def.label }}
                  </button>
                </th>
                <td v-for="m in modes" :key="m.mode" class="num">
                  <template v-if="!m.available">—</template>
                  <template v-else>
                    {{ panelValue(def, m.metrics) ?? '—' }}
                    <template v-if="m.mode === 'new'">
                      <TrendingUp
                        v-if="panelDelta(def, oldMode?.metrics, m.metrics) === 'better'"
                        :size="12"
                        class="pt__up"
                        aria-label="лучше прежнего"
                      />
                      <TrendingDown
                        v-else-if="panelDelta(def, oldMode?.metrics, m.metrics) === 'worse'"
                        :size="12"
                        class="pt__down"
                        aria-label="хуже прежнего"
                      />
                    </template>
                  </template>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-for="m in modes.filter((x) => !x.available)" :key="m.mode" class="t-sm t-muted">
          {{ m.label }}: не прогнан — {{ m.reason }}
        </p>

        <section v-if="checks.length" class="pt__section" aria-labelledby="pt-checks">
          <h2 id="pt-checks" class="t-h2">Блокирующие проверки панели</h2>
          <ul class="pt__checks">
            <li v-for="c in checks" :key="c.key" class="panel pt__check">
              <CircleCheck v-if="c.ok" :size="16" class="pt__up" aria-hidden="true" />
              <CircleX v-else :size="16" class="pt__down" aria-hidden="true" />
              <span>
                <span class="pt__check-label">{{ c.label }}: {{ c.ok ? 'да' : 'нет' }}</span>
                <span class="t-xs t-muted">{{ c.detail }}</span>
              </span>
            </li>
          </ul>
          <p v-if="newMode?.metrics.to_empty_examples.length" class="t-xs t-muted">
            Подсказки в пустую выдачу: {{ newMode.metrics.to_empty_examples.join(', ') }}
          </p>
        </section>

        <section class="pt__section" aria-labelledby="pt-changes">
          <h2 id="pt-changes" class="t-h2">Ухудшения и улучшения по префиксам</h2>
          <p v-if="!run.changes.length" class="panel pt__box t-sm t-muted">
            Место цели не изменилось ни на одном префиксе
            <template v-if="!newMode?.available"> — панель контура выдачи не прогнана</template>.
          </p>
          <div v-else class="pt__cols">
            <div v-for="col in ['worse', 'better'] as const" :key="col" class="panel pt__col">
              <p class="pt__col-title" :class="col === 'worse' ? 'pt__down' : 'pt__up'">
                <TrendingDown v-if="col === 'worse'" :size="14" aria-hidden="true" />
                <TrendingUp v-else :size="14" aria-hidden="true" />
                {{ col === 'worse' ? 'Хуже' : 'Лучше' }}: {{ formatInt(changes[col].length) }}
              </p>
              <ul class="pt__list">
                <li v-for="c in changes[col].slice(0, SHOWN)" :key="c.id + c.section" class="t-sm">
                  <span class="t-mono">«{{ c.prefix }}»</span> → {{ c.target }}
                  <span class="t-xs t-muted">
                    · {{ SECTION_LABELS[c.section] ?? c.section }} ·
                    {{ PREFIX_TYPE_LABELS[c.type] ?? c.type }} · {{ rankLabel(c.old_rank) }} →
                    {{ rankLabel(c.new_rank) }}
                  </span>
                </li>
              </ul>
              <p v-if="changes[col].length > SHOWN" class="t-xs t-muted">
                и ещё {{ formatInt(changes[col].length - SHOWN) }} — в JSON прогона (data/eval)
              </p>
            </div>
          </div>
        </section>

        <ul v-if="run.warnings.length" class="pt__notes t-xs t-muted">
          <li v-for="w in run.warnings" :key="w">{{ w }}</li>
        </ul>
        <p class="t-xs t-muted pt__repro">
          Воспроизводимость: хэш набора
          <span class="t-mono">{{ shortHash(run.set_hash) }}</span> · каталог
          {{ run.catalog_version || '—' }} · коммит
          <span class="t-mono">{{ shortHash(run.commit) }}</span> · прогон
          {{ formatDateTime(run.started_at) }} · <span class="t-mono">make eval SET=panel</span>
        </p>
      </template>

      <section class="pt__section" aria-labelledby="pt-intents">
        <h2 id="pt-intents" class="t-h2">Конверсия поиска в двух определениях</h2>
        <p class="t-sm t-muted">
          События стенда за {{ data.intents_days }} дней, единица учёта — поисковое намерение. Рост
          не объявляется по одной из конверсий: при запуске панели меняется и знаменатель.
          <template v-if="intents"> {{ sourcesLabel(intents.sources) }}.</template>
        </p>
        <div class="pt__tiles">
          <StatTile
            label="Конверсия, прежняя"
            :value="rateValue(intents?.conversion_legacy)"
            :sub="rateSub(intents?.conversion_legacy, ['намерение', 'намерения', 'намерений'])"
            missing="нет отправленных запросов с intent_id"
            @explain="
              explainRate(
                CONV_LEGACY.title,
                intents?.conversion_legacy,
                CONV_LEGACY.what,
                CONV_LEGACY.how,
              )
            "
          />
          <StatTile
            label="Конверсия, единая"
            :value="rateValue(intents?.conversion_unified)"
            :sub="rateSub(intents?.conversion_unified, ['намерение', 'намерения', 'намерений'])"
            missing="нет намерений с исходом"
            @explain="
              explainRate(
                CONV_UNIFIED.title,
                intents?.conversion_unified,
                CONV_UNIFIED.what,
                CONV_UNIFIED.how,
              )
            "
          />
          <StatTile
            label="Пустые выдачи"
            :value="rateValue(intents?.empty_results)"
            :sub="rateSub(intents?.empty_results, ['запрос', 'запроса', 'запросов'])"
            missing="нет отправленных запросов"
            @explain="
              explainRate(
                EMPTY_RESULTS.title,
                intents?.empty_results,
                EMPTY_RESULTS.what,
                EMPTY_RESULTS.how,
              )
            "
          />
          <StatTile
            label="Пустые подсказки"
            :value="rateValue(intents?.empty_suggestions)"
            :sub="rateSub(intents?.empty_suggestions, ['показ', 'показа', 'показов'])"
            missing="нет показов панели"
            @explain="
              explainRate(
                EMPTY_SUGGEST.title,
                intents?.empty_suggestions,
                EMPTY_SUGGEST.what,
                EMPTY_SUGGEST.how,
              )
            "
          />
        </div>
        <ul v-if="intents && intents.intents" class="pt__outcomes t-sm">
          <li v-for="o in intents.outcomes" :key="o.outcome">
            {{ o.label }}: {{ formatInt(o.count) }}
            <span class="t-muted">(с действием {{ formatInt(o.converted) }})</span>
          </li>
        </ul>
      </section>

      <p v-if="res.status.value === 'error'" class="t-xs pt__stale" role="alert">
        <CircleAlert :size="12" aria-hidden="true" />
        Не удалось обновить: {{ res.error.value?.message }}. Показан последний удачный прогон.
        <button type="button" class="link" @click="res.load()">Повторить</button>
      </p>
    </template>

    <ExplainDialog v-model:open="explainOpen" :explanation="explain" />
  </div>
</template>

<style scoped>
.pt {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.pt__box {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-4);
}
.pt__retry {
  align-self: flex-start;
}
.pt__lead {
  max-width: 52rem;
}
.pt__table th[scope='row'] {
  white-space: nowrap;
}
.pt__table td svg {
  vertical-align: middle;
  margin-left: 2px;
}
.pt__section {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.pt__checks {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 15rem), 1fr));
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.pt__check {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
  padding: var(--space-3);
}
.pt__check > svg {
  flex: none;
  margin-top: 2px;
}
.pt__check > span {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.pt__check-label {
  font-size: var(--fs-sm);
}
.pt__cols {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 20rem), 1fr));
  gap: var(--space-3);
}
.pt__col {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
  min-width: 0;
}
.pt__col-title {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  font-weight: var(--fw-medium);
}
.pt__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin: 0;
  padding: 0;
  list-style: none;
  overflow-wrap: anywhere;
}
.pt__tiles {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 11rem), 1fr));
  gap: var(--space-3);
}
.pt__outcomes {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-4);
  margin: 0;
  padding: 0;
  list-style: none;
}
.pt__notes {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  padding-left: var(--space-4);
}
.pt__repro {
  overflow-wrap: anywhere;
}
.pt__up {
  color: var(--delta-up);
}
.pt__down {
  color: var(--delta-down);
}
.pt__stale {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  color: var(--danger);
}
</style>
