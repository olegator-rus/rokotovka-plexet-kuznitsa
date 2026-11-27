<script setup lang="ts">
// Сообщение помощника (CHAT.md §3): блок на всю ширину без пузыря и аватара — фраза, предупреждение, таблица,
// карточки группами, «Показать все в поиске», источник, вопрос с кнопками, подсказки. Только нужные блоки.
import { computed } from 'vue'
import type { ChatCard, ChatSuggestion } from '@/api/typesGuide'
import type { AssistantEntry } from '@/lib/assistant'
import AssistantComparison from './AssistantComparison.vue'
import AssistantProductCard from './AssistantProductCard.vue'
import AssistantSources from './AssistantSources.vue'
import AssistantSuggestions from './AssistantSuggestions.vue'

const props = defineProps<{
  entry: AssistantEntry
  /** Последний ответ: подсказки и кнопки вопроса активны только у него. */
  last: boolean
  busy: boolean
  /** Текущая строка условий: у старых карточек с другими условиями — «Предыдущие условия». */
  currentConditions: string
  inSpec: (id: string) => boolean
  adding: Set<string>
  failed: Set<string>
}>()
const emit = defineEmits<{
  spec: [entry: AssistantEntry, card: ChatCard]
  quote: [entry: AssistantEntry, card: ChatCard]
  suggest: [s: ChatSuggestion | { label: string; key: string; value: string }]
  showAll: [query: string]
}>()

const chat = computed(() => props.entry.chat)
const stale = computed(
  () =>
    !props.last &&
    Boolean(props.currentConditions) &&
    chat.value.conditions.label !== props.currentConditions,
)
</script>

<template>
  <section
    class="as-msg"
    data-testid="assistant-message"
    :data-status="chat.status"
    aria-label="Ответ помощника"
  >
    <p v-for="(t, i) in chat.text" :key="i" class="as-msg__text">{{ t }}</p>
    <p v-if="chat.warning" class="as-msg__warn" role="note">{{ chat.warning }}</p>

    <AssistantComparison v-if="chat.comparison" :table="chat.comparison" />

    <div v-for="g in chat.groups" :key="g.key" class="as-msg__group" :data-group="g.key">
      <h3 v-if="g.title" class="as-msg__group-title">{{ g.title }}</h3>
      <AssistantProductCard
        v-for="c in g.cards"
        :key="c.offer_id"
        :card="c"
        :in-spec="inSpec(c.offer_id)"
        :adding="adding.has(c.offer_id)"
        :failed="failed.has(c.offer_id)"
        :stale="stale"
        @spec="emit('spec', entry, $event)"
        @quote="emit('quote', entry, $event)"
      />
    </div>

    <button
      v-if="chat.more"
      type="button"
      class="as-msg__more"
      data-testid="assistant-more"
      @click="emit('showAll', chat.more.query)"
    >
      {{ chat.more.label }} →
    </button>

    <AssistantSources v-if="chat.sources.length && !chat.comparison" :sources="chat.sources" />

    <ul v-if="chat.links.length" class="as-msg__links">
      <li v-for="l in chat.links" :key="l.url">
        <RouterLink v-if="l.url.startsWith('/')" :to="l.url" class="link"
          >{{ l.label }} →</RouterLink
        >
        <a v-else :href="l.url" target="_blank" rel="noopener" class="link">{{ l.label }} ↗</a>
      </li>
    </ul>

    <div v-if="chat.question" class="as-msg__question" data-testid="assistant-question">
      <p class="as-msg__q">{{ chat.question.text }}</p>
      <AssistantSuggestions
        :items="chat.question.chips"
        :disabled="!last || busy"
        testid="assistant-question-chips"
        @pick="(i: number) => emit('suggest', chat.question!.chips[i]!)"
      />
    </div>

    <AssistantSuggestions
      v-if="chat.suggestions.length && last"
      :items="chat.suggestions"
      :disabled="busy"
      @pick="(i: number) => emit('suggest', chat.suggestions[i]!)"
    />
  </section>
</template>

<style scoped>
.as-msg {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.as-msg__text {
  margin: 0;
  font-size: 14px;
  line-height: 21px;
}
.as-msg__text + .as-msg__text {
  margin-top: -12px;
}
.as-msg__warn {
  margin: 0;
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-sm);
  background: var(--status-pending-bg);
  color: var(--status-pending);
  font-size: 12px;
  line-height: 16px;
}
.as-msg__group {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.as-msg__group-title {
  margin: 0;
  color: var(--text-2);
  font-size: 12px;
  line-height: 16px;
  font-weight: var(--fw-semibold);
}
.as-msg__more {
  align-self: flex-start;
  padding: 0;
  border: 0;
  background: none;
  color: var(--accent);
  font: inherit;
  font-size: var(--fs-sm);
  font-weight: var(--fw-semibold);
  cursor: pointer;
}
.as-msg__links {
  margin: 0;
  padding: 0;
  list-style: none;
  font-size: var(--fs-sm);
}
.as-msg__question {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.as-msg__q {
  margin: 0;
  font-size: 14px;
  line-height: 21px;
  font-weight: var(--fw-semibold);
}
</style>
