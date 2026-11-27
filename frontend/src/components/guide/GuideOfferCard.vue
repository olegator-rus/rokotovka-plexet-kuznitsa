<script setup lang="ts">
// Подобранная лицензия крупно (SPEC §8–§9): понятное название из семейства и редакции, отличие позиции, артикул,
// ступень, срок и расчёт «цена за единицу × количество = итог». Все числа — из ответа сервера по каталогу; «0 ₽» не
// выводится (lib/guide.ts, priceMath). Кнопки — у карточки-обёртки в GuidePanel.
import { computed } from 'vue'
import { ExternalLink } from 'lucide-vue-next'
import type { GuideOffer } from '@/api/typesGuide'
import { formatInt } from '@/lib/format'
import { offerName, offerRole, packText, priceMath } from '@/lib/guide'

const props = defineProps<{ offer: GuideOffer }>()

const name = computed(() => offerName(props.offer))
const math = computed(() => priceMath(props.offer))
</script>

<template>
  <article
    class="goffer"
    :data-offer="offer.offer_id"
    :data-role="offerRole(offer)"
    data-testid="guide-offer"
  >
    <h4 class="goffer__name">{{ name.name }}</h4>
    <p v-if="name.detail" class="goffer__detail">{{ name.detail }}</p>
    <p v-if="offer.note" class="goffer__note">{{ offer.note }}</p>
    <dl class="goffer__facts">
      <div v-if="offer.sku">
        <dt>Артикул</dt>
        <dd class="goffer__sku" data-testid="guide-offer-sku">{{ offer.sku }}</dd>
      </div>
      <div v-if="offer.tier">
        <dt>Ступень</dt>
        <dd class="t-num">{{ offer.tier }}</dd>
      </div>
      <div v-if="offer.term">
        <dt>Срок</dt>
        <dd>{{ offer.term }}</dd>
      </div>
    </dl>

    <div class="goffer__price">
      <template v-if="math.times">
        <p class="goffer__math t-num" data-testid="guide-offer-unit">
          {{ math.unit }} {{ math.per }} <span aria-hidden="true">×</span
          ><span class="sr-only">, количество</span>
          {{ formatInt(math.times) }}
        </p>
        <p class="goffer__sum t-num" data-testid="guide-offer-total">
          <span class="sr-only">Итого </span>{{ math.total }}
        </p>
      </template>
      <p v-else class="goffer__sum goffer__sum--unit t-num" data-testid="guide-offer-unit">
        {{ math.unit }}
        <span v-if="math.per" class="goffer__per">{{ math.per }}</span>
      </p>
      <p v-if="math.pack" class="goffer__pack" data-testid="guide-offer-pack">
        {{ packText(math.pack) }}
      </p>
      <a
        v-if="offer.offer_url"
        :href="offer.offer_url"
        class="goffer__site"
        target="_blank"
        rel="noopener noreferrer"
        :title="offer.title"
      >
        В магазине <ExternalLink :size="12" aria-hidden="true" />
        <span class="sr-only">(откроется в новой вкладке)</span>
      </a>
    </div>
  </article>
</template>

<style scoped>
.goffer {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.goffer__name {
  margin: 0;
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
  font-weight: var(--fw-semibold);
  overflow-wrap: anywhere;
}
.goffer__detail {
  margin: 2px 0 0;
  color: var(--text-2);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
  overflow-wrap: anywhere;
}
.goffer__detail::first-letter {
  text-transform: uppercase;
}
.goffer__note {
  margin: var(--space-1) 0 0;
  color: var(--text-2);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.goffer__facts {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-4);
  margin: var(--space-3) 0 0;
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.goffer__facts div {
  display: flex;
  gap: var(--space-1);
}
.goffer__facts dt {
  color: var(--text-3);
}
.goffer__facts dd {
  margin: 0;
}
.goffer__sku {
  font-variant-numeric: tabular-nums;
  letter-spacing: 0.02em;
}
.goffer__price {
  display: grid;
  grid-template-columns: 1fr auto;
  align-items: end;
  gap: 0 var(--space-3);
  margin-top: var(--space-4);
  padding-top: var(--space-3);
  border-top: var(--border-w) solid var(--border);
}
.goffer__math {
  grid-column: 1;
  margin: 0;
  color: var(--text-2);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
  white-space: normal;
}
.goffer__sum {
  grid-column: 1;
  margin: 2px 0 0;
  font-size: 1.5rem;
  line-height: 2rem;
  font-weight: var(--fw-semibold);
  letter-spacing: -0.01em;
}
.goffer__sum--unit {
  margin: 0;
}
.goffer__per {
  margin-left: var(--space-1);
  color: var(--text-2);
  font-size: var(--fs-sm);
  font-weight: var(--fw-regular);
  letter-spacing: 0;
}
.goffer__pack {
  grid-column: 1;
  margin: 2px 0 0;
  color: var(--text-3);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
}
.goffer__site {
  grid-column: 2;
  grid-row: 1 / span 3;
  align-self: end;
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  padding-bottom: var(--space-1);
  color: var(--text-2);
  font-size: var(--fs-sm);
  text-decoration: none;
  white-space: nowrap;
}
.goffer__site:hover {
  color: var(--text);
  text-decoration: underline;
  text-underline-offset: 3px;
}
</style>
