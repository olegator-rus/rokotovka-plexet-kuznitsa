<script setup lang="ts">
// Условия подбора одной строкой (SPEC §2): введённое покупателем — плотный чип, подставленное по умолчанию или из
// запроса — приглушённый пунктирный. Нажатие — правка в том же чипе (Enter — изменить, Esc — отменить), крестик —
// снять условие.
import { nextTick, ref, watch } from 'vue'
import { X } from 'lucide-vue-next'
import type { GuideCondition } from '@/api/typesGuide'
import { conditionAria, conditionOptions, conditionText, originLabel } from '@/lib/guide'

const props = defineProps<{ conditions: GuideCondition[]; disabled?: boolean }>()
const emit = defineEmits<{ edit: [condition: GuideCondition, value: string | null] }>()

const editing = ref<string | null>(null)
const editText = ref('')
function startEdit(c: GuideCondition) {
  editing.value = c.key
  editText.value = c.value
  void nextTick(() => {
    const el = document.getElementById(`gcond-${c.key}`)
    el?.focus()
    if (el instanceof HTMLInputElement) el.select()
  })
}
function commitEdit(c: GuideCondition) {
  const v = editText.value.trim()
  editing.value = null
  if (v && v !== c.value) emit('edit', c, v)
}
function pickOption(c: GuideCondition, e: Event) {
  editText.value = (e.target as HTMLSelectElement).value
  commitEdit(c)
}
watch(
  () => props.conditions,
  () => (editing.value = null),
)

const hint = (c: GuideCondition) => {
  const o = originLabel(c.origin)
  return o ? `Подставлено ${o} — нажмите, чтобы изменить` : 'Нажмите, чтобы изменить'
}
</script>

<template>
  <ul v-if="conditions.length" class="gconds" aria-label="Условия подбора">
    <li
      v-for="c in conditions"
      :key="c.key"
      :class="['gcond', c.origin === 'user' ? 'gcond--user' : 'gcond--auto']"
      :data-cond="c.key"
      :data-origin="c.origin"
    >
      <form v-if="editing === c.key" class="gcond__form" @submit.prevent="commitEdit(c)">
        <label :for="`gcond-${c.key}`" class="sr-only">Изменить: {{ c.label || c.value }}</label>
        <select
          v-if="conditionOptions(c.key)"
          :id="`gcond-${c.key}`"
          class="gcond__input gcond__select"
          :value="c.value"
          @change="pickOption(c, $event)"
          @keydown.esc.prevent="editing = null"
          @blur="editing = null"
        >
          <option v-for="o in conditionOptions(c.key)" :key="o.value" :value="o.value">
            {{ o.label }}
          </option>
        </select>
        <input
          v-else
          :id="`gcond-${c.key}`"
          v-model="editText"
          class="gcond__input"
          autocomplete="off"
          @keydown.esc.prevent="editing = null"
          @blur="editing = null"
        />
      </form>
      <button
        v-else
        type="button"
        class="gcond__label"
        :title="hint(c)"
        :aria-label="`Изменить условие: ${conditionAria(c)}`"
        :disabled="disabled"
        @click="startEdit(c)"
      >
        {{ conditionText(c)
        }}<span v-if="originLabel(c.origin)" class="sr-only">{{
          ` — ${originLabel(c.origin)}`
        }}</span>
      </button>
      <button
        type="button"
        class="gcond__x"
        :aria-label="`Убрать условие: ${c.label || c.value}`"
        :disabled="disabled"
        data-testid="guide-cond-remove"
        @click="emit('edit', c, null)"
      >
        <X :size="12" :stroke-width="2" aria-hidden="true" />
      </button>
    </li>
  </ul>
</template>

<style scoped>
.gconds {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.gcond {
  display: inline-flex;
  align-items: stretch;
  height: var(--control-h-sm);
  border: var(--border-w) solid transparent;
  border-radius: var(--radius-pill);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
  transition: border-color var(--dur-fast) var(--ease-standard);
}
.gcond--user {
  background: var(--surface-3);
  color: var(--text);
}
.gcond--auto {
  border-style: dashed;
  border-color: var(--border-control);
  color: var(--text-2);
}
.gcond__label,
.gcond__x {
  display: inline-flex;
  align-items: center;
  border: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  cursor: pointer;
}
.gcond__label {
  padding: 0 var(--space-1) 0 var(--space-3);
  border-radius: var(--radius-pill) 0 0 var(--radius-pill);
  white-space: nowrap;
}
.gcond--user .gcond__label {
  font-weight: var(--fw-medium);
}
.gcond__x {
  padding: 0 var(--space-2) 0 var(--space-1);
  border-radius: 0 var(--radius-pill) var(--radius-pill) 0;
  color: var(--text-3);
}
.gcond__label:hover:not(:disabled) {
  text-decoration: underline;
  text-underline-offset: 3px;
}
.gcond__x:hover:not(:disabled) {
  color: var(--text);
}
.gcond__label:disabled,
.gcond__x:disabled {
  cursor: default;
}
.gcond__form {
  display: flex;
}
.gcond__input {
  width: 6.5rem;
  padding: 0 var(--space-3);
  border: 0;
  border-radius: var(--radius-pill);
  background: var(--surface);
  color: var(--text);
  font: inherit;
  font-size: var(--fs-input);
}
.gcond__select {
  width: auto;
  padding-right: var(--space-2);
  font-size: var(--fs-sm);
  cursor: pointer;
}
.gcond:has(.gcond__form) {
  border-color: var(--text);
  background: var(--surface);
}
</style>
