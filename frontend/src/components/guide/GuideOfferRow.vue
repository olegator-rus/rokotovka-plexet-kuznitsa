<script setup lang="ts">
// Другой вариант компактной строкой: название, отличие или причина, цена за единицу и итог. Нажатие делает его
// главным — крупная карточка и одна кнопка «В спецификацию» остаются на своём месте.
import { computed } from 'vue'
import type { GuideOffer } from '@/api/typesGuide'
import { offerName, offerRole, priceMath } from '@/lib/guide'

const props = defineProps<{ offer: GuideOffer; disabled?: boolean }>()
defineEmits<{ pick: [offer: GuideOffer] }>()

const name = computed(() => offerName(props.offer))
const math = computed(() => priceMath(props.offer))
const sub = computed(() => props.offer.note || name.value.detail)
</script>

<template>
  <article
    class="grow"
    :data-offer="offer.offer_id"
    :data-role="offerRole(offer)"
    data-testid="guide-offer"
  >
    <div class="grow__text">
      <h4 class="grow__name" :title="offer.title">{{ name.name }}</h4>
      <p v-if="sub" class="grow__sub">{{ sub }}</p>
    </div>
    <p class="grow__price t-num">
      <span class="grow__total">{{ math.total || math.unit }}</span>
      <span v-if="math.times" class="grow__unit">{{ math.unit }} {{ math.per }}</span>
      <span v-else-if="math.per" class="grow__unit">{{ math.per }}</span>
    </p>
    <button
      type="button"
      class="grow__pick"
      :disabled="disabled"
      :aria-label="`Показать подробнее: ${name.name}${name.detail ? `, ${name.detail}` : ''}`"
      @click="$emit('pick', offer)"
    ></button>
  </article>
</template>

<style scoped>
.grow {
  position: relative;
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: start;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-control);
  transition: background-color var(--dur-fast) var(--ease-standard);
}
.grow:hover {
  background: var(--surface-2);
}
.grow__name {
  margin: 0;
  overflow: hidden;
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
  font-weight: var(--fw-medium);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.grow__sub {
  margin: 2px 0 0;
  overflow: hidden;
  color: var(--text-2);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.grow__sub::first-letter {
  text-transform: uppercase;
}
.grow__price {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  margin: 0;
}
.grow__total {
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
  font-weight: var(--fw-medium);
}
.grow__unit {
  color: var(--text-3);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
}
.grow__pick {
  position: absolute;
  inset: 0;
  padding: 0;
  border: 0;
  border-radius: inherit;
  background: transparent;
  cursor: pointer;
}
.grow__pick:disabled {
  cursor: default;
}
</style>
