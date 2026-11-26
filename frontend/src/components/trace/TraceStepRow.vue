<script setup lang="ts">
// Шаг трассы: название, итог одной строкой, время. Пропущенный — серым с подписью «пропущен», не скрыт.
// Если есть подробности — шаг раскрывается (reka Collapsible): замены, условия, кандидаты, веса.
import { computed, ref } from 'vue'
import { CollapsibleContent, CollapsibleRoot, CollapsibleTrigger } from 'reka-ui'
import { ChevronDown, ChevronRight, CircleAlert } from 'lucide-vue-next'
import type { TraceStep } from '@/api/types'
import { formatDecimal, formatMs } from '@/lib/format'
import {
  WEIGHT_LABELS,
  changeKindLabel,
  rankLabel,
  stepExtraFields,
  stepHasDetails,
  stepLabel,
  stepSummary,
} from '@/lib/trace'
import { marginGuardLines, marginPolicyLabel, marginShiftLines } from '@/lib/traceMargin'
import TraceStepP2 from './TraceStepP2.vue'

const props = defineProps<{ step: TraceStep; titles: Record<string, string> }>()

const TOP_LIMIT = 10
const open = ref(false)
const expandable = computed(() => stepHasDetails(props.step))
const top = computed(() => (props.step.top ?? []).slice(0, TOP_LIMIT))
const topRest = computed(() => Math.max(0, (props.step.top?.length ?? 0) - TOP_LIMIT))

function show(v: unknown): string {
  if (typeof v === 'string') return v
  if (typeof v === 'number') return Number.isInteger(v) ? String(v) : formatDecimal(v, 3)
  if (v && typeof v === 'object' && 'message' in v && typeof v.message === 'string')
    return v.message
  return JSON.stringify(v)
}
</script>

<template>
  <CollapsibleRoot
    v-model:open="open"
    :disabled="!expandable"
    as="li"
    :class="['step', { 'step--skipped': step.skipped, 'step--error': step.error }]"
  >
    <CollapsibleTrigger class="step__head" :as="expandable ? 'button' : 'div'">
      <span class="step__chevron" aria-hidden="true">
        <template v-if="expandable">
          <ChevronDown v-if="open" :size="14" />
          <ChevronRight v-else :size="14" />
        </template>
      </span>
      <span class="step__name">
        <CircleAlert v-if="step.error" :size="14" aria-hidden="true" class="step__error-icon" />
        {{ stepLabel(step.name) }}
      </span>
      <span class="step__summary">{{ stepSummary(step) }}</span>
      <span class="step__ms t-num">{{ step.skipped ? '—' : formatMs(step.ms) }}</span>
    </CollapsibleTrigger>

    <CollapsibleContent class="step__body">
      <p v-if="step.error" class="step__error">Ошибка: {{ step.error }}</p>

      <ul v-if="step.changes?.length" class="step__list">
        <li v-for="(c, i) in step.changes" :key="i">
          <span class="t-mono">{{ c.from }}</span> →
          <span class="t-mono">{{ c.to }}</span>
          <span class="t-muted"> · {{ changeKindLabel(c.kind) }}</span>
        </li>
      </ul>

      <ul v-if="step.found?.length" class="step__list">
        <li v-for="(f, i) in step.found" :key="i">
          <span class="t-mono">{{ f.field }} = {{ f.value }}</span>
          <span class="t-muted">
            · {{ f.strict ? 'строго' : 'по возможности' }} · {{ f.source }}</span
          >
        </li>
      </ul>
      <p v-if="step.unresolved?.length" class="t-sm">
        Не разрешено: <span class="t-mono">{{ step.unresolved.map(show).join(', ') }}</span>
      </p>

      <p v-if="step.signals?.length" class="t-sm">
        Сигналы: <span class="t-mono">{{ step.signals.join(', ') }}</span>
      </p>

      <ol v-if="top.length" class="step__top">
        <li v-for="c in top" :key="c.product_id">
          <span class="t-num t-muted">{{ rankLabel(c.rank) }}</span>
          <span class="step__top-title">{{ titles[c.product_id] ?? c.product_id }}</span>
          <span class="t-num t-muted">{{ formatDecimal(c.score, 3) }}</span>
        </li>
      </ol>
      <p v-if="topRest" class="t-xs t-muted">и ещё {{ topRest }} в JSON трассы</p>

      <ul v-if="step.weights && Object.keys(step.weights).length" class="step__list">
        <li v-for="(w, k) in step.weights" :key="k">
          {{ WEIGHT_LABELS[k] ?? k }}: <span class="t-num">{{ show(w) }}</span>
        </li>
      </ul>

      <ol v-if="step.ladder?.length" class="step__list">
        <li v-for="(l, i) in step.ladder" :key="i">{{ show(l) }}</li>
      </ol>

      <template v-if="step.name === 'margin'">
        <p v-if="marginPolicyLabel(step)" class="t-xs t-muted">{{ marginPolicyLabel(step) }}</p>
        <ul
          v-if="marginShiftLines(step, titles).length"
          class="step__list"
          aria-label="Сдвиги маржи"
        >
          <li v-for="(m, i) in marginShiftLines(step, titles)" :key="i">
            <span class="step__top-title">{{ m.title }}</span>
            <span class="t-num t-muted"> · {{ m.text }}</span>
          </li>
        </ul>
        <ul v-if="marginGuardLines(step).length" class="step__list" aria-label="Сработавшие защиты">
          <li v-for="(g, i) in marginGuardLines(step)" :key="i">{{ g }}</li>
        </ul>
      </template>

      <TraceStepP2
        v-if="step.name === 'hardware' || step.name === 'companions'"
        :step="step"
        :titles="titles"
      />

      <dl v-if="stepExtraFields(step).length" class="step__extra">
        <template v-for="[k, v] in stepExtraFields(step)" :key="k">
          <dt class="t-mono">{{ k }}</dt>
          <dd class="t-mono">{{ show(v) }}</dd>
        </template>
      </dl>
    </CollapsibleContent>
  </CollapsibleRoot>
