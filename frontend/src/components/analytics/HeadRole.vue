<script setup lang="ts">
// Руководитель e-com: верхний ряд — спрос, доля проблемных, доля спасённых, конверсия;
// блок «Потенциальный эффект» — три сценария в заказах и рублях из GET /api/effect (в ответе аналитики роли
// head — с допущениями по умолчанию; изменённые допущения — новый запрос к /api/effect). Доля спасённых — одна
// на все экраны: реплей отложенной недели B0 → B3 с номерами прогонов. Рядом — стоимость эксплуатации в месяц.
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { Coins, GitCompareArrows, Info, RotateCcw, TriangleAlert } from 'lucide-vue-next'
import { getEffect } from '@/api/analytics'
import { getCosts } from '@/api/costs'
import type { AnalyticsTiles, EffectParam, EffectResponse } from '@/api/types'
import type { Explanation } from '@/components/common/ExplainDialog.vue'
import StatTile from '@/components/common/StatTile.vue'
import {
  inputToParam,
  isShare,
  orderScenarios,
  paramToInput,
  sourceLabel,
  upliftText,
} from '@/lib/effect'
import { formatInt, formatPercent, formatRub, plural } from '@/lib/format'

const props = defineProps<{
  tiles: AnalyticsTiles
  periodLabel: string
  days: number
  effect: EffectResponse | null
}>()
const emit = defineEmits<{ explain: [e: Explanation] }>()

// Вилка: сначала из ответа аналитики, после правки допущений — из /api/effect.
const current = shallowRef<EffectResponse | null>(props.effect)
watch(
  () => props.effect,
  (v) => {
    current.value = v
    resetInputs()
  },
)
const busy = ref(false)
const failed = ref(false)

const inputs = ref<Record<string, string>>({})
const overrides = ref<Record<string, number>>({})
function resetInputs() {
  const m: Record<string, string> = {}
  for (const p of current.value?.assumptions ?? []) m[p.key] = paramToInput(p, p.value)
  inputs.value = m
}
resetInputs()

const invalid = computed(() => {
  const bad = new Set<string>()
  for (const p of current.value?.assumptions ?? []) {
    const raw = inputs.value[p.key]
    if (raw !== undefined && inputToParam(p, raw) === null) bad.add(p.key)
  }
  return bad
})

let timer: ReturnType<typeof setTimeout> | null = null
let ctrl: AbortController | null = null
function onInput(p: EffectParam, raw: string) {
  inputs.value = { ...inputs.value, [p.key]: raw }
  const v = inputToParam(p, raw)
  if (v === null) return
  const next = { ...overrides.value }
  if (Math.abs(v - p.default) < 1e-9) delete next[p.key]
  else next[p.key] = v
  overrides.value = next
  if (timer) clearTimeout(timer)
  timer = setTimeout(() => void refetch(), 400)
}

async function refetch() {
  ctrl?.abort()
  const c = new AbortController()
  ctrl = c
  busy.value = true
  try {
    const res = await getEffect(props.days, overrides.value, { signal: c.signal })
    if (ctrl !== c) return
    current.value = res
    failed.value = false
  } catch (e) {
    if (e instanceof DOMException && e.name === 'AbortError') return
    failed.value = true
  } finally {
    if (ctrl === c) busy.value = false
  }
}

function reset() {
  overrides.value = {}
  if (props.effect) {
    current.value = props.effect
    failed.value = false
    resetInputs()
  } else void refetch()
}
const changed = computed(() => Object.keys(overrides.value).length > 0)
onBeforeUnmount(() => {
  if (timer) clearTimeout(timer)
  ctrl?.abort()
})

const rescued = computed(() => current.value?.measured.rescued ?? null)
const scenarios = computed(() => orderScenarios(current.value?.scenarios ?? []))
const base = computed(() => scenarios.value.find((s) => s.id === 'base') ?? null)
const param = (key: string) => current.value?.assumptions.find((p) => p.key === key) ?? null

const problems = computed(() => props.tiles.empty + props.tiles.no_click)
const problemShare = computed(() =>
  props.tiles.searches > 0 ? problems.value / props.tiles.searches : null,
)
const share = (v: number | null | undefined) =>
  typeof v === 'number' && Number.isFinite(v) ? formatPercent(v) : null

