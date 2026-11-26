<script setup lang="ts">
// Руководитель e-com: верхний ряд — спрос, доля проблемных, доля спасённых, конверсия;
// блок эффекта честно ждёт прогона B4 (действующие правила с правками агента); стоимость — в «Экономике».
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import { Coins, GitCompareArrows } from 'lucide-vue-next'
import { getEvaluations } from '@/api/evaluations'
import type { AnalyticsTiles } from '@/api/types'
import type { Explanation } from '@/components/common/ExplainDialog.vue'
import StatTile from '@/components/common/StatTile.vue'
import { useResource } from '@/composables/useResource'
import { formatDateTime, formatInt, formatPercent } from '@/lib/format'

const props = defineProps<{ tiles: AnalyticsTiles; periodLabel: string }>()
const emit = defineEmits<{ explain: [e: Explanation] }>()

const evals = useResource((opts) => getEvaluations(opts))
void evals.load()

const b4 = computed(() => evals.data.value?.configs?.find((c) => c.config === 'B4') ?? null)
const rescued = computed(() => b4.value?.metrics?.rescued_rate ?? null)
const problems = computed(() => props.tiles.empty + props.tiles.no_click)
const problemShare = computed(() =>
  props.tiles.searches > 0 ? problems.value / props.tiles.searches : null,
)
const share = (v: number | null | undefined) =>
  typeof v === 'number' && Number.isFinite(v) ? formatPercent(v) : null

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
          what: 'Поиски, после которых покупатель ничего не увидел или ничего не открыл.',
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
      label: 'Спасённые запросы',
      value: share(rescued.value),
      sub: b4.value ? `прогон B4 · ${formatDateTime(b4.value.created_at)}` : undefined,
      missing: 'появится после прогона B4',
      explain: () =>
        emit('explain', {
          title: 'Доля спасённых запросов',
          value: share(rescued.value) ?? '—',
          what: 'Запросы, которые на исходных правилах (B0) не находили нужного, а на действующих (B4) находят.',
          how: 'Считает прогон оценки на размеченном наборе; подробности — в «До / После».',
          source: b4.value?.repro,
        }),
    },
    {
      key: 'conv',
      label: 'Поиск → заказ',
      value: share(t.order_rate),
      sub: `${formatInt(t.orders)} поисков с заказом`,
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
        <h2 id="effect-title" class="t-h2">Потенциальный эффект</h2>
        <template v-if="b4 && rescued !== null">
          <p>
            На действующих правилах (B4) находится
            <strong class="t-num">{{ share(rescued) }}</strong> запросов, которые раньше терялись.
          </p>
          <p class="t-sm t-muted">
            Сценарии в заказах и рублях строятся от этой доли и появятся, когда будут посчитаны на
            данных заказчика.
          </p>
        </template>
        <p v-else class="t-muted">
          Эффект появится после прогона B4 — действующих правил с принятыми правками агента. Без
          прогона оценки чисел здесь нет.
        </p>
        <RouterLink to="/compare" class="link t-sm head__link">
          <GitCompareArrows :size="14" aria-hidden="true" /> Сравнение «До / После»
        </RouterLink>
        <p class="t-xs t-muted">
          Сценарная оценка на тестовых данных. Фактический эффект измеряется A/B-тестом в пилоте.
        </p>
      </section>

      <section class="panel head__block" aria-labelledby="cost-title">
        <h2 id="cost-title" class="t-h2">Стоимость эксплуатации</h2>
        <p class="t-muted">
          Стоимость запроса по маршрутам, кэш и прогноз на месяц — по фактическим вызовам модели.
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
}
.head__block {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--space-2);
  padding: var(--space-4);
}
.head__link {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
}
@media (max-width: 1023px) {
  .head__tiles {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .head__row {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
