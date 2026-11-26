<script setup lang="ts">
// Выпадающий список (reka Select): компактный триггер для шапки, список — панель под ним, выбранный
// пункт с галочкой. Клавиатура — стрелки, Enter, Esc, поиск по первой букве (из коробки reka).
import {
  SelectContent,
  SelectItem,
  SelectItemIndicator,
  SelectItemText,
  SelectPortal,
  SelectRoot,
  SelectTrigger,
  SelectValue,
  SelectViewport,
} from 'reka-ui'
import { Check, ChevronDown } from 'lucide-vue-next'

export interface UiSelectOption {
  value: string
  label: string
  hint?: string
}

const model = defineModel<string>({ required: true })

defineProps<{
  options: UiSelectOption[]
  label: string
  /** Короткая подпись слева в триггере: «Клиент», «Роль». */
  prefix?: string
  disabled?: boolean
}>()
</script>

<template>
  <SelectRoot v-model="model" :disabled="disabled">
    <SelectTrigger class="ui-select" :aria-label="label">
      <span v-if="prefix" class="ui-select__prefix">{{ prefix }}</span>
      <SelectValue class="ui-select__value" />
      <ChevronDown :size="14" aria-hidden="true" class="ui-select__chevron" />
    </SelectTrigger>
    <SelectPortal>
      <SelectContent position="popper" :side-offset="4" align="end" class="ui-select__content">
        <SelectViewport class="ui-select__viewport">
          <SelectItem
            v-for="o in options"
            :key="o.value"
            :value="o.value"
            :text-value="o.label"
            class="ui-select__item"
          >
            <span class="ui-select__item-text">
              <SelectItemText>{{ o.label }}</SelectItemText>
              <span v-if="o.hint" class="ui-select__hint">{{ o.hint }}</span>
            </span>
            <SelectItemIndicator class="ui-select__check">
              <Check :size="14" aria-hidden="true" />
            </SelectItemIndicator>
          </SelectItem>
        </SelectViewport>
      </SelectContent>
    </SelectPortal>
  </SelectRoot>
</template>

<style>
.ui-select {
  box-sizing: border-box;
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  min-width: 0;
  max-width: 100%;
  min-height: var(--control-h);
  padding: 0 var(--space-2) 0 var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
  background: var(--surface);
  color: var(--text);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
  cursor: pointer;
}
.ui-select:hover:not([data-disabled]) {
  border-color: var(--border-control);
}
.ui-select[data-disabled] {
  cursor: not-allowed;
  color: var(--text-3);
}
.ui-select__prefix {
  color: var(--text-2);
}
.ui-select__value {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: var(--fw-medium);
}
.ui-select__chevron {
  flex: none;
  color: var(--text-2);
}
.ui-select__content {
  z-index: 60;
  min-width: var(--reka-select-trigger-width);
  max-height: var(--reka-select-content-available-height);
  overflow: hidden;
  background: var(--surface-2);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  box-shadow: var(--shadow-overlay);
}
.ui-select__viewport {
  padding: var(--space-1);
}
.ui-select__item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  min-height: var(--control-h);
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-control);
  color: var(--text);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
  cursor: pointer;
  outline: none;
  user-select: none;
}
.ui-select__item[data-highlighted] {
  background: var(--surface-3);
}
.ui-select__item[data-state='checked'] {
  font-weight: var(--fw-medium);
}
.ui-select__item-text {
  display: flex;
  flex-direction: column;
}
.ui-select__hint {
  color: var(--text-2);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
}
.ui-select__check {
  display: inline-flex;
  color: var(--text);
}
</style>
