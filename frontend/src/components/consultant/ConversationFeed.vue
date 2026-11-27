<script setup lang="ts">
// Лента (CONSULTANT.md §4–§5): приветствие обычными сообщениями и четыре подсказки до первой реплики; группы
// реплик; «Печатает…»; быстрые ответы под последней репликой; прокрутка с порогом 80 px и «Новый ответ».
import { computed, onMounted, ref, watch } from 'vue'
import { ArrowDown } from 'lucide-vue-next'
import { type ChatMessage, GREETING, STARTERS, visibleMessage } from '@/lib/consultant'
import MessageGroup from './MessageGroup.vue'
import QuickReplies from './QuickReplies.vue'
import TypingIndicator from './TypingIndicator.vue'
import { useConversationScroll } from './composables/useConversationScroll'

const props = defineProps<{
  messages: ChatMessage[]
  generating: boolean
  step: string | null
  quick: { id: string; replies: string[] } | null
}>()
const emit = defineEmits<{ send: [text: string]; retry: [] }>()

const feed = ref<HTMLElement | null>(null)
const scroll = useConversationScroll(feed)

const started = computed(() => props.messages.some((m) => m.role === 'user'))
const greeting: ChatMessage[] = GREETING.map((text, i) => ({
  id: `greet-${i}`,
  role: 'assistant',
  text,
  attachments: [],
  status: 'complete',
  turn: 0,
}))

/** Группы подряд идущих реплик одного собеседника; приветствие — первая группа консультанта. */
const groups = computed(() => {
  const list = [...greeting, ...props.messages.filter(visibleMessage)]
  const out: { key: string; role: 'user' | 'assistant'; messages: ChatMessage[] }[] = []
  for (const m of list) {
    const last = out[out.length - 1]
    if (last && last.role === m.role) last.messages.push(m)
    else out.push({ key: m.id, role: m.role, messages: [m] })
  }
  return out
})

const lastText = computed(() => {
  const m = props.messages[props.messages.length - 1]
  return m ? `${m.id}|${m.text.length}|${m.attachments.length}` : ''
})

watch(
  () => [props.messages.length, lastText.value, props.generating] as const,
  (_, prev) => {
    const m = props.messages[props.messages.length - 1]
    // Своя реплика — всегда вниз.
    if (m?.role === 'user' && prev && props.messages.length !== prev[0]) void scroll.toBottom()
    else void scroll.onContent()
  },
)

onMounted(() => void scroll.toBottom())

defineExpose({ toBottom: () => scroll.toBottom() })
</script>

<template>
  <div class="cf-wrap">
    <div
      ref="feed"
      class="cf consultant-feed"
      role="log"
      aria-live="polite"
      aria-label="Переписка с консультантом"
      data-testid="consultant-feed"
      tabindex="0"
      @scroll.passive="scroll.measure"
    >
      <MessageGroup
        v-for="g in groups"
        :key="g.key"
        :role="g.role"
        :messages="g.messages"
        @retry="emit('retry')"
      />
      <QuickReplies
        v-if="!started"
        :items="STARTERS"
        testid="consultant-starters"
        @pick="emit('send', $event)"
      />
      <div v-if="generating" class="cf__typing"><TypingIndicator :step="step" /></div>
      <QuickReplies v-if="quick" :items="quick.replies" @pick="emit('send', $event)" />
    </div>
    <button
      v-if="scroll.hasNew.value"
      type="button"
      class="cf__new"
      data-testid="consultant-new-answer"
      @click="scroll.toBottom(true)"
    >
      Новый ответ <ArrowDown :size="14" aria-hidden="true" />
    </button>
  </div>
</template>

<style scoped>
.cf-wrap {
  position: relative;
  display: flex;
  flex: 1;
  min-height: 0;
  flex-direction: column;
}
.cf {
  display: flex;
  flex: 1;
  min-height: 0;
  flex-direction: column;
  gap: var(--space-4);
  padding: var(--cw-pad, var(--space-4));
  overflow-x: hidden;
  overflow-y: auto;
  overscroll-behavior: contain;
}
.cf:focus-visible {
  outline: var(--focus-w) solid var(--focus-ring);
  outline-offset: -2px;
}
.cf__new {
  position: absolute;
  bottom: var(--space-3);
  left: 50%;
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--border);
  border-radius: var(--radius-pill);
  background: var(--surface);
  box-shadow: var(--shadow-overlay);
  color: var(--text);
  font: inherit;
  font-size: var(--fs-sm);
  font-weight: var(--fw-medium);
  cursor: pointer;
  transform: translateX(-50%);
}
@media (max-width: 640px) {
  .cf {
    padding: var(--space-3);
  }
}
</style>
