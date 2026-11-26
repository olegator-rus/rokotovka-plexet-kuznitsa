<script setup lang="ts">
// «Дополните решение к …» («Правила блока допродажи»): под топом выдачи, не над
// ней и не баннером; карточки как в выдаче, без выделения; у каждой — подпись класса, строка «почему» и
// иконка основания; ничего не выбрано заранее. Пусто — компонент не рисуется вовсе (решает страница).
import { Check, ShieldQuestion } from 'lucide-vue-next'
import type { Companion } from '@/api/typesP2'
import CompanionSource from '@/components/search/CompanionSource.vue'
import { classLabel } from '@/lib/companions'
import { priceModeLabel } from '@/lib/spec'
import { useSpecStore } from '@/stores/spec'

defineProps<{ items: Companion[]; anchor: string }>()
const emit = defineEmits<{
  open: [item: Companion, index: number]
  add: [item: Companion, index: number]
  search: [text: string]
}>()

const spec = useSpecStore()
</script>

<template>
  <section class="cmp" aria-labelledby="cmp-title">
    <h2 id="cmp-title" class="t-h2">Дополните решение к {{ anchor }}</h2>
    <ul class="cmp__list">
      <li v-for="(c, i) in items" :key="c.product_id">
        <div class="cmp-card" role="group" :aria-label="c.title">
          <div class="cmp-card__main">
            <p class="cmp-card__class t-xs">
              <span class="badge">{{ classLabel(c) }}</span>
              <span v-if="c.category" class="t-muted">{{ c.category }}</span>
            </p>
            <h3 class="cmp-card__title">
              <button type="button" class="cmp-card__open" @click="emit('open', c, i)">
                {{ c.title }}
              </button>
            </h3>
            <p class="t-sm t-muted">
              {{ c.vendor }}<template v-if="c.license_line"> · {{ c.license_line }}</template>
            </p>
            <p v-if="c.reason" class="t-sm cmp-card__why">{{ c.reason }}</p>
            <p v-if="c.compatibility === 'unknown'" class="t-xs cmp-card__unknown">
              <ShieldQuestion :size="12" aria-hidden="true" />
              Совместимость не подтверждена — уточните у менеджера
            </p>
            <p class="cmp-card__foot">
              <CompanionSource :source="c.source" :url="c.source_url" />
              <button
                v-if="c.category"
                type="button"
                class="link t-xs"
                @click="emit('search', c.category)"
              >
                все: {{ c.category.toLowerCase() }}
              </button>
            </p>
          </div>
          <p class="cmp-card__price t-num">{{ priceModeLabel(c) }}</p>
          <div class="cmp-card__actions">
            <span v-if="spec.has(c.product_id)" class="badge badge--strong">
              <Check :size="12" aria-hidden="true" /> В спецификации
            </span>
            <button
              v-else
              type="button"
              class="btn btn--secondary btn--sm"
              :aria-label="`В спецификацию: ${c.title}`"
              @click="emit('add', c, i)"
            >
              В спецификацию
            </button>
          </div>
        </div>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.cmp {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: var(--space-3) 0;
}
.cmp__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.cmp-card {
  container-type: inline-size;
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
.cmp-card__main {
  grid-area: main;
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.cmp-card__class {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
  margin-bottom: 2px;
}
.cmp-card__title {
  margin: 0;
  font-size: var(--fs-body);
  line-height: var(--lh-body);
  font-weight: var(--fw-semibold);
}
.cmp-card__open {
  padding: 0;
  border: 0;
  background: none;
  color: var(--text);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.cmp-card__open:hover {
  text-decoration: underline;
  text-decoration-color: var(--text-3);
  text-underline-offset: 3px;
}
.cmp-card__why {
  color: var(--text);
}
.cmp-card__unknown {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  color: var(--text-2);
}
.cmp-card__foot {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1) var(--space-3);
  margin-top: 2px;
}
.cmp-card__price {
  grid-area: price;
  font-weight: var(--fw-semibold);
}
.cmp-card__actions {
  grid-area: actions;
  justify-self: end;
}
@media (min-width: 640px) {
  .cmp-card {
    grid-template-columns: minmax(0, 1fr) 8.5rem auto;
    grid-template-areas: 'main price actions';
    padding: var(--space-2) var(--space-3);
  }
  .cmp-card__price {
    text-align: right;
  }
}
</style>
