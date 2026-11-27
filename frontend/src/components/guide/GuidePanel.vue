<script setup lang="ts">
// Рабочая область подбора (контур экрана, UI.md §3–§4): справа от выдачи на десктопе, компактным блоком над выдачей на
// телефоне. Одно актуальное состояние сверху вниз: задача → условия полями → что изменилось → вопрос или результат →
// сравнение → «При изменении условий» → закреплённая итоговая строка с одной кнопкой. Новый ответ заменяет прежний;
// пока ход идёт, прежний остаётся с пометкой «Обновляем подбор». Тексты, числа и порядок — из `screen` ответа.
import { computed, ref, watch } from 'vue'
import { ArrowUp, Check, ChevronDown, ChevronUp, RotateCcw, X } from 'lucide-vue-next'
import type { GuideChipAnswer, GuideOffer, GuideResponse } from '@/api/typesGuide'
import type { GuideStatus } from '@/composables/useGuide'
import type { ApiError } from '@/lib/apiError'
import { formatMs } from '@/lib/format'
import { isRefusal, specOffers } from '@/lib/guide'
import {
  offerById,
  offerSubtitle,
  offerTitle,
  screenOf,
  summaryFor,
  taskText,
} from '@/lib/guideView'
import GuideCompare from './GuideCompare.vue'
import GuideFields from './GuideFields.vue'
import GuideQuestion from './GuideQuestion.vue'
import GuideSource from './GuideSource.vue'
import GuideTaskCost from './GuideTaskCost.vue'

const props = defineProps<{
  status: GuideStatus
  response: GuideResponse | null
  error: ApiError | null
  /** Позиция из этого ответа уже добавлена: «Добавлено в спецификацию». */
  added: string | null
  inSpec: (offerId: string) => boolean
  /** Телефон: блок над выдачей, условия строкой, таблица строками. */
  compact?: boolean
  /** Последняя фраза покупателя: «Вы: …». */
  said?: string
}>()
const collapsed = defineModel<boolean>('collapsed', { default: false })
const emit = defineEmits<{
  hide: []
  answer: [chip: GuideChipAnswer]
  /** Реплика из поля подбора — тот же ход, что и ввод в строке поиска. */
  say: [message: string]
  edit: [key: string, value: string | null]
  retry: []
  spec: [offer: GuideOffer]
  quote: [offer: GuideOffer | null]
  /** «Продолжить поиск»: область скрывается, выдача остаётся. */
  continue: []
  openSpec: []
  /** «Изменить» без списка направлений: уточнить задачу в строке поиска. */
  refine: []
}>()

const r = computed(() => props.response)
const loading = computed(() => props.status === 'loading')
const stale = computed(() => loading.value && Boolean(r.value))
const failed = computed(() => props.status === 'error' || r.value?.type === 'fallback')
const screen = computed(() => screenOf(r.value))
const conditions = computed(() => r.value?.conditions ?? [])
const task = computed(() => taskText(r.value))
const hasDirection = computed(() => conditions.value.some((c) => c.key === 'direction'))

const picked = ref<string>('')
watch(r, () => (picked.value = ''))
const selectedId = computed(() => picked.value || screen.value?.selected_offer_id || '')
const selected = computed(() => offerById(r.value, selectedId.value))
const summary = computed(() => summaryFor(screen.value, selected.value))

const result = computed(() => screen.value?.result ?? null)
/**
 * Карточка результата — при основании, которое выделяет одну позицию. Если рядом сравнение и основание — только
 * стоимость, карточки нет: редакции стоят колонками, основание — строкой разницы и переключателем «Выбрано».
 */
const resultOffer = computed(() => {
  const res = result.value
  if (!res || (screen.value?.comparison && res.basis === 'lowest_total')) return null
  return offerById(r.value, res.offer_id)
})
const details = computed(() => resultOffer.value?.display?.details ?? [])
const detailsOpen = ref(false)
watch(r, () => (detailsOpen.value = false))
const others = computed(() => screen.value?.others ?? [])
const changed = computed(() => screen.value?.changed ?? null)
const steps = computed(() => r.value?.trace ?? [])
const addedHere = computed(() => Boolean(props.added && props.added === summary.value?.offer_id))
const collapsedTitle = computed(() => offerTitle(selected.value) || task.value || 'Подбор лицензии')

