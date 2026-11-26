<script setup lang="ts">
// Строка поиска на reka Combobox: подсказки из GET /api/suggest от двух символов,
// сгруппированы, не больше 8 строк; совпавшая часть — начертанием. Эндпоинт не отвечает или 404 —
// подсказок молча нет, строка работает как обычное поле.
// Enter ищет введённый текст; подсказку выбирают стрелками + Enter или мышью. Esc закрывает список.
import { computed, onBeforeUnmount, ref } from 'vue'
import {
  ComboboxAnchor,
  ComboboxContent,
  ComboboxGroup,
  ComboboxInput,
  ComboboxItem,
  ComboboxLabel,
  ComboboxPortal,
  ComboboxRoot,
  ComboboxViewport,
} from 'reka-ui'
import { Search, X } from 'lucide-vue-next'
import { getSuggest } from '@/api/search'
import {
  highlightParts,
  shouldSuggest,
  toSuggestGroups,
  type SuggestRow,
  type SuggestRowGroup,
} from '@/lib/suggest'

const text = defineModel<string>({ required: true })
const props = defineProps<{ clientId: string | null }>()
const emit = defineEmits<{
  submit: [text: string]
  pick: [row: SuggestRow]
}>()

const SUGGEST_DEBOUNCE_MS = 150

const groups = ref<SuggestRowGroup[]>([])
const openRequested = ref(false)
/** Пользователь двигался по списку стрелками: только тогда Enter выбирает подсветку. */
const navigated = ref(false)
const highlighted = ref(false)
const open = computed(() => openRequested.value && groups.value.length > 0)
const inputRef = ref<InstanceType<typeof ComboboxInput> | null>(null)

let timer: ReturnType<typeof setTimeout> | undefined
let ctrl: AbortController | null = null

function cancelSuggest() {
  if (timer) clearTimeout(timer)
  timer = undefined
  ctrl?.abort()
  ctrl = null
}

function requestSuggest(q: string) {
  cancelSuggest()
  if (!shouldSuggest(q)) {
    groups.value = []
    return
  }
  timer = setTimeout(() => {
    const c = new AbortController()
    ctrl = c
    getSuggest(q.trim(), props.clientId, { signal: c.signal })
      .then((resp) => {
        if (ctrl === c) groups.value = toSuggestGroups(resp)
      })
      .catch(() => {
        // Подсказки — не критичная часть: 404, сеть, отмена — просто без них.
        if (ctrl === c) groups.value = []
      })
  }, SUGGEST_DEBOUNCE_MS)
}

function onType(v: string) {
  text.value = v
  navigated.value = false
  requestSuggest(v)
}

function onOpenUpdate(v: boolean) {
  openRequested.value = v
  if (!v) navigated.value = false
}

function onHighlight(payload: unknown) {
  highlighted.value = Boolean(payload)
}

function submit() {
  cancelSuggest()
  openRequested.value = false
  navigated.value = false
  emit('submit', text.value)
}

/** Перехват Enter до обработчика reka: без навигации стрелками ищем введённое, а не первую подсказку. */
function onKeydownCapture(e: KeyboardEvent) {
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    if (open.value) navigated.value = true
    return
  }
  if (e.key !== 'Enter' || e.isComposing) return
  if (open.value && navigated.value && highlighted.value) return
  e.preventDefault()
  e.stopImmediatePropagation()
  submit()
}

function onSelect(e: Event, row: SuggestRow) {
  e.preventDefault()
  cancelSuggest()
  text.value = row.text
  openRequested.value = false
  navigated.value = false
  emit('pick', row)
}

function clear() {
  text.value = ''
  groups.value = []
  cancelSuggest()
  focus()
}

function focus() {
  const el = (inputRef.value?.$el ?? null) as HTMLInputElement | null
  el?.focus()
}

onBeforeUnmount(cancelSuggest)
defineExpose({ focus })
</script>

