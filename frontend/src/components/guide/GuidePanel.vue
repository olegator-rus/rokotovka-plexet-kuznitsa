<script setup lang="ts">
// Блок подбора над выдачей (контур подбора, SPEC §1–§3, §6, §8–§11). Читается сверху вниз как разговор: условия в шапке,
// реплика покупателя, ответ подбора, следующий шаг; поле ввода всегда внизу блока. На широком блоке — две колонки:
// слева разговор и сравнение, справа подобранная лицензия. Сворачивается и закрывается, выдачу и спецификацию не
// трогает. Цены, артикулы, строки таблицы приходят структурой; сырой текст модели не выводится.
import { computed, nextTick, ref, watch } from 'vue'
import {
  ArrowUp,
  Check,
  ChevronDown,
  ChevronUp,
  CircleSlash,
  FilePlus2,
  FileText,
  ListChecks,
  LoaderCircle,
  RotateCcw,
  X,
} from 'lucide-vue-next'
import type { GuideChipAnswer, GuideCondition, GuideOffer, GuideResponse } from '@/api/typesGuide'
import type { GuideStatus } from '@/composables/useGuide'
import { useMediaQuery } from '@/composables/useMediaQuery'
import type { ApiError } from '@/lib/apiError'
import { formatMs, plural } from '@/lib/format'
import {
  type ChipView,
  type GuideTurnView,
  chipView,
  columnOffer,
  hasTable,
  isOtherChip,
  isRefusal,
  keepNumbers,
  offerName,
  offerPrice,
  PENDING_STEPS,
  priceWithUnit,
  rejectedTitle,
  sharedNote,
  sourceName,
  specOffers,
  splitOffers,
} from '@/lib/guide'
import GuideConditions from './GuideConditions.vue'
import GuideOfferCard from './GuideOfferCard.vue'
import GuideOfferRow from './GuideOfferRow.vue'
import GuideTable from './GuideTable.vue'