/** Строки одной закупки (рабочие станции и серверы): обе идут в спецификацию одной кнопкой, сумма — у каждой. */
const lines = computed(() => {
  const list = specOffers(r.value?.offers)
  return list.length > 1 ? list : []
})
const linesInSpec = computed(
  () => lines.value.length > 0 && lines.value.every((o) => props.inSpec(o.offer_id)),
)
function addLines() {
  if (loading.value) return
  for (const o of lines.value) emit('spec', o)
}

function select(id: string) {
  picked.value = id
}
function onMain() {
  const s = summary.value
  if (!s || loading.value) return
  const o = offerById(r.value, s.offer_id)
  if (s.action === 'request_quote') emit('quote', o)
  else if (o) emit('spec', o)
}
const reply = ref('')
function onReply() {
  const m = reply.value.trim()
  if (!m) return
  emit('say', m)
  reply.value = ''
}

function onChangeTask() {
  if (hasDirection.value) emit('answer', { key: 'direction', value: '' })
  else emit('refine')
}
</script>

<template>
  <section
    :class="['gw', { 'gw--compact': compact, 'gw--collapsed': collapsed }]"
    aria-labelledby="guide-title"
    data-testid="guide-panel"
    :data-type="r?.type ?? ''"
    :aria-busy="loading || undefined"
  >
    <header class="gw__head">
      <div class="gw__heading">
        <h2 id="guide-title" class="gw__title">{{ screen?.title || 'Подбор лицензии' }}</h2>
        <p v-if="collapsed" class="gw__task" data-testid="guide-collapsed-title">
          {{ collapsedTitle }}
        </p>
        <p v-else-if="task" class="gw__task">
          <span data-testid="guide-task">{{ task }}</span>
          <button
            type="button"
            class="gw__link"
            :disabled="loading"
            data-testid="guide-change-task"
            @click="onChangeTask"
          >
            Изменить
          </button>
        </p>
      </div>
      <div class="gw__tools">
        <button
          v-if="compact"
          type="button"
          class="btn btn--ghost btn--icon btn--sm"
          :aria-expanded="!collapsed"
          aria-controls="guide-body"
          :aria-label="collapsed ? 'Развернуть подбор' : 'Свернуть подбор'"
          data-testid="guide-collapse"
          @click="collapsed = !collapsed"
        >
          <ChevronDown v-if="collapsed" :size="18" aria-hidden="true" />
          <ChevronUp v-else :size="18" aria-hidden="true" />
        </button>
        <button
          v-else
          type="button"
          class="btn btn--ghost btn--icon btn--sm"
          aria-label="Свернуть подбор — условия сохранятся"
          title="Свернуть подбор"
          data-testid="guide-close"
          @click="emit('hide')"
        >
          <X :size="18" aria-hidden="true" />
        </button>
      </div>
    </header>

    <div v-show="!collapsed" id="guide-body" class="gw__scroll">
      <GuideFields
        v-if="conditions.length"
        :conditions="conditions"
        :compact="compact"
        @edit="(k, v) => emit('edit', k, v)"
      />

      <p v-if="said" class="gw__said" data-testid="guide-said">Вы: {{ said }}</p>

      <div v-if="changed && !stale" class="gw__changed" data-testid="guide-changes">
        <p class="gw__changed-text">{{ changed.text }}</p>
        <p v-if="changed.price_change" class="gw__tier" data-testid="guide-price-change">
          <span class="gw__tier-label">{{ changed.price_change.label }}</span>
          <span>
            {{ changed.price_change.from.tier }}: {{ changed.price_change.from.text }} →
            {{ changed.price_change.to.tier }}: <b>{{ changed.price_change.to.text }}</b>
          </span>
        </p>
      </div>

      <!-- Ожидание первого ответа: скелетон только на месте результата. -->
      <div v-if="loading && !r" class="gw__wait" role="status" data-testid="guide-loading">
        <span class="skeleton gw__sk gw__sk--title" />
        <span class="skeleton gw__sk" />
        <span class="skeleton gw__sk gw__sk--short" />
        <p class="gw__wait-text">Проверяем условия лицензирования</p>
        <p class="gw__muted">Результаты поиска уже доступны.</p>
      </div>

      <div v-else-if="failed && !loading" class="gw__error" role="alert" data-testid="guide-error">
        <p>Не удалось завершить подбор. Результаты поиска доступны, ваши условия сохранены.</p>
        <div class="gw__error-actions">
          <button type="button" class="btn btn--secondary" @click="emit('retry')">
            <RotateCcw :size="16" aria-hidden="true" /> Повторить подбор
          </button>
          <button type="button" class="gw__link" @click="emit('continue')">Продолжить поиск</button>
        </div>
      </div>

      <div
        v-if="r && !failed"
        :class="['gw__result', { 'gw__result--stale': stale }]"
        :inert="stale || undefined"
      >
        <p v-if="stale" class="gw__updating" role="status" data-testid="guide-updating">
          Обновляем подбор
        </p>

        <GuideQuestion
          v-if="r.clarification"
          :question="r.clarification"
          :busy="loading"
          @answer="(c) => emit('answer', c)"
        />

        <article
          v-if="result && resultOffer"
          class="gres"
          data-testid="guide-result"
          :data-basis="result.basis"
          :data-offer="resultOffer.offer_id"
        >
          <p class="gres__heading" data-testid="guide-pick-status">
            <Check :size="16" :stroke-width="2.5" aria-hidden="true" />
            {{ result.heading }}
          </p>
          <h3 class="gres__title" data-testid="guide-result-title">
            {{ offerTitle(resultOffer) }}
          </h3>
          <p v-if="offerSubtitle(resultOffer)" class="gres__sub">
            {{ offerSubtitle(resultOffer) }}
          </p>
          <p v-if="result.reason" class="gres__reason" data-testid="guide-reason">
            {{ result.reason }}
          </p>
          <p v-if="resultOffer.package?.text" class="gres__pack">{{ resultOffer.package.text }}</p>
          <p class="gres__links">
            <GuideSource v-if="result.source" :source="result.source" />
            <button
              v-if="details.length"
              type="button"
              class="gres__more"
              :aria-expanded="detailsOpen"
              @click="detailsOpen = !detailsOpen"
            >
              Сведения о лицензии
              <ChevronUp v-if="detailsOpen" :size="14" aria-hidden="true" />
              <ChevronDown v-else :size="14" aria-hidden="true" />
            </button>
          </p>
          <dl v-if="detailsOpen" class="gres__details" data-testid="guide-details">
            <div v-for="d in details" :key="d.label">
              <dt>{{ d.label }}</dt>
              <dd>{{ d.value }}</dd>
            </div>
          </dl>
        </article>
        <p
          v-else-if="result?.basis === 'none' && result.heading"
          class="gres__plain"
          data-testid="guide-pick-status"
        >
          {{ result.heading }}
        </p>
        <p v-else-if="r.lead && !r.clarification" class="gw__lead" data-testid="guide-lead">
          {{ r.lead }}
        </p>

        <GuideCompare
          v-if="screen?.comparison"
          :table="screen.comparison"
          :selected-id="selectedId"
          :narrow="compact"
          :busy="loading"
          @select="select"
        />

        <GuideTaskCost
          v-if="screen?.task_cost"
          :cost="screen.task_cost"
          :selected-id="selectedId"
          :busy="loading"
          @select="select"
        />

        <p v-if="r.preliminary" class="gw__note" data-testid="guide-preliminary">
          Результат предварительный: не все условия, от которых зависит право использования,
          подтверждены.
        </p>
        <p v-if="r.notice" class="gw__note" data-testid="guide-notice">{{ r.notice }}</p>
        <p v-if="isRefusal(r)" class="gw__note" data-testid="guide-refusal">
          {{
            r.type === 'out_of_scope'
              ? 'Здесь можно подобрать и сравнить лицензии.'
              : 'Вопрос можно передать менеджеру в запросе КП.'
          }}
        </p>
        <a
          v-if="r.type === 'no_data' && r.docs?.url"
          :href="r.docs.url"
          class="link gw__docs"
          target="_blank"
          rel="noopener noreferrer"
          >Документация вендора<span class="sr-only"> (откроется в новой вкладке)</span></a
        >

        <section
          v-if="others.length"
          class="gothers"
          aria-labelledby="guide-others-title"
          data-testid="guide-rejected"
        >
          <h3 id="guide-others-title" class="gothers__title">При изменении условий</h3>
          <ul>
            <li
              v-for="o in others"
              :key="o.offer_id"
              :data-offer="o.offer_id"
              :data-reason="o.reason?.code ?? ''"
              data-testid="guide-other"
            >
              <span class="gothers__name">
                {{ o.title }}<template v-if="o.subtitle"> · {{ o.subtitle }}</template>
              </span>
              <span class="gothers__price t-num">{{ o.price_text }}</span>
              <span v-if="o.reason?.text" class="gothers__why">{{ o.reason.text }}</span>
            </li>
          </ul>
        </section>

        <details v-if="steps.length" class="gtrace" data-testid="guide-trace">
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
      </div>
    </div>

    <form v-show="!collapsed" class="gw__reply" data-testid="guide-reply" @submit.prevent="onReply">
      <label for="guide-reply" class="sr-only">Уточнить подбор</label>
      <input
        id="guide-reply"
        v-model="reply"
        class="gw__reply-input"
        placeholder="Уточните: например, 50 компьютеров и шифрование"
        autocomplete="off"
        enterkeyhint="send"
        maxlength="300"
        data-testid="guide-text"
      />
      <button
        type="submit"
        class="gw__reply-send"
        :disabled="!reply.trim()"
        aria-label="Отправить уточнение"
      >
        <ArrowUp :size="16" :stroke-width="2.25" aria-hidden="true" />
      </button>
    </form>

    <footer
      v-if="!collapsed && r && !failed && (summary || addedHere || lines.length)"
      class="gw__total"
      data-testid="guide-total"
    >
      <template v-if="lines.length">
        <ul class="gw__lines">
          <li v-for="o in lines" :key="o.offer_id" data-testid="guide-line">
            <span class="gw__line-name">{{ offerTitle(o) }}</span>
            <span class="gw__line-sum t-num">{{ summaryFor(null, o)?.total_text }}</span>
            <span v-if="o.price_line" class="gw__formula t-num">{{ o.price_line }}</span>
          </li>
        </ul>
        <button
          type="button"
          class="btn btn--primary gw__cta"
          :disabled="loading"
          data-testid="guide-spec"
          @click="addLines"
        >
          {{ linesInSpec ? 'В спецификации' : 'В спецификацию' }}
        </button>
      </template>
      <template v-else-if="addedHere && summary">
        <div class="gw__added" role="status" data-testid="guide-added">
          <p class="gw__added-title">
            <Check :size="16" :stroke-width="2.5" aria-hidden="true" /> Добавлено в спецификацию
          </p>
          <p class="gw__added-name">
            {{ offerTitle(selected) }}
            <span class="gw__muted">{{ offerSubtitle(selected) }}</span>
          </p>
          <p class="gw__sum gw__sum--sm t-num">{{ summary.total_text }}</p>
          <p class="gw__added-links">
            <button type="button" class="gw__link" @click="emit('continue')">
              Продолжить поиск
            </button>
            <span aria-hidden="true">·</span>
            <button type="button" class="gw__link" @click="emit('openSpec')">
              Открыть спецификацию
            </button>
          </p>
        </div>
      </template>
      <template v-else-if="summary && summary.action === 'choose'">
        <p class="gw__choose" data-testid="guide-choose">
          {{ summary.hint || 'Выберите вариант, чтобы добавить в спецификацию' }}
        </p>
      </template>
      <template v-else-if="summary">
        <div class="gw__money">
          <span class="gw__sum-label">{{ summary.label }}</span>
          <span class="gw__sum t-num" data-testid="guide-sum">{{ summary.total_text }}</span>
          <span v-if="summary.formula" class="gw__formula t-num" data-testid="guide-formula">{{
            summary.formula
          }}</span>
        </div>
        <button
          type="button"
          class="btn btn--primary gw__cta"
          :disabled="loading"
          data-testid="guide-spec"
          @click="onMain"
        >
          {{
            summary.action === 'request_quote'
              ? 'Запросить КП'
              : inSpec(summary.offer_id)
                ? 'В спецификации'
                : 'В спецификацию'
          }}
        </button>
      </template>
    </footer>
  </section>
