<script setup lang="ts">
// Трасса поиска из журнала «Экономики»: GET /api/traces/{request_id} в окне с той же
// панелью трассы, что и в «Поиске». Нет трассы — 404 с понятным текстом; сбой — повтор.
import { computed, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { getTrace } from '@/api/search'
import type { TraceRecord } from '@/api/types'
import BlockError from '@/components/common/BlockError.vue'
import TracePanel from '@/components/trace/TracePanel.vue'
import UiDialog from '@/components/ui/UiDialog.vue'
import { type ApiError, isApiError, MSG_UNKNOWN } from '@/lib/apiError'
import { formatDateTime } from '@/lib/format'

const open = defineModel<boolean>('open', { default: false })
const props = defineProps<{ requestId: string | null; query?: string | null }>()

const record = ref<TraceRecord | null>(null)
const loading = ref(false)
const error = ref<ApiError | null>(null)
const selected = ref<string | null>(null)
let ctrl: AbortController | null = null

async function load() {
  const id = props.requestId
  if (!id) return
  ctrl?.abort()
  const c = new AbortController()
  ctrl = c
  loading.value = true
  error.value = null
  record.value = null
  selected.value = null
  try {
    const r = await getTrace(id, { signal: c.signal })
    if (ctrl !== c) return
    record.value = r
  } catch (e) {
    if (ctrl !== c || (e instanceof DOMException && e.name === 'AbortError')) return
    error.value = isApiError(e) ? e : { status: 0, code: 'UNKNOWN', message: MSG_UNKNOWN }
  } finally {
    if (ctrl === c) loading.value = false
  }
}

watch(
  () => [open.value, props.requestId] as const,
  ([o]) => {
    if (o) void load()
    else ctrl?.abort()
  },
)

const title = computed(() => {
  const q = record.value?.query || props.query
  return q ? `Трасса «${q}»` : 'Трасса поиска'
})
const sourceLabel = computed(() => {
  const s = record.value?.source
  if (s === 'compare') return 'вкладка «Один запрос»'
  if (s === 'mock_log') return 'исторический лог'
  return 'поиск покупателя'
})
const searchLink = computed(() => {
  const r = record.value
  if (!r?.query) return null
  return { path: '/search', query: { q: r.query, config: String(r.config), trace: '1' } }
})
</script>

<template>
  <UiDialog v-model:open="open" :title="title" wide>
    <p v-if="record" class="t-sm t-muted tdlg__meta">
      {{ formatDateTime(record.created_at) }} · {{ sourceLabel }} · конфигурация
      {{ record.config }}
      <template v-if="searchLink">
        ·
        <RouterLink :to="searchLink" class="link" @click="open = false"
          >Повторить в поиске</RouterLink
        >
      </template>
    </p>
    <BlockError
      v-if="error"
      :error="error"
      :title="error.status === 404 ? 'Трасса не сохранилась' : 'Трасса не загрузилась'"
      @retry="load()"
    />
    <TracePanel
      v-else
      :trace="record?.trace ?? null"
      :loading="loading"
      :titles="{}"
      embedded
      :selected="selected"
      @select="selected = $event"
    />
  </UiDialog>
</template>

<style scoped>
.tdlg__meta {
  overflow-wrap: anywhere;
}
</style>
