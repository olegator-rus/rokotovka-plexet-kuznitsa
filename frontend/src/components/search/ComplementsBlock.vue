<script setup lang="ts">
// «С этим покупают» (экран «Поиск», нижние блоки): связи complement из справочника для первой
// позиции выдачи. Строка: название, вендор и лицензия, почему вместе, цена, действие.
import type { Complement } from '@/api/types'
import { isPurchasable, priceLabel } from '@/lib/license'

defineProps<{ items: Complement[]; anchor?: string }>()
defineEmits<{ action: [type: 'add_to_cart' | 'price_request', item: Complement, index: number] }>()
</script>

<template>
  <section class="compl" aria-labelledby="compl-title">
    <h2 id="compl-title" class="t-h2">С этим покупают</h2>
    <p v-if="anchor" class="t-sm t-muted">К позиции «{{ anchor }}»</p>
    <ul class="compl__list">
      <li v-for="(c, i) in items" :key="c.family_id" class="compl__row">
        <div class="compl__main">
          <p class="compl__title">{{ c.title }}</p>
          <p class="t-sm t-muted">
            {{ c.vendor }}<template v-if="c.license_line"> · {{ c.license_line }}</template>
          </p>
          <p v-if="c.note" class="t-sm compl__note">{{ c.note }}</p>
        </div>
        <span class="compl__price t-num">{{ priceLabel(c) }}</span>
        <button
          type="button"
          class="btn btn--secondary btn--sm"
          :aria-label="`${isPurchasable(c) ? 'В корзину' : 'Запросить цену'}: ${c.title}`"
          @click="$emit('action', isPurchasable(c) ? 'add_to_cart' : 'price_request', c, i)"
        >
          {{ isPurchasable(c) ? 'В корзину' : 'Запросить цену' }}
        </button>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.compl {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin-top: var(--space-3);
}
.compl__list {
  margin: 0;
  padding: 0;
  list-style: none;
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
}
.compl__row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto auto;
  align-items: center;
  gap: var(--space-2) var(--space-4);
  padding: var(--space-2) var(--space-3);
  border-bottom: var(--border-w) solid var(--border);
}
.compl__row:last-child {
  border-bottom: 0;
}
.compl__main {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.compl__title {
  font-weight: var(--fw-medium);
  overflow-wrap: anywhere;
}
.compl__note {
  color: var(--text-2);
}
.compl__price {
  white-space: nowrap;
  font-weight: var(--fw-medium);
}
@media (max-width: 639px) {
  .compl__row {
    grid-template-columns: minmax(0, 1fr) auto;
  }
  .compl__main {
    grid-column: 1 / -1;
  }
}
</style>
