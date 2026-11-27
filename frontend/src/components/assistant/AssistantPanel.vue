<script setup lang="ts">
// Панель помощника (CHAT.md §2, §6, §7): шапка 56 px, лента, поле ввода. Справа от выдачи 440 px; уже 1024 px — на
// весь экран, «назад» возвращает к выдаче без потери разговора. Прокручивается только лента; после ответа — к началу
// нового ответа; если покупатель читает старое, прокрутку не перехватываем — «К последнему ответу ↓».
import { nextTick, onMounted, ref, watch } from 'vue'
import { ArrowLeft, ChevronDown, ClipboardList, RotateCcw, X } from 'lucide-vue-next'
import type { ChatCard, ChatSuggestion } from '@/api/typesGuide'
import type { AssistantSession } from '@/composables/useAssistant'
import type { AssistantEntry } from '@/lib/assistant'
import AssistantComposer from './AssistantComposer.vue'
import AssistantMessage from './AssistantMessage.vue'
import AssistantWelcome from './AssistantWelcome.vue'

const props = defineProps<{
  session: AssistantSession
  specCount: number
  inSpec: (id: string) => boolean
  /** На весь экран (телефон, узкое окно). */
  full: boolean
}>()
const emit = defineEmits<{
  openSpec: []
  showAll: [query: string]
  quote: [entry: AssistantEntry, card: ChatCard]
}>()

const s = props.session
const feed = ref<HTMLElement | null>(null)
const composer = ref<InstanceType<typeof AssistantComposer> | null>(null)
const behind = ref(false)
let reading = false

function nearBottom(): boolean {
  const el = feed.value
  if (!el) return true
  return el.scrollHeight - el.scrollTop - el.clientHeight < 160
}

function onScroll() {
  if (behind.value && nearBottom()) behind.value = false
  // Пока ждём ответ, покупатель ушёл вверх читать старое — не перехватываем прокрутку.
  if (s.pending.value) reading = !nearBottom()
}

function scrollToEntry(id: number) {
  const el = feed.value?.querySelector<HTMLElement>(`[data-entry="${id}"]`)
  const f = feed.value
  if (!el || !f) return
  const top = el.getBoundingClientRect().top - f.getBoundingClientRect().top + f.scrollTop - 12
  f.scrollTo({ top, behavior: 'smooth' })
}

function toLatest() {
  behind.value = false
  scrollToEntry(s.lastId.value)
}

watch(
  () => s.entries.value.length,
  async (n, prev) => {
    const last = s.entries.value[n - 1]
    if (!last || n <= (prev ?? 0)) return
    await nextTick()
    if (last.kind === 'user') {
      reading = false
      feed.value?.scrollTo({ top: feed.value.scrollHeight, behavior: 'auto' })
      return
    }
    if (last.kind === 'assistant' || last.kind === 'error') {
      if (reading) behind.value = true
      else scrollToEntry(last.id)
    } else if (last.kind === 'system') {
      feed.value?.scrollTo({ top: feed.value.scrollHeight, behavior: 'smooth' })
    }
  },
)

function onSend(text: string) {
  reading = false
  void s.send(text)
}
function onSuggest(x: ChatSuggestion | { label: string; key: string; value: string }) {
  reading = false
  void s.suggest(x)
}
function onPick(message: string, label: string) {
  reading = false
  if (message === label) void s.send(message)
  else void s.suggest({ label, message })
}
function onReset() {
  s.resetConditions()
  composer.value?.focus()
}

onMounted(() => {
  const id = s.lastId.value
  void nextTick(() => {
    if (id >= 0) scrollToEntry(id)
    composer.value?.focus()
  })
})
</script>

