<script setup lang="ts">
// Блок подбора над выдачей (контур подбора, SPEC §1–§3, §6, §8–§11). Сворачивается и закрывается, выдачу и
// спецификацию не трогает. Порядок: условия → статус хода → фраза → таблица или карточки → следующий шаг.
// Цены, артикулы, строки таблицы приходят структурой; сырой текст модели не выводится.
import { computed, nextTick, ref, watch } from 'vue'
import {
  ChevronDown,
  ChevronUp,
  CornerDownLeft,
  FileText,
  ListChecks,
  LoaderCircle,
  Pencil,
  RotateCcw,
  X,
} from 'lucide-vue-next'
import type { GuideChipAnswer, GuideCondition, GuideOffer } from '@/api/typesGuide'
import type { GuideStatus } from '@/composables/useGuide'
import type { GuideResponse } from '@/api/typesGuide'
import type { ApiError } from '@/lib/apiError'
import { formatMs, plural } from '@/lib/format'
import {
  type ChipView,
  chipView,
  conditionAria,
  hasTable,
  isOtherChip,
  isRefusal,
  priceWithUnit,
  splitOffers,
  originLabel,
  specOffers,
} from '@/lib/guide'
import GuideOfferCard from './GuideOfferCard.vue'
import GuideTable from './GuideTable.vue'

const props = defineProps<{
  status: GuideStatus
  response: GuideResponse | null
  error: ApiError | null
  pendingText: string
  inSpec: (offerId: string) => boolean
}>()
const collapsed = defineModel<boolean>('collapsed', { default: false })
const emit = defineEmits<{
  close: []
  answer: [chip: GuideChipAnswer]
  say: [message: string]
  edit: [condition: GuideCondition, value: string | null]
  retry: []
  spec: [offer: GuideOffer]
  quote: []
}>()

const r = computed(() => props.response)
const loading = computed(() => props.status === 'loading')
const conditions = computed(() => r.value?.conditions ?? [])
const split = computed(() => splitOffers(r.value?.offers))
/** Подходящие и альтернативы — карточками; «Не подходит» — отдельным блоком без кнопок. */
const offers = computed(() => [...split.value.match, ...split.value.alternative])
const rejected = computed(() => split.value.rejected)
const chips = computed(() => {
  const cl = r.value?.clarification
  return (cl?.chips ?? []).map((c) => chipView(c, cl?.key ?? ''))
})
const refusal = computed(() => isRefusal(r.value))
const table = computed(() => (hasTable(r.value) ? r.value!.comparison! : null))
const tableFirst = computed(() => r.value?.type === 'comparison')
const steps = computed(() => r.value?.trace ?? [])
const changes = computed(() => r.value?.changes ?? [])
const primary = computed(() => offers.value[0] ?? null)
/** Общая кнопка: все строки подбора (рабочие станции и серверы) или первый оффер. */
const toSpec = computed(() => specOffers(r.value?.offers))
const allInSpec = computed(
  () => toSpec.value.length > 0 && toSpec.value.every((o) => props.inSpec(o.offer_id)),
)
function addAll() {
  for (const o of toSpec.value) emit('spec', o)
}

const text = ref('')
const input = ref<HTMLInputElement | null>(null)
const placeholder = computed(() =>
  r.value?.clarification
    ? 'Или ответьте своими словами'
    : 'Спросите про редакции или измените условия',
)
function onSay() {
  const m = text.value.trim()
  if (!m || loading.value) return
  emit('say', m)
  text.value = ''
}
function onChip(ch: ChipView) {
  if (isOtherChip(ch)) {
    void nextTick(() => input.value?.focus())
    return
  }
  emit('answer', { key: ch.key, value: ch.value })
}

// Правка условия: чип раскрывается полем, Enter — изменить, Esc — отменить.
const editing = ref<string | null>(null)
const editText = ref('')
function startEdit(c: GuideCondition) {
  editing.value = c.key
  editText.value = c.value
  void nextTick(() => document.getElementById(`gcond-${c.key}`)?.focus())
}
function commitEdit(c: GuideCondition) {
  const v = editText.value.trim()
  editing.value = null
  if (v && v !== c.value) emit('edit', c, v)
}
watch(r, () => (editing.value = null))

const summary = computed(() => conditions.value.map((c) => c.label || c.value).join(' · '))
const fitCount = computed(() => {
  const n = split.value.match.length
  return `${n} ${plural(n, ['подходит', 'подходят', 'подходят'])}`
})
</script>