function explainRescued() {
  const r = rescued.value
  if (!r) return
  emit('explain', {
    title: 'Доля спасённых поисков',
    value: formatPercent(r.share),
    what: `Среди поисков отложенной недели, где исходный поиск (${r.baseline}) не дал подходящего товара в первых 10, — доля, где его дал наш поиск. Это то же число, что на вкладке «Эффект агента» в «До / После».`,
    how: `Реплей недели, которую агент не видел: каждый запрос прогнан через ${r.baseline} и ${r.config}, вес — число его поисков за неделю. 95 % интервал Уилсона — по числу различных запросов. Вклад агента (B3 → B4) засчитывается, только если все его правки сделаны без доступа к этой неделе.`,
    rows: [
      { label: `Поисков без подходящего у ${r.baseline}`, value: formatInt(r.failed) },
      { label: 'Из них спасено', value: formatInt(r.rescued) },
      {
        label: 'Различных запросов',
        value: `${formatInt(r.rescued_queries)} из ${formatInt(r.failed_queries)}`,
      },
      { label: '95 % интервал', value: `${formatPercent(r.ci_low)} – ${formatPercent(r.ci_high)}` },
      { label: `Вклад поиска, ${r.baseline} → B3`, value: formatPercent(r.search_share) },
      {
        label: 'Вклад агента, B3 → B4',
        value: `${formatPercent(r.agent_share)} · ${r.agent_counted ? 'засчитан' : 'не засчитан'}`,
      },
      { label: `Прогон ${r.baseline}`, value: r.run_baseline },
      { label: 'Прогон B3', value: r.run_config },
    ],
    source: `GET /api/effect · ${r.set}@${r.set_hash}`,
  })
}

function explainEffect() {
  const e = current.value
  if (!e) return
  const m = e.measured
  const conv = param('order_prob')
  const factor = param('rescue_factor')
  const rows = [
    { label: 'Поисков в месяц', value: formatInt(m.searches_month) },
    { label: 'Доля проблемных (пусто или без клика)', value: share(m.problem_share) ?? '—' },
    {
      label: 'Поправка на сессии',
      value: `${share(param('session_factor')?.value) ?? '—'} · ${sourceLabel(param('session_factor')?.source ?? '')}`,
    },
    { label: 'Доля спасённых (реплей недели)', value: share(m.rescued?.share) ?? '—' },
    {
      label: 'Конверсия успешного поиска в заказ',
      value: `${share(conv?.value) ?? '—'} · ${sourceLabel(conv?.source ?? '')}`,
    },
    {
      label: 'Понижающий коэффициент',
      value: `${share(factor?.value) ?? '—'} · ${sourceLabel(factor?.source ?? '')}`,
    },
    {
      label: 'Вероятность заказа спасённого (базовый)',
      value: share(base.value?.order_prob) ?? '—',
    },
    { label: 'Заказов из поиска сейчас, в месяц', value: formatInt(m.orders_month) },
    { label: 'Средний чек', value: formatRub(param('avg_check_rub')?.value ?? NaN) },
  ]
  emit('explain', {
    title: 'Потенциальный эффект: как посчитано',
    value: base.value ? `${formatInt(base.value.orders_month)} заказов в месяц (базовый)` : '—',
    what: 'Сколько дополнительных заказов в месяц дадут поиски, которые сейчас теряются, если наш поиск их спасает. Это сценарная оценка, а не замер: фактический эффект показывает A/B-тест в пилоте.',
    how: `${e.formula}. Ограничения: вероятность заказа спасённого поиска = конверсия успешного × понижающий коэффициент и никогда не выше конверсии успешного поиска. «Спасён» на реплее значит «подходящий товар в первых 10», а не покупка, и часть покупателей уже ушла, поэтому коэффициент меньше 1 (допущение, его можно изменить). Сценарии: консервативный — нижняя граница интервала доли спасённых и коэффициент × 0,5; оптимистичный — верхняя граница и × 1,5 (не больше 1). Если базовый сценарий даёт больше +50 % к текущим заказам, экран это помечает.`,
    rows,
    source: 'GET /api/effect',
  })
}

// Стоимость эксплуатации — из «Экономики» (сценарий 20 тыс. запросов в месяц).
const monthCost = ref<number | null>(null)
onMounted(async () => {
  try {
    const c = await getCosts(0)
    const s = c.monthly?.scenarios?.find((x) => x.requests === 20000)
    monthCost.value = s ? s.total_rub : null
  } catch {
    monthCost.value = null
  }
})

