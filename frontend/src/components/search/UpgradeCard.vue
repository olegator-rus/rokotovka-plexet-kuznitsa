<script setup lang="ts">
// «Старшая редакция»: рядом с аналогами, не в блоке дополнений; одна-две строки «что
// добавится» из связи upgrade. Запрошенная позиция остаётся первой — карточка её не подменяет.
import { ArrowUpRight, Check, Plus } from 'lucide-vue-next'
import type { Upgrade } from '@/api/typesP2'
import CompanionSource from '@/components/search/CompanionSource.vue'
import { priceModeLabel } from '@/lib/spec'
import { useSpecStore } from '@/stores/spec'

defineProps<{ upgrade: Upgrade; base: string }>()
const emit = defineEmits<{ open: [item: Upgrade]; add: [item: Upgrade] }>()
const spec = useSpecStore()
</script>

<template>
  <section class="upg" aria-labelledby="upg-title">
    <h2 id="upg-title" class="t-h2">Старшая редакция</h2>
    <div class="upg-card" role="group" :aria-label="upgrade.title">
      <div class="upg-card__main">
        <p class="t-xs">
          <span class="badge">{{ upgrade.label || 'Старшая редакция' }}</span>
          <span class="t-muted"> вместо «{{ base }}»</span>
        </p>
        <h3 class="upg-card__title">
          <button type="button" class="upg-card__open" @click="emit('open', upgrade)">
            {{ upgrade.title }}
            <ArrowUpRight :size="14" aria-hidden="true" />
          </button>
        </h3>
        <p class="t-sm t-muted">
          {{ upgrade.vendor
          }}<template v-if="upgrade.license_line"> · {{ upgrade.license_line }}</template>
        </p>
        <ul class="upg-card__adds" aria-label="Что добавится">
          <li v-for="a in upgrade.upgrade_adds" :key="a" class="t-sm">
            <Plus :size="14" aria-hidden="true" />
            <span>{{ a }}</span>
          </li>
        </ul>
        <p class="upg-card__foot">
          <CompanionSource :source="upgrade.source" :url="upgrade.source_url" />
        </p>
      </div>
      <p class="upg-card__price t-num">{{ priceModeLabel(upgrade) }}</p>
      <div class="upg-card__actions">
        <span v-if="spec.has(upgrade.product_id)" class="badge badge--strong">
          <Check :size="12" aria-hidden="true" /> В спецификации
        </span>
        <button
          v-else
          type="button"
          class="btn btn--secondary btn--sm"
          :aria-label="`В спецификацию: ${upgrade.title}`"
          @click="emit('add', upgrade)"
        >
          В спецификацию
        </button>
      </div>
    </div>
  </section>
</template>

<style scoped>
.upg {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin-top: var(--space-3);
}
.upg-card {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  grid-template-areas:
    'main main'
    'price actions';
  align-items: center;
  gap: var(--space-2) var(--space-3);
  padding: var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
}
.upg-card__main {
  grid-area: main;
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.upg-card__title {
  margin: 0;
  font-size: var(--fs-body);
  line-height: var(--lh-body);
  font-weight: var(--fw-semibold);
}
.upg-card__open {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  padding: 0;
  border: 0;
  background: none;
  color: var(--text);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.upg-card__open:hover {
  text-decoration: underline;
  text-decoration-color: var(--text-3);
  text-underline-offset: 3px;
}
.upg-card__adds {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: var(--space-1) 0 0;
  padding: 0;
  list-style: none;
}
.upg-card__adds li {
  display: flex;
  align-items: flex-start;
  gap: var(--space-1);
}
.upg-card__adds svg {
  flex: none;
  margin-top: 3px;
  color: var(--text-2);
}
.upg-card__foot {
  margin-top: 2px;
}
.upg-card__price {
  grid-area: price;
  font-weight: var(--fw-semibold);
}
.upg-card__actions {
  grid-area: actions;
  justify-self: end;
}
@media (min-width: 640px) {
  .upg-card {
    grid-template-columns: minmax(0, 1fr) 8.5rem auto;
    grid-template-areas: 'main price actions';
  }
  .upg-card__price {
    text-align: right;
  }
}
</style>
