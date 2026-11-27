<script setup lang="ts">
// Условия покупки видимыми полями (UI 6.5): количество со степпером, «Новая / Продление», срок, флажки функций.
// Подставленное сервером по умолчанию подписано. Любая правка сразу пересчитывает подбор; степпер ждёт паузы, чтобы
// пять нажатий подряд дали один ход. На телефоне — одна строка «50 ПК · Новая · 1 год» и «Изменить условия».
import { onBeforeUnmount, ref, watch } from 'vue'
import { Minus, Plus, X } from 'lucide-vue-next'
import type { GuideCondition } from '@/api/typesGuide'
import { originLabel } from '@/lib/guide'
import {
  clampCount,
  conditionsLine,
  countFields,
  featureConditions,
  featureLabel,
  otherConditions,
  termOptions,
} from '@/lib/guideView'

const props = defineProps<{
  conditions: GuideCondition[]
  compact?: boolean
}>()
const emit = defineEmits<{ edit: [key: string, value: string | null] }>()

const counts = ref<Record<string, number | null>>({})
watch(
  () => props.conditions,
  (list) => {
    const next: Record<string, number | null> = {}
    for (const f of countFields(list)) next[f.key] = f.value
    counts.value = next
  },
  { immediate: true },
)

const timers = new Map<string, ReturnType<typeof setTimeout>>()
function commitCount(key: string, delay = 0) {
  clearTimeout(timers.get(key))
  const run = () => {
    timers.delete(key)
    const v = clampCount(counts.value[key])
    const was = countFields(props.conditions).find((f) => f.key === key)?.value ?? null
    if (v && v !== was) emit('edit', key, String(v))
    else counts.value = { ...counts.value, [key]: was }
  }
  if (delay) timers.set(key, setTimeout(run, delay))
  else run()
}
function step(key: string, d: number) {
  const v = clampCount((counts.value[key] ?? 0) + d) ?? 1
  counts.value = { ...counts.value, [key]: v }
  commitCount(key, 700)
}
function onCountInput(key: string, e: Event) {
  const raw = (e.target as HTMLInputElement).value.replace(/[^\d]/g, '')
  counts.value = { ...counts.value, [key]: raw ? Number(raw) : null }
}
onBeforeUnmount(() => timers.forEach((t) => clearTimeout(t)))

const cond = (key: string) => props.conditions.find((c) => c.key === key) ?? null
const isDefault = (c: GuideCondition | null) => c?.origin === 'default'

function onOperation(v: 'new' | 'renewal') {
  if (cond('operation')?.value !== v) emit('edit', 'operation', v)
}
function onTerm(e: Event) {
  const v = (e.target as HTMLSelectElement).value
  if (v && v !== cond('term_months')?.value) emit('edit', 'term_months', v)
}

const editing = ref(false)
watch(
  () => props.compact,
  (c) => {
    if (!c) editing.value = false
  },
)
</script>

<template>
  <div class="gf" data-testid="guide-fields">
    <div v-if="compact && !editing" class="gf__line">
      <span class="gf__summary" data-testid="guide-fields-line">{{
        conditionsLine(conditions) || 'Условия не заданы'
      }}</span>
      <button type="button" class="gf__link" @click="editing = true">Изменить условия</button>
    </div>

    <template v-else>
      <div class="gf__row">
        <div
          v-for="f in countFields(conditions)"
          :key="f.key"
          class="gf__field"
          :data-cond="f.key"
          :data-origin="f.origin"
        >
          <label :for="`gf-${f.key}`" class="gf__label">{{ f.label }}</label>
          <div class="gf__stepper">
            <button
              type="button"
              class="gf__step"
              :aria-label="`Меньше: ${f.label}`"
              :disabled="(counts[f.key] ?? 0) <= 1"
              data-testid="guide-count-minus"
              @click="step(f.key, -1)"
            >
              <Minus :size="14" aria-hidden="true" />
            </button>
            <input
              :id="`gf-${f.key}`"
              class="gf__num t-num"
              inputmode="numeric"
              autocomplete="off"
              :value="counts[f.key] ?? ''"
              data-testid="guide-count"
              @input="onCountInput(f.key, $event)"
              @change="commitCount(f.key)"
              @keydown.enter.prevent="commitCount(f.key)"
            />
            <button
              type="button"
              class="gf__step"
              :aria-label="`Больше: ${f.label}`"
              data-testid="guide-count-plus"
              @click="step(f.key, 1)"
            >
              <Plus :size="14" aria-hidden="true" />
            </button>
          </div>
        </div>

        <div
          v-if="cond('operation')"
          class="gf__field"
          data-cond="operation"
          :data-origin="cond('operation')!.origin"
        >
          <span id="gf-operation" class="gf__label">
            Тип покупки
            <span v-if="isDefault(cond('operation'))" class="gf__default">по умолчанию</span>
          </span>
          <div class="gf__seg" role="group" aria-labelledby="gf-operation">
            <button
              type="button"
              :aria-pressed="cond('operation')!.value !== 'renewal'"
              data-testid="guide-op-new"
              @click="onOperation('new')"
            >
              Новая
            </button>
            <button
              type="button"
              :aria-pressed="cond('operation')!.value === 'renewal'"
              data-testid="guide-op-renewal"
              @click="onOperation('renewal')"
            >
              Продление
            </button>
          </div>
        </div>

        <div
          v-if="cond('term_months')"
          class="gf__field"
          data-cond="term_months"
          :data-origin="cond('term_months')!.origin"
        >
          <label for="gf-term" class="gf__label">
            Срок
            <span v-if="isDefault(cond('term_months'))" class="gf__default">по умолчанию</span>
          </label>
          <select
            id="gf-term"
            class="gf__select"
            :value="cond('term_months')!.value"
            data-testid="guide-term"
            @change="onTerm"
          >
            <option
              v-for="o in termOptions(cond('term_months')!.value)"
              :key="o.value"
              :value="o.value"
            >
              {{
                o.value === cond('term_months')!.value
                  ? cond('term_months')!.label || o.label
                  : o.label
              }}
            </option>
          </select>
        </div>
      </div>

      <ul
        v-if="featureConditions(conditions).length"
        class="gf__features"
        aria-label="Обязательные функции"
      >
        <li v-for="c in featureConditions(conditions)" :key="c.key">
          <label class="gf__check">
            <input
              type="checkbox"
              checked
              data-testid="guide-feature"
              @change="emit('edit', c.key, null)"
            />
            {{ featureLabel(c.label) }}
          </label>
        </li>
      </ul>

      <ul v-if="otherConditions(conditions).length" class="gf__others">
        <li v-for="c in otherConditions(conditions)" :key="c.key" :data-cond="c.key">
          <span>{{ c.label || c.value }}</span>
          <span v-if="originLabel(c.origin)" class="gf__default">{{ originLabel(c.origin) }}</span>
          <button
            type="button"
            class="gf__remove"
            :aria-label="`Снять условие: ${c.label || c.value}`"
            @click="emit('edit', c.key, null)"
          >
            <X :size="12" aria-hidden="true" />
          </button>
        </li>
      </ul>

      <button v-if="compact" type="button" class="gf__link" @click="editing = false">Готово</button>
    </template>
  </div>
