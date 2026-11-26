<script setup lang="ts">
// «Дополните решение к …»: под топом выдачи, не над ней. По решению владельца
// Блок выделен как одна карточка «Рекомендуем» (прежнее правило «без выделения» отменено); позиции
// внутри равны между собой: у каждой подпись класса, строка «почему» и иконка основания; ничего не выбрано
// заранее. Пусто — компонент не рисуется вовсе (решает страница).
import { Check, Plus, ShieldQuestion, Sparkles } from 'lucide-vue-next'
import VendorLogo from '@/components/common/VendorLogo.vue'
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
    <header class="cmp__head">
      <p class="cmp__ribbon t-xs"><Sparkles :size="14" aria-hidden="true" /> Рекомендуем</p>
      <h2 id="cmp-title" class="t-h2">Дополните решение к {{ anchor }}</h2>
    </header>
    <ul class="cmp__list">
      <li v-for="(c, i) in items" :key="c.product_id">
        <div class="cmp-card" role="group" :aria-label="c.title">
          <VendorLogo :vendor="c.vendor" class="cmp-card__logo" />
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
              class="btn btn--primary btn--sm"
              :aria-label="`В спецификацию: ${c.title}`"
              @click="emit('add', c, i)"
            >
              <Plus :size="14" aria-hidden="true" />
              В спецификацию
            </button>
          </div>
        </div>
      </li>
    </ul>
  </section>
</template>

<style scoped>
/* Одна выделенная карточка: красная рамка и подложка шапки, позиции — строки внутри на белом. */
.cmp {
  overflow: hidden;
  margin: var(--space-2) 0;
  border: var(--focus-w) solid var(--accent);
  border-radius: var(--radius-card);
  background: var(--surface);
}
.cmp__head {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
  background: var(--accent-soft);
}
.cmp__ribbon {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  margin: 0;
  padding: var(--space-1) var(--space-3);
  border-radius: var(--radius-pill);
  background: var(--accent);
  color: var(--on-accent);
  font-weight: var(--fw-bold);
}
.cmp__head h2 {
  margin: 0;
  overflow-wrap: anywhere;
}
.cmp__list {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  list-style: none;
}
.cmp__list li + li {
  border-top: var(--border-w) solid var(--border);
}
.cmp-card {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  grid-template-areas:
    'logo main main'
    'logo price actions';
  align-items: center;
  gap: var(--space-2) var(--space-3);
  padding: var(--space-3) var(--space-4);
}
.cmp-card__logo {
  grid-area: logo;
  align-self: start;
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
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
  font-weight: var(--fw-bold);
}
.cmp-card__actions {
  grid-area: actions;
  justify-self: end;
}
@media (min-width: 640px) {
  .cmp-card {
    grid-template-columns: auto minmax(0, 1fr) 8.5rem auto;
    grid-template-areas: 'logo main price actions';
  }
  .cmp-card__price {
    text-align: right;
  }
}
</style>
