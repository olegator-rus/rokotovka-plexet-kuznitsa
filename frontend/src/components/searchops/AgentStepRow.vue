<script setup lang="ts">
// Шаг ленты агента: иконка вида, вид и инструмент, итог одной строкой, время.
// Аргументы и полный ответ раскрываются (reka Collapsible). Отклонённая гипотеза зачёркнута, причина видна.
import { computed, ref, type Component } from 'vue'
import { CollapsibleContent, CollapsibleRoot, CollapsibleTrigger } from 'reka-ui'
import {
  BookMarked,
  ChevronDown,
  ChevronRight,
  CircleAlert,
  CircleCheck,
  CircleX,
  Inbox,
  Lightbulb,
  MessageSquareText,
  Stethoscope,
  Wrench,
} from 'lucide-vue-next'
import type { StepView } from '@/lib/agentFeed'
import { formatClock } from '@/lib/agentFeed'
import { formatMs } from '@/lib/format'

const props = defineProps<{ view: StepView }>()

const open = ref(false)
const s = computed(() => props.view.step)

const icon = computed<Component>(() => {
  const k = s.value.kind
  if (k === 'tool_call') return Wrench
  if (k === 'diagnosis') return Stethoscope
  if (k === 'hypothesis') return Lightbulb
  if (k === 'check') return props.view.passed ? CircleCheck : CircleX
  if (k === 'memory') return BookMarked
  if (k === 'proposal') return Inbox
  if (k === 'error') return CircleAlert
  return MessageSquareText
})

/** Цвет только у смысловых шагов: проверка прошла/нет, отклонённая гипотеза, ошибка, предложение. */
const tone = computed(() => {
  if (props.view.rejected) return 'status-rejected'
  if (props.view.passed === true) return 'status-accepted'
  if (props.view.passed === false) return 'status-rejected'
  if (s.value.kind === 'proposal') return 'status-pending'
  return ''
})

const head = computed(() => {
  const parts = [props.view.kind.label]
  if (s.value.hypothesis_no && (s.value.kind === 'hypothesis' || s.value.kind === 'check'))
    parts[0] += ` №${s.value.hypothesis_no}`
  if (props.view.toolLabel) parts.push(props.view.toolLabel)
  return parts.join(' · ')
})

const checkLabel = computed(() => {
  if (props.view.passed === null) return ''
  return props.view.passed ? 'прошла' : 'не прошла'
})

function pretty(v: unknown): string {
  return JSON.stringify(v, null, 2)
}
const hasArgs = computed(() => !!s.value.args && Object.keys(s.value.args).length > 0)
const hasResult = computed(() => !!s.value.result && Object.keys(s.value.result).length > 0)
</script>

<template>
  <CollapsibleRoot
    v-model:open="open"
    :disabled="!view.expandable"
    as="li"
    :class="[
      'astep',
      `astep--${view.kind.group}`,
      tone,
      {
        'astep--rejected': view.rejected,
        'astep--error': s.kind === 'error' || (s.kind === 'tool_call' && !s.ok),
      },
    ]"
    :data-seq="s.seq"
  >
    <CollapsibleTrigger class="astep__head" :as="view.expandable ? 'button' : 'div'">
      <span class="astep__icon" aria-hidden="true">
        <component :is="icon" :size="16" />
      </span>
      <span class="astep__kind">
        {{ head }}
        <span v-if="checkLabel" class="astep__check">· {{ checkLabel }}</span>
      </span>
      <span class="astep__meta t-num">
        <template v-if="s.ms != null">{{ formatMs(s.ms) }} · </template>{{ formatClock(s.ts) }}
        <template v-if="view.expandable">
          <ChevronDown v-if="open" :size="14" aria-hidden="true" />
          <ChevronRight v-else :size="14" aria-hidden="true" />
        </template>
      </span>
      <span class="astep__title">
        <s v-if="view.rejected && s.kind === 'hypothesis'">{{ view.title }}</s>
        <template v-else>{{ view.title }}</template>
      </span>
      <span v-if="view.showReason" class="astep__reason">
        Гипотеза отклонена: {{ view.rejected }}
      </span>
    </CollapsibleTrigger>

    <CollapsibleContent class="astep__body">
      <div v-if="hasArgs">
        <p class="t-xs t-muted">Аргументы</p>
        <pre class="astep__json">{{ pretty(s.args) }}</pre>
      </div>
      <div v-if="hasResult">
        <p class="t-xs t-muted">Ответ</p>
        <pre class="astep__json">{{ pretty(s.result) }}</pre>
      </div>
    </CollapsibleContent>
  </CollapsibleRoot>
</template>

<style scoped>
.astep {
  border-bottom: var(--border-w) solid var(--border);
}
/* Живой запуск: новый шаг проявляется и чуть сдвигается вверх; история открывается без анимации. */
:global(.run--live) .astep {
  animation: astep-in var(--dur-panel) var(--ease-standard);
}
@keyframes astep-in {
  from {
    opacity: 0;
    transform: translateY(4px);
  }
}
.astep__head {
  display: grid;
  grid-template-columns: var(--icon-md) minmax(0, 1fr) auto;
  grid-template-areas:
    'icon kind meta'
    '. title title'
    '. reason reason';
  align-items: baseline;
  gap: 2px var(--space-2);
  width: 100%;
  padding: var(--space-2) var(--space-2);
  border: 0;
  background: none;
  color: var(--text);
  font: inherit;
  text-align: left;
}
button.astep__head {
  cursor: pointer;
}
button.astep__head:hover {
  background: var(--surface-2);
}
.astep__icon {
  grid-area: icon;
  display: inline-flex;
  align-self: center;
  color: var(--text-2);
}
.astep__kind {
  grid-area: kind;
  color: var(--text-2);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
  font-weight: var(--fw-medium);
}
.astep__meta {
  grid-area: meta;
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  color: var(--text-3);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
}
.astep__title {
  grid-area: title;
  overflow-wrap: anywhere;
}
.astep__reason {
  grid-area: reason;
  color: var(--tone);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
/* Вывод агента читается крупнее инструментов: это ход расследования. */
.astep--reasoning .astep__title,
.astep--proposal .astep__title {
  font-weight: var(--fw-medium);
}
.astep--tool .astep__title {
  color: var(--text-2);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.astep--check .astep__icon,
.astep--check .astep__check,
.astep--proposal .astep__icon,
.astep--rejected .astep__icon {
  color: var(--tone);
}
.astep--rejected .astep__title s {
  color: var(--text-2);
  text-decoration-color: var(--tone);
  text-decoration-thickness: 2px;
}
.astep--memory {
  background: var(--surface-2);
}
.astep--memory .astep__icon,
.astep--memory .astep__kind {
  color: var(--text);
}
.astep--error .astep__icon {
  color: var(--danger);
}
.astep__body {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: 0 var(--space-2) var(--space-3) calc(var(--icon-md) + var(--space-4));
}
.astep__json {
  max-height: 20rem;
  margin: var(--space-1) 0 0;
  padding: var(--space-2);
  overflow: auto;
  border-radius: var(--radius-sm);
  background: var(--bg);
  color: var(--text-2);
  font-family: var(--font-mono);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
</style>
