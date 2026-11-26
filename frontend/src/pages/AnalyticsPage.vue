<script setup lang="ts">
// «Аналитика». Период — в адресе (?days=30), роль — из шапки. Общие для всех ролей
// плитки и воронка (у руководителя — свой верхний ряд); ниже блок роли: менеджер e-com — проблемные
// запросы, продукт-менеджер — спрос без предложения, контент-менеджер — карточки и термины. Каждое число открывает объяснение: из каких событий и как посчитано.
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { getAnalytics } from '@/api/analytics'
import type { AnalyticsResponse } from '@/api/types'
import BlockError from '@/components/common/BlockError.vue'
import ExplainDialog, { type Explanation } from '@/components/common/ExplainDialog.vue'
import StatTile from '@/components/common/StatTile.vue'
import FunnelBlock from '@/components/analytics/FunnelBlock.vue'
import ContentRole from '@/components/analytics/ContentRole.vue'
import HeadRole from '@/components/analytics/HeadRole.vue'
import ProblemQueries from '@/components/analytics/ProblemQueries.vue'
import ProductRole from '@/components/analytics/ProductRole.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import { useResource } from '@/composables/useResource'
import { formatInt, formatPercent, plural } from '@/lib/format'
import { ROLES } from '@/lib/roles'
import { useAppStore } from '@/stores/app'

const PERIODS = [
  { value: '7', label: '7 дней' },
  { value: '30', label: '30 дней' },
  { value: '90', label: '90 дней' },
]

const route = useRoute()
const router = useRouter()
const app = useAppStore()
const { role } = storeToRefs(app)

const days = computed<string>({
  get: () => {
    const d = typeof route.query.days === 'string' ? route.query.days : ''
    return PERIODS.some((p) => p.value === d) ? d : '30'
  },
  set: (v) => void router.replace({ query: { ...route.query, days: v } }),
})

const res = useResource((opts) => getAnalytics(Number(days.value), role.value, opts))
watch([days, role], () => void res.load(), { immediate: true })

const data = computed<AnalyticsResponse | null>(() => res.data.value)
const searches = computed(() => data.value?.tiles?.searches ?? 0)
const isEmpty = computed(() => res.status.value === 'ready' && searches.value === 0)
const roleLabel = computed(() => ROLES.find((r) => r.id === role.value)?.label ?? '')
const periodLabel = computed(() => PERIODS.find((p) => p.value === days.value)?.label ?? '')

const open = ref(false)
const explanation = ref<Explanation | null>(null)
function explain(e: Explanation) {
  explanation.value = { source: `GET /api/analytics?days=${days.value}`, ...e }
  open.value = true
}

const share = (v: number | null | undefined) =>
  typeof v === 'number' && Number.isFinite(v) ? formatPercent(v) : null
const searchesRow = () => ({
  label: `Поисков за ${periodLabel.value}`,
  value: formatInt(searches.value),
})

const tiles = computed(() => {
  const t = data.value?.tiles
  if (!t) return []
  const nonEmpty = t.searches - t.empty
  return [
    {
      key: 'empty',
      label: 'Пустые выдачи',
      value: share(t.empty_rate),
      sub: `${formatInt(t.empty)} из ${formatInt(t.searches)} поисков`,
      missing: 'поисков не было',
      explain: () =>
        explain({
          title: 'Доля пустых выдач',
          value: share(t.empty_rate) ?? '—',
          what: 'Поиски, на которые не нашлось ни одного товара.',
          how: 'Поиски с нулём результатов / все поиски за период. Буквы, набранные до Enter, и прогоны оценки поиском не считаются.',
          rows: [searchesRow(), { label: 'С пустой выдачей', value: formatInt(t.empty) }],
        }),
    },
    {
      key: 'noclick',
      label: 'Поиски без клика',
      value: share(t.no_click_rate),
      sub: `${formatInt(t.no_click)} из ${formatInt(nonEmpty)} непустых`,
      missing: 'непустых выдач не было',
      explain: () =>
        explain({
          title: 'Доля поисков без клика',
          value: share(t.no_click_rate) ?? '—',
          what: 'Покупатель увидел выдачу и ничего не открыл.',
          how: 'Непустые поиски, после которых не было клика по товару (по request_id, а без него — в той же сессии за 30 минут) / все непустые поиски.',
          rows: [
            { label: 'Непустых поисков', value: formatInt(nonEmpty) },
            { label: 'Из них без клика', value: formatInt(t.no_click) },
          ],
        }),
    },
    {
      key: 'conv',
      label: 'Поиск → заказ',
      value: share(t.order_rate),
      sub: `${formatInt(t.orders)} ${plural(t.orders, ['поиск', 'поиска', 'поисков'])} с заказом`,
      missing: 'поисков не было',
      explain: () =>
        explain({
          title: 'Конверсия поиска в заказ',
          value: share(t.order_rate) ?? '—',
          what: 'Какая часть поисков закончилась заказом.',
          how: 'Поиски, после которых в той же сессии был заказ / все поиски за период.',
          rows: [searchesRow(), { label: 'Поисков с заказом', value: formatInt(t.orders) }],
        }),
    },
    {
      key: 'pending',
      label: 'Предложения ждут решения',
      value: formatInt(t.pending_proposals),
      sub: 'очередь SearchOps',
      missing: undefined,
      explain: () =>
        explain({
          title: 'Предложения агента, ждущие решения',
          value: formatInt(t.pending_proposals),
          what: 'Правки, которые агент проверил прогоном и положил человеку на подтверждение.',
          how: 'Число предложений в статусе «ждёт решения» на сейчас, без учёта периода. Пока агент SearchOps не запускался, их 0.',
        }),
    },
  ]
})

