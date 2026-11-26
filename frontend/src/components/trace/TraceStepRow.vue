<script setup lang="ts">
// Шаг трассы в таймлайне: значок, название, итог одной строкой, время и его полоска. Пропущенный — серым с подписью «пропущен», не скрыт.
// Если есть подробности — шаг раскрывается (reka Collapsible): замены, условия, кандидаты, веса.
import { computed, ref } from 'vue'
import { CollapsibleContent, CollapsibleRoot, CollapsibleTrigger } from 'reka-ui'
import {
  ArrowUpCircle,
  Boxes,
  Briefcase,
  ChevronDown,
  ChevronUp,
  CircleAlert,
  Combine,
  Cpu,
  Dot,
  LayoutTemplate,
  ListFilter,
  ListOrdered,
  Percent,
  PlusCircle,
  Route,
  Search,
  ShoppingCart,
  Sparkles,
  Tags,
  Type,
  Unlock,
} from 'lucide-vue-next'
import type { Component } from 'vue'
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

const props = defineProps<{
  step: TraceStep
  titles: Record<string, string>
  /** Самый долгий шаг трассы — для полоски времени. */
  maxMs?: number
}>()

/** Значок шага по имени сервера; незнакомый шаг — точка. */
const STEP_ICONS: Record<string, Component> = {
  normalize: Type,
  attributes: Tags,
  route: Route,
  intent: Sparkles,
  bm25: Search,
  dense: Boxes,
  fusion: Combine,
  rerank: ListOrdered,
  business: Briefcase,
  relaxation: Unlock,
  filters: ListFilter,
  margin: Percent,
  hardware: Cpu,
  companions: PlusCircle,
  upgrade: ArrowUpCircle,
  solution: LayoutTemplate,
  procurement: ShoppingCart,
}

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
      <span class="step__dot" aria-hidden="true">
        <component :is="STEP_ICONS[step.name] ?? Dot" :size="14" />
      </span>
      <span class="step__name">
        <CircleAlert v-if="step.error" :size="14" aria-hidden="true" class="step__error-icon" />
        {{ stepLabel(step.name) }}
      </span>
      <span class="step__summary">{{ stepSummary(step) }}</span>
      <span class="step__ms t-num">{{ step.skipped ? '—' : formatMs(step.ms) }}</span>
      <span class="step__chevron" aria-hidden="true">
        <template v-if="expandable">
          <ChevronUp v-if="open" :size="16" />
          <ChevronDown v-else :size="16" />
        </template>
      </span>
      <!-- Доля самого долгого шага: где ушло время, видно без чтения чисел. -->
      <span v-if="!step.skipped && maxMs" class="step__time" aria-hidden="true">
        <span :style="{ width: `${Math.max(2, (step.ms / maxMs) * 100)}%` }" />
      </span>
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
  position: relative;
}
/* Линия таймлайна между значками */
.step:not(:last-child)::before {
  content: '';
  position: absolute;
  top: 2.5rem;
  bottom: calc(-1 * var(--space-2));
  left: calc(var(--space-3) + 0.875rem - 1px);
  width: 2px;
  background: var(--border);
}
.step__head {
  display: grid;
  grid-template-columns: 1.75rem minmax(0, 1fr) auto var(--icon-sm);
  grid-template-areas:
    'dot name ms chev'
    'dot summary summary summary'
    'dot time time time';
  align-items: center;
  gap: 2px var(--space-3);
  box-sizing: border-box;
  width: 100%;
  padding: var(--space-2) var(--space-3);
  border: 0;
  border-radius: var(--radius-control);
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
.step__dot {
  position: relative;
  z-index: 1;
  grid-area: dot;
  display: grid;
  align-self: start;
  place-items: center;
  width: 1.75rem;
  height: 1.75rem;
  border-radius: var(--radius-pill);
  background: var(--accent-soft);
  color: var(--accent);
}
.step__chevron {
  grid-area: chev;
  display: inline-flex;
  color: var(--text-3);
}
.step__name {
  grid-area: name;
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  font-weight: var(--fw-bold);
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
  padding: 1px var(--space-2);
  border-radius: var(--radius-pill);
  background: var(--surface-2);
  color: var(--text-2);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
  font-weight: var(--fw-semibold);
  white-space: nowrap;
}
.step__time {
  grid-area: time;
  display: block;
  height: 3px;
  margin-top: var(--space-1);
  overflow: hidden;
  border-radius: var(--radius-pill);
  background: var(--surface-3);
}
.step__time span {
  display: block;
  height: 100%;
  border-radius: var(--radius-pill);
  background: var(--accent);
}
/* Пропущенный шаг виден, но не спорит с выполненными: пунктирный значок и приглушённый текст. */
.step--skipped .step__dot {
  border: var(--border-w) dashed var(--border-control);
  box-sizing: border-box;
  background: var(--surface);
  color: var(--text-3);
}
.step--skipped .step__name,
.step--skipped .step__summary,
.step--skipped .step__ms {
  color: var(--text-3);
  font-weight: var(--fw-regular);
}
.step--error .step__dot {
  background: var(--danger-bg);
}
.step__error-icon,
.step__error {
  color: var(--danger);
}
.step__body {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0 var(--space-3) var(--space-2) calc(var(--space-3) + 1.75rem + var(--space-3));
  padding: var(--space-3);
  border-radius: var(--radius-control);
  background: var(--surface-2);
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
