<script setup lang="ts">
// Событие спецификации (CONSULTANT.md §6.4): только после успешного изменения; не удалось — ошибка, а не
// «Добавлено» (счётчик при этом не меняется — стор не тронут).
import { computed } from 'vue'
import { AlertTriangle, Check } from 'lucide-vue-next'
import type { SpecEventAttachment } from '@/api/typesConsultant'
import { formatMinor } from '@/lib/consultant'
import { useSpecStore } from '@/stores/spec'

const props = defineProps<{ data: SpecEventAttachment; failed?: string }>()
const spec = useSpecStore()

const label = computed(() => {
  if (props.data.action === 'removed') return 'Удалено из спецификации'
  if (props.data.action === 'updated') return 'Количество изменено'
  return 'Добавлено в спецификацию'
})
const price = computed(() =>
  props.data.action === 'removed' ? '' : formatMinor(props.data.totalPriceMinor),
)
</script>

<template>
  <div v-if="failed" class="se se--error" role="alert" data-testid="consultant-spec-error">
    <AlertTriangle :size="16" aria-hidden="true" />
    <span class="se__body">
      <span class="se__label">{{ failed }}</span>
      <span class="se__title">{{ data.title }}</span>
    </span>
  </div>
  <div
    v-else
    class="se"
    role="status"
    data-testid="consultant-spec-event"
    :data-action="data.action"
  >
    <Check :size="16" class="se__icon" aria-hidden="true" />
    <span class="se__body">
      <span class="se__row">
        <span class="se__label">{{ label }}</span>
        <span v-if="price" class="se__price t-num">{{ price }}</span>
      </span>
      <span class="se__title"
        >{{ data.title
        }}<template v-if="data.quantity && data.action !== 'removed'">
          · {{ data.quantity }} шт.</template
        ></span
      >
      <button type="button" class="se__open" @click="spec.open = true">Открыть спецификацию</button>
    </span>
  </div>
</template>

<style scoped>
.se {
  display: flex;
  min-width: 0;
  align-items: flex-start;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--border);
  border-radius: 0.75rem;
  background: var(--surface);
  font-size: var(--fs-sm);
}
.se__icon {
  flex-shrink: 0;
  margin-top: 2px;
  color: var(--status-accepted);
}
.se--error {
  background: var(--surface-2);
}
.se--error svg {
  flex-shrink: 0;
  margin-top: 2px;
}
.se__body {
  display: flex;
  flex: 1;
  min-width: 0;
  flex-direction: column;
  gap: 2px;
}
.se__row {
  display: flex;
  justify-content: space-between;
  gap: var(--space-2);
}
.se__label {
  font-weight: var(--fw-semibold);
}
.se__price {
  font-weight: var(--fw-semibold);
  white-space: nowrap;
}
.se__title {
  color: var(--text-2);
  overflow-wrap: anywhere;
}
.se__open {
  align-self: flex-end;
  min-height: 2rem;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--text);
  font: inherit;
  text-decoration: underline;
  text-underline-offset: 2px;
  cursor: pointer;
}
</style>