const props = withDefaults(
  defineProps<{
    status: GuideStatus
    response: GuideResponse | null
    error: ApiError | null
    pendingText: string
    pendingStep?: number
    ask?: string
    pendingAsk?: string
    turns?: GuideTurnView[]
    inSpec: (offerId: string) => boolean
  }>(),
  { pendingStep: 0, ask: '', pendingAsk: '', turns: () => [] },
)
const collapsed = defineModel<boolean>('collapsed', { default: false })
const emit = defineEmits<{
  close: []
  answer: [chip: GuideChipAnswer, label: string]
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
const rejected = computed(() => split.value.rejected)
const rejectedNote = computed(() => sharedNote(rejected.value))
const ruleLink = computed(() => {
  const url = rejected.value.find((o) => o.rule_source)?.rule_source ?? ''
  return url ? { url, name: sourceName({ url }) } : null
})
const chips = computed(() => {
  const cl = r.value?.clarification
  return (cl?.chips ?? []).map((c) => chipView(c, cl?.key ?? ''))
})
const refusal = computed(() => isRefusal(r.value))
const table = computed(() => (hasTable(r.value) ? r.value!.comparison! : null))
const steps = computed(() => r.value?.trace ?? [])
const changes = computed(() => r.value?.changes ?? [])

/** Офферы, которые уже стоят под колонками таблицы: строкой их не повторяем, кнопка — под колонкой. */
const inTable = computed(() => {
  const ids = new Set<string>()
  if (r.value?.type !== 'comparison') return ids
  for (const c of table.value?.columns ?? []) {
    const o = columnOffer(c.family_id, r.value?.offers)
    if (o && offerPrice(o).known) ids.add(o.offer_id)
  }
  return ids
})
/** Рекомендация ядра: в offers она первая, на экране — главная карточка. */
const rec = computed(() => {
  const id = r.value?.recommendation?.offer_id
  return id ? ((r.value?.offers ?? []).find((o) => o.offer_id === id) ?? null) : null
})
const asked = computed(() => r.value?.type === 'comparison')
const tableOpen = ref(false)
const options = computed(() => [...split.value.match, ...split.value.alternative])
/** Строки подбора (D3: рабочие станции и серверы) идут в карточку все; иначе главный — один. */
const lines = computed(() =>
  specOffers(r.value?.offers).filter((o) => !inTable.value.has(o.offer_id)),
)
const multiLine = computed(() => lines.value.length > 1)
const picked = ref<string | null>(null)
watch(r, () => (picked.value = null))
/**
 * Главная карточка: выбранная покупателем строка → рекомендация → первый подходящий. В сравнении по вопросу
 * («чем отличается…») карточки нет: редакции уже стоят колонками с кнопками.
 */
const featured = computed<GuideOffer[]>(() => {
  if (multiLine.value) return lines.value
  const chosen = options.value.find((o) => o.offer_id === picked.value)
  if (chosen) return [chosen]
  if (rec.value) return [rec.value]
  const pool = asked.value
    ? options.value.filter((o) => !inTable.value.has(o.offer_id))
    : options.value
  return pool[0] ? [pool[0]] : []
})
const candidates = computed(() => options.value.filter((o) => !inTable.value.has(o.offer_id)))
const isRec = computed(
  () => featured.value.length === 1 && featured.value[0]!.offer_id === rec.value?.offer_id,
)
const featuredIds = computed(() => new Set(featured.value.map((o) => o.offer_id)))
const moreMatch = computed(() =>
  candidates.value.filter((o) => o.role !== 'alternative' && !featuredIds.value.has(o.offer_id)),
)
const moreAlt = computed(() =>
  candidates.value.filter((o) => o.role === 'alternative' && !featuredIds.value.has(o.offer_id)),
)
const featuredAlt = computed(() => featured.value[0]?.role === 'alternative')
const allInSpec = computed(
  () => featured.value.length > 0 && featured.value.every((o) => props.inSpec(o.offer_id)),
)
const featuredKnown = computed(() => featured.value.every((o) => offerPrice(o).known))
const split2 = computed(() => featured.value.length > 0)
function addFeatured() {
  for (const o of featured.value) emit('spec', o)
}
function pick(o: GuideOffer) {
  picked.value = o.offer_id
}

/** Реплика показанного хода; новая, пока ход идёт, стоит у шагов внизу — ответ под старой репликой не путается. */
const shownAsk = computed(() => props.ask || (loading.value && !r.value ? props.pendingAsk : ''))
const history = computed(() => props.turns)
const historyOpen = ref(false)
const HISTORY_SHORT = 2
const historyShown = computed(() =>
  historyOpen.value ? history.value : history.value.slice(-HISTORY_SHORT),
)

const text = ref('')
const input = ref<HTMLInputElement | null>(null)
const narrow = useMediaQuery('(max-width: 40rem)')
const placeholder = computed(() =>
  r.value?.clarification
    ? 'Или ответьте своими словами'
    : narrow.value
      ? 'Спросите или уточните условия'
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
  emit('answer', { key: ch.key, value: ch.value }, ch.label)
}

const moreTitle = computed(() => {
  const n = moreMatch.value.length
  return `Ещё ${plural(n, ['подходит', 'подходят', 'подходят'])}`
})
</script>

<template>
  <section
    :class="['guide', { 'guide--collapsed': collapsed }]"
    aria-labelledby="guide-title"
    data-testid="guide-panel"
    :data-type="r?.type ?? ''"
    :aria-busy="loading || undefined"
  >
    <header class="guide__head">
      <h2 id="guide-title" class="guide__title">
        <ListChecks :size="18" aria-hidden="true" /> Подбор лицензии
      </h2>
      <GuideConditions
        class="guide__conds"
        :conditions="conditions"
        :disabled="loading"
        @edit="(c, v) => emit('edit', c, v)"
      />
      <div class="guide__tools">
        <button
          type="button"
          class="btn btn--ghost btn--icon btn--sm"
          :aria-expanded="!collapsed"
          aria-controls="guide-body"
          :aria-label="collapsed ? 'Развернуть подбор' : 'Свернуть подбор'"
          :title="collapsed ? 'Развернуть' : 'Свернуть'"
          data-testid="guide-collapse"
          @click="collapsed = !collapsed"
        >
          <ChevronDown v-if="collapsed" :size="16" aria-hidden="true" />
          <ChevronUp v-else :size="16" aria-hidden="true" />
        </button>
        <button
          type="button"
          class="btn btn--ghost btn--icon btn--sm"
          aria-label="Закрыть подбор — выдача останется"
          title="Закрыть"
          data-testid="guide-close"
          @click="emit('close')"
        >
          <X :size="16" aria-hidden="true" />
        </button>
      </div>
    </header>

    <div v-show="!collapsed" id="guide-body" class="guide__frame">
      <div :class="['guide__body', { 'guide__body--split': split2 }]">
        <!-- Разговор: прошлые ходы коротко, реплика покупателя, ответ, следующий шаг. -->
        <div :class="['guide__dialog', { guide__stale: loading && r }]">
          <div v-if="history.length" class="gturns">
            <button
              v-if="history.length > HISTORY_SHORT"
              type="button"
              class="gturns__more"
              :aria-expanded="historyOpen"
              @click="historyOpen = !historyOpen"
            >
              {{ historyOpen ? 'Скрыть ранние' : `Ещё ${history.length - HISTORY_SHORT} ранее` }}
            </button>
            <ol class="gturns__list" aria-label="Предыдущие шаги подбора">
              <li v-for="(t, i) in historyShown" :key="i" class="gturns__item">
                <span class="gturns__ask"><span class="gwho">Вы</span> {{ t.ask }}</span>
                <span v-if="t.answer" class="gturns__answer">{{ t.answer }}</span>
              </li>
            </ol>
          </div>

          <p v-if="shownAsk" class="gask" data-testid="guide-ask">
            <span class="gwho">Вы</span> {{ shownAsk }}
          </p>

          <div v-if="loading && !r" class="gwait" aria-hidden="true">
            <span class="skeleton gwait__line" />
            <span class="skeleton gwait__line gwait__line--short" />
          </div>

          <div v-else-if="status === 'error'" class="gerror" role="alert" data-testid="guide-error">
            <p>
              Подобрать не получилось{{ error?.message ? `: ${error.message}` : '' }}. Выдача ниже
              работает как обычно.
            </p>
            <button type="button" class="btn btn--secondary btn--sm" @click="emit('retry')">
              <RotateCcw :size="16" aria-hidden="true" /> Повторить
            </button>
          </div>

          <template v-if="r && status !== 'error'">
            <p v-if="r.lead" class="glead" data-testid="guide-lead">{{ keepNumbers(r.lead) }}</p>

            <div
              v-if="r.clarification"
              class="gquestion"
              role="group"
              aria-labelledby="guide-question"
              data-testid="guide-question"
            >
              <p id="guide-question" :class="r.lead ? 'gquestion__text' : 'glead'">
                {{ r.clarification.question }}
              </p>
              <ul class="gquestion__chips">
                <li v-for="ch in chips" :key="ch.value">
                  <button
                    type="button"
                    :class="[
                      'gchip',
                      {
                        'gchip--other': isOtherChip(ch) || ch.value === '',
                        'gchip--num': /^\d+$/.test(ch.label),
                      },
                    ]"
                    :disabled="loading"
                    @click="onChip(ch)"
                  >
                    {{ ch.label }}
                  </button>
                </li>
              </ul>
            </div>

            <p v-if="r.preliminary" class="gnote gnote--pending" data-testid="guide-preliminary">
              Результат предварительный: не все условия, от которых зависит право использования,
              подтверждены.
            </p>
            <p v-if="r.notice" class="gnote" data-testid="guide-notice">{{ r.notice }}</p>
            <p v-if="refusal" class="gnote" data-testid="guide-refusal">
              {{
                r.type === 'no_data'
                  ? 'Ниже — обычная выдача по запросу. Вопрос можно передать менеджеру в запросе КП.'
                  : 'Подбор отвечает про выбор лицензий. Ниже — обычная выдача по запросу.'
              }}
            </p>

            <section
              v-if="changes.length"
              class="gchanges"
              aria-labelledby="guide-changes"
              data-testid="guide-changes"
            >
              <h3 id="guide-changes" class="gh3">Что изменилось</h3>
              <ul>
                <li v-for="(ch, i) in changes" :key="i">
                  <span class="gchanges__what">{{ ch.condition }}</span>
                  <span class="gchanges__effect">{{ ch.effect }}</span>
                </li>
              </ul>
            </section>

            <button
              v-if="table && !asked"
              type="button"
              class="gtoggle"
              :aria-expanded="tableOpen"
              aria-controls="guide-compare"
              data-testid="guide-compare-toggle"
              @click="tableOpen = !tableOpen"
            >
              <ChevronUp v-if="tableOpen" :size="16" aria-hidden="true" />
              <ChevronDown v-else :size="16" aria-hidden="true" />
              {{ tableOpen ? 'Скрыть сравнение' : `Сравнить таблицей: ${table.columns.length}` }}
            </button>
          </template>
        </div>

        <!-- Рекомендация: одна лицензия крупно, одна главная кнопка; другие варианты — компактно. -->
        <aside
          v-if="r && status !== 'error' && featured.length"
          :class="['guide__rec', { guide__stale: loading }]"
          aria-label="Подобранная лицензия"
        >
          <div v-if="featured.length" class="gpick">
            <p
              :class="['gpick__status', { 'gpick__status--alt': featuredAlt }]"
              data-testid="guide-pick-status"
            >
              <Check v-if="!featuredAlt" :size="14" :stroke-width="2.25" aria-hidden="true" />
              {{
                isRec
                  ? 'Рекомендуем'
                  : featuredAlt
                    ? 'Подойдёт, если изменить условия'
                    : 'Подходит под условия'
              }}
            </p>
            <p
              v-if="isRec && r.recommendation?.reason"
              class="gpick__why"
              data-testid="guide-reason"
            >
              {{ keepNumbers(r.recommendation.reason) }}
            </p>
            <div class="gpick__offers">
              <GuideOfferCard v-for="o in featured" :key="o.offer_id" :offer="o" />
            </div>
            <button
              type="button"
              :class="['btn gpick__cta', allInSpec ? 'btn--secondary' : 'btn--primary']"
              :disabled="loading || featured[0]?.price_state === 'unavailable'"
              data-testid="guide-spec"
              @click="addFeatured"
            >
              <Check v-if="allInSpec" :size="16" aria-hidden="true" />
              <FilePlus2 v-else :size="16" aria-hidden="true" />
              {{
                allInSpec
                  ? 'В спецификации'
                  : featuredKnown
                    ? 'В спецификацию'
                    : 'Добавить в запрос КП'
              }}
            </button>
          </div>
        </aside>

        <!-- Другие варианты и «Не подходит»: под разговором, рядом с главной карточкой. -->
        <div
          v-if="r && status !== 'error' && (moreMatch.length || moreAlt.length || rejected.length)"
          :class="['guide__more', { guide__stale: loading }]"
        >
          <section v-if="moreMatch.length" class="gmore" aria-label="Другие подходящие варианты">
            <h3 class="gh3">
              {{ moreTitle }} <span class="gh3__count t-num">{{ moreMatch.length }}</span>
            </h3>
            <GuideOfferRow
              v-for="o in moreMatch"
              :key="o.offer_id"
              :offer="o"
              :disabled="loading"
              @pick="pick"
            />
          </section>

          <section v-if="moreAlt.length" class="gmore" aria-label="При изменении условий">
            <h3 class="gh3">При изменении условий</h3>
            <GuideOfferRow
              v-for="o in moreAlt"
              :key="o.offer_id"
              :offer="o"
              :disabled="loading"
              @pick="pick"
            />
          </section>

          <section
            v-if="rejected.length"
            class="greject"
            aria-labelledby="guide-rejected"
            data-testid="guide-rejected"
          >
            <h3 id="guide-rejected" class="greject__title">
              <CircleSlash :size="14" aria-hidden="true" /> {{ rejectedTitle(conditions) }}
            </h3>
            <p v-if="rejectedNote" class="greject__why">
              {{ rejectedNote }}
              <a
                v-if="ruleLink"
                :href="ruleLink.url"
                class="greject__rule"
                target="_blank"
                rel="noopener noreferrer"
                >Правило: {{ ruleLink.name
                }}<span class="sr-only"> (откроется в новой вкладке)</span></a
              >
            </p>
            <ul>
              <li v-for="o in rejected" :key="o.offer_id" :data-offer="o.offer_id">
                <span class="greject__name" :title="o.title">{{
                  offerName(o).detail || o.title
                }}</span>
                <span class="greject__price t-num">{{ priceWithUnit(o) }}</span>
                <span v-if="!rejectedNote && o.note" class="greject__why">{{ o.note }}</span>
              </li>
            </ul>
          </section>
        </div>

        <!-- Сравнение на всю ширину блока: в ответе на «чем отличается» открыто сразу, в подборе — по кнопке. -->
        <div
          v-if="r && status !== 'error' && table && (asked || tableOpen)"
          id="guide-compare"
          :class="['guide__compare', { guide__stale: loading }]"
        >
          <GuideTable
            :comparison="table"
            :claims="r.claims ?? []"
            :offers="r.offers ?? []"
            :in-spec="inSpec"
            :busy="loading"
            :featured-id="featured.length === 1 ? featured[0]!.offer_id : ''"
            :recommended-id="rec?.offer_id ?? ''"
            @pick="pick"
          />
        </div>
      </div>

      <div class="guide__foot">
        <form class="gsay" @submit.prevent="onSay">
          <label for="guide-text" class="sr-only">Сообщение подбору</label>
          <input
            id="guide-text"
            ref="input"
            v-model="text"
            class="gsay__input"
            :placeholder="placeholder"
            autocomplete="off"
            enterkeyhint="send"
            maxlength="300"
            data-testid="guide-text"
          />
          <button
            type="submit"
            class="gsay__send"
            :disabled="loading || !text.trim()"
            aria-label="Отправить"
          >
            <ArrowUp :size="16" :stroke-width="2.25" aria-hidden="true" />
          </button>
        </form>

        <div class="gmeta">
          <ol
            v-if="loading"
            class="gsteps"
            role="status"
            :aria-label="`${pendingText}…`"
            data-testid="guide-loading"
          >
            <li v-if="pendingAsk && r" class="gsteps__ask">
              <span class="gwho">Вы</span> {{ pendingAsk }}
            </li>
            <li
              v-for="(s, i) in PENDING_STEPS"
              :key="s"
              :class="[
                'gsteps__step',
                i < pendingStep ? 'is-done' : i === pendingStep ? 'is-now' : 'is-next',
              ]"
            >
              <Check v-if="i < pendingStep" :size="14" aria-hidden="true" />
              <LoaderCircle
                v-else-if="i === pendingStep"
                :size="14"
                aria-hidden="true"
                class="gspin"
              />
              <span v-else class="gsteps__dot" aria-hidden="true" />
              {{ s }}
            </li>
          </ol>
          <details v-else-if="steps.length" class="gtrace" data-testid="guide-trace">
            <summary>Как подобрано</summary>
            <ol class="gtrace__steps">
              <li v-for="(s, i) in steps" :key="i">
                <span>{{ s.step }}</span>
                <span v-if="s.result" class="gtrace__res">: {{ s.result }}</span>
                <span v-if="s.ms != null && s.ms >= 1" class="gtrace__ms t-num">
                  {{ formatMs(s.ms) }}</span
                >
              </li>
            </ol>
          </details>
          <span v-else class="gmeta__gap" />
          <button
            type="button"
            class="btn btn--ghost btn--sm gmeta__quote"
            data-testid="guide-quote"
            @click="emit('quote')"
          >
            <FileText :size="16" aria-hidden="true" /> Запросить КП
          </button>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.guide {
  container-type: inline-size;
  min-width: 0;
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
}