const sourcesLine = computed(() => {
  const src = data.value?.sources ?? {}
  const parts: string[] = []
  if (src.live) parts.push(`живой поиск ${formatInt(src.live)}`)
  if (src.mock_log) parts.push(`исторические логи ${formatInt(src.mock_log)}`)
  return parts.join(' · ')
})
</script>

<template>
  <section class="page analytics" aria-labelledby="analytics-title">
    <header class="analytics__head">
      <div>
        <h1 id="analytics-title" class="t-h1">Аналитика</h1>
        <p class="t-sm t-muted">
          Роль: {{ roleLabel }} · по событиям поиска и поведения
          <template v-if="sourcesLine"> · поисков: {{ sourcesLine }}</template>
        </p>
      </div>
      <UiSelect v-model="days" :options="PERIODS" label="Период" prefix="Период" />
    </header>

    <!-- Загрузка -->
    <template v-if="res.status.value === 'loading' && !data">
      <div class="analytics__tiles" aria-busy="true">
        <div v-for="i in 4" :key="i" class="panel analytics__tile-skel">
          <span class="skeleton" style="width: 60%" />
          <span class="skeleton" style="width: 40%; height: 1.25rem" />
        </div>
      </div>
      <div class="panel analytics__table-skel" aria-hidden="true">
        <span v-for="i in 5" :key="i" class="skeleton" :style="{ width: `${92 - i * 6}%` }" />
      </div>
    </template>

    <BlockError
      v-else-if="res.status.value === 'error'"
      :error="res.error.value!"
      title="Показатели не загрузились"
      @retry="res.load()"
    />

    <!-- Пусто -->
    <div v-else-if="isEmpty" class="panel analytics__empty">
      <p class="t-h2">Событий за {{ periodLabel }} нет</p>
      <p class="t-muted">Поисков в этом периоде не было. Выберите другой период.</p>
      <div class="analytics__periods">
        <button
          v-for="p in PERIODS.filter((x) => x.value !== days)"
          :key="p.value"
          type="button"
          class="btn btn--secondary btn--sm"
          @click="days = p.value"
        >
          {{ p.label }}
        </button>
      </div>
    </div>

    <template v-else-if="data">
      <HeadRole
        v-if="role === 'head'"
        :tiles="data.tiles"
        :period-label="periodLabel"
        @explain="explain"
      />
      <template v-else>
        <div class="analytics__tiles">
          <StatTile
            v-for="t in tiles"
            :key="t.key"
            :label="t.label"
            :value="t.value"
            :sub="t.sub"
            :missing="t.missing"
            @explain="t.explain()"
          />
        </div>

        <FunnelBlock v-if="data.funnel?.length" :funnel="data.funnel" @explain="explain" />
      </template>

      <ProblemQueries v-if="role === 'ecom'" :problems="data.problems ?? []" @explain="explain" />
      <ProductRole v-else-if="role === 'product'" />
      <ContentRole v-else-if="role === 'content'" />
    </template>

    <ExplainDialog v-model:open="open" :explanation="explanation" />
  </section>
</template>

<style scoped>
.analytics {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
.analytics__head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-3);
}
.analytics__head > div {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}
.analytics__tiles {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--space-3);
}
.analytics__tile-skel {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-4);
}
.analytics__table-skel {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
}
.analytics__empty {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-5);
}
.analytics__periods {
  display: flex;
  gap: var(--space-2);
}
@media (max-width: 1023px) {
  .analytics__tiles {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
