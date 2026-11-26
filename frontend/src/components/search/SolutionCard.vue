<script setup lang="ts">
// «Закупка по сценарию»: запрос сам просит комплект, поэтому карточка шаблона стоит над
// выдачей — единственное исключение из «Правил блока допродажи». Слот: категория, выбранная позиция, цена или «по
// запросу», количество от сервера и «заменить» из присланных альтернатив. Условный слот («ОС, если ноутбук без
// ОС») появляется, только когда выбранная позиция его требует. Ничего не добавляется само: «Добавить всё».
import { computed, ref, watch } from 'vue'
import { Check, ChevronDown, ChevronUp, PackagePlus, RotateCcw, X } from 'lucide-vue-next'
import type { Solution, SolutionSlot } from '@/api/typesP2'
import type { SpecLine } from '@/lib/spec'
import { plural } from '@/lib/format'
import {
  type SlotChoice,
  chosenItem,
  defaultChoice,
  slotActive,
  slotOptions,
  slotQty,
  solutionLines,
  solutionSummary,
} from '@/lib/solution'
import { priceModeLabel, priceModeOf } from '@/lib/spec'
import { useSpecStore } from '@/stores/spec'

const props = defineProps<{ solution: Solution }>()
const emit = defineEmits<{
  apply: [lines: SpecLine[], choice: SlotChoice]
  replace: [slot: string, productId: string]
}>()

const spec = useSpecStore()
const choice = ref<SlotChoice>(defaultChoice(props.solution))
const removed = ref<Set<string>>(new Set())
const openSlot = ref<string | null>(null)
watch(
  () => props.solution,
  (s) => {
    choice.value = defaultChoice(s)
    removed.value = new Set()
    openSlot.value = null
  },
)

const shown = computed(() =>
  props.solution.slots.filter((s) => slotActive(s, props.solution, choice.value)),
)
const empty = computed(() => props.solution.slots.filter((s) => !s.item && !s.condition))
const lines = computed(() => solutionLines(props.solution, choice.value, removed.value))
const summary = computed(() => solutionSummary(props.solution, choice.value, removed.value))
const allInSpec = computed(
  () => lines.value.length > 0 && lines.value.every((l) => spec.has(l.product_id)),
)

const seatsLabel = computed(() => {
  const n = props.solution.seats
  return `${n} ${plural(n, ['место', 'места', 'мест'])}`
})

function priceOf(slot: SolutionSlot) {
  const it = chosenItem(slot, choice.value)
  if (!it) return ''
  return priceModeLabel({ price_rub: it.price_rub, price_mode: priceModeOf(it) })
}

function pick(slot: SolutionSlot, productId: string) {
  choice.value = { ...choice.value, [slot.id]: productId }
  emit('replace', slot.id, productId)
}

