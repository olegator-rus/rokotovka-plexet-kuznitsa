<script lang="ts">
import type { Companion as CompanionT } from '@/api/typesP2'
// Ответ один на позицию за жизнь вкладки: справочник связей между открытиями панели не меняется.
const cache = new Map<string, CompanionT[]>()
</script>

<script setup lang="ts">
// «К этой позиции: …» в спецификации: позиция добавлена из обычной (категорийной)
// выдачи, где блока «Дополните решение» нет, — дополнения приходят после выбора товара. GET /api/companions;
// ошибка или пусто — строки нет (служебная подсказка не мешает спецификации). Ничего не добавляется само.
import { computed, onMounted, ref } from 'vue'
import { Plus } from 'lucide-vue-next'
import { getProductCompanions } from '@/api/companions'
import type { Companion } from '@/api/typesP2'
import { classLabel } from '@/lib/companions'
import { lineFromCompanion, priceModeLabel } from '@/lib/spec'
import { useSpecStore } from '@/stores/spec'

const props = defineProps<{ productId: string; title: string }>()
const spec = useSpecStore()

const items = ref<Companion[]>(cache.get(props.productId) ?? [])

onMounted(async () => {
  if (cache.has(props.productId)) return
  try {
    const r = await getProductCompanions(props.productId, 0)
    const list = [...(r.required ?? []), ...(r.items ?? [])].filter(
      (c) => c.compatibility !== 'incompatible',
    )
    cache.set(props.productId, list)
    items.value = list
  } catch {
    cache.set(props.productId, [])
  }
})

const shown = computed(() => items.value.filter((c) => !spec.has(c.product_id)).slice(0, 3))

// Только первая буква строчная: в подписи бывают названия моделей («Совместимо с Dell PowerEdge R740»).
function lowerFirst(s: string): string {
  return s.charAt(0).toLowerCase() + s.slice(1)
}

function add(c: Companion, index: number) {
  const qty = c.suggested_quantity ?? 1
  spec.add(lineFromCompanion(c, qty, props.title))
  spec.track('companion_add', null, {
    productId: c.product_id,
    position: index + 1,
    payload: { class: c.class, quantity: qty, place: 'spec' },
  })
}
</script>

<template>
  <div v-if="shown.length" class="spec-cmp">
    <p class="spec-cmp__label t-xs">К этой позиции</p>
    <button
      v-for="(c, i) in shown"
      :key="c.product_id"
      type="button"
      class="spec-cmp__add"
      :title="c.reason"
      :aria-label="`В спецификацию: ${c.title} (${classLabel(c)})`"
      @click="add(c, i)"
    >
      <span class="spec-cmp__text">
        <span class="t-sm spec-cmp__title">{{ c.title }}</span>
        <span class="t-xs t-muted">
          <strong v-if="c.price_rub != null && c.price_rub > 0" class="spec-cmp__price t-num">
            {{ priceModeLabel({ price_rub: c.price_rub, price_mode: c.price_mode || 'fixed' }) }}
          </strong>
          {{ lowerFirst(classLabel(c)) }}
        </span>
      </span>
      <span class="spec-cmp__plus" aria-hidden="true"><Plus :size="16" /></span>
    </button>
  </div>
</template>

<style scoped>
.spec-cmp {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding-top: var(--space-3);
  border-top: var(--border-w) dashed var(--border);
}
.spec-cmp__label {
  margin: 0;
  color: var(--text-3);
  font-weight: var(--fw-semibold);
}
.spec-cmp__add {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  width: 100%;
  padding: var(--space-2) var(--space-2) var(--space-2) var(--space-3);
  border: var(--border-w) solid transparent;
  border-radius: var(--radius-control);
  background: var(--surface-2);
  color: var(--text);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.spec-cmp__add:hover {
  border-color: var(--accent);
  background: var(--accent-soft);
}
.spec-cmp__text {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.spec-cmp__title {
  overflow-wrap: anywhere;
}
.spec-cmp__price {
  margin-right: var(--space-2);
  color: var(--text);
  font-weight: var(--fw-bold);
}
.spec-cmp__plus {
  display: grid;
  flex: none;
  place-items: center;
  width: var(--control-h-sm);
  height: var(--control-h-sm);
  border-radius: var(--radius-pill);
  background: var(--surface);
  color: var(--accent);
}
.spec-cmp__add:hover .spec-cmp__plus {
  background: var(--accent);
  color: var(--on-accent);
}
</style>