<template>
  <section
    id="assistant-panel"
    :class="['as-panel', { 'as-panel--full': full }]"
    role="dialog"
    :aria-modal="full || undefined"
    aria-labelledby="assistant-title"
    data-testid="assistant-panel"
  >
    <header class="as-panel__head">
      <button
        v-if="full"
        type="button"
        class="btn btn--ghost btn--icon as-panel__icon"
        aria-label="Назад к выдаче"
        data-testid="assistant-back"
        @click="s.close()"
      >
        <ArrowLeft :size="20" aria-hidden="true" />
      </button>
      <div class="as-panel__titles">
        <h2 id="assistant-title" class="as-panel__title">Помощник по выбору</h2>
        <p v-if="s.product.value" class="as-panel__about">О продукте: {{ s.product.value }}</p>
      </div>
      <button
        type="button"
        class="btn btn--ghost btn--sm as-panel__spec"
        :aria-label="`Спецификация: ${specCount}`"
        data-testid="assistant-spec-count"
        @click="emit('openSpec')"
      >
        <ClipboardList :size="18" aria-hidden="true" />
        <span class="t-num">{{ specCount }}</span>
      </button>
      <button
        v-if="s.hasConversation.value"
        type="button"
        class="btn btn--ghost btn--icon as-panel__icon"
        aria-label="Начать новый подбор"
        title="Начать новый подбор"
        data-testid="assistant-clear"
        @click="s.clearAll()"
      >
        <RotateCcw :size="18" aria-hidden="true" />
      </button>
      <button
        v-if="!full"
        type="button"
        class="btn btn--ghost btn--icon as-panel__icon"
        aria-label="Закрыть помощника"
        data-testid="assistant-close"
        @click="s.close()"
      >
        <X :size="20" aria-hidden="true" />
      </button>
    </header>

    <div ref="feed" class="as-panel__feed" data-testid="assistant-feed" @scroll.passive="onScroll">
      <AssistantWelcome v-if="!s.hasConversation.value" :context="s.context.value" @pick="onPick" />
      <template v-for="e in s.entries.value" :key="e.id">
        <div v-if="e.kind === 'user'" class="as-panel__user" :data-entry="e.id">
          <p class="as-panel__bubble" data-testid="assistant-user">{{ e.text }}</p>
        </div>
        <div v-else-if="e.kind === 'assistant'" :data-entry="e.id">
          <AssistantMessage
            :entry="e"
            :last="e.id === s.lastId.value"
            :busy="s.pending.value"
            :current-conditions="s.conditionsLabel.value"
            :in-spec="inSpec"
            :adding="s.adding.value"
            :failed="s.failedAdd.value"
            @spec="(en, c) => void s.addToSpec(en, c)"
            @quote="(en, c) => emit('quote', en, c)"
            @suggest="onSuggest"
            @show-all="(q: string) => emit('showAll', q)"
          />
        </div>
        <p
          v-else-if="e.kind === 'system'"
          class="as-panel__system"
          role="status"
          :data-entry="e.id"
          data-testid="assistant-system"
        >
          {{ e.text }}
          <button v-if="e.openSpec" type="button" class="link" @click="emit('openSpec')">
            · Открыть спецификацию
          </button>
        </p>
        <p
          v-else
          class="as-panel__error"
          role="alert"
          :data-entry="e.id"
          data-testid="assistant-error"
        >
          {{ e.text }}
          <button type="button" class="link" :disabled="s.pending.value" @click="s.retry()">
            Повторить
          </button>
        </p>
      </template>
      <p v-if="s.pending.value" class="as-panel__wait" role="status" data-testid="assistant-wait">
        <span class="as-panel__dots" aria-hidden="true"><i /><i /><i /></span>
        {{ s.waitText.value }}
      </p>
    </div>

    <button
      v-if="behind"
      type="button"
      class="as-panel__latest"
      data-testid="assistant-latest"
      @click="toLatest"
    >
      К последнему ответу <ChevronDown :size="16" aria-hidden="true" />
    </button>

    <AssistantComposer
      ref="composer"
      v-model="s.draft.value"
      :conditions="s.conditionsLabel.value"
      :busy="s.pending.value"
      :has-conversation="s.hasConversation.value"
      @send="onSend"
      @reset="onReset"
    />
  </section>
</template>

<style scoped>
.as-panel {
  position: relative;
  display: flex;
  flex-direction: column;
  min-height: 0;
  height: 100%;
  background: var(--bg);
}
.as-panel--full {
  position: fixed;
  inset: 0;
  z-index: 60;
  height: 100dvh;
}
.as-panel__head {
  display: flex;
  flex: none;
  align-items: center;
  gap: var(--space-1);
  min-height: 56px;
  padding: 0 var(--space-2) 0 16px;
  border-bottom: var(--border-w) solid var(--border);
  background: var(--surface);
}
.as-panel--full .as-panel__head {
  padding-left: var(--space-1);
}
.as-panel__titles {
  flex: 1;
  min-width: 0;
}
.as-panel__title {
  margin: 0;
  font-size: 16px;
  line-height: 22px;
  font-weight: var(--fw-semibold);
}
.as-panel__about {
  margin: 0;
  color: var(--text-2);
  font-size: 12px;
  line-height: 16px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.as-panel__spec {
  gap: var(--space-1);
}
.as-panel__feed {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 24px;
  min-height: 0;
  padding: 16px;
  overflow-x: hidden;
  overflow-y: auto;
  overscroll-behavior: contain;
}
.as-panel__user {
  display: flex;
  justify-content: flex-end;
}
.as-panel__bubble {
  max-width: 85%;
  margin: 0;
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-card) var(--radius-card) var(--radius-sm) var(--radius-card);
  background: var(--accent-soft);
  font-size: 14px;
  line-height: 21px;
  overflow-wrap: anywhere;
}
.as-panel__system,
.as-panel__error,
.as-panel__wait {
  margin: 0;
  font-size: 12px;
  line-height: 16px;
}
.as-panel__system {
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-sm);
  background: var(--status-accepted-bg);
  color: var(--status-accepted);
  font-weight: var(--fw-medium);
}
.as-panel__error {
  color: var(--danger);
}
.as-panel__wait {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  color: var(--text-2);
}
.as-panel__dots {
  display: inline-flex;
  gap: 3px;
}
.as-panel__dots i {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: var(--text-3);
  animation: as-dot 1.2s infinite ease-in-out;
}
.as-panel__dots i:nth-child(2) {
  animation-delay: 0.15s;
}
.as-panel__dots i:nth-child(3) {
  animation-delay: 0.3s;
}
@keyframes as-dot {
  0%,
  80%,
  100% {
    opacity: 0.3;
  }
  40% {
    opacity: 1;
  }
}
@media (prefers-reduced-motion: reduce) {
  .as-panel__dots i {
    animation: none;
  }
}
.as-panel__latest {
  position: absolute;
  bottom: 120px;
  left: 50%;
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  padding: var(--space-1) var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-pill);
  background: var(--surface);
  box-shadow: var(--shadow-overlay);
  font: inherit;
  font-size: var(--fs-sm);
  transform: translateX(-50%);
  cursor: pointer;
}
@media (max-width: 1023px) {
  .as-panel__icon {
    min-width: var(--touch-min);
    min-height: var(--touch-min);
  }
}
</style>