</template>

<style scoped>
.gw {
  container-type: inline-size;
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
  background: var(--surface);
}
.gw__head {
  display: flex;
  align-items: flex-start;
  gap: var(--space-3);
  padding: var(--space-4) var(--space-5) var(--space-3);
  border-bottom: var(--border-w) solid var(--border);
}
.gw__heading {
  flex: 1;
  min-width: 0;
}
.gw__title {
  margin: 0;
  color: var(--text-2);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
  font-weight: var(--fw-medium);
}
.gw__task {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 0 var(--space-3);
  margin: var(--space-1) 0 0;
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
  font-weight: var(--fw-semibold);
  overflow-wrap: anywhere;
}
.gw__tools {
  display: flex;
  margin: -2px calc(-1 * var(--space-2)) 0 0;
}
.gw__link {
  padding: 0;
  border: 0;
  background: none;
  color: var(--accent);
  font: inherit;
  font-size: var(--fs-sm);
  font-weight: var(--fw-regular);
  text-decoration: underline;
  text-underline-offset: 3px;
  cursor: pointer;
}
.gw__link:disabled {
  color: var(--text-3);
  cursor: default;
}
.gw__scroll {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-5);
  min-height: 0;
  padding: var(--space-4) var(--space-5) var(--space-6);
  overflow-y: auto;
  overscroll-behavior: contain;
}
.gw__said {
  margin: calc(-1 * var(--space-2)) 0 0;
  color: var(--text-3);
  font-size: var(--fs-sm);
  overflow-wrap: anywhere;
}
.gw__reply {
  display: flex;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-5);
  border-top: var(--border-w) solid var(--border);
  background: var(--surface);
}
.gw__reply-input {
  flex: 1;
  min-width: 0;
  height: 2.5rem;
  padding: 0 var(--space-3);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-control);
  background: var(--surface);
  color: var(--text);
  font: inherit;
  font-size: var(--fs-input);
}
.gw__reply-input::placeholder {
  color: var(--text-3);
}
.gw__reply-send {
  display: grid;
  flex: none;
  place-items: center;
  width: 2.5rem;
  height: 2.5rem;
  border: 0;
  border-radius: var(--radius-control);
  background: var(--text);
  color: var(--surface);
  cursor: pointer;
}
.gw__reply-send:disabled {
  background: var(--surface-3);
  color: var(--text-3);
  cursor: default;
}
.gw--compact .gw__reply {
  padding: var(--space-3) var(--space-4);
}
.gw__changed {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin-top: calc(-1 * var(--space-2));
}
.gw__changed-text {
  margin: 0;
  color: var(--text-2);
}
.gw__tier {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: var(--space-2) var(--space-3);
  border-left: 3px solid var(--border-control);
  font-size: var(--fs-sm);
}
.gw__tier-label {
  color: var(--text-2);
}
.gw__wait {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.gw__sk {
  display: block;
  height: 1rem;
  border-radius: var(--radius-sm);
}
.gw__sk--title {
  width: 60%;
  height: 1.5rem;
}
.gw__sk--short {
  width: 40%;
}
.gw__wait-text {
  margin: var(--space-2) 0 0;
  font-weight: var(--fw-medium);
}
.gw__muted {
  margin: 0;
  color: var(--text-2);
  font-size: var(--fs-sm);
  font-weight: var(--fw-regular);
}
.gw__error {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
  background: var(--surface-2);
}
.gw__error p {
  margin: 0;
}
.gw__error-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-3) var(--space-4);
}
.gw__result {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
  transition: opacity 0.15s;
}
.gw__result--stale > :not(.gw__updating) {
  opacity: 0.5;
}
.gw__updating {
  margin: 0 0 calc(-1 * var(--space-3));
  color: var(--text-2);
  font-size: var(--fs-sm);
  font-weight: var(--fw-medium);
}
.gw__lead {
  margin: 0;
}
.gw__note {
  margin: 0;
  color: var(--text-2);
  font-size: var(--fs-sm);
}
.gw__docs {
  align-self: flex-start;
  font-size: var(--fs-sm);
}