</template>

<style scoped>
.gf {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.gf__line {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: var(--space-1) var(--space-3);
}
.gf__summary {
  font-weight: var(--fw-medium);
}
.gf__link {
  align-self: flex-start;
  padding: 0;
  border: 0;
  background: none;
  color: var(--accent);
  font: inherit;
  font-size: var(--fs-sm);
  text-decoration: underline;
  text-underline-offset: 3px;
  cursor: pointer;
}
.gf__row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  padding: var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
  background: var(--surface-2);
}
.gf__field {
  display: flex;
  flex: 1 1 8rem;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 0;
}
.gf__field[data-cond^='count:'] {
  flex: 0 0 9.5rem;
}
.gf__field[data-cond='operation'] {
  flex: 1 1 12rem;
}
.gf__label {
  display: flex;
  flex-wrap: nowrap;
  white-space: nowrap;
  align-items: baseline;
  gap: 0 var(--space-2);
  color: var(--text-2);
  font-size: var(--fs-sm);
}
.gf__default {
  color: var(--text-3);
  font-size: var(--fs-xs);
}
.gf__stepper {
  display: grid;
  grid-template-columns: 2.5rem minmax(2.5rem, 1fr) 2.5rem;
  height: 2.5rem;
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-control);
  background: var(--surface);
  overflow: hidden;
}
.gf__step {
  display: grid;
  place-items: center;
  border: 0;
  background: none;
  color: var(--text);
  cursor: pointer;
}
.gf__step:hover:not(:disabled) {
  background: var(--surface-2);
}
.gf__step:disabled {
  color: var(--text-3);
  cursor: default;
}
.gf__num {
  min-width: 0;
  border: 0;
  border-inline: var(--border-w) solid var(--border);
  background: none;
  color: var(--text);
  font: inherit;
  font-size: var(--fs-input);
  font-weight: var(--fw-semibold);
  text-align: center;
}
.gf__seg {
  display: grid;
  grid-template-columns: 1fr 1fr;
  height: 2.5rem;
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-control);
  background: var(--surface);
  overflow: hidden;
}
.gf__seg button {
  min-width: 0;
  padding: 0 var(--space-2);
  border: 0;
  background: none;
  color: var(--text-2);
  font: inherit;
  cursor: pointer;
}
.gf__seg button + button {
  border-left: var(--border-w) solid var(--border);
}
.gf__seg button[aria-pressed='true'] {
  background: var(--text);
  color: var(--surface);
  font-weight: var(--fw-medium);
}
.gf__select {
  height: 2.5rem;
  min-width: 0;
  padding: 0 var(--space-2);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-control);
  background: var(--surface);
  color: var(--text);
  font: inherit;
}
.gf__features,
.gf__others {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-4);
  margin: 0;
  padding: 0;
  list-style: none;
}
.gf__check {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  min-height: 1.75rem;
  cursor: pointer;
}
.gf__check input {
  width: 1rem;
  height: 1rem;
  accent-color: var(--accent);
}
.gf__others li {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--fs-sm);
}
.gf__remove {
  display: grid;
  place-items: center;
  width: 1.5rem;
  height: 1.5rem;
  padding: 0;
  border: 0;
  border-radius: var(--radius-pill);
  background: var(--surface-2);
  color: var(--text-2);
  cursor: pointer;
}
</style>