const items = computed(() => {
  const t = props.tiles
  return [
    {
      key: 'demand',
      label: 'Поисковый спрос',
      value: formatInt(t.searches),
      sub: `поисков за ${props.periodLabel}`,
      missing: undefined,
      explain: () =>
        emit('explain', {
          title: 'Объём поискового спроса',
          value: formatInt(t.searches),
          what: 'Сколько раз покупатели искали на сайте за период.',
          how: 'Число событий search за период: живой поиск и импорт исторических логов.',
        }),
    },
    {
      key: 'problems',
      label: 'Проблемные поиски',
      value: share(problemShare.value),
      sub: `${formatInt(problems.value)} пустых или без клика`,
      missing: 'поисков не было',
      explain: () =>
        emit('explain', {
          title: 'Доля проблемных поисков',
          value: share(problemShare.value) ?? '—',
          what: 'Поиски, после которых покупатель ничего не увидел или ничего не открыл. Это старый поиск магазина за период, по журналам.',
          how: '(пустые выдачи + непустые без клика) / все поиски за период. Обе части считает сервер, здесь только сложение.',
          rows: [
            { label: 'Пустые выдачи', value: formatInt(t.empty) },
            { label: 'Без клика', value: formatInt(t.no_click) },
            { label: 'Все поиски', value: formatInt(t.searches) },
          ],
        }),
    },
    {
      key: 'rescued',
      label: 'Спасённые поиски',
      value: share(rescued.value?.share),
      sub: rescued.value
        ? `реплей отложенной недели · ${rescued.value.baseline} → ${rescued.value.config}`
        : undefined,
      missing: 'нужен прогон отложенной недели B0, B3',
      explain: explainRescued,
    },
    {
      key: 'conv',
      label: 'Поиск → заказ',
      value: share(t.order_rate),
      sub: `${formatInt(t.orders)} ${plural(t.orders, ['поиск', 'поиска', 'поисков'])} с заказом`,
      missing: 'поисков не было',
      explain: () =>
        emit('explain', {
          title: 'Конверсия поиска в заказ',
          value: share(t.order_rate) ?? '—',
          what: 'Какая часть поисков закончилась заказом.',
          how: 'Поиски, после которых в той же сессии был заказ / все поиски за период.',
        }),
    },
  ]
})
</script>

