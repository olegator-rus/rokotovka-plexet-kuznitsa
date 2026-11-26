<script setup lang="ts">
// «Как посчитано»: каждое число, которое посчитал сервер, открывает это окно — значение, что оно
// значит, формула, из каких чисел сложилось и откуда данные. Слот — подробности раздела (списки, ссылки).
import UiDialog from '@/components/ui/UiDialog.vue'

export interface ExplainRow {
  label: string
  value: string
}

export interface Explanation {
  title: string
  value: string
  what?: string
  how?: string
  rows?: ExplainRow[]
  source?: string
}

const open = defineModel<boolean>('open', { default: false })
defineProps<{ explanation: Explanation | null }>()
</script>

<template>
  <UiDialog v-model:open="open" :title="explanation?.title ?? ''">
    <div v-if="explanation" class="explain">
      <p class="explain__value t-num">{{ explanation.value }}</p>
      <p v-if="explanation.what">{{ explanation.what }}</p>
      <div v-if="explanation.how" class="explain__how">
        <p class="t-xs t-muted">Как считается</p>
        <p class="t-sm">{{ explanation.how }}</p>
      </div>
      <dl v-if="explanation.rows?.length" class="explain__rows">
        <template v-for="r in explanation.rows" :key="r.label">
          <dt class="t-muted">{{ r.label }}</dt>
          <dd class="t-num">{{ r.value }}</dd>
        </template>
      </dl>
      <slot />
      <p v-if="explanation.source" class="t-xs t-muted">
        Данные: <span class="t-mono">{{ explanation.source }}</span>
      </p>
    </div>
  </UiDialog>
</template>

<style scoped>
.explain {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.explain :slotted(.btn) {
  align-self: flex-start;
}
.explain__value {
  font-size: var(--fs-h1);
  line-height: var(--lh-h1);
  font-weight: var(--fw-semibold);
}
.explain__how {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding: var(--space-3);
  border-radius: var(--radius-control);
  background: var(--surface-2);
}
.explain__rows {
  display: grid;
  grid-template-columns: minmax(7rem, 45%) minmax(0, 1fr);
  gap: var(--space-1) var(--space-3);
  margin: 0;
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.explain__rows dd {
  margin: 0;
  text-align: right;
  white-space: normal;
  overflow-wrap: anywhere;
}
</style>