<template>
  <section
    class="guide panel"
    aria-labelledby="guide-title"
    data-testid="guide-panel"
    :data-type="r?.type ?? ''"
    :aria-busy="loading || undefined"
  >
    <header class="guide__head">
      <h2 id="guide-title" class="guide__title">
        <ListChecks :size="18" aria-hidden="true" /> Подбор лицензии
      </h2>
      <p v-if="collapsed && summary" class="guide__summary t-sm t-muted">{{ summary }}</p>
      <div class="guide__tools">
        <button
          type="button"
          class="btn btn--ghost btn--sm"
          :aria-expanded="!collapsed"
          aria-controls="guide-body"
          data-testid="guide-collapse"
          @click="collapsed = !collapsed"
        >
          <ChevronDown v-if="collapsed" :size="16" aria-hidden="true" />
          <ChevronUp v-else :size="16" aria-hidden="true" />
          {{ collapsed ? 'Развернуть' : 'Свернуть' }}
        </button>
        <button
          type="button"
          class="btn btn--ghost btn--icon btn--sm"
          aria-label="Закрыть подбор — выдача останется"
          data-testid="guide-close"
          @click="emit('close')"
        >
          <X :size="16" aria-hidden="true" />
        </button>
      </div>
    </header>

    <div v-show="!collapsed" id="guide-body" class="guide__body">
      <!-- Условия: чипы с происхождением; значения по умолчанию помечены и меняются так же. -->
      <ul v-if="conditions.length" class="guide__conds" aria-label="Условия подбора">
        <li
          v-for="c in conditions"
          :key="c.key"
          :class="['gcond', `gcond--${c.origin}`]"
          :data-cond="c.key"
          :data-origin="c.origin"
        >
          <form v-if="editing === c.key" class="gcond__form" @submit.prevent="commitEdit(c)">
            <label :for="`gcond-${c.key}`" class="sr-only">Изменить: {{ c.label }}</label>
            <input
              :id="`gcond-${c.key}`"
              v-model="editText"
              class="gcond__input"
              autocomplete="off"
              @keydown.esc.prevent="editing = null"
              @blur="editing = null"
            />
          </form>
          <button
            v-else
            type="button"
            class="gcond__label"
            :aria-label="`Изменить условие: ${conditionAria(c)}`"
            :disabled="loading"
            @click="startEdit(c)"
          >
            <span>{{ c.label || c.value }}</span>
            <span v-if="originLabel(c.origin)" class="gcond__origin">{{
              originLabel(c.origin)
            }}</span>
            <Pencil :size="12" aria-hidden="true" class="gcond__pen" />
          </button>
          <button
            type="button"
            class="gcond__x"
            :aria-label="`Убрать условие: ${c.label || c.value}`"
            :disabled="loading"
            data-testid="guide-cond-remove"
            @click="emit('edit', c, null)"
          >
            <X :size="12" aria-hidden="true" />
          </button>
        </li>
      </ul>

      <!-- Статус хода: сразу; проверенные строки — когда сервер их вернул. -->
      <p v-if="loading" class="guide__status t-sm" role="status" data-testid="guide-loading">
        <LoaderCircle :size="16" aria-hidden="true" class="guide__spin" />
        {{ pendingText }}…
      </p>
      <div
        v-else-if="status === 'error'"
        class="guide__error"
        role="alert"
        data-testid="guide-error"
      >
        <p class="t-sm">
          Подобрать не получилось{{ error?.message ? `: ${error.message}` : '' }}. Выдача ниже
          работает как обычно.
        </p>
        <button type="button" class="btn btn--secondary btn--sm" @click="emit('retry')">
          <RotateCcw :size="16" aria-hidden="true" /> Повторить
        </button>
      </div>

      <div v-if="r" :class="['guide__answer', { 'guide__answer--stale': loading }]">
        <p v-if="r.lead" class="guide__lead" data-testid="guide-lead">{{ r.lead }}</p>
        <p v-if="r.preliminary" class="guide__prelim t-sm" data-testid="guide-preliminary">
          Результат предварительный: не все условия, от которых зависит право использования,
          подтверждены.
        </p>
        <p v-if="r.notice" class="t-sm t-muted" data-testid="guide-notice">{{ r.notice }}</p>
        <p v-if="refusal" class="t-sm t-muted" data-testid="guide-refusal">
          {{
            r.type === 'no_data'
              ? 'Ниже — обычная выдача по запросу. Вопрос можно передать менеджеру в запросе КП.'
              : 'Подбор отвечает про выбор лицензий. Ниже — обычная выдача по запросу.'
          }}
        </p>

        <div
          v-if="r.clarification"
          class="guide__ask"
          role="group"
          aria-labelledby="guide-question"
          data-testid="guide-question"
        >
          <p id="guide-question" class="guide__question">{{ r.clarification.question }}</p>
          <ul class="guide__chips">
            <li v-for="ch in chips" :key="ch.value">
              <button type="button" class="guide__chip" :disabled="loading" @click="onChip(ch)">
                {{ ch.label }}
              </button>
            </li>
          </ul>
        </div>

        <GuideTable v-if="table && tableFirst" :comparison="table" :claims="r.claims ?? []" />

        <div v-if="offers.length" class="guide__offers">
          <p v-if="split.match.length > 1" class="t-sm t-muted guide__count">{{ fitCount }}</p>
          <GuideOfferCard
            v-for="(o, i) in offers"
            :key="o.offer_id"
            :offer="o"
            :primary="i === 0"
            :show-spec="offers.length > 1"
            :in-spec="inSpec(o.offer_id)"
            @spec="emit('spec', $event)"
          />
        </div>

        <section
          v-if="rejected.length"
          class="guide__rejected"
          aria-labelledby="guide-rejected"
          data-testid="guide-rejected"
        >
          <h3 id="guide-rejected" class="guide__h3">Не подходит</h3>
          <ul>
            <li v-for="o in rejected" :key="o.offer_id" :data-offer="o.offer_id">
              <span class="guide__rej-title">{{ o.title }}</span>
              <span class="t-num t-muted"> · {{ priceWithUnit(o) }}</span>
              <span v-if="o.note" class="guide__rej-note t-sm">{{ o.note }}</span>
            </li>
          </ul>
        </section>

        <GuideTable v-if="table && !tableFirst" :comparison="table" :claims="r.claims ?? []" />

        <section
          v-if="changes.length"
          class="guide__changes"
          aria-labelledby="guide-changes"
          data-testid="guide-changes"
        >
          <h3 id="guide-changes" class="guide__h3">При изменении условий</h3>
          <ul>
            <li v-for="(ch, i) in changes" :key="i" class="t-sm">
              <strong>{{ ch.condition }}</strong> — {{ ch.effect }}
            </li>
          </ul>
        </section>
      </div>

      <form class="guide__say" @submit.prevent="onSay">
        <label for="guide-text" class="sr-only">Сообщение подбору</label>
        <input
          id="guide-text"
          ref="input"
          v-model="text"
          class="guide__text"
          :placeholder="placeholder"
          autocomplete="off"
          maxlength="300"
          data-testid="guide-text"
        />
        <button
          type="submit"
          class="btn btn--secondary btn--sm"
          :disabled="loading || !text.trim()"
          aria-label="Отправить"
        >
          <CornerDownLeft :size="16" aria-hidden="true" />
        </button>
      </form>

      <div class="guide__actions">
        <button
          v-if="primary"
          type="button"
          class="btn btn--primary btn--sm"
          :disabled="primary.price_state === 'unavailable'"
          data-testid="guide-spec"
          @click="addAll"
        >
          {{ allInSpec ? 'В спецификации' : 'В спецификацию' }}
        </button>
        <button
          type="button"
          class="btn btn--secondary btn--sm"
          data-testid="guide-quote"
          @click="emit('quote')"
        >
          <FileText :size="16" aria-hidden="true" /> Запросить КП
        </button>
      </div>

      <details v-if="steps.length" class="guide__trace" data-testid="guide-trace">
        <summary class="t-sm">Как подобрано</summary>
        <ol class="guide__steps">
          <li v-for="(s, i) in steps" :key="i" class="t-sm">
            <span>{{ s.step }}</span>
            <span v-if="s.result" class="t-muted">: {{ s.result }}</span>
            <span v-if="s.ms != null && s.ms >= 1" class="t-xs t-muted t-num">
              · {{ formatMs(s.ms) }}</span
            >
          </li>
        </ol>
      </details>
    </div>
  </section>