<template>
  <div class="head">
    <div class="head__tiles">
      <StatTile
        v-for="t in items"
        :key="t.key"
        :label="t.label"
        :value="t.value"
        :sub="t.sub"
        :missing="t.missing"
        @explain="t.explain()"
      />
    </div>

    <div class="head__row">
      <section class="panel head__block" aria-labelledby="effect-title">
        <div class="head__title">
          <h2 id="effect-title" class="t-h2">Потенциальный эффект</h2>
          <button
            v-if="current && scenarios.length"
            type="button"
            class="btn btn--ghost btn--sm"
            @click="explainEffect"
          >
            <Info :size="14" aria-hidden="true" /> Как посчитано
          </button>
        </div>

        <p v-if="!current" class="t-muted">Вилка эффекта не загрузилась.</p>
        <template v-else-if="current.missing.length">
          <p class="t-muted">Сценариев нет: не хватает измерений.</p>
          <ul class="t-sm head__missing">
            <li v-for="m in current.missing" :key="m">{{ m }}</li>
          </ul>
        </template>
        <template v-else>
          <ul
            class="head__scenarios"
            aria-label="Сценарии эффекта в месяц"
            :aria-busy="busy"
            :class="{ 'head__scenarios--busy': busy }"
          >
            <li
              v-for="s in scenarios"
              :key="s.id"
              :class="['head__scenario', { 'head__scenario--base': s.id === 'base' }]"
            >
              <span class="t-xs t-muted">{{ s.label }}</span>
              <span class="head__orders t-num">
                +{{ formatInt(s.orders_month) }}
                <span class="t-sm">{{
                  plural(s.orders_month, ['заказ', 'заказа', 'заказов'])
                }}</span>
              </span>
              <span class="t-num">{{ formatRub(s.revenue_rub_month) }} в месяц</span>
              <span class="t-xs t-muted t-num">{{
                upliftText(s, current.measured.orders_month)
              }}</span>
              <span class="t-xs t-muted t-num">
                спасено {{ formatPercent(s.rescued_share) }} · заказ
                {{ formatPercent(s.order_prob) }}
              </span>
            </li>
          </ul>
          <p v-for="w in current.warnings" :key="w" class="t-sm head__warn">
            <TriangleAlert :size="14" aria-hidden="true" /> {{ w }}
          </p>
          <p v-if="failed" class="t-sm head__warn" role="alert">
            Пересчёт не удался — показаны прежние числа.
            <button type="button" class="link" @click="refetch()">Повторить</button>
          </p>

          <fieldset class="head__params">
            <legend class="t-xs t-muted">Допущения — можно изменить</legend>
            <label
              v-for="p in current.assumptions"
              :key="p.key"
              class="head__param"
              :title="p.hint"
            >
              <span class="t-xs">{{ p.label }}</span>
              <span class="head__field">
                <input
                  class="head__input"
                  inputmode="decimal"
                  :value="inputs[p.key] ?? ''"
                  :aria-invalid="invalid.has(p.key)"
                  :aria-label="`${p.label}, ${isShare(p) ? 'проценты' : 'рубли'}`"
                  @input="onInput(p, ($event.target as HTMLInputElement).value)"
                />
                <span class="t-xs t-muted">{{ isShare(p) ? '%' : '₽' }}</span>
                <span
                  :class="['badge', 'head__src', { 'head__src--assumed': p.source !== 'measured' }]"
                  >{{ sourceLabel(p.source) }}</span
                >
              </span>
              <span v-if="invalid.has(p.key)" class="t-xs head__err">
                {{ isShare(p) ? 'от 0 до 100 %' : 'больше 0 ₽' }}
              </span>
            </label>
          </fieldset>
          <button v-if="changed" type="button" class="btn btn--secondary btn--sm" @click="reset">
            <RotateCcw :size="14" aria-hidden="true" /> Вернуть замеры
          </button>
        </template>

        <p class="t-xs t-muted head__note">
          Сценарная оценка на тестовых данных. Фактический эффект измеряется A/B-тестом в пилоте.
        </p>
        <RouterLink to="/compare" class="link t-sm head__link">
          <GitCompareArrows :size="14" aria-hidden="true" /> Сравнение «До / После»
        </RouterLink>
      </section>

      <section class="panel head__block" aria-labelledby="cost-title">
        <h2 id="cost-title" class="t-h2">Стоимость эксплуатации</h2>
        <p v-if="monthCost !== null">
          <strong class="t-num">{{ formatRub(monthCost) }}</strong> в месяц при 20&nbsp;000 запросов
        </p>
        <p v-if="monthCost !== null && base" class="t-sm t-muted">
          Против {{ formatRub(base.revenue_rub_month) }} выручки в месяц в базовом сценарии.
        </p>
        <p class="t-sm t-muted">
          Сервер, вызовы модели по фактическим замерам и запуски агента — подробно в «Экономике».
        </p>
        <RouterLink to="/costs" class="link t-sm head__link">
          <Coins :size="14" aria-hidden="true" /> Открыть «Экономику»
        </RouterLink>
      </section>
    </div>
  </div>
</template>

<style scoped>
.head {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
.head__tiles {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--space-3);
}
.head__row {
  display: grid;
  grid-template-columns: minmax(0, 2fr) minmax(0, 1fr);
  gap: var(--space-3);
  align-items: start;
}
.head__block {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--space-2);
  padding: var(--space-4);
}
.head__title {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
  width: 100%;
}
.head__scenarios {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--space-2);
  width: 100%;
  margin: 0;
  padding: 0;
  list-style: none;
  transition: opacity 0.15s;
}
.head__scenarios--busy {
  opacity: 0.6;
}
.head__scenario {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
}
.head__scenario--base {
  border-color: var(--accent);
}
.head__scenario > span {
  white-space: normal;
  overflow-wrap: anywhere;
}
.head__orders {
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
  font-weight: var(--fw-semibold);
}
.head__params {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--space-2) var(--space-3);
  width: 100%;
  margin: 0;
  padding: 0;
  border: 0;
}
.head__params legend {
  padding: 0;
  margin-bottom: var(--space-1);
}
.head__param {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.head__field {
  display: flex;
  align-items: center;
  gap: var(--space-1);
}
.head__input {
  box-sizing: border-box;
  width: 6rem;
  min-width: 0;
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
.head__input[aria-invalid='true'] {
  border-color: var(--danger);
}
.head__src {
  white-space: nowrap;
}
.head__src--assumed {
  color: var(--status-pending);
}
.head__err {
  color: var(--danger);
}
.head__warn {
  color: var(--status-pending);
}
.head__warn {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
}
.head__missing {
  margin: 0;
  padding-left: var(--space-4);
}
.head__link {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
}
@media (max-width: 1279px) {
  .head__params {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
@media (max-width: 1023px) {
  .head__tiles {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .head__row {
    grid-template-columns: minmax(0, 1fr);
  }
}
@media (max-width: 639px) {
  .head__scenarios,
  .head__params {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