/* Шапка: название, условия, свернуть и закрыть */
.guide__head {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  grid-template-areas: 'title conds tools';
  align-items: start;
  gap: var(--space-2) var(--space-4);
  padding: var(--space-3) var(--space-3) var(--space-3) var(--space-5);
  border-bottom: var(--border-w) solid var(--border);
}
.guide--collapsed .guide__head {
  border-bottom: 0;
}
.guide__title {
  grid-area: title;
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  min-height: var(--control-h-sm);
  margin: 0;
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
  font-weight: var(--fw-semibold);
  white-space: nowrap;
}
.guide__title svg {
  color: var(--text-2);
}
.guide__conds {
  grid-area: conds;
  min-width: 0;
}
.guide__tools {
  grid-area: tools;
  display: flex;
  gap: var(--space-1);
}
@container (max-width: 40rem) {
  .guide__head {
    grid-template-columns: minmax(0, 1fr) auto;
    grid-template-areas:
      'title tools'
      'conds conds';
    padding-left: var(--space-4);
  }
}

/* Тело: разговор слева, рекомендация справа на широком блоке */
.guide__body {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
  padding: var(--space-5);
}
.guide__dialog {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  min-width: 0;
}
.guide__dialog > :not(.gcmp) {
  max-width: 46rem;
}
.guide__rec {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  min-width: 0;
}
.guide__dialog {
  grid-area: dialog;
}
.guide__rec {
  grid-area: rec;
}
.guide__more {
  grid-area: more;
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  min-width: 0;
  max-width: 46rem;
}
@container (min-width: 52rem) {
  .guide__body--split {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(20rem, 25rem);
    grid-template-rows: auto 1fr auto;
    grid-template-areas:
      'dialog rec'
      'more rec'
      'compare compare';
    column-gap: var(--space-8);
  }
}
@container (max-width: 40rem) {
  .guide__body {
    padding: var(--space-4);
    gap: var(--space-4);
  }
}
.guide__stale {
  opacity: 0.5;
  pointer-events: none;
  transition: opacity var(--dur-panel) var(--ease-standard);
}

