<script setup lang="ts">
import { computed } from 'vue'
import { formatPrice, type Category, type Product } from '@/lib/catalog'
import { categoryVisual } from './categoryVisual'

const props = defineProps<{ product: Product; category?: Category }>()
const visual = computed(() => categoryVisual(props.product.category))
</script>

<template>
  <article class="card">
    <div
      class="card__tile"
      :style="{ background: visual.bg, color: visual.ink }"
      aria-hidden="true"
    >
      <component :is="visual.icon" :size="44" :stroke-width="1.5" />
    </div>
    <div class="card__body">
      <p v-if="category" class="t-caption m-0 text-text-2">{{ category.title }}</p>
      <h3 class="card__title">{{ product.title }}</h3>
      <p class="card__price m-0">
        <span class="card__now">{{ formatPrice(product.price) }}</span>
        <s v-if="product.oldPrice" class="card__old">
          <span class="sr-only">Цена без скидки:</span> {{ formatPrice(product.oldPrice) }}
        </s>
      </p>
    </div>
  </article>
</template>

<style scoped>
.card {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
  box-shadow: var(--shadow-card);
  transition:
    box-shadow 160ms ease-out,
    transform 160ms ease-out;
}
.card:hover {
  box-shadow: var(--shadow-card-hover);
  transform: translateY(-2px);
}
.card__tile {
  display: grid;
  place-items: center;
  aspect-ratio: 4 / 3;
}
.card__body {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 0.25rem;
  padding: 0.75rem 0.875rem 1rem;
}
.card__title {
  display: -webkit-box;
  margin: 0;
  overflow: hidden;
  font-size: var(--fs-body-2);
  font-weight: 500;
  line-height: 1.35;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  line-clamp: 2;
}
.card__price {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 0.25rem 0.5rem;
  margin-top: auto;
  padding-top: 0.5rem;
}
.card__now {
  font-size: 1.0625rem;
  font-weight: 600;
}
.card__old {
  color: var(--text-2);
  font-size: var(--fs-caption);
}
</style>
