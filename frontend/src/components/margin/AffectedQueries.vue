<script setup lang="ts">
// Затронутые запросы выбранного положения: сменившие топ-1 или топ-3 и худшие по потере
// качества. Раскрытый запрос — две колонки «было / стало», переставленные позиции со стрелкой.
import { computed, ref } from 'vue'
import { CollapsibleContent, CollapsibleRoot, CollapsibleTrigger } from 'reka-ui'
import { ArrowDown, ArrowUp, ChevronDown, ChevronRight, Plus } from 'lucide-vue-next'
import type { MarginAffected } from '@/api/typesCommerce'
import { formatDecimal } from '@/lib/format'
import { AFFECTED_REASON_LABELS, guardLabel, rankColumns, weightLabel } from '@/lib/margin'

const props = defineProps<{ items: MarginAffected[]; limit?: number; base?: number }>()
const openQ = ref<string | null>(null)
const showAll = ref(false)
const shown = computed(() =>
  showAll.value ? props.items : props.items.slice(0, props.limit ?? 10),
)
const cols = (a: MarginAffected) => rankColumns(a)
const moveIcon = (m: string) =>
  m === 'up' ? ArrowUp : m === 'down' ? ArrowDown : m === 'new' ? Plus : null
const moveText = (m: string) =>
  m === 'up' ? 'поднялась' : m === 'down' ? 'опустилась' : m === 'new' ? 'новая в десятке' : ''
</script>

<template>
  <section class="aff" aria-labelledby="aff-title">
    <h2 id="aff-title" class="t-h2">Затронутые запросы</h2>
    <p v-if="!items.length" class="panel aff__empty t-sm t-muted">
      На этом положении ни один запрос набора не меняет первую тройку — выдача как при весе
      {{ weightLabel(base ?? 0.05) }}.
    </p>
    <ul v-else class="aff__list" aria-label="Затронутые запросы">
      <CollapsibleRoot
        v-for="a in shown"
        :key="a.query"
        as="li"
        class="panel aff__item"
        :open="openQ === a.query"
        @update:open="(v: boolean) => (openQ = v ? a.query : null)"
      >
        <CollapsibleTrigger class="aff__head">
          <component
            :is="openQ === a.query ? ChevronDown : ChevronRight"
            :size="14"
            aria-hidden="true"
          />
          <span class="aff__q">{{ a.query }}</span>
          <span v-if="a.reason" class="badge">{{
            AFFECTED_REASON_LABELS[a.reason] ?? a.reason
          }}</span>
          <span v-if="typeof a.ndcg_delta === 'number'" class="t-xs t-muted t-num aff__d">
            ΔNDCG@10 {{ formatDecimal(a.ndcg_delta, 3) }}
          </span>
        </CollapsibleTrigger>
        <CollapsibleContent class="aff__body">
          <div class="aff__cols">
            <div>
              <p class="t-xs t-muted aff__colh">Было (вес {{ weightLabel(base ?? 0.05) }})</p>
              <ol class="aff__rank">
                <li v-for="r in cols(a).before" :key="r.id">
                  <span class="t-num t-muted">{{ r.pos }}</span>
                  <span class="aff__title">{{ r.title }}</span>
                </li>
              </ol>
            </div>
            <div>
              <p class="t-xs t-muted aff__colh">Стало</p>
              <ol class="aff__rank">
                <li
                  v-for="r in cols(a).after"
                  :key="r.id"
                  :class="{ aff__moved: r.move !== 'same' }"
                >
                  <span class="t-num t-muted">{{ r.pos }}</span>
                  <span class="aff__title">{{ r.title }}</span>
                  <span v-if="r.move !== 'same'" class="aff__move t-xs">
                    <component :is="moveIcon(r.move)" :size="12" aria-hidden="true" />
                    {{ moveText(r.move) }}<template v-if="r.otherPos"> с {{ r.otherPos }}</template>
                  </span>
                </li>
              </ol>
            </div>
          </div>
          <p v-if="a.guards?.length" class="t-xs t-muted">
            Защиты: {{ a.guards.map(guardLabel).join(', ') }}
          </p>
        </CollapsibleContent>
      </CollapsibleRoot>
    </ul>
    <button
      v-if="items.length > (limit ?? 10)"
      type="button"
      class="link t-sm aff__more"
      @click="showAll = !showAll"
    >
      {{ showAll ? 'Свернуть' : `Показать все ${items.length}` }}
    </button>
  </section>
</template>

<style scoped>
.aff {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.aff__empty {
  padding: var(--space-3) var(--space-4);
}
.aff__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.aff__item {
  padding: 0;
}
.aff__head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1) var(--space-2);
  width: 100%;
  min-height: var(--touch-min);
  padding: var(--space-2) var(--space-3);
  border: 0;
  background: none;
  color: var(--text);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.aff__head:hover {
  background: var(--surface-2);
}
.aff__q {
  font-weight: var(--fw-medium);
  overflow-wrap: anywhere;
}
.aff__d {
  margin-left: auto;
}
.aff__body {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: 0 var(--space-3) var(--space-3);
}
.aff__cols {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(14rem, 1fr));
  gap: var(--space-3);
}
.aff__colh {
  margin-bottom: var(--space-1);
}
.aff__rank {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  padding: 0;
  list-style: none;
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.aff__rank li {
  display: grid;
  grid-template-columns: 1.5rem minmax(0, 1fr);
  gap: 0 var(--space-2);
  padding: 2px var(--space-1);
  border-radius: var(--radius-sm);
}
.aff__moved {
  background: var(--surface-2);
}
.aff__title {
  overflow-wrap: anywhere;
}
.aff__move {
  grid-column: 2;
  display: inline-flex;
  align-items: center;
  gap: 2px;
  color: var(--text-2);
}
.aff__more {
  align-self: flex-start;
}
</style>
