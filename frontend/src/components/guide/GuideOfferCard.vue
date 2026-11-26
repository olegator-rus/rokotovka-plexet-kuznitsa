<script setup lang="ts">
// Карточка подобранной лицензии (SPEC §8–§9): артикул, ступень, цена за единицу, итог, состояние цены. Все числа —
// из ответа сервера по каталогу; «0 ₽» не выводится никогда (lib/guide.ts, offerPrice).
import { computed } from 'vue'
import { Check, ExternalLink, FilePlus2 } from 'lucide-vue-next'
import type { GuideOffer } from '@/api/typesGuide'
import { offerPrice, offerRole, perUnit } from '@/lib/guide'

const props = withDefaults(
  defineProps<{ offer: GuideOffer; inSpec: boolean; primary?: boolean; showSpec?: boolean }>(),
  { primary: false, showSpec: true },
)
defineEmits<{ spec: [offer: GuideOffer] }>()

const price = computed(() => offerPrice(props.offer))
</script>

<template>
  <article
    :class="['goffer', { 'goffer--primary': primary }]"
    :data-offer="offer.offer_id"
    :data-role="offerRole(offer)"
    data-testid="guide-offer"
  >
    <div class="goffer__main">
      <p v-if="offerRole(offer) === 'alternative'" class="goffer__alt t-xs">Альтернатива</p>
      <h4 class="goffer__title">
        <a
          v-if="offer.offer_url"
          :href="offer.offer_url"
          class="link"
          target="_blank"
          rel="noopener noreferrer"
        >
          {{ offer.title }} <ExternalLink :size="12" aria-hidden="true" />
          <span class="sr-only">(откроется в новой вкладке)</span>
        </a>
        <template v-else>{{ offer.title }}</template>
      </h4>
      <dl class="goffer__facts t-sm">
        <div v-if="offer.sku">
          <dt class="t-muted">Артикул</dt>
          <dd class="t-mono" data-testid="guide-offer-sku">{{ offer.sku }}</dd>
        </div>
        <div v-if="offer.tier">
          <dt class="t-muted">Ступень</dt>
          <dd class="t-num">{{ offer.tier }}</dd>
        </div>

        <div v-if="offer.term">
          <dt class="t-muted">Срок</dt>
          <dd>{{ offer.term }}</dd>
        </div>
      </dl>
      <p v-if="offer.note" class="t-sm t-muted goffer__note">{{ offer.note }}</p>
    </div>
    <div class="goffer__buy">
      <p class="goffer__unit t-num" data-testid="guide-offer-unit">
        {{ price.unit }}
        <span v-if="price.known && perUnit(offer.unit)" class="t-sm t-muted">{{
          perUnit(offer.unit)
        }}</span>
      </p>
      <p v-if="price.total" class="goffer__total t-num" data-testid="guide-offer-total">
        Итого {{ price.total }}
      </p>
      <p v-if="price.state && price.known" class="t-xs t-muted">{{ price.state }}</p>
      <button
        v-if="showSpec"
        type="button"
        :class="['btn btn--sm', primary ? 'btn--primary' : 'btn--secondary']"
        :disabled="offer.price_state === 'unavailable'"
        data-testid="guide-offer-spec"
        @click="$emit('spec', offer)"
      >
        <Check v-if="inSpec" :size="16" aria-hidden="true" />
        <FilePlus2 v-else :size="16" aria-hidden="true" />
        {{ inSpec ? 'В спецификации' : price.known ? 'В спецификацию' : 'Добавить в запрос КП' }}
      </button>
    </div>
  </article>
</template>

<style scoped>
.goffer {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-4);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
}
.goffer--primary {
  border-color: var(--border-control);
  box-shadow: inset 3px 0 0 var(--accent);
}
.goffer__main {
  flex: 1 1 16rem;
  min-width: 0;
}
.goffer__alt {
  display: inline-block;
  margin: 0 0 var(--space-1);
  padding: 0 var(--space-2);
  border-radius: var(--radius-pill);
  background: var(--surface-2);
  color: var(--text-2);
  font-weight: var(--fw-medium);
}
.goffer__title {
  margin: 0 0 var(--space-2);
  font-size: var(--fs-body);
  line-height: var(--lh-body);
  font-weight: var(--fw-semibold);
  overflow-wrap: anywhere;
}
.goffer__title svg {
  display: inline-block;
  vertical-align: -1px;
}
.goffer__facts {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-4);
  margin: 0;
}
.goffer__facts div {
  display: flex;
  gap: var(--space-1);
}
.goffer__facts dd {
  margin: 0;
}
.goffer__note {
  margin: var(--space-2) 0 0;
}
.goffer__buy {
  display: flex;
  flex: 0 1 auto;
  flex-direction: column;
  align-items: flex-end;
  gap: var(--space-1);
  text-align: right;
}
.goffer__unit {
  margin: 0;
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
  font-weight: var(--fw-semibold);
}
.goffer__total {
  margin: 0;
  font-size: var(--fs-sm);
  font-weight: var(--fw-medium);
}
@media (max-width: 40rem) {
  .goffer__buy {
    align-items: flex-start;
    width: 100%;
    text-align: left;
  }
}
</style>
