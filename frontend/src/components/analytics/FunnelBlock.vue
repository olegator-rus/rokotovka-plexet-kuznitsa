<script setup lang="ts">
// Воронка поиска: поиски → клики → корзины → заказы с долей перехода от предыдущего шага. Полоса —
// доля от числа поисков, нейтральным цветом (цвет здесь смысла не несёт). Число открывает объяснение.
import { computed } from 'vue'
import type { FunnelStepRaw } from '@/api/types'
import type { Explanation } from '@/components/common/ExplainDialog.vue'
import { formatInt, formatPercent } from '@/lib/format'
import { funnelSteps } from '@/lib/funnel'

const props = defineProps<{ funnel: FunnelStepRaw[] }>()
const emit = defineEmits<{ explain: [e: Explanation] }>()

const HOW: Record<string, string> = {
  searches:
    'Отправленные запросы (Enter или выбор подсказки); набор букв без отправки не считается.',
  clicks: 'Поиски, после которых покупатель открыл хотя бы одну карточку из выдачи.',
  carts: 'Поиски, после которых товар из выдачи добавили в корзину.',
  orders: 'Поиски, после которых товар из выдачи заказали.',
}

const steps = computed(() =>
  funnelSteps(props.funnel.map((f) => ({ key: f.step, label: f.label, count: f.count }))),
)

function explain(i: number) {
  const s = steps.value[i]
  const prev = i > 0 ? steps.value[i - 1] : null
  emit('explain', {
    title: `Воронка: ${s.label.toLowerCase()}`,
    value: formatInt(s.count),
    how: HOW[s.key] ?? 'Число поисков, дошедших до этого шага.',
    rows: [
      ...(prev && s.fromPrev !== null
        ? [{ label: `Переход от шага «${prev.label}»`, value: formatPercent(s.fromPrev) }]
        : []),
      { label: 'От числа поисков', value: formatPercent(s.fromFirst) },
    ],
  })
}
</script>

<template>
  <section class="panel funnel" aria-labelledby="funnel-title">
    <h2 id="funnel-title" class="t-h2">Воронка поиска</h2>
    <ol class="funnel__steps">
      <li v-for="(s, i) in steps" :key="s.key" class="funnel__step">
        <div class="funnel__label">
          <span>{{ s.label }}</span>
          <span v-if="s.fromPrev !== null" class="t-xs t-muted t-num">
            {{ formatPercent(s.fromPrev) }} от предыдущего
          </span>
        </div>
        <div class="funnel__bar" aria-hidden="true">
          <span class="funnel__fill" :style="{ width: `${Math.max(s.fromFirst * 100, 0.5)}%` }" />
        </div>
        <button
          type="button"
          class="num-btn funnel__count"
          :aria-label="`${s.label}: ${formatInt(s.count)}. Как посчитано`"
          @click="explain(i)"
        >
          {{ formatInt(s.count) }}
        </button>
      </li>
    </ol>
  </section>
</template>

<style scoped>
.funnel {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
}
.funnel__steps {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.funnel__step {
  display: grid;
  grid-template-columns: 11rem minmax(0, 1fr) 5rem;
  align-items: center;
  gap: var(--space-3);
}
.funnel__label {
  display: flex;
  flex-direction: column;
}
.funnel__bar {
  height: var(--score-bar-h);
  border-radius: var(--radius-sm);
  background: var(--surface-2);
  overflow: hidden;
}
.funnel__fill {
  display: block;
  height: 100%;
  background: var(--score-relevance);
}
.funnel__count {
  justify-self: end;
  font-weight: var(--fw-medium);
}
@media (max-width: 639px) {
  .funnel__step {
    grid-template-columns: minmax(0, 1fr) auto;
  }
  .funnel__bar {
    grid-column: 1 / -1;
    grid-row: 2;
  }
}
</style>
