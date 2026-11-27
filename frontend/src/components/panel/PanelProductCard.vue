<script setup lang="ts">
// Товар в панели («Карточка товара в панели») — строка: иконка или фото; семейство и под ним
// вендор со строкой лицензии (у оборудования — модель, парт-номер, характеристики и поставка); справа цена и
// число вариантов или конфигураций. Набор полей одинаков у всех строк одного типа; выделения отдельных товаров нет.
import { computed } from 'vue'
import { HardDrive, Package } from 'lucide-vue-next'
import VendorLogo from '@/components/common/VendorLogo.vue'
import type { PanelProduct } from '@/api/typesPanel'
import { formatPrice } from '@/lib/format'
import { panelPrice, productLines } from '@/lib/panel'

const props = defineProps<{ product: PanelProduct }>()

const lines = computed(() => productLines(props.product))
const price = computed(() => panelPrice(props.product, formatPrice))
const hardware = computed(() => props.product.kind === 'hardware')
</script>

<template>
  <span class="ppc">
    <span v-if="hardware && product.image_url" class="ppc__media" aria-hidden="true">
      <img
        :src="product.image_url"
        alt=""
        loading="lazy"
        referrerpolicy="no-referrer"
        class="ppc__img"
      />
    </span>
    <VendorLogo v-else :vendor="product.vendor" :image="product.image_url">
      <HardDrive v-if="hardware" :size="18" />
      <Package v-else :size="18" />
    </VendorLogo>
    <span class="ppc__body">
      <span class="ppc__title">{{ lines.primary }}</span>
      <span class="ppc__line t-xs t-muted">
        <span v-if="lines.secondary" :class="{ 't-mono': hardware }">{{ lines.secondary }}</span>
        <template v-if="lines.secondary && lines.meta"> · </template>
        <span v-if="lines.meta">{{ lines.meta }}</span>
      </span>
    </span>
    <span class="ppc__side">
      <span class="ppc__price t-num">{{ price }}</span>
      <span v-if="lines.count" class="ppc__count t-xs t-muted">{{ lines.count }}</span>
    </span>
  </span>
</template>

<style scoped>
.ppc {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: center;
  column-gap: var(--space-3);
  min-width: 0;
}
.ppc__media {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 2.25rem;
  height: 2.25rem;
  border-radius: var(--radius-control);
  background: var(--surface-2);
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
  font-weight: var(--fw-semibold);
  overflow-wrap: anywhere;
}
.ppc__line {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ppc__side {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  white-space: nowrap;
}
.ppc__price {
  color: var(--text);
  font-weight: var(--fw-bold);
}
@media (max-width: 479px) {
  .ppc__count {
    display: none;
  }
}
</style>
