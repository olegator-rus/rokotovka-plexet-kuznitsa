<script setup lang="ts">
// Окно консультанта (CONSULTANT.md §3): монтируется один раз в оболочке, поэтому разговор переживает смену
// страницы. На десктопе — немодальное окно 440 × 680 (развёрнутое 640 × 760) над кнопкой; на телефоне (≤ 640 px) —
// полный экран, фон не прокручивается, поле ввода держится над клавиатурой (visualViewport).
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useMediaQuery } from '@/composables/useMediaQuery'
import { useSpecStore } from '@/stores/spec'
import ConsultantHeader from './ConsultantHeader.vue'
import ConversationFeed from './ConversationFeed.vue'
import MessageComposer from './MessageComposer.vue'
import NewConversationDialog from './NewConversationDialog.vue'
import { useConsultant } from './composables/useConsultant'

const c = useConsultant()
const spec = useSpecStore()
const phone = useMediaQuery('(max-width: 640px)')
const confirmNew = ref(false)
const composer = ref<InstanceType<typeof MessageComposer> | null>(null)
const feed = ref<InstanceType<typeof ConversationFeed> | null>(null)
const root = ref<HTMLElement | null>(null)

function send(text: string) {
  if (c.send(text)) {
    c.draft.value = ''
    void feed.value?.toBottom()
  }
}

function openSpec() {
  if (phone.value) c.hide()
  spec.open = true
}

function reset() {
  c.newConversation()
  void nextTick(() => composer.value?.focus())
}

// Фокус в поле при открытии — только на десктопе (на телефоне клавиатура сама не открывается).
watch(
  () => c.open.value,
  async (open) => {
    if (!open || phone.value) return
    await nextTick()
    composer.value?.focus()
  },
  { immediate: true },
)

// Телефон: высота окна — видимая область (клавиатура не закрывает поле), фон не прокручивается.
const vvHeight = ref<string>('')
function onViewport() {
  const vv = window.visualViewport
  vvHeight.value = vv ? `${Math.round(vv.height)}px` : ''
  if (vv && root.value) root.value.style.transform = `translateY(${Math.round(vv.offsetTop)}px)`
}
watch(
  () => c.open.value && phone.value,
  (full) => {
    if (typeof document === 'undefined') return
    document.documentElement.style.overflow = full ? 'hidden' : ''
    const vv = window.visualViewport
    if (!vv) return
    if (full) {
      vv.addEventListener('resize', onViewport)
      vv.addEventListener('scroll', onViewport)
      onViewport()
    } else {
      vv.removeEventListener('resize', onViewport)
      vv.removeEventListener('scroll', onViewport)
      vvHeight.value = ''
      if (root.value) root.value.style.transform = ''
    }
  },
  { immediate: true },
)
onBeforeUnmount(() => {
  if (typeof document !== 'undefined') document.documentElement.style.overflow = ''
  window.visualViewport?.removeEventListener('resize', onViewport)
  window.visualViewport?.removeEventListener('scroll', onViewport)
})

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && !confirmNew.value) c.hide()
}
</script>

<template>
  <section
    v-show="c.open.value"
    id="consultant-window"
    ref="root"
    class="cw consultant-window"
    :data-expanded="c.expanded.value"
    :style="phone && vvHeight ? { height: vvHeight } : undefined"
    role="dialog"
    aria-labelledby="consultant-title"
    data-testid="consultant-window"
    @keydown="onKey"
  >
    <ConsultantHeader
      :spec-count="spec.count"
      :expanded="c.expanded.value"
      @spec="openSpec"
      @expand="c.expanded.value = !c.expanded.value"
      @reset="confirmNew = true"
      @close="c.hide()"
    />
    <ConversationFeed
      ref="feed"
      :messages="c.messages.value"
      :generating="c.generating.value"
      :step="c.step.value"
      :quick="c.quick.value"
      @send="send"
      @retry="c.retry()"
    />
    <MessageComposer
      ref="composer"
      v-model="c.draft.value"
      :busy="c.generating.value"
      @send="send"
    />
    <NewConversationDialog v-model:open="confirmNew" @confirm="reset" />
  </section>
</template>

<style scoped>
.cw {
  --cw-fs: var(--fs-body);
  --cw-lh: 1.5;
  --cw-price-fs: 1.125rem;
  --cw-pad: var(--space-4);
  position: fixed;
  right: calc(var(--fab-right) + env(safe-area-inset-right, 0px));
  bottom: calc(var(--fab-bottom) + var(--fab-size) + var(--space-4));
  z-index: 45;
  display: flex;
  flex-direction: column;
  width: min(440px, calc(100vw - 48px));
  height: min(680px, calc(100dvh - var(--fab-bottom) - var(--fab-size) - var(--space-8)));
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: var(--radius-panel);
  background: var(--surface);
  box-shadow: var(--shadow-overlay);
  color: var(--text);
}
/* Развёрнутый режим — для показа по видеосвязи: текст 16 / 24, стоимость 20 px. */
.cw[data-expanded='true'] {
  --cw-fs: 1rem;
  --cw-lh: 1.5;
  --cw-price-fs: 1.25rem;
  --cw-pad: var(--space-5);
  width: min(640px, calc(100vw - 48px));
  height: min(760px, calc(100dvh - var(--fab-bottom) - var(--fab-size) - var(--space-8)));
}
@media (max-width: 640px) {
  .cw,
  .cw[data-expanded='true'] {
    --cw-fs: 0.9375rem;
    --cw-price-fs: 1.125rem;
    --cw-pad: var(--space-3);
    inset: 0;
    z-index: 55;
    width: 100%;
    height: 100dvh;
    max-height: none;
    border: 0;
    border-radius: 0;
    box-shadow: none;
  }
}
</style>
