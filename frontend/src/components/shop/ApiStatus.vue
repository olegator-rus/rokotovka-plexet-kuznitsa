<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { getHealth, type Health } from '@/api/health'
import { isApiError } from '@/lib/apiError'

// Связь витрины с API: GET /api/health. Показывает, что прокси, сервер и база на месте.
type State =
  { kind: 'loading' } | { kind: 'ok'; health: Health } | { kind: 'error'; message: string }

const state = ref<State>({ kind: 'loading' })
let ctrl: AbortController | null = null

async function check(): Promise<void> {
  ctrl?.abort()
  ctrl = new AbortController()
  state.value = { kind: 'loading' }
  try {
    state.value = { kind: 'ok', health: await getHealth({ signal: ctrl.signal }) }
  } catch (e) {
    if (e instanceof DOMException && e.name === 'AbortError') return
    state.value = { kind: 'error', message: isApiError(e) ? e.message : 'Сервер не ответил' }
  }
}

onMounted(check)
onBeforeUnmount(() => ctrl?.abort())
</script>

<template>
  <p class="status t-caption m-0" role="status" aria-live="polite">
    <template v-if="state.kind === 'loading'">
      <span class="dot dot--wait" aria-hidden="true" />Проверяем сервер…
    </template>
    <template v-else-if="state.kind === 'ok'">
      <span class="dot dot--ok" aria-hidden="true" />Сервер на связи ·
      {{ state.health.db === 'ok' ? 'база подключена' : 'база не подключена' }}
    </template>
    <template v-else>
      <span class="dot dot--down" aria-hidden="true" />Сервер недоступен: {{ state.message }}
      <button type="button" class="retry" @click="check">Повторить</button>
    </template>
  </p>
</template>

<style scoped>
.status {
  display: inline-flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.25rem 0.5rem;
  color: var(--text-2);
}
.dot {
  width: 0.5rem;
  height: 0.5rem;
  border-radius: var(--radius-pill);
}
.dot--wait {
  background: var(--border-control);
}
.dot--ok {
  background: var(--ok);
}
.dot--down {
  background: var(--danger);
}
.retry {
  padding: 0;
  border: 0;
  background: none;
  color: var(--accent);
  font: inherit;
  font-weight: 600;
  text-decoration: underline;
  text-underline-offset: 2px;
  cursor: pointer;
}
</style>