</template>

<style scoped>
.guide {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-4) var(--space-4);
  border-color: var(--border-control);
  min-width: 0;
}
.guide__head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2) var(--space-3);
}
.guide__title {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  margin: 0;
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
  font-weight: var(--fw-semibold);
}
.guide__title svg {
  color: var(--accent);
}
.guide__summary {
  flex: 1 1 12rem;
  margin: 0;
  overflow-wrap: anywhere;
}
.guide__tools {
  display: flex;
  gap: var(--space-1);
  margin-left: auto;
}
.guide__body {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  min-width: 0;
}
.guide__conds {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.gcond {
  display: inline-flex;
  align-items: stretch;
  min-height: var(--control-h-sm);
  border: var(--border-w) solid var(--text);
  border-radius: var(--radius-pill);
  background: var(--surface);
  font-size: var(--fs-sm);
}
.gcond--default,
.gcond--inferred {
  border-style: dashed;
  border-color: var(--border-control);
}
.gcond__label,
.gcond__x {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  border: 0;
  background: transparent;
  color: var(--text);
  font: inherit;
  cursor: pointer;
}
.gcond__label {
  padding: 0 var(--space-1) 0 var(--space-3);
  font-weight: var(--fw-medium);
}
.gcond__origin {
  color: var(--text-3);
  font-size: var(--fs-xs);
  font-weight: var(--fw-regular);
}
.gcond__pen {
  color: var(--text-3);
}
.gcond__x {
  padding: 0 var(--space-2) 0 var(--space-1);
  color: var(--text-2);
  border-radius: 0 var(--radius-pill) var(--radius-pill) 0;
}
.gcond__label:hover,
.gcond__x:hover {
  color: var(--accent);
}
.gcond__form {
  display: flex;
}
.gcond__input {
  width: 8rem;
  padding: 0 var(--space-3);
  border: 0;
  border-radius: var(--radius-pill);
  background: transparent;
  color: var(--text);
  font: inherit;
}
.guide__status {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin: 0;
  color: var(--text-2);
}
.guide__spin {
  animation: guide-spin 1s linear infinite;
}
@keyframes guide-spin {
  to {
    transform: rotate(360deg);
  }
}
@media (prefers-reduced-motion: reduce) {
  .guide__spin {
    animation: none;
  }
}
.guide__error {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2) var(--space-3);
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-control);
  background: var(--danger-bg);
  color: var(--danger);
}
.guide__error p {
  margin: 0;
}
.guide__answer {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  min-width: 0;
  transition: opacity var(--dur-fast);
}
.guide__answer--stale {
  opacity: 0.55;
}
.guide__lead {
  margin: 0;
  font-size: var(--fs-reading);
  line-height: var(--lh-reading);
  font-weight: var(--fw-medium);
}
.guide__prelim {
  margin: 0;
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-sm);
  background: var(--status-pending-bg);
  color: var(--status-pending);
}
.guide__ask {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.guide__question {
  margin: 0;
  font-weight: var(--fw-semibold);
}
.guide__chips {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.guide__chip {
  min-height: var(--control-h);
  min-width: 2.75rem;
  padding: 0 var(--space-4);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-pill);
  background: var(--surface);
  color: var(--text);
  font: inherit;
  font-weight: var(--fw-medium);
  cursor: pointer;
}
.guide__chip:hover:not(:disabled) {
  border-color: var(--accent);
  color: var(--accent);
}
.guide__offers {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.guide__count {
  margin: 0;
}
.guide__rejected {
  padding: var(--space-2) var(--space-3);
  border: var(--border-w) solid var(--danger-bg);
  border-left: 3px solid var(--danger);
  border-radius: var(--radius-control);
}
.guide__rejected ul {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.guide__rejected li {
  overflow-wrap: anywhere;
}
.guide__rej-title {
  font-size: var(--fs-sm);
  font-weight: var(--fw-medium);
}
.guide__rej-note {
  display: block;
  color: var(--danger);
}
.guide__changes {
  padding: var(--space-2) var(--space-3);
  border: var(--border-w) dashed var(--border-control);
  border-radius: var(--radius-control);
  background: var(--surface-2);
}
.guide__h3 {
  margin: 0 0 var(--space-1);
  font-size: var(--fs-sm);
  font-weight: var(--fw-semibold);
}
.guide__changes ul {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin: 0;
  padding-left: var(--space-4);
}
.guide__say {
  display: flex;
  gap: var(--space-2);
}
.guide__text {
  flex: 1;
  min-width: 0;
  min-height: var(--control-h);
  padding: 0 var(--space-3);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-control);
  background: var(--surface);
  color: var(--text);
  font: inherit;
  font-size: var(--fs-input);
}
.guide__say .btn {
  min-height: var(--control-h);
}
.guide__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}
.guide__trace summary {
  cursor: pointer;
  color: var(--text-2);
}
.guide__steps {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin: var(--space-2) 0 0;
  padding-left: var(--space-5);
}
</style>
