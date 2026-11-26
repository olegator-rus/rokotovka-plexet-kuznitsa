<script setup lang="ts">
// Разложение скора товара: полоса из четырёх сегментов с подписями и числами —
// релевантность, популярность, маржа, контекст; под ней позиции в текстовом и векторном списках.
// Сегменты различаются ступенью нейтральной шкалы и подписью, не цветом.
import { computed } from 'vue'
import type { TraceItem } from '@/api/types'
import { formatScore } from '@/lib/format'
import { scoreSegments, scoreSum } from '@/lib/score'
import { rankLabel } from '@/lib/trace'

const props = defineProps<{ item: TraceItem; title: string }>()

const segments = computed(() => scoreSegments(props.item.parts))
const sum = computed(() => scoreSum(props.item.parts))
const groupLabel: Record<string, string> = {
  exact: 'точное совпадение',
  match: 'подходит',
  analog: 'аналог',
}
</script>

<template>
  <div class="score">
    <p class="score__head">
      <span class="score__title">{{ title }}</span>
      <span class="t-num">итог {{ formatScore(item.final) }}</span>
    </p>
    <p class="t-xs t-muted">
      {{ groupLabel[item.group] ?? item.group }} · сумма частей {{ formatScore(sum) }}
    </p>
    <div
      class="score__bar"
      role="img"
      :aria-label="segments.map((s) => `${s.label} ${formatScore(s.value)}`).join(', ')"
    >
      <span
        v-for="s in segments"
        v-show="s.percent > 0"
        :key="s.key"
        :class="['score__seg', `score__seg--${s.key}`]"
        :style="{ flexBasis: `${s.percent}%` }"
      />
      <span v-if="segments.every((s) => s.percent === 0)" class="score__empty" />
    </div>
    <ul class="score__legend">
      <li v-for="s in segments" :key="s.key">
        <span :class="['score__swatch', `score__seg--${s.key}`]" aria-hidden="true" />
        <span class="score__label">{{ s.label }}</span>
        <span class="t-num">{{ formatScore(s.value) }}</span>
      </li>
    </ul>
    <p class="t-sm">
      BM25: <span class="t-num">{{ rankLabel(item.bm25_rank) }}</span> · векторный:
      <span class="t-num">{{ rankLabel(item.dense_rank) }}</span>
    </p>
  </div>
</template>

<style scoped>
.score {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.score__head {
  display: flex;
  justify-content: space-between;
  gap: var(--space-3);
  font-weight: var(--fw-medium);
}
.score__title {
  min-width: 0;
  overflow-wrap: anywhere;
}
.score__bar {
  display: flex;
  gap: 2px;
  height: var(--score-bar-h);
  overflow: hidden;
  border-radius: var(--radius-sm);
  background: var(--surface-3);
}
.score__seg {
  display: block;
  min-width: 2px;
}
.score__seg--relevance {
  background: var(--score-relevance);
}
.score__seg--popularity {
  background: var(--score-popularity);
}
.score__seg--margin {
  background: var(--score-margin);
}
.score__seg--context {
  background: var(--score-context);
  box-shadow: inset 0 0 0 1px var(--border-control);
}
.score__empty {
  flex: 1;
}
.score__legend {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-1) var(--space-3);
  margin: 0;
  padding: 0;
  list-style: none;
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.score__legend li {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}
.score__label {
  flex: 1;
  color: var(--text-2);
}
.score__swatch {
  flex: none;
  width: var(--space-3);
  height: var(--space-3);
  border-radius: var(--radius-sm);
}
</style>
