<script setup lang="ts">
// Вложение по типу (CONTRACT.md, «Вложения»). Неизвестный тип не рисуется.
import type { AttachmentView } from '@/lib/consultant'
import ComparisonTable from './ComparisonTable.vue'
import EstimateSummary from './EstimateSummary.vue'
import LinkPreview from './LinkPreview.vue'
import ProductCard from './ProductCard.vue'
import ProductList from './ProductList.vue'
import SourceQuote from './SourceQuote.vue'
import SpecificationEvent from './SpecificationEvent.vue'
import SpecificationSummary from './SpecificationSummary.vue'

defineProps<{ view: AttachmentView }>()
</script>

<template>
  <ProductCard v-if="view.data.type === 'product_card'" :product="view.data.product" />
  <ProductList v-else-if="view.data.type === 'product_list'" :data="view.data" />
  <ComparisonTable v-else-if="view.data.type === 'comparison'" :data="view.data" />
  <SpecificationEvent
    v-else-if="view.data.type === 'spec_event'"
    :data="view.data"
    :failed="view.failed"
  />
  <SpecificationSummary v-else-if="view.data.type === 'spec_summary'" :data="view.data" />
  <LinkPreview v-else-if="view.data.type === 'link'" :data="view.data" />
  <SourceQuote v-else-if="view.data.type === 'source'" :data="view.data" />
  <EstimateSummary v-else-if="view.data.type === 'estimate'" :data="view.data" />
</template>
