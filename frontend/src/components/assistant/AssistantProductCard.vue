<script setup lang="ts">
// Карточка товара помощника (CHAT.md §4): одна пометка, человеческое название, условия, полная стоимость под условия
// покупателя и расчёт под ней, причина, акция, действие. Все тексты и числа — с сервера.
import { ref } from 'vue'
import { Check, ExternalLink } from 'lucide-vue-next'
import type { ChatCard } from '@/api/typesGuide'

const props = defineProps<{
  card: ChatCard
  inSpec?: boolean
  adding?: boolean
  failed?: boolean
  /** Условия карточки устарели (покупатель их изменил) — «Предыдущие условия». */
  stale?: boolean
}>()
const emit = defineEmits<{ spec: [card: ChatCard]; quote: [card: ChatCard] }>()
const more = ref(false)

function act() {
  if (props.card.action === 'request_quote') emit('quote', props.card)
  else emit('spec', props.card)
}
</script>

<template>
  <article
    :class="['as-card', { 'as-card--off': card.unsuitable }]"
    data-testid="assistant-card"
    :data-offer="card.offer_id"
  >
    <p
      v-if="card.badge"
      :class="['as-card__badge', { 'as-card__badge--off': card.unsuitable }]"
      data-testid="assistant-badge"
    >
      {{ card.badge.text }}
    </p>
    <div class="as-card__head">
      <h4 class="as-card__title">{{ card.title }}</h4>
      <p class="as-card__price t-num" data-testid="assistant-price">{{ card.price_text }}</p>
    </div>
    <div class="as-card__meta">
      <span class="as-card__sub">{{ card.subtitle }}</span>
      <span v-if="card.calc" class="as-card__calc t-num" data-testid="assistant-calc">{{
        card.calc
      }}</span>
    </div>
    <p v-if="card.promo" class="as-card__promo">
      <span class="badge badge--tone">Акционная цена</span>
      <s class="t-num">{{ card.old_price_text }}</s>
    </p>
    <p v-if="card.note" class="as-card__note">{{ card.note }}</p>
    <p v-if="card.reason" class="as-card__reason" data-testid="assistant-reason">
      {{ card.reason }}
    </p>
    <p v-if="stale" class="as-card__stale">Предыдущие условия</p>
    <div class="as-card__foot">
      <button type="button" class="as-card__more" :aria-expanded="more" @click="more = !more">
        Подробнее
      </button>
      <a
        v-if="card.action === 'open_store'"
        class="btn btn--secondary btn--sm"
        :href="card.offer_url"
        target="_blank"
        rel="noopener"
        data-testid="assistant-store"
      >
        Открыть в магазине <ExternalLink :size="14" aria-hidden="true" />
      </a>
      <span v-else-if="inSpec" class="as-card__done" data-testid="assistant-in-spec">
        <Check :size="16" :stroke-width="2.5" aria-hidden="true" /> В спецификации
      </span>
      <button
        v-else
        type="button"
        :class="[
          'btn btn--sm',
          card.action === 'request_quote' ? 'btn--secondary' : 'btn--primary',
        ]"
        :disabled="adding"
        data-testid="assistant-action"
        @click="act"
      >
        {{
          adding
            ? 'Добавляю…'
            : card.action === 'request_quote'
              ? 'Запросить цену'
              : 'В спецификацию'
        }}
      </button>
    </div>
    <p v-if="failed" class="as-card__fail" role="alert">
      Не удалось добавить.
      <button type="button" class="link" @click="act">Повторить</button>
    </p>
    <dl v-if="more" class="as-card__details" data-testid="assistant-details">
      <template v-for="d in card.details" :key="d.label">
        <dt>{{ d.label }}</dt>
        <dd>{{ d.value }}</dd>
      </template>
      <dt class="sr-only">Страница</dt>
      <dd class="as-card__store">
        <a :href="card.offer_url" target="_blank" rel="noopener" class="link">Открыть в магазине</a>
      </dd>
    </dl>
  </article>
</template>

<style scoped>
.as-card {
  min-width: 0;
  overflow-wrap: anywhere;
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding: var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
}
.as-card--off {
  background: var(--surface-2);
}
.as-card__badge {
  align-self: flex-start;
  margin: 0;
  padding: 1px var(--space-2);
  border-radius: var(--radius-pill);
  background: var(--status-accepted-bg);
  color: var(--status-accepted);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
  font-weight: var(--fw-semibold);
}
.as-card__badge--off {
  background: var(--danger-bg);
  color: var(--danger);
}
.as-card__head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--space-3);
}
.as-card__title {
  margin: 0;
  font-size: 14px;
  line-height: 19px;
  font-weight: var(--fw-semibold);
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
  overflow-wrap: anywhere;
}
.as-card__price {
  flex: none;
  margin: 0;
  font-size: 18px;
  line-height: 24px;
  font-weight: var(--fw-bold);
  white-space: nowrap;
}
.as-card--off .as-card__price {
  color: var(--text-3);
  font-weight: var(--fw-medium);
}
.as-card__meta {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: 0 var(--space-3);
  color: var(--text-2);
  font-size: 12px;
  line-height: 16px;
}
.as-card__calc {
  margin-left: auto;
}
.as-card__promo,
.as-card__note,
.as-card__reason,
.as-card__stale,
.as-card__fail {
  margin: 0;
  font-size: 12px;
  line-height: 16px;
}
.as-card__promo {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  color: var(--text-3);
}
.as-card__note {
  color: var(--text-2);
  font-variant-numeric: tabular-nums;
}
.as-card__reason {
  color: var(--text);
  font-weight: var(--fw-medium);
}
.as-card--off .as-card__reason {
  color: var(--danger);
}
.as-card__stale {
  color: var(--text-3);
}
.as-card__fail {
  color: var(--danger);
}
.as-card__foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
  margin-top: var(--space-1);
}
.as-card__more {
  padding: 0;
  border: 0;
  background: none;
  color: var(--text-2);
  font: inherit;
  font-size: var(--fs-sm);
  text-decoration: underline;
  text-underline-offset: 2px;
  cursor: pointer;
}
.as-card__done {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  color: var(--status-accepted);
  font-size: var(--fs-sm);
  font-weight: var(--fw-semibold);
}
.as-card__details {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 2px var(--space-3);
  margin: var(--space-2) 0 0;
  font-size: 12px;
  line-height: 16px;
}
.as-card__details dt {
  color: var(--text-3);
}
.as-card__details dd {
  margin: 0;
  overflow-wrap: anywhere;
}
.as-card__store {
  grid-column: 1 / -1;
}
@media (max-width: 1023px) {
  .as-card__head {
    flex-direction: column;
    gap: 0;
  }
  .as-card__calc {
    margin-left: 0;
  }
  .as-card__foot .btn,
  .as-card__more {
    min-height: var(--touch-min);
  }
}
</style>