</template>

<style scoped>
.step {
  border-bottom: var(--border-w) solid var(--border);
}
.step__head {
  display: grid;
  grid-template-columns: var(--icon-sm) minmax(0, 1fr) auto;
  grid-template-areas:
    'chev name ms'
    '. summary summary';
  align-items: baseline;
  gap: 0 var(--space-2);
  width: 100%;
  padding: var(--space-2) var(--space-1);
  border: 0;
  background: none;
  color: var(--text);
  font: inherit;
  text-align: left;
}
button.step__head {
  cursor: pointer;
}
button.step__head:hover {
  background: var(--surface-2);
}
.step__chevron {
  grid-area: chev;
  color: var(--text-2);
  align-self: center;
  display: inline-flex;
}
.step__name {
  grid-area: name;
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  font-weight: var(--fw-medium);
}
.step__summary {
  grid-area: summary;
  color: var(--text-2);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
  overflow-wrap: anywhere;
}
.step__ms {
  grid-area: ms;
  color: var(--text-2);
  font-size: var(--fs-sm);
}
.step--skipped .step__name,
.step--skipped .step__summary,
.step--skipped .step__ms {
  color: var(--text-3);
  font-weight: var(--fw-regular);
}
.step__error-icon,
.step__error {
  color: var(--danger);
}
.step__body {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: 0 var(--space-1) var(--space-3) calc(var(--icon-sm) + var(--space-3));
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.step__list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  padding-left: var(--space-4);
}
.step__top {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  list-style: none;
}
.step__top li {
  display: grid;
  grid-template-columns: 3rem minmax(0, 1fr) auto;
  gap: var(--space-2);
  padding: 2px 0;
}
.step__top-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.step__extra {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: 2px var(--space-3);
  margin: 0;
}
.step__extra dt {
  color: var(--text-2);
}
.step__extra dd {
  margin: 0;
  overflow-wrap: anywhere;
}
</style>
