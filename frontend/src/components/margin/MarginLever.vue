<script setup lang="ts">
// Рычаг «Приоритет маржи»: дискретные положения, упор, действующее положение отмечено.
// Положения за упором недоступны, подпись под рычагом объясняет почему. Клавиатура — стрелки (reka ToggleGroup).
import { computed } from 'vue'
import { ToggleGroupItem, ToggleGroupRoot } from 'reka-ui'
import { Lock } from 'lucide-vue-next'
import { stopReason, weightLabel } from '@/lib/margin'

const props = defineProps<{
  positions: number[]
  /** Веса за упором (недоступны). */
  locked: number[]
  /** Вес действующей политики. */
  active: number
  epsilon?: number
  disabled?: boolean
}>()
const model = defineModel<number>({ required: true })

const value = computed({
  get: () => String(model.value),
  set: (v: string | string[] | undefined) => {
    const n = Number(Array.isArray(v) ? v[0] : v)
    if (typeof v === 'string' && Number.isFinite(n)) model.value = n
  },
})
const isLocked = (w: number) => props.locked.some((x) => Math.abs(x - w) < 1e-9)
const hasStop = computed(() => props.positions.some(isLocked))
/** Выбранное положение упирается в недоступное соседнее. */
const atStop = computed(() => {
  const i = props.positions.findIndex((p) => Math.abs(p - model.value) < 1e-9)
  if (i < 0) return false
  const next = props.positions[i + 1]
  const prev = props.positions[i - 1]
  return (next !== undefined && isLocked(next)) || (prev !== undefined && isLocked(prev))
})
</script>

<template>
  <div class="lever">
    <p id="lever-label" class="t-sm lever__label">Приоритет маржи</p>
    <ToggleGroupRoot
      v-model="value"
      type="single"
      :disabled="disabled"
      aria-labelledby="lever-label"
      class="lever__track"
    >
      <ToggleGroupItem
        v-for="p in positions"
        :key="p"
        :value="String(p)"
        :disabled="isLocked(p)"
        :class="[
          'lever__pos',
          { 'lever__pos--locked': isLocked(p), 'lever__pos--active': p === active },
        ]"
        :aria-label="`Вес ${weightLabel(p)}${p === active ? ', действует' : ''}${isLocked(p) ? ', за упором' : ''}`"
      >
        <span class="t-num">{{ weightLabel(p) }}</span>
        <Lock v-if="isLocked(p)" :size="12" aria-hidden="true" />
        <span v-if="p === active" class="lever__now t-xs"
          ><span class="lever__now-text">действует</span></span
        >
      </ToggleGroupItem>
    </ToggleGroupRoot>
    <!-- Узкий экран: в кнопке не помещается слово — точка в кнопке и строка-расшифровка. -->
    <p class="lever__now-line t-xs t-muted" aria-hidden="true">
      <span class="lever__dot" /> действует вес {{ weightLabel(active) }}
    </p>
    <div class="lever__scale t-xs t-muted" aria-hidden="true">
      <span>только соответствие</span><span>больше маржи</span>
    </div>
    <p v-if="hasStop" class="t-xs lever__stop" :class="{ 'lever__stop--hit': atStop }">
      <template v-if="atStop">Упор. </template>{{ stopReason(epsilon) }}
    </p>
  </div>
</template>

<style scoped>
.lever {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.lever__label {
  font-weight: var(--fw-medium);
}
.lever__track {
  display: grid;
  grid-auto-columns: minmax(0, 1fr);
  grid-auto-flow: column;
  gap: var(--space-1);
  padding: var(--space-1);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
  background: var(--surface-2);
}
.lever__pos {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 2px;
  min-height: var(--touch-min);
  padding: var(--space-1) 0;
  border: var(--border-w) solid transparent;
  border-radius: var(--radius-sm);
  background: none;
  color: var(--text-2);
  font: inherit;
  cursor: pointer;
}
.lever__pos:hover:not(:disabled) {
  color: var(--text);
  background: var(--surface-3);
}
.lever__pos[data-state='on'] {
  background: var(--accent);
  color: var(--on-accent);
  font-weight: var(--fw-semibold);
}
.lever__pos--active:not([data-state='on']) {
  border-color: var(--border-control);
  color: var(--text);
}
.lever__pos--locked {
  color: var(--text-3);
  cursor: not-allowed;
  background: repeating-linear-gradient(135deg, transparent 0 4px, var(--surface-3) 4px 5px);
}
.lever__now {
  line-height: 1;
}
.lever__now-line {
  display: none;
  align-items: center;
  gap: var(--space-1);
}
.lever__dot {
  display: inline-block;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
}
@media (max-width: 479px) {
  .lever__now-text {
    display: none;
  }
  .lever__now {
    display: block;
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: currentColor;
  }
  .lever__now-line {
    display: flex;
  }
}
.lever__scale {
  display: flex;
  justify-content: space-between;
}
.lever__stop {
  color: var(--text-2);
}
.lever__stop--hit {
  color: var(--text);
}
</style>
