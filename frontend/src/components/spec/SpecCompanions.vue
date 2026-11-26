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
import { lineFromCompanion } from '@/lib/spec'
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
  <div v-if="shown.length" class="spec-cmp t-xs">
    <span class="t-muted">К этой позиции:</span>
    <button
      v-for="(c, i) in shown"
      :key="c.product_id"
      type="button"
      class="spec-cmp__add"
      :title="c.reason"
      :aria-label="`В спецификацию: ${c.title} (${classLabel(c)})`"
      @click="add(c, i)"
    >
      <Plus :size="12" aria-hidden="true" />
      <span>{{ c.title }}</span>
      <span class="t-muted">· {{ classLabel(c).toLowerCase() }}</span>
    </button>
  </div>
</template>

<style scoped>
.spec-cmp {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1) var(--space-2);
  margin-top: var(--space-1);
}
.spec-cmp__add {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  max-width: 100%;
  padding: 2px var(--space-2);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: none;
  color: var(--text);
  font: inherit;
  text-align: left;
  cursor: pointer;
  overflow-wrap: anywhere;
}
.spec-cmp__add:hover {
  border-color: var(--text-3);
}
</style>