/* История и реплика покупателя */
.gturns {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--space-1);
}
.gturns__more {
  padding: 0;
  border: 0;
  background: none;
  color: var(--text-3);
  font: inherit;
  font-size: var(--fs-xs);
  text-decoration: underline;
  text-underline-offset: 3px;
  cursor: pointer;
}
.gturns__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  width: 100%;
  margin: 0;
  padding: 0;
  list-style: none;
}
.gturns__item {
  display: flex;
  gap: var(--space-2);
  min-width: 0;
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.gturns__ask {
  flex: none;
  max-width: 50%;
  overflow: hidden;
  color: var(--text-2);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.gturns__answer {
  min-width: 0;
  overflow: hidden;
  color: var(--text-3);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.gturns__answer::before {
  content: '— ';
}
.gwho {
  margin-right: var(--space-1);
  color: var(--text-3);
  font-size: var(--fs-xs);
}
.gask {
  margin: var(--space-2) 0 0;
  padding: var(--space-1) var(--space-3);
  align-self: flex-start;
  max-width: 100%;
  border-radius: var(--radius-control);
  background: var(--surface-2);
  color: var(--text);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
  overflow-wrap: anywhere;
}
.gturns + .gask {
  margin-top: var(--space-2);
}
.gwait {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-1) 0;
}
.gwait__line {
  height: 1.125rem;
  width: 85%;
}
.gwait__line--short {
  width: 55%;
}

/* Ответ: лид, вопрос с чипами, пометки, что изменилось */
.glead {
  margin: 0;
  font-size: var(--fs-h1);
  line-height: var(--lh-h1);
  font-weight: var(--fw-medium);
  letter-spacing: -0.005em;
  text-wrap: pretty;
}
@container (max-width: 40rem) {
  .glead {
    font-size: 1.0625rem;
    line-height: 1.5rem;
  }
}
.gquestion {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.gquestion__chips {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.gchip {
  min-height: 2.5rem;
  min-width: 3.5rem;
  padding: 0 var(--space-5);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-pill);
  background: var(--surface);
  color: var(--text);
  font: inherit;
  font-size: var(--fs-body);
  font-weight: var(--fw-medium);
  font-variant-numeric: tabular-nums;
  cursor: pointer;
  transition:
    background-color var(--dur-fast) var(--ease-standard),
    border-color var(--dur-fast) var(--ease-standard);
}
.gchip:hover:not(:disabled) {
  border-color: var(--text);
  background: var(--surface-2);
}
.gchip--num {
  font-size: var(--fs-h2);
}
.gquestion__text {
  margin: 0;
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
  font-weight: var(--fw-semibold);
}
.gtoggle {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  align-self: flex-start;
  min-height: var(--control-h-sm);
  padding: 0;
  border: 0;
  background: none;
  color: var(--text-2);
  font: inherit;
  font-size: var(--fs-sm);
  cursor: pointer;
}
.gtoggle:hover {
  color: var(--text);
}
.guide__compare {
  grid-area: compare;
  min-width: 0;
}
.gchip--other {
  border-style: dashed;
  color: var(--text-2);
  font-size: var(--fs-body);
  font-weight: var(--fw-regular);
}
.gnote {
  margin: 0;
  color: var(--text-2);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.gnote--pending {
  align-self: flex-start;
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-sm);
  background: var(--status-pending-bg);
  color: var(--status-pending);
}
.gerror {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2) var(--space-3);
  color: var(--text);
}
.gerror p {
  margin: 0;
  font-size: var(--fs-body);
}
.gh3 {
  margin: 0 0 var(--space-1);
  color: var(--text-2);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
  font-weight: var(--fw-medium);
}
.gh3__count {
  color: var(--text-3);
  font-weight: var(--fw-regular);
}
.gchanges ul {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin: 0;
  padding: 0;
  list-style: none;
}
.gchanges li {
  display: flex;
  flex-wrap: wrap;
  gap: 0 var(--space-2);
  padding-left: var(--space-3);
  border-left: 2px solid var(--surface-3);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.gchanges__what {
  font-weight: var(--fw-medium);
}
.gchanges__effect {
  color: var(--text-2);
}

/* Рекомендация */
.gpick {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4) var(--space-5) var(--space-5);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
}
.gpick__status {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  margin: 0;
  color: var(--route-a);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
  font-weight: var(--fw-medium);
}
.gpick__why {
  margin: calc(-1 * var(--space-2)) 0 0;
  color: var(--text-2);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.gpick__status--alt {
  color: var(--text-2);
}
.gpick__offers {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}
.gpick__cta {
  width: 100%;
  min-height: 2.5rem;
}
.gmore {
  display: flex;
  flex-direction: column;
  margin: 0 calc(-1 * var(--space-3));
}
.gmore .gh3 {
  padding: 0 var(--space-3);
}
.greject {
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-control);
  background: var(--surface-2);
}
.greject__title {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin: 0;
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
  font-weight: var(--fw-semibold);
}
.greject__title svg {
  color: var(--text-2);
}
.greject__why {
  display: block;
  margin: var(--space-1) 0 0;
  color: var(--text-2);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.greject__rule {
  display: block;
  width: fit-content;
  margin-top: 2px;
  color: var(--text-3);
  font-size: var(--fs-xs);
  text-decoration: underline;
  text-underline-offset: 3px;
  white-space: nowrap;
}
.greject ul {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: var(--space-3) 0 0;
  padding: 0;
  list-style: none;
}
.greject li {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: 0 var(--space-3);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.greject__name {
  min-width: 0;
  color: var(--text-2);
}
.greject__name::first-letter {
  text-transform: uppercase;
}
.greject__price {
  color: var(--text-2);
}

/* Низ: поле ввода на одном месте, шаги хода, «Как подобрано», запрос КП */
.guide__foot {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-5) var(--space-3);
  border-top: var(--border-w) solid var(--border);
}
@container (max-width: 40rem) {
  .guide__foot {
    padding: var(--space-3) var(--space-4);
  }
}
.gsay {
  position: relative;
  display: flex;
}
.gsay__input {
  flex: 1;
  min-width: 0;
  height: 2.5rem;
  padding: 0 3rem 0 var(--space-4);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-pill);
  background: var(--surface);
  color: var(--text);
  font: inherit;
  font-size: var(--fs-input);
  transition: border-color var(--dur-fast) var(--ease-standard);
}
.gsay__input::placeholder {
  color: var(--text-3);
}
.gsay__input:hover {
  border-color: var(--border-control);
}
.gsay__input:focus-visible {
  outline: none;
  border-color: var(--text-2);
}
.gsay__send {
  position: absolute;
  top: 50%;
  right: var(--space-1);
  display: grid;
  place-items: center;
  width: 2rem;
  height: 2rem;
  padding: 0;
  border: 0;
  border-radius: var(--radius-pill);
  background: var(--text);
  color: var(--bg);
  transform: translateY(-50%);
  cursor: pointer;
  transition: background-color var(--dur-fast) var(--ease-standard);
}
.gsay__send:disabled {
  background: var(--surface-3);
  color: var(--text-3);
  cursor: default;
}
.gmeta {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-2) var(--space-4);
  min-height: var(--control-h-sm);
}
.gmeta__gap {
  flex: 1;
}
.gmeta__quote {
  flex: none;
}
.gsteps {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1) var(--space-4);
  min-height: var(--control-h-sm);
  margin: 0;
  padding: 0;
  list-style: none;
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.gsteps__ask {
  max-width: 16rem;
  overflow: hidden;
  color: var(--text);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.gsteps__step {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
}
.gsteps__step.is-done {
  color: var(--text-2);
}
.gsteps__step.is-now {
  color: var(--text);
  font-weight: var(--fw-medium);
}
.gsteps__step.is-next {
  color: var(--text-3);
}
.gsteps__dot {
  display: inline-block;
  width: 14px;
  height: 14px;
  border: 1.5px solid var(--border);
  border-radius: 50%;
  box-sizing: border-box;
}
.gspin {
  animation: guide-spin 1s linear infinite;
}
@keyframes guide-spin {
  to {
    transform: rotate(360deg);
  }
}
@media (prefers-reduced-motion: reduce) {
  .gspin {
    animation: none;
  }
}
.gtrace {
  flex: 1 1 auto;
  min-width: 0;
}
.gtrace summary {
  display: inline-flex;
  align-items: center;
  min-height: var(--control-h-sm);
  color: var(--text-2);
  font-size: var(--fs-sm);
  cursor: pointer;
}
.gtrace summary:hover {
  color: var(--text);
}
.gtrace__steps {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin: var(--space-1) 0 var(--space-2);
  padding-left: var(--space-5);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
}
.gtrace__steps li::marker {
  color: var(--text-3);
}
.gtrace__res {
  color: var(--text-2);
}
.gtrace__ms {
  margin-left: var(--space-2);
  color: var(--text-3);
  font-size: var(--fs-xs);
}
</style>
