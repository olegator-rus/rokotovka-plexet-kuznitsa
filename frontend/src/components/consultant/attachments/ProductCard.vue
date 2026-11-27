<script setup lang="ts">
// Карточка товара (CONSULTANT.md §6.1): похожа на прикреплённую ссылку. Полная стоимость и расчёт — из копеек
// ответа; цены 0 нет — «Цена по запросу». Не подходит — нейтральный блок с причиной, добавить нельзя.
// «В спецификацию» — действие фронта (без запроса к ядру); кнопка следует за текущей спецификацией.
import { computed } from 'vue'
import { AlertTriangle, Check, ExternalLink } from 'lucide-vue-next'
import type { ProductReference } from '@/api/typesConsultant'
import VendorLogo from '@/components/common/VendorLogo.vue'
import {
  isStoreUrl,
  productCalc,
  productOldUnit,
  productPrice,
  productTerms,
  unfitText,
} from '@/lib/consultant'
import { useSpecStore } from '@/stores/spec'
import { useConsultant } from '../composables/useConsultant'

const props = defineProps<{ product: ProductReference }>()

const c = useConsultant()
const spec = useSpecStore()

const p = computed(() => props.product)
const inSpec = computed(() => spec.has(p.value.offerId))
const unfit = computed(() => p.value.compatibility === 'incompatible')
const price = computed(() => productPrice(p.value))
const calc = computed(() => productCalc(p.value))
const oldUnit = computed(() => productOldUnit(p.value))
const terms = computed(() => productTerms(p.value))
const url = computed(() => (isStoreUrl(p.value.productUrl) ? p.value.productUrl : ''))
</script>

<template>
  <article class="pc" :class="{ 'pc--unfit': unfit }" data-testid="consultant-card">
    <div class="pc__head">
      <VendorLogo class="pc__logo" :vendor="p.vendor" :image="p.imageUrl" />
      <div class="pc__name">
        <h3 class="pc__title">{{ p.title }}</h3>
        <p v-if="terms" class="pc__terms">{{ terms }}</p>
      </div>
    </div>
    <div class="pc__price-block">
      <span class="pc__price t-num" data-testid="consultant-card-price">{{ price }}</span>
      <span v-if="calc || oldUnit" class="pc__calc t-num">
        {{ calc }}
        <template v-if="oldUnit">
          <s class="pc__old">{{ oldUnit }}</s> · Цена по акции
        </template>
      </span>
    </div>
    <p v-if="unfit" class="pc__warn" data-testid="consultant-card-unfit">
      <AlertTriangle :size="14" aria-hidden="true" />
      <span>{{ unfitText(p.reason) }}</span>
    </p>
    <p v-else-if="p.compatibility === 'unverified'" class="pc__reason pc__reason--muted">
      Совместимость не подтверждена
    </p>
    <p v-else-if="p.reason" class="pc__reason">
      <Check :size="14" aria-hidden="true" /> <span>{{ p.reason }}</span>
    </p>
    <div class="pc__actions">
      <button
        type="button"
        class="btn pc__add"
        :class="inSpec ? 'btn--secondary' : 'btn--primary'"
        :disabled="unfit || inSpec"
        data-testid="consultant-card-add"
        @click="c.addProduct(p)"
      >
        <Check v-if="inSpec" :size="16" aria-hidden="true" />
        {{ inSpec ? 'В спецификации' : 'В спецификацию' }}
      </button>
      <a v-if="url" class="pc__open" :href="url" target="_blank" rel="noopener noreferrer">
        Открыть в магазине <ExternalLink :size="13" aria-hidden="true" />
      </a>
    </div>
  </article>
</template>

<style scoped>
.pc {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-3);
  border: 1px solid var(--border);
  border-radius: 0.75rem;
  background: var(--surface);
}
.pc__head {
  display: flex;
  min-width: 0;
  align-items: flex-start;
  gap: var(--space-3);
}
.pc__logo {
  flex-shrink: 0;
  width: 2.25rem;
  height: 2.25rem;
}
.pc__name {
  min-width: 0;
}
.pc__title {
  margin: 0;
  font-size: var(--fs-body);
  font-weight: var(--fw-medium);
  line-height: 1.35;
  overflow-wrap: anywhere;
}
.pc__terms {
  margin: 2px 0 0;
  color: var(--text-2);
  font-size: var(--fs-xs);
  line-height: 1.4;
}
.pc__price-block {
  display: flex;
  flex-direction: column;
}
.pc__price {
  font-size: var(--cw-price-fs, 1.125rem);
  font-weight: var(--fw-semibold);
  line-height: 1.35;
  white-space: nowrap;
}
.pc__calc {
  color: var(--text-2);
  font-size: var(--fs-xs);
}
.pc__old {
  color: var(--text-3);
}
.pc__reason,
.pc__warn {
  display: flex;
  align-items: flex-start;
  gap: var(--space-1);
  margin: 0;
  font-size: var(--fs-sm);
  line-height: 1.4;
}
.pc__reason svg,
.pc__warn svg {
  flex-shrink: 0;
  margin-top: 2px;
}
.pc__reason--muted {
  color: var(--text-2);
}
.pc__warn {
  padding: var(--space-2);
  border-radius: var(--radius-control);
  background: var(--surface-2);
  color: var(--text);
}
.pc__actions {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: var(--space-1);
}
.pc__add {
  justify-content: center;
  min-height: 2.5rem;
}
.pc__open {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--space-1);
  min-height: 2.25rem;
  color: var(--text);
  font-size: var(--fs-sm);
  text-decoration: underline;
  text-underline-offset: 2px;
}
</style>
