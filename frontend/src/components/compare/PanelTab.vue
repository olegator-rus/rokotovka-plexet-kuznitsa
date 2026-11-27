<script setup lang="ts">
// «До / После» → «Панель»: прогон по префиксам для прежних подсказок (контур коммерции) и панели контура выдачи,
// задержки, блокирующие проверки, ухудшения рядом с улучшениями, две конверсии по событиям стенда.
// Числа — GET /api/evaluations?set=panel; непосчитанное не показывается. Грузится при открытии вкладки.
import { computed, ref } from 'vue'
import type { Component } from 'vue'
import {
  Building2,
  Circle,
  CircleAlert,
  CircleCheck,
  CircleX,
  Folder,
  ListChecks,
  Package,
  RotateCcw,
  Search,
  TrendingDown,
  TrendingUp,
} from 'lucide-vue-next'
import { getPanelEvaluation } from '@/api/evalPanel'
import type { IntentRate } from '@/api/typesEvalPanel'
import BlockError from '@/components/common/BlockError.vue'
import PanelChanges from '@/components/compare/PanelChanges.vue'
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
  rateSub,
  rateValue,
} from '@/lib/evalPanel'
import { shortHash } from '@/lib/evalView'
import { formatInt, formatPercent, plural } from '@/lib/format'
import { MOCK_RUN_LABEL, catalogLabel, isMockCatalog, runTail } from '@/lib/feedSummary'

