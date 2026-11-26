<script setup lang="ts">
// Схема «дорожки»: две-три колонки работают параллельно и сходятся в один блок внизу. На узком экране
// колонки идут друг под другом.
import { ArrowDown } from 'lucide-vue-next'

export interface Lane {
  title: string
  items: readonly string[]
}

const props = defineProps<{
  lanes: readonly Lane[]
  merge: { title: string; text?: string }
  label: string
  top?: string
}>()
</script>

<template>
  <figure class="lanes">
    <figcaption class="sr-only">{{ props.label }}</figcaption>
    <div v-if="props.top" class="lanes__top">
      <strong>{{ props.top }}</strong>
    </div>
    <span v-if="props.top" class="lanes__arrow" aria-hidden="true"><ArrowDown :size="16" /></span>
    <div class="lanes__grid">
      <div v-for="l in props.lanes" :key="l.title" class="lanes__lane">
        <strong class="lanes__title">{{ l.title }}</strong>
        <ul class="lanes__items t-sm">
          <li v-for="it in l.items" :key="it">{{ it }}</li>
        </ul>
      </div>
    </div>
    <span class="lanes__arrow" aria-hidden="true"><ArrowDown :size="16" /></span>
    <div class="lanes__merge">
      <strong>{{ props.merge.title }}</strong>
      <p v-if="props.merge.text" class="t-sm t-muted">{{ props.merge.text }}</p>
    </div>
  </figure>
</template>

<style scoped>
.lanes {
  display: flex;
  flex-direction: column;
  margin: 0;
  min-width: 0;
}
.lanes__grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: var(--space-2);
}
.lanes__lane,
.lanes__merge,
.lanes__top {
  padding: var(--space-2) var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
  background: var(--surface);
  min-width: 0;
}
.lanes__top,
.lanes__merge {
  background: var(--surface-2);
  text-align: center;
}
.lanes__merge p {
  margin: var(--space-1) 0 0;
}
.lanes__title {
  display: block;
  margin-bottom: var(--space-1);
}
.lanes__items {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin: 0;
  padding-left: var(--space-4);
  color: var(--text-2);
}
.lanes__arrow {
  display: flex;
  justify-content: center;
  padding: 2px 0;
  color: var(--text-3);
}

@media (min-width: 768px) {
  .lanes__grid {
    grid-template-columns: repeat(auto-fit, minmax(14rem, 1fr));
  }
}
</style>
