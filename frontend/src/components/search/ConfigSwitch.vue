<script setup lang="ts">
// Переключатель конфигурации поиска (reka ToggleGroup): стрелки двигают фокус, Enter/пробел выбирают.
// Пустого значения нет — повторный клик по выбранному ничего не снимает.
import { ToggleGroupItem, ToggleGroupRoot } from 'reka-ui'
import type { ConfigId } from '@/api/types'
import { configHint } from '@/lib/configs'
import { isConfigId } from '@/lib/searchState'
import UiTooltip from '@/components/ui/UiTooltip.vue'

defineProps<{ configs: readonly ConfigId[]; modelValue: ConfigId }>()
const emit = defineEmits<{ 'update:modelValue': [value: ConfigId] }>()

function onUpdate(v: unknown) {
  if (isConfigId(v)) emit('update:modelValue', v)
}
</script>

<template>
  <ToggleGroupRoot
    type="single"
    :model-value="modelValue"
    class="config-switch"
    aria-label="Конфигурация поиска"
    @update:model-value="onUpdate"
  >
    <UiTooltip v-for="c in configs" :key="c" :text="`${c}: ${configHint(c)}`" side="bottom">
      <ToggleGroupItem
        :value="c"
        class="config-switch__item"
        :aria-label="`${c}: ${configHint(c)}`"
      >
        {{ c }}
      </ToggleGroupItem>
    </UiTooltip>
  </ToggleGroupRoot>
</template>

<style scoped>
.config-switch {
  display: inline-flex;
  padding: 2px;
  gap: 2px;
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
  background: var(--surface);
}
.config-switch__item {
  min-width: 2.5rem;
  min-height: var(--control-h-sm);
  padding: 0 var(--space-2);
  border: 0;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-2);
  font-size: var(--fs-sm);
  font-weight: var(--fw-medium);
  font-variant-numeric: tabular-nums;
  cursor: pointer;
}
.config-switch__item:hover {
  color: var(--text);
  background: var(--surface-2);
}
.config-switch__item[aria-pressed='true'] {
  background: var(--accent);
  color: var(--on-accent);
}
</style>