/* Результат: основание, название, одна фраза причины, источник */
.gres {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding: var(--space-4);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
}
.gres__heading {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  margin: 0;
  color: var(--status-accepted);
  font-weight: var(--fw-semibold);
}
.gres__title {
  margin: var(--space-1) 0 0;
  font-size: 1.375rem;
  line-height: 1.75rem;
  font-weight: var(--fw-semibold);
  overflow-wrap: anywhere;
}
.gres__sub {
  margin: 0;
  color: var(--text-2);
}
.gres__reason {
  margin: var(--space-2) 0 0;
  font-size: var(--fs-reading);
}
.gres__pack {
  margin: 0;
  color: var(--text-2);
  font-size: var(--fs-sm);
}
.gres__links {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-4);
  margin: var(--space-2) 0 0;
}
.gres__more {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  padding: 0;
  border: 0;
  background: none;
  color: var(--text-2);
  font: inherit;
  font-size: var(--fs-sm);
  cursor: pointer;
}
.gres__details {
  display: grid;
  gap: var(--space-1);
  margin: var(--space-2) 0 0;
  font-size: var(--fs-sm);
}
.gres__details div {
  display: grid;
  grid-template-columns: 9rem minmax(0, 1fr);
  gap: var(--space-2);
}
.gres__details dt {
  color: var(--text-2);
}
.gres__details dd {
  margin: 0;
  overflow-wrap: anywhere;
}
.gres__plain {
  margin: 0;
  font-size: var(--fs-h1);
  line-height: var(--lh-h1);
  font-weight: var(--fw-semibold);
}