<template>
  <form class="search-box" role="search" @submit.prevent="submit">
    <div class="search-box__root">
      <ComboboxRoot
        :open="open"
        :ignore-filter="true"
        :reset-search-term-on-blur="false"
        :reset-search-term-on-select="false"
        @update:open="onOpenUpdate"
        @highlight="onHighlight"
      >
        <ComboboxAnchor class="search-box__field">
          <Search :size="18" aria-hidden="true" class="search-box__icon" />
          <ComboboxInput
            ref="inputRef"
            :model-value="text"
            auto-focus
            class="search-box__input"
            type="text"
            inputmode="search"
            enterkeyhint="search"
            placeholder="Название, задача своими словами или артикул"
            aria-label="Поиск по каталогу"
            @update:model-value="onType"
            @keydown.capture="onKeydownCapture"
          />
          <button
            v-if="text"
            type="button"
            class="btn btn--ghost btn--icon btn--sm search-box__clear"
            aria-label="Очистить строку"
            @click="clear"
          >
            <X :size="16" aria-hidden="true" />
          </button>
        </ComboboxAnchor>
        <ComboboxPortal>
          <ComboboxContent
            position="popper"
            :side-offset="4"
            :class="['suggest', { 'suggest--kbd': navigated }]"
          >
            <ComboboxViewport class="suggest__viewport">
              <ComboboxGroup v-for="g in groups" :key="g.type" class="suggest__group">
                <ComboboxLabel class="suggest__label">{{ g.label }}</ComboboxLabel>
                <ComboboxItem
                  v-for="row in g.rows"
                  :key="row.key"
                  :value="row.key"
                  :text-value="row.text"
                  class="suggest__item"
                  @select="onSelect($event, row)"
                >
                  <span>
                    <template v-for="(part, i) in highlightParts(row.text, row.highlight)" :key="i">
                      <strong v-if="part.match" class="suggest__match">{{ part.text }}</strong>
                      <template v-else>{{ part.text }}</template>
                    </template>
                  </span>
                </ComboboxItem>
              </ComboboxGroup>
            </ComboboxViewport>
          </ComboboxContent>
        </ComboboxPortal>
      </ComboboxRoot>
    </div>
    <button type="submit" class="btn btn--primary search-box__submit">Найти</button>
  </form>
</template>

<style scoped>
.search-box {
  display: flex;
  gap: var(--space-2);
  min-width: 0;
}
.search-box__root {
  flex: 1;
  min-width: 0;
}
.search-box__field {
  position: relative;
  display: flex;
  align-items: center;
  min-height: var(--search-h);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-control);
  background: var(--surface);
}
.search-box__field:focus-within {
  border-color: var(--focus-ring);
  box-shadow: inset 0 0 0 1px var(--focus-ring);
}
.search-box__icon {
  flex: none;
  margin-left: var(--space-3);
  color: var(--text-2);
}
.search-box__input {
  flex: 1;
  min-width: 0;
  height: var(--search-h);
  padding: 0 var(--space-2);
  border: 0;
  background: transparent;
  color: var(--text);
  font: inherit;
  font-size: var(--fs-input);
  outline: none;
}
.search-box__input::placeholder {
  color: var(--text-3);
  opacity: 1;
}
.search-box__clear {
  margin-right: var(--space-1);
}
.search-box__submit {
  min-height: var(--search-h);
}
@media (max-width: 479px) {
  .search-box__submit {
    display: none;
  }
}
</style>

<style>
.suggest {
  z-index: 40;
  box-sizing: border-box;
  width: var(--reka-combobox-trigger-width);
  max-height: min(var(--popover-max-h), var(--reka-combobox-content-available-height));
  overflow: auto;
  background: var(--surface-2);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  box-shadow: var(--shadow-overlay);
}
.suggest__viewport {
  padding: var(--space-1);
}
.suggest__group + .suggest__group {
  margin-top: var(--space-1);
  padding-top: var(--space-1);
  border-top: var(--border-w) solid var(--border);
}
.suggest__label {
  padding: var(--space-1) var(--space-2);
  color: var(--text-3);
  font-size: var(--fs-xs);
  line-height: var(--lh-xs);
}
.suggest__item {
  display: flex;
  align-items: center;
  min-height: var(--control-h);
  padding: 0 var(--space-2);
  border-radius: var(--radius-control);
  color: var(--text-2);
  cursor: pointer;
  outline: none;
}
.suggest__item:hover,
.suggest--kbd .suggest__item[data-highlighted] {
  background: var(--surface-3);
  color: var(--text);
}
.suggest__match {
  color: var(--text);
  font-weight: var(--fw-semibold);
}
</style>
