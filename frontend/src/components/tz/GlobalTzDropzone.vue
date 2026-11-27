<script setup lang="ts">
// Зона сброса ТЗ на любой странице (TZ-UI.md §2 «Перетаскивание») и единственное поле выбора файла приложения.
// Зона появляется только при перетаскивании файла из системы: перетаскивание текста и картинок внутри страницы её не
// вызывает. Курсор ушёл за окно — зона исчезает. Консультант (кнопка и окно) — поверх зоны и в неё не входит:
// файл, брошенный на него, разбор ТЗ не запускает.
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { FileText } from 'lucide-vue-next'
import { TZ_ACCEPT, TZ_TEXT } from '@/lib/tz'
import { useTzStore } from '@/stores/tz'

const tz = useTzStore()
const input = ref<HTMLInputElement | null>(null)
const shown = ref(false)
let depth = 0
let internal = false

const CONSULTANT = '.cl, .cw, .consultant-window, .as-entry, .search-page__guide'

function hasFiles(e: DragEvent): boolean {
  return Array.from(e.dataTransfer?.types ?? []).includes('Files')
}

function onDragStart() {
  internal = true
}
function onDragEnd() {
  internal = false
}

function hide() {
  shown.value = false
  depth = 0
}

function onDragEnter(e: DragEvent) {
  if (internal || !hasFiles(e)) return
  depth++
  shown.value = true
  e.preventDefault()
}

function onDragOver(e: DragEvent) {
  if (!shown.value) return
  e.preventDefault()
  const overConsultant = (e.target as Element | null)?.closest?.(CONSULTANT)
  if (e.dataTransfer) e.dataTransfer.dropEffect = overConsultant ? 'none' : 'copy'
}

function onDragLeave(e: DragEvent) {
  if (!shown.value) return
  depth = Math.max(0, depth - 1)
  // relatedTarget null — курсор ушёл за окно.
  if (depth === 0 || e.relatedTarget === null) hide()
}

function onDrop(e: DragEvent) {
  if (!shown.value) return
  e.preventDefault()
  hide()
  if ((e.target as Element | null)?.closest?.(CONSULTANT)) return
  tz.startFiles(Array.from(e.dataTransfer?.files ?? []))
}

function onPicked() {
  const el = input.value
  if (!el) return
  const files = Array.from(el.files ?? [])
  // Тот же файл можно выбрать снова (повтор после ошибки).
  el.value = ''
  if (files.length) tz.startFiles(files)
}

onMounted(() => {
  tz.registerPicker(() => input.value?.click())
  document.addEventListener('dragstart', onDragStart)
  document.addEventListener('dragend', onDragEnd)
  window.addEventListener('dragenter', onDragEnter)
  window.addEventListener('dragover', onDragOver)
  window.addEventListener('dragleave', onDragLeave)
  window.addEventListener('drop', onDrop)
})
onBeforeUnmount(() => {
  tz.registerPicker(null)
  document.removeEventListener('dragstart', onDragStart)
  document.removeEventListener('dragend', onDragEnd)
  window.removeEventListener('dragenter', onDragEnter)
  window.removeEventListener('dragover', onDragOver)
  window.removeEventListener('dragleave', onDragLeave)
  window.removeEventListener('drop', onDrop)
})
</script>

<template>
  <input
    ref="input"
    type="file"
    class="sr-only"
    :accept="TZ_ACCEPT"
    tabindex="-1"
    aria-hidden="true"
    data-testid="tz-file-input"
    @change="onPicked"
  />
  <div v-if="shown" class="tzdrop" data-testid="tz-dropzone" aria-hidden="true">
    <div class="tzdrop__frame">
      <FileText :size="32" class="tzdrop__icon" />
      <p class="tzdrop__title">{{ TZ_TEXT.dropTitle }}</p>
      <p class="tzdrop__text">{{ TZ_TEXT.dropDescription }}</p>
      <p class="tzdrop__text t-sm">{{ TZ_TEXT.formats }}</p>
    </div>
  </div>
</template>

<style scoped>
/* Ниже консультанта (z-index 40 и 45): его кнопка и окно остаются поверх и в зону не входят. */
.tzdrop {
  position: fixed;
  inset: 0;
  z-index: 39;
  padding: var(--space-6);
  background: var(--scrim);
  pointer-events: none;
}
.tzdrop__frame {
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--space-2);
  height: 100%;
  padding: var(--space-4);
  border: 2px dashed var(--border-control);
  border-radius: 16px;
  background: color-mix(in srgb, var(--surface) 88%, transparent);
  color: var(--text);
  text-align: center;
}
.tzdrop__icon {
  color: var(--text-2);
}
.tzdrop__title {
  margin: 0;
  font-size: var(--fs-h1);
  line-height: var(--lh-h1);
  font-weight: var(--fw-semibold);
}
.tzdrop__text {
  margin: 0;
  color: var(--text-2);
}
@media (max-width: 767px) {
  .tzdrop {
    padding: var(--space-3);
  }
}
</style>