/* При изменении условий: констатация, причина серым */
.gothers__title {
  margin: 0 0 var(--space-2);
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
  font-weight: var(--fw-semibold);
}
.gothers ul {
  margin: 0;
  padding: 0;
  list-style: none;
}
.gothers li {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 0 var(--space-3);
  padding: var(--space-2) 0;
  border-top: var(--border-w) solid var(--border);
  color: var(--text-2);
}
.gothers__name {
  overflow-wrap: anywhere;
}
.gothers__price {
  text-align: right;
  white-space: nowrap;
}
.gothers__why {
  grid-column: 1 / -1;
  color: var(--text-3);
  font-size: var(--fs-sm);
}
.gtrace {
  color: var(--text-2);
  font-size: var(--fs-sm);
}
.gtrace summary {
  cursor: pointer;
}
.gtrace__steps {
  margin: var(--space-2) 0 0;
  padding-left: var(--space-5);
}
.gtrace__res,
.gtrace__ms {
  color: var(--text-3);
}

/* Итоговая строка: сумма один раз, расчёт, одна кнопка */
.gw__total {
  position: sticky;
  bottom: 0;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-5);
  border-top: var(--border-w) solid var(--border);
  background: var(--surface);
}
.gw__money {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.gw__sum-label {
  color: var(--text-2);
  font-size: var(--fs-sm);
}
.gw__sum {
  font-size: 1.875rem;
  line-height: 2.25rem;
  font-weight: var(--fw-semibold);
  white-space: nowrap;
}
.gw__sum--sm {
  font-size: var(--fs-h1);
  line-height: var(--lh-h1);
}
.gw__formula {
  color: var(--text-2);
  font-size: var(--fs-sm);
}
.gw__cta {
  min-height: 2.75rem;
  padding-inline: var(--space-5);
}
.gw__lines {
  display: flex;
  flex: 1 1 16rem;
  flex-direction: column;
  gap: var(--space-1);
  margin: 0;
  padding: 0;
  list-style: none;
}
.gw__lines li {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 0 var(--space-3);
}
.gw__line-name {
  overflow-wrap: anywhere;
}
.gw__line-sum {
  font-weight: var(--fw-semibold);
  text-align: right;
}
.gw__lines .gw__formula {
  grid-column: 1 / -1;
}
.gw__choose {
  margin: 0;
  color: var(--text-2);
}
.gw__added {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  width: 100%;
}
.gw__added p {
  margin: 0;
}
.gw__added-title {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  color: var(--status-accepted);
  font-weight: var(--fw-semibold);
}
.gw__added-name {
  display: flex;
  flex-direction: column;
}
.gw__added-links {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  color: var(--text-3);
}

/* Телефон: блок над выдачей с рамкой */
.gw--compact {
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
}
.gw--compact .gw__head {
  padding: var(--space-3) var(--space-4);
}
.gw--compact.gw--collapsed .gw__head {
  border-bottom: 0;
}
.gw--compact .gw__scroll {
  padding: var(--space-3) var(--space-4) var(--space-4);
  overflow: visible;
}
.gw--compact .gw__total {
  position: sticky;
  bottom: 0;
  z-index: 5;
  padding: var(--space-3) var(--space-4);
  border-radius: 0 0 var(--radius-card) var(--radius-card);
  box-shadow: 0 -6px 16px rgb(15 18 25 / 0.06);
}
.gw--compact .gw__sum {
  font-size: var(--fs-banner);
  line-height: 1.2;
}
</style>
