<script setup lang="ts">
// Строка «условно обязательное» в карточке якоря: одна строка со щитом,
// раскрывается по клику — причина, основание, переключатели «Уже есть подходящие лицензии» и
// «Планируется RDS», количество из запроса (редактируется) и «Добавить в спецификацию». Совместимость
// не подтверждена — товар не подставляется, строка зовёт к менеджеру. Ничего не выбрано заранее.
import { computed, reactive, ref, useId, watch } from 'vue'
import { Check, ChevronDown, ChevronRight, Shield, ShieldCheck } from 'lucide-vue-next'
import type { Companion } from '@/api/typesP2'
import CompanionSource from '@/components/search/CompanionSource.vue'
import UiSwitch from '@/components/ui/UiSwitch.vue'
import { QTY_HINT, isUnconfirmed, requiredHeadline, splitRequired } from '@/lib/companions'
import { clampQty, lineFromCompanion, priceModeLabel } from '@/lib/spec'
import { useSpecStore } from '@/stores/spec'

const props = defineProps<{
  required: Companion[]
  anchorId: string
  anchorTitle: string
  requestId: string | null
}>()

const spec = useSpecStore()
const open = ref(false)
const rds = ref(false)
const bodyId = useId()

const view = computed(() => splitRequired(props.required))
const head = computed(() => view.value.base[0] ?? view.value.rds[0] ?? null)
const shown = computed(() =>
  rds.value ? [...view.value.base, ...view.value.rds] : view.value.base,
)
const qty = reactive<Record<string, string>>({})
// Новый поиск — количество снова из запроса.
watch(
  () => props.required,
  (list) => {
    for (const c of list) {
      const n = c.suggested_quantity
      qty[c.product_id] = n && n > 0 ? String(n) : ''
    }
  },
  { immediate: true },
)

/** «Уже есть» — по якорю: строка сворачивается для всех его позиций. */
const dismissed = computed(() => spec.dismissed.has(props.anchorId))
const headQty = computed(() => {
  const v = head.value ? Number(qty[head.value.product_id]) : NaN
  return Number.isFinite(v) && v > 0 ? v : null
})
const headline = computed(() =>
  dismissed.value
    ? 'Не требуется, по вашим данным'
    : head.value
      ? requiredHeadline(head.value, headQty.value)
      : '',
)

function toggle() {
  open.value = !open.value
  if (open.value && head.value)
    spec.track('companion_click', props.requestId, {
      productId: head.value.product_id,
      position: 0,
      payload: { class: head.value.class, place: 'required' },
    })
}

function setDismissed(on: boolean) {
  spec.setDismissed(props.anchorId, on)
  if (on && head.value) {
    spec.track('companion_dismiss', props.requestId, {
      productId: head.value.product_id,
      payload: {
        condition: head.value.conditions?.[0] ?? 'no_existing_equivalent_license',
        anchor: props.anchorId,
      },
    })
    open.value = false
  }
}

function qtyOf(c: Companion): number | null {
  const v = Number(qty[c.product_id])
  return qty[c.product_id] && Number.isFinite(v) && v > 0 ? clampQty(v) : null
}

function add(c: Companion) {
  const n = qtyOf(c)
  if (n === null) return
  spec.add(lineFromCompanion(c, n, props.anchorTitle))
  spec.track('companion_add', props.requestId, {
    productId: c.product_id,
    payload: { class: c.class, quantity: n, place: 'required', anchor: props.anchorId },
  })
}
</script>

<template>
  <div v-if="head" :class="['req', { 'req--dismissed': dismissed }]">
    <button
      type="button"
      class="req__head"
      :aria-expanded="open"
      :aria-controls="bodyId"
      @click="toggle"
    >
      <component :is="dismissed ? ShieldCheck : Shield" :size="14" aria-hidden="true" />
      <span class="req__text">{{ headline }}</span>
      <component :is="open ? ChevronDown : ChevronRight" :size="14" aria-hidden="true" />
    </button>

    <div v-if="open" :id="bodyId" class="req__body">
      <p v-if="head.reason" class="t-sm">{{ head.reason }}</p>
      <CompanionSource :source="head.source" :url="head.source_url" />

      <div class="req__toggles">
        <UiSwitch
          label="Уже есть подходящие лицензии"
          :model-value="dismissed"
          @update:model-value="setDismissed"
        />
        <UiSwitch
          v-if="view.rds.length && !dismissed"
          v-model="rds"
          label="Планируется удалённый рабочий стол (RDS)"
        />
      </div>

      <ul v-if="!dismissed" class="req__list">
        <li v-for="c in shown" :key="c.product_id" class="req__row">
          <div class="req__main">
            <p class="req__title">{{ isUnconfirmed(c) ? requiredHeadline(c, null) : c.title }}</p>
            <p v-if="!isUnconfirmed(c)" class="t-xs t-muted">
              {{ c.vendor }}<template v-if="c.license_line"> · {{ c.license_line }}</template>
            </p>
          </div>
          <template v-if="!isUnconfirmed(c)">
            <span class="req__price t-num t-sm">{{ priceModeLabel(c) }}</span>
            <label class="req__qty">
              <span class="sr-only">Количество: {{ c.title }}</span>
              <input
                v-model="qty[c.product_id]"
                class="req__input t-num"
                type="number"
                inputmode="numeric"
                min="1"
                :placeholder="QTY_HINT"
              />
            </label>
            <span v-if="spec.has(c.product_id)" class="badge badge--strong">
              <Check :size="12" aria-hidden="true" /> В спецификации
            </span>
            <button
              v-else
              type="button"
              class="btn btn--secondary btn--sm"
              :disabled="qtyOf(c) === null"
              :title="qtyOf(c) === null ? `Укажите количество: ${QTY_HINT}` : undefined"
              @click="add(c)"
            >
              Добавить в спецификацию
            </button>
          </template>
        </li>
      </ul>
    </div>
  </div>
</template>

<style scoped>
.req {
  margin: 0 var(--space-3) var(--space-3) calc(var(--space-4) + var(--icon-sm));
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
  background: var(--surface-2);
}
.req__head {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  width: 100%;
  min-height: var(--control-h);
  padding: var(--space-1) var(--space-3);
  border: 0;
  background: none;
  color: var(--text);
  font: inherit;
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
  text-align: left;
  cursor: pointer;
}
.req__head svg {
  flex: none;
  color: var(--text-2);
}
.req__text {
  flex: 1;
  min-width: 0;
  overflow-wrap: anywhere;
}
.req--dismissed .req__text {
  color: var(--text-2);
}
.req__body {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3) var(--space-3);
  border-top: var(--border-w) solid var(--border);
}
.req__toggles {
  display: flex;
  flex-wrap: wrap;
  gap: 0 var(--space-4);
}
.req__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.req__row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2) var(--space-3);
}
.req__main {
  flex: 1 1 14rem;
  min-width: 0;
}
.req__title {
  font-size: var(--fs-sm);
  font-weight: var(--fw-medium);
  overflow-wrap: anywhere;
}
.req__price {
  white-space: nowrap;
}
.req__input {
  box-sizing: border-box;
  width: 12rem;
  max-width: 100%;
  min-height: var(--control-h-sm);
  padding: 0 var(--space-2);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-control);
  background: var(--bg);
  color: var(--text);
  font: inherit;
  font-size: var(--fs-sm);
}
.req__input::placeholder {
  color: var(--text-3);
  font-size: var(--fs-xs);
}
@media (max-width: 479px) {
  .req {
    margin-left: var(--space-3);
  }
}
</style>
