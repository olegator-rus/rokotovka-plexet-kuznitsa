<script setup lang="ts">
// Тело карточки комплекта (BUNDLE_OPPORTUNITY, action: bundle): якорь, позиции
// с классом, количеством, совместимостью и условием, маржа комплекта; дальше — те же доказательства, что у связи.
// Принять = «взять в работу»: справочник и выдача не меняются.
import { computed } from 'vue'
import { Package } from 'lucide-vue-next'
import type { Proposal } from '@/api/types'
import type { BundleCard } from '@/api/typesKiller'
import { bundleView } from '@/lib/killer'
import CopurchaseEvidence from './CopurchaseEvidence.vue'

const props = defineProps<{ proposal: Proposal }>()
const b = computed(() => {
  const patch = props.proposal.patch as unknown as { bundle?: BundleCard } | null
  const ev = props.proposal.evidence as unknown as { bundle?: BundleCard } | null
  return bundleView(patch?.bundle ?? ev?.bundle)
})
</script>

<template>
  <section v-if="b" class="bv" aria-label="Комплект">
    <p class="bv__label">Комплект</p>
    <div class="bv__anchor">
      <Package :size="16" aria-hidden="true" />
      <span class="bv__title">{{ b.anchor.title }}</span>
      <span class="t-sm t-muted"
        >якорь · {{ b.anchor.price
        }}<template v-if="b.anchor.margin"> · маржа {{ b.anchor.margin }}</template></span
      >
    </div>
    <ul class="bv__items" aria-label="Позиции комплекта">
      <li v-for="r in b.rows" :key="r.id" class="bv__item">
        <span class="bv__title">{{ r.title }}</span>
        <span class="t-sm">
          <span class="badge">{{ r.cls }}</span>
          × {{ r.quantity }} · {{ r.price
          }}<template v-if="r.margin"> · маржа {{ r.margin }}</template>
        </span>
        <span class="t-sm">
          {{ r.compat }}<template v-if="r.condition">: {{ r.condition }}</template>
        </span>
        <span class="t-xs t-muted"
          >{{ r.relation }}<template v-if="r.stats"> · {{ r.stats }}</template></span
        >
      </li>
    </ul>
    <p class="bv__total t-sm">
      <span
        >Сумма комплекта: <strong class="t-num">{{ b.total }}</strong></span
      >
      <span v-if="b.marginTotal"
        >Маржа комплекта: <strong class="t-num">{{ b.marginTotal }}</strong></span
      >
    </p>
  </section>
  <p v-else class="t-sm t-muted">Состав комплекта не приложен</p>
  <CopurchaseEvidence :proposal="proposal" />
</template>

<style scoped>
.bv {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.bv__label {
  color: var(--text-2);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
  font-weight: var(--fw-medium);
}
.bv__anchor {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: var(--space-1) var(--space-2);
}
.bv__anchor svg {
  align-self: center;
  color: var(--text-2);
}
.bv__title {
  font-weight: var(--fw-medium);
  overflow-wrap: anywhere;
}
.bv__items {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin: 0;
  padding: 0;
  list-style: none;
}
.bv__item {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-sm);
  background: var(--surface-2);
  overflow-wrap: anywhere;
}
.bv__total {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-4);
}
</style>
