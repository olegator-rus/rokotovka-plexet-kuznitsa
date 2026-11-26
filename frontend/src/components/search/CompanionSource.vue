<script setup lang="ts">
// Основание связи: иконка источника и подпись; ссылка «Основание» — на документ, если он есть.
// «Часто берут вместе» опирается на заказы — рядом плашка «Синтетические данные», если заказы придуманы.
import { computed } from 'vue'
import { BookCheck, Bot, ExternalLink, FileText, ShoppingBasket, UserCheck } from 'lucide-vue-next'
import SyntheticBadge from '@/components/common/SyntheticBadge.vue'
import { sourceLabel } from '@/lib/companions'
import { useAppStore } from '@/stores/app'

const props = defineProps<{ source: string; url?: string; evidence?: string }>()

const ICONS = {
  vendor_documentation: FileText,
  catalog_rule: BookCheck,
  expert: UserCheck,
  co_purchase: ShoppingBasket,
  agent_proposal: Bot,
} as const
const icon = computed(() => ICONS[props.source as keyof typeof ICONS] ?? BookCheck)
const app = useAppStore()
/** Подпись «Часто берут вместе» опирается на заказы: придуманы (meta.synthetic) — плашка рядом. */
const syntheticOrders = computed(
  () => props.source === 'co_purchase' && (app.meta?.synthetic ?? []).includes('orders'),
)
const safeUrl = computed(() => (/^https?:\/\//i.test(props.url ?? '') ? props.url : ''))
</script>

<template>
  <span class="source t-xs">
    <component :is="icon" :size="12" aria-hidden="true" />
    <span>{{ sourceLabel(source) }}</span>
    <a v-if="safeUrl" class="link" :href="safeUrl" target="_blank" rel="noopener noreferrer">
      Основание
      <ExternalLink :size="11" aria-hidden="true" />
      <span class="sr-only">(откроется в новой вкладке)</span>
    </a>
    <SyntheticBadge
      v-if="syntheticOrders"
      hint="Заказы в демо-данных придуманы: «Часто берут вместе» показывает механику. Плашка снимется, когда заказы придут из данных заказчика."
    />
  </span>
</template>

<style scoped>
.source {
  display: inline-flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1) var(--space-2);
  color: var(--text-2);
}
.source .link {
  display: inline-flex;
  align-items: center;
  gap: 2px;
}
</style>