function toggleRemoved(id: string) {
  const next = new Set(removed.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  removed.value = next
}

function apply() {
  if (lines.value.length) emit('apply', lines.value, { ...choice.value })
}
</script>

<template>
  <section class="sol panel" aria-labelledby="sol-title">
    <header class="sol__head">
      <p class="t-xs sol__kicker">
        <span class="badge">Комплексная закупка</span>
        <span class="t-muted">{{ solution.source }}</span>
      </p>
      <h2 id="sol-title" class="t-h2">{{ solution.title }}</h2>
      <p class="t-sm t-muted t-num">
        На {{ seatsLabel }} · {{ shown.length }}
        {{ plural(shown.length, ['позиция', 'позиции', 'позиций']) }}
      </p>
    </header>

    <ul class="sol__slots" aria-label="Слоты решения">
      <li
        v-for="s in shown"
        :key="s.id"
        :class="['sol-slot', { 'sol-slot--off': removed.has(s.id) }]"
        :data-slot="s.id"
      >
        <div class="sol-slot__main">
          <p class="t-xs t-muted">{{ s.label }}</p>
          <p class="sol-slot__title">{{ chosenItem(s, choice)?.title }}</p>
          <p class="t-xs t-muted">
            {{ chosenItem(s, choice)?.vendor
            }}<template v-if="chosenItem(s, choice)?.license_line">
              · {{ chosenItem(s, choice)?.license_line }}</template
            >
          </p>
        </div>
        <p class="sol-slot__price t-num">
          <span>{{ priceOf(s) }}</span>
          <span class="t-xs t-muted">× {{ slotQty(s, choice) }}</span>
        </p>
        <div class="sol-slot__actions">
          <button
            v-if="s.alternatives.length"
            type="button"
            class="btn btn--ghost btn--sm"
            :aria-expanded="openSlot === s.id"
            :aria-controls="`sol-alt-${s.id}`"
            :disabled="removed.has(s.id)"
            @click="openSlot = openSlot === s.id ? null : s.id"
          >
            Заменить
            <span class="t-muted t-num">({{ s.alternatives.length }})</span>
            <ChevronUp v-if="openSlot === s.id" :size="14" aria-hidden="true" />
            <ChevronDown v-else :size="14" aria-hidden="true" />
          </button>
          <button
            type="button"
            class="btn btn--ghost btn--icon btn--sm"
            :aria-label="removed.has(s.id) ? `Вернуть: ${s.label}` : `Не нужно: ${s.label}`"
            :title="removed.has(s.id) ? 'Вернуть' : 'Не нужно'"
            @click="toggleRemoved(s.id)"
          >
            <RotateCcw v-if="removed.has(s.id)" :size="14" aria-hidden="true" />
            <X v-else :size="14" aria-hidden="true" />
          </button>
        </div>
        <fieldset v-if="openSlot === s.id" :id="`sol-alt-${s.id}`" class="sol-slot__alts">
          <legend class="sr-only">Заменить: {{ s.label }}</legend>
          <label v-for="o in slotOptions(s)" :key="o.product_id" class="sol-alt">
            <input
              type="radio"
              :name="`sol-${s.id}`"
              :value="o.product_id"
              :checked="chosenItem(s, choice)?.product_id === o.product_id"
              @change="pick(s, o.product_id)"
            />
            <span class="sol-alt__title">
              {{ o.title }}
              <span v-if="o.license_line" class="t-xs t-muted">· {{ o.license_line }}</span>
            </span>
            <span class="sol-alt__price t-sm t-num">
              {{ priceModeLabel({ price_rub: o.price_rub, price_mode: priceModeOf(o) }) }}
              <span class="t-xs t-muted">× {{ s.quantities[o.product_id] ?? s.quantity }}</span>
            </span>
          </label>
        </fieldset>
      </li>
    </ul>

    <p v-if="empty.length" class="t-xs t-muted">
      Не подобрано: {{ empty.map((s) => s.label.toLowerCase()).join(', ') }} — уточните у менеджера.
    </p>

    <footer class="sol__foot">
      <p class="t-sm t-num" role="status">{{ summary.text }}</p>
      <span v-if="allInSpec" class="badge badge--strong">
        <Check :size="12" aria-hidden="true" /> В спецификации
      </span>
      <button
        v-else
        type="button"
        class="btn btn--primary btn--sm"
        :disabled="!lines.length"
        @click="apply"
      >
        <PackagePlus :size="16" aria-hidden="true" />
        Добавить всё в спецификацию
      </button>
    </footer>
  </section>
</template>

<style scoped>
.sol {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
}
.sol__head {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.sol__kicker {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
  margin-bottom: 2px;
}
.sol__slots {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.sol-slot {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  grid-template-areas:
    'main main'
    'price actions'
    'alts alts';
  align-items: center;
  gap: var(--space-1) var(--space-3);
  padding: var(--space-2) var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
}
.sol-slot--off .sol-slot__main,
.sol-slot--off .sol-slot__price {
  opacity: 0.5;
  text-decoration: line-through;
}
.sol-slot__main {
  grid-area: main;
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.sol-slot__title {
  font-weight: var(--fw-semibold);
  overflow-wrap: anywhere;
}
.sol-slot__price {
  grid-area: price;
  display: flex;
  flex-direction: column;
  font-weight: var(--fw-semibold);
}
.sol-slot__actions {
  grid-area: actions;
  display: flex;
  align-items: center;
  justify-self: end;
  gap: var(--space-1);
}
.sol-slot__alts {
  grid-area: alts;
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin: var(--space-1) 0 0;
  padding: var(--space-2) 0 0;
  border: 0;
  border-top: var(--border-w) solid var(--border);
}
.sol-alt {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: 2px var(--space-2);
  align-items: start;
  padding: var(--space-1) 0;
  cursor: pointer;
}
.sol-alt input {
  margin-top: 3px;
}
.sol-alt__title {
  overflow-wrap: anywhere;
}
.sol-alt__price {
  grid-column: 2;
}
.sol__foot {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
}
@media (min-width: 640px) {
  .sol-slot {
    grid-template-columns: minmax(0, 1fr) 9rem auto;
    grid-template-areas:
      'main price actions'
      'alts alts alts';
  }
  .sol-slot__price {
    text-align: right;
  }
  .sol-alt {
    grid-template-columns: auto minmax(0, 1fr) auto;
  }
  .sol-alt__price {
    grid-column: 3;
    text-align: right;
  }
}
</style>
