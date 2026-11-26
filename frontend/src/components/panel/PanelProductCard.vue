<script setup lang="ts">
// Карточка товара в панели («Карточка товара в панели»): ПО — иконка категории, семейство,
// вендор, строка лицензии, цена, число вариантов; оборудование — фото, модель и парт-номер, характеристики и
// поставка, цена, число конфигураций. Набор строк одинаков у всех карточек одного типа; выделения отдельных
// товаров нет.
import { computed } from 'vue'
import { HardDrive, Package } from 'lucide-vue-next'
import type { PanelProduct } from '@/api/typesPanel'
import { formatPrice } from '@/lib/format'
import { panelPrice, productLines } from '@/lib/panel'

const props = defineProps<{ product: PanelProduct; row?: boolean }>()

const lines = computed(() => productLines(props.product))
const price = computed(() => panelPrice(props.product, formatPrice))
const hardware = computed(() => props.product.kind === 'hardware')
</script>

<template>
  <span :class="['ppc', { 'ppc--row': row }]">
    <span class="ppc__media" aria-hidden="true">
      <img
        v-if="hardware && product.image_url"
        :src="product.image_url"
        alt=""
        loading="lazy"
        class="ppc__img"
      />
      <HardDrive v-else-if="hardware" :size="20" />
      <Package v-else :size="20" />
    </span>
    <span class="ppc__body">
      <span class="ppc__title">{{ lines.primary }}</span>
      <span class="ppc__line t-xs t-muted" :class="{ 't-mono': hardware }">{{
        lines.secondary || ' '
      }}</span>
      <span class="ppc__line t-xs t-muted">{{ lines.meta || ' ' }}</span>
      <span class="ppc__price t-sm t-num">{{ price }}</span>
      <span class="ppc__line t-xs t-muted">{{ lines.count || ' ' }}</span>
    </span>
  </span>
</template>

<style scoped>
.ppc {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}
.ppc--row {
  flex-direction: row;
  align-items: flex-start;
}
.ppc__media {
  display: flex;
  align-items: center;
  justify-content: center;
  flex: none;
  width: 2.5rem;
  height: 2.5rem;
  border-radius: var(--radius-control);
  background: var(--surface-3);
  color: var(--text-2);
  overflow: hidden;
}
.ppc__img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.ppc__body {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.ppc__title {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  color: var(--text);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
  font-weight: var(--fw-medium);
  overflow-wrap: anywhere;
}
.ppc__line {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ppc__price {
  margin-top: var(--space-1);
  color: var(--text);
  font-weight: var(--fw-semibold);
}
</style>
