<script setup lang="ts">
// Несколько вендоров (сценарий 4): полная стоимость одной задачи — как продаётся и сколько стоит всё, цена пакета на
// места не делится. Победитель не назначается; строка выбирается покупателем, после выбора появляется главная кнопка.
import type { GuideTaskCost } from '@/api/typesGuide'

defineProps<{ cost: GuideTaskCost; selectedId: string; busy?: boolean }>()
const emit = defineEmits<{ select: [offerId: string] }>()
</script>

<template>
  <section class="gt" aria-labelledby="guide-cost-title" data-testid="guide-task-cost">
    <h3 id="guide-cost-title" class="gt__title">{{ cost.title }}</h3>
    <div class="gt__cols" aria-hidden="true">
      <span />
      <span>{{ cost.columns[0] }}</span>
      <span>{{ cost.columns[1] }}</span>
    </div>
    <fieldset class="gt__list" :disabled="busy">
      <legend class="sr-only">{{ cost.title }}</legend>
      <label
        v-for="r in cost.rows"
        :key="r.offer_id"
        :class="['gt__row', { 'is-picked': r.offer_id === selectedId }]"
        :data-offer="r.offer_id"
        data-testid="guide-cost-row"
      >
        <input
          type="radio"
          name="guide-cost"
          class="gt__radio"
          :value="r.offer_id"
          :checked="r.offer_id === selectedId"
          @change="emit('select', r.offer_id)"
        />
        <span class="gt__name">{{ r.title }}</span>
        <span class="gt__how">
          <span class="sr-only">{{ cost.columns[0] }}: </span>{{ r.how_sold }}
        </span>
        <span class="gt__total t-num">
          <span class="sr-only">{{ cost.columns[1] }}: </span>{{ r.total_text }}
        </span>
        <span v-if="r.note" class="gt__note">{{ r.note }}</span>
      </label>
    </fieldset>
    <p v-if="cost.note" class="gt__meta">{{ cost.note }}</p>
    <p v-if="cost.cheapest" class="gt__cheapest" data-testid="guide-cheapest">
      {{ cost.cheapest }}
    </p>
  </section>
</template>

<style scoped>
.gt {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.gt__title {
  margin: 0;
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
  font-weight: var(--fw-semibold);
}
.gt__cols,
.gt__row {
  display: grid;
  grid-template-columns: 1.5rem minmax(0, 1fr) minmax(5.5rem, auto) minmax(6.5rem, auto);
  gap: var(--space-1) var(--space-3);
  align-items: baseline;
}
.gt__cols {
  color: var(--text-3);
  font-size: var(--fs-xs);
}
.gt__cols span:nth-child(2) {
  grid-column: 3;
}
.gt__cols span:nth-child(3) {
  grid-column: 4;
  text-align: right;
}
.gt__list {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  border: 0;
}
.gt__row {
  padding: var(--space-3) var(--space-2) var(--space-3) 0;
  border-top: var(--border-w) solid var(--border);
  cursor: pointer;
}
.gt__row.is-picked {
  background: var(--surface-2);
}
.gt__radio {
  width: 1rem;
  height: 1rem;
  margin: 0 0 0 var(--space-1);
  accent-color: var(--accent);
}
.gt__name {
  min-width: 0;
  overflow-wrap: anywhere;
  font-weight: var(--fw-medium);
}
.gt__how {
  color: var(--text-2);
  font-size: var(--fs-sm);
}
.gt__total {
  font-weight: var(--fw-semibold);
  text-align: right;
  white-space: nowrap;
}
.gt__note {
  grid-column: 2 / -1;
  color: var(--text-3);
  font-size: var(--fs-sm);
}
.gt__meta {
  margin: 0;
  color: var(--text-2);
  font-size: var(--fs-sm);
}
.gt__cheapest {
  margin: 0;
  font-weight: var(--fw-medium);
}
@container (max-width: 26rem) {
  .gt__cols {
    display: none;
  }
  .gt__row {
    grid-template-columns: 1.5rem minmax(0, 1fr) auto;
  }
  .gt__how {
    grid-column: 2;
    grid-row: 2;
  }
  .gt__total {
    grid-column: 3;
    grid-row: 1;
  }
}
</style>