/** Значок исхода по коду сервера; незнакомый код — нейтральный кружок. */
const OUTCOME_ICONS: Record<string, Component> = {
  commit: Search,
  suggestion: ListChecks,
  product: Package,
  category: Folder,
  vendor: Building2,
  abandon: CircleX,
}
/** Длина полоски: какая часть случаев с этим исходом дошла до товара (оба числа — с сервера). */
const doneWidth = (converted: number, count: number) =>
  `${count > 0 ? Math.min(100, (converted / count) * 100) : 0}%`

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
const intents = computed(() => data.value?.intents ?? null)

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
        <p v-if="isMockCatalog(run.catalog_version)" class="panel t-sm pt__mock">
          Прогон на каталоге «{{ MOCK_RUN_LABEL }}». Набор префиксов собран по журналу mock-каталога
          (data/mock/prefixes.json): цели префиксов — товары mock, на фиде их нет, а набора
          префиксов по фиду пока нет. Поэтому эти числа качество подсказок на фиде не показывают.
          Числа на фиде магазина —
          <RouterLink :to="{ query: { tab: 'feed' } }">«На данных заказчика»</RouterLink>.
        </p>
        <p class="t-sm pt__lead">
          Набор <strong>panel</strong>: {{ formatInt(run.prefixes) }}
          {{ plural(run.prefixes, ['префикс', 'префикса', 'префиксов']) }} — широкие, семейства,
          артикулы, раскладка, опечатки, дубли лицензий и запросы без результатов. Индекс подсказок
          построен на раннем периоде журнала, проверка — на позднем<template
            v-if="run.split.late_targets"
          >
            ({{ formatInt(run.split.late_targets) }}
            {{ plural(run.split.late_targets, ['цель', 'цели', 'целей']) }})</template
          >. Прежние подсказки собраны из всего журнала, как работают на витрине.
        </p>

        <div class="data-scroll">
          <table class="data-table pt__table">
            <caption class="sr-only">
              Метрики панели: прежние подсказки и новая панель
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
                    <span class="pt__cell">
                      <span
                        v-if="
                          m.mode === 'new' &&
                          panelDelta(def, oldMode?.metrics, m.metrics) === 'better'
                        "
                        class="pt__delta pt__delta--better"
                      >
                        <TrendingUp :size="14" aria-hidden="true" /> лучше
                      </span>
                      <span
                        v-else-if="
                          m.mode === 'new' &&
                          panelDelta(def, oldMode?.metrics, m.metrics) === 'worse'
                        "
                        class="pt__delta pt__delta--worse"
                      >
                        <TrendingDown :size="14" aria-hidden="true" /> хуже
                      </span>
                      <span>{{ panelValue(def, m.metrics) ?? '—' }}</span>
                    </span>
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
              <span class="pt__check-body">
                <span class="pt__check-label">{{ c.label }}</span>
                <span class="t-xs t-muted">{{ c.detail }}</span>
              </span>
              <!-- Статус отдельной меткой справа: подпись не переносит «да» на новую строку -->
              <span class="pt__check-status" :class="c.ok ? 'pt__up' : 'pt__down'">{{
                c.ok ? 'да' : 'нет'
              }}</span>
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
            <template v-if="!newMode?.available"> — новая панель не прогнана</template>.
          </p>
          <PanelChanges v-else :changes="run.changes" />
        </section>

        <ul v-if="run.warnings.length" class="pt__notes t-xs t-muted">
          <li v-for="w in run.warnings" :key="w">{{ w }}</li>
        </ul>
        <p class="t-xs t-muted pt__repro">
          Воспроизводимость: хэш набора
          <span class="t-mono">{{ shortHash(run.set_hash) }}</span> · каталог
          {{ catalogLabel(run.catalog_version) }} · коммит
          <span class="t-mono">{{ shortHash(run.commit) }}</span> · прогон
          {{ runTail(run.run_id) }} · <span class="t-mono">make eval SET=panel</span>
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
        <div v-if="intents && intents.intents" class="pt__flow">
          <header class="pt__flow-head">
            <h3 class="pt__flow-title">Чем заканчивается поиск</h3>
            <p class="t-xs t-muted">
              Всего {{ formatInt(intents.intents) }}
              {{ plural(intents.intents, ['намерение', 'намерения', 'намерений']) }}. «Дошли до
              товара» — после исхода был клик, корзина, заказ или запрос цены.
            </p>
          </header>
          <p class="pt__flow-cols t-xs" aria-hidden="true">
            <span>Исход</span>
            <span>Сколько раз</span>
            <span>Дошли до товара</span>
          </p>
          <ul class="pt__outcomes">
            <li v-for="o in intents.outcomes" :key="o.outcome" class="pt__outcome">
              <span class="pt__outcome-name">
                <span class="pt__outcome-icon" aria-hidden="true">
                  <component :is="OUTCOME_ICONS[o.outcome] ?? Circle" :size="16" />
                </span>
                {{ o.label }}
              </span>
              <span class="pt__outcome-count t-num">
                <strong>{{ formatInt(o.count) }}</strong>
                <span v-if="o.share !== null" class="t-xs t-muted">
                  {{ formatPercent(o.share) }} всех
                </span>
              </span>
              <span class="pt__outcome-done">
                <span class="t-sm t-num">
                  <span class="sr-only">дошли до товара: </span>
                  <strong>{{ formatInt(o.converted) }}</strong>
                  <span class="t-muted"> из {{ formatInt(o.count) }}</span>
                </span>
                <span class="pt__bar" aria-hidden="true">
                  <span :style="{ width: doneWidth(o.converted, o.count) }" />
                </span>
              </span>
            </li>
          </ul>
        </div>
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
.pt__table td {
  vertical-align: middle;
}
.pt__cell {
  display: inline-flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--space-2);
}
.pt__delta {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  padding: 2px var(--space-2);
  border-radius: var(--radius-pill);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
  font-weight: var(--fw-semibold);
}
.pt__delta--better {
  background: var(--status-accepted-bg);
  color: var(--delta-up);
}
.pt__delta--worse {
  background: var(--danger-bg);
  color: var(--delta-down);
}
.pt__section {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.pt__checks {
  display: grid;
  /* Четыре проверки: 2×2 на обычной ширине, в один ряд на очень широкой, столбиком на узкой */
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 19rem), 1fr));
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
.pt__check-body {
  display: flex;
  flex: 1;
  min-width: 0;
  flex-direction: column;
  gap: 2px;
}
.pt__check-status {
  flex: none;
  font-size: var(--fs-sm);
  font-weight: 600;
}
.pt__check-label {
  font-size: var(--fs-sm);
}
.pt__tiles {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 11rem), 1fr));
  gap: var(--space-3);
}
/* «Чем заканчивается поиск»: исход — сколько раз — сколько дошли до товара */
.pt__flow {
  container-type: inline-size;
  overflow: hidden;
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
}
.pt__flow-head {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding: var(--space-4);
}
.pt__flow-head p {
  margin: 0;
}
.pt__flow-title {
  margin: 0;
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
  font-weight: var(--fw-bold);
}
.pt__flow-cols,
.pt__outcome {
  display: grid;
  grid-template-columns: minmax(0, 1.4fr) 7rem minmax(0, 1.6fr);
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-4);
}
.pt__flow-cols {
  margin: 0;
  border-top: var(--border-w) solid var(--border);
  background: var(--surface-2);
  color: var(--text-2);
  font-weight: var(--fw-semibold);
}
.pt__outcomes {
  margin: 0;
  padding: 0;
  list-style: none;
}
.pt__outcome {
  min-height: 3.5rem;
  border-top: var(--border-w) solid var(--border);
}
.pt__outcome-name {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  min-width: 0;
  font-weight: var(--fw-semibold);
}
.pt__outcome-name::first-letter {
  text-transform: uppercase;
}
.pt__outcome-icon {
  display: grid;
  flex: none;
  place-items: center;
  width: 2rem;
  height: 2rem;
  border-radius: var(--radius-pill);
  background: var(--accent-soft);
  color: var(--accent);
}
.pt__outcome-count {
  display: flex;
  flex-direction: column;
}
.pt__outcome-count strong {
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
  font-weight: var(--fw-bold);
}
.pt__outcome-done {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 0;
}
.pt__bar {
  display: block;
  height: 6px;
  overflow: hidden;
  border-radius: var(--radius-pill);
  background: var(--surface-3);
}
.pt__bar span {
  display: block;
  height: 100%;
  border-radius: var(--radius-pill);
  background: var(--accent);
}
@container (max-width: 32rem) {
  .pt__flow-cols {
    display: none;
  }
  .pt__outcome {
    grid-template-columns: minmax(0, 1fr) auto;
    padding-block: var(--space-3);
  }
  .pt__outcome-count {
    align-items: flex-end;
  }
  .pt__outcome-done {
    grid-column: 1 / -1;
  }
}
.pt__mock {
  padding: var(--space-3) var(--space-4);
  color: var(--text-2);
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
