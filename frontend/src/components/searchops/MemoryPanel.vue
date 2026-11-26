<script setup lang="ts">
// Вкладка «Память» («Экран «Память»»): три вкладки — Решения / Не предлагать / Эпизоды
// (GET /api/agent/memory?layer=…). Записи только читаются; из каждой — ссылка на запуск. Запись
// «не предлагать» человек может снять с причиной — это тоже решение (POST …/suppress/{id}/remove). Ошибка одной
// вкладки не мешает остальным. Подвкладка в адресе: ?mem=decisions|suppress|episodes.
import { computed, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { TabsContent, TabsList, TabsRoot, TabsTrigger } from 'reka-ui'
import { BookOpen, CircleAlert, Lock } from 'lucide-vue-next'
import { getMemoryLayer, removeSuppress } from '@/api/agent'
import type { MemoryLayerId, MemorySuppress } from '@/api/typesAgent'
import BlockError from '@/components/common/BlockError.vue'
import UiDialog from '@/components/ui/UiDialog.vue'
import { useToast } from '@/components/ui/useToast'
import { useResource } from '@/composables/useResource'
import { triggerLabel } from '@/lib/agentRuns'
import { denyMessage } from '@/lib/auth'
import { isApiError, MSG_UNKNOWN } from '@/lib/apiError'
import { causeOfCode } from '@/lib/causes'
import { formatDateTime, formatInt } from '@/lib/format'
import {
  DECISION_LABELS,
  EFFECT_LABELS,
  SUPPRESS_STATUS,
  conditionNow,
  episodeOutcome,
} from '@/lib/agentMemory'
import { useAuthStore } from '@/stores/auth'

const LAYERS: { id: MemoryLayerId; label: string; empty: string }[] = [
  { id: 'decisions', label: 'Решения', empty: 'Решений пока нет' },
  { id: 'suppress', label: 'Не предлагать', empty: 'Отклонённого человеком пока нет' },
  { id: 'episodes', label: 'Эпизоды', empty: 'Эпизодов пока нет — их пишут запуски агента' },
]

const route = useRoute()
const router = useRouter()
const toast = useToast()
const auth = useAuthStore()

const layer = computed<MemoryLayerId>({
  get: () => {
    const v = typeof route.query.mem === 'string' ? route.query.mem : ''
    return LAYERS.some((l) => l.id === v) ? (v as MemoryLayerId) : 'decisions'
  },
  set: (v) => void router.replace({ query: { ...route.query, mem: v } }),
})

const res = {
  decisions: useResource((o) => getMemoryLayer('decisions', o)),
  suppress: useResource((o) => getMemoryLayer('suppress', o)),
  episodes: useResource((o) => getMemoryLayer('episodes', o)),
}
watch(layer, (l) => void res[l].load({ quiet: !!res[l].data.value }), { immediate: true })

const decisions = computed(() => res.decisions.data.value?.decisions ?? [])
const suppress = computed(() => res.suppress.data.value?.suppress ?? [])
const episodes = computed(() => res.episodes.data.value?.episodes ?? [])
const counts = computed<Record<MemoryLayerId, number | null>>(() => ({
  decisions: res.decisions.data.value ? decisions.value.length : null,
  suppress: res.suppress.data.value
    ? suppress.value.filter((s) => s.status === 'active').length
    : null,
  episodes: res.episodes.data.value ? episodes.value.length : null,
}))

/** Причина человека: подпись «принято» повторяет решение — тогда только комментарий. */
function humanReason(d: {
  decision: string
  reason: string
  reason_label: string
  comment: string
}) {
  const r = d.reason_label || d.reason
  if (!r || r === DECISION_LABELS[d.decision]?.label || r === d.decision)
    return d.comment ? '' : '—'
  return r
}

const runHref = (id: string | null | undefined) =>
  id ? `/searchops/runs/${encodeURIComponent(id)}` : null
const proposalHref = (id: string | null | undefined) =>
  id ? { path: '/searchops', query: { tab: 'queue', status: 'all', p: id } } : null

// ── Снятие «не предлагать» ──
const ownerRole = (s: MemorySuppress) => causeOfCode(s.diagnosis)?.role ?? 'head'
const denyFor = (s: MemorySuppress) => (auth.can(ownerRole(s)) ? '' : denyMessage(ownerRole(s)))

const target = ref<MemorySuppress | null>(null)
const reason = ref('')
const busy = ref(false)
const err = ref('')
const dialogOpen = computed({
  get: () => !!target.value,
  set: (v) => {
    if (!v && !busy.value) target.value = null
  },
})
function openRemove(s: MemorySuppress) {
  target.value = s
  reason.value = ''
  err.value = ''
}
async function confirmRemove() {
  const s = target.value
  if (!s || !reason.value.trim()) return
  busy.value = true
  err.value = ''
  try {
    await removeSuppress(s.id, reason.value.trim())
    busy.value = false
    target.value = null
    toast.add({
      tone: 'ok',
      title: 'Запись снята',
      detail: `Агент снова может предложить «${s.cluster_label}». Снятие записано в «Решения».`,
    })
    void res.suppress.load({ quiet: true })
    void res.decisions.load({ quiet: true })
  } catch (e) {
    busy.value = false
    err.value = isApiError(e) ? e.message : MSG_UNKNOWN
  }
}
</script>

<template>
  <TabsRoot v-model="layer" class="mem">
    <TabsList class="tabs" aria-label="Слои памяти агента">
      <TabsTrigger v-for="l in LAYERS" :key="l.id" :value="l.id" class="tabs__trigger">
        {{ l.label }}
        <span v-if="counts[l.id]" class="t-xs t-muted t-num">{{ formatInt(counts[l.id]!) }}</span>
      </TabsTrigger>
    </TabsList>

    <TabsContent v-for="l in LAYERS" :key="l.id" :value="l.id" class="mem__panel">
      <div
        v-if="res[l.id].status.value === 'loading' && !res[l.id].data.value"
        class="panel mem__skel"
        aria-busy="true"
      >
        <span v-for="i in 4" :key="i" class="skeleton" :style="{ width: `${92 - i * 8}%` }" />
      </div>

      <BlockError
        v-else-if="res[l.id].status.value === 'error'"
        :error="res[l.id].error.value!"
        :title="`«${l.label}» не загрузились`"
        @retry="res[l.id].load()"
      />

      <template v-else-if="res[l.id].data.value">
        <!-- Решения -->
        <template v-if="l.id === 'decisions'">
          <div v-if="!decisions.length" class="panel mem__empty">
            <BookOpen :size="20" aria-hidden="true" class="mem__empty-icon" />
            <div>
              <p class="t-h2">{{ l.empty }}</p>
              <p class="t-muted">
                Здесь появятся принятые и отклонённые предложения с причиной человека.
              </p>
              <RouterLink to="/searchops?tab=queue" class="btn btn--secondary">
                Открыть очередь
              </RouterLink>
            </div>
          </div>
          <div v-else class="data-scroll" role="region" aria-label="Решения" tabindex="0">
            <table class="data-table mem__table">
              <thead>
                <tr>
                  <th scope="col">Предложение</th>
                  <th scope="col">Причина агента</th>
                  <th scope="col">Решение человека</th>
                  <th scope="col">Причина человека</th>
                  <th scope="col">Дата</th>
                  <th scope="col">Кто</th>
                  <th scope="col">Запуск</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="d in decisions" :key="d.id">
                  <th scope="row">
                    <RouterLink
                      v-if="proposalHref(d.proposal_id)"
                      :to="proposalHref(d.proposal_id)!"
                      class="link"
                      >{{ d.cluster_label }}</RouterLink
                    >
                    <template v-else>{{ d.cluster_label }}</template>
                    <span class="t-xs t-muted mem__sub">{{ d.cause_label || d.proposal }}</span>
                  </th>
                  <td class="mem__long">
                    <span :title="d.agent_reason">{{ d.agent_reason || '—' }}</span>
                  </td>
                  <td>
                    <span
                      :class="['badge', 'badge--tone', DECISION_LABELS[d.decision]?.tone ?? '']"
                      >{{ DECISION_LABELS[d.decision]?.label ?? d.decision }}</span
                    >
                  </td>
                  <td>
                    {{ humanReason(d) }}
                    <span v-if="d.comment" class="t-xs t-muted mem__sub">«{{ d.comment }}»</span>
                  </td>
                  <td class="t-num mem__nowrap">{{ formatDateTime(d.ts) }}</td>
                  <td>{{ d.decided_by_label || '—' }}</td>
                  <td class="mem__nowrap">
                    <RouterLink v-if="runHref(d.run_id)" :to="runHref(d.run_id)!" class="link">
                      {{ triggerLabel(d.trigger) }}
                    </RouterLink>
                    <span v-else class="t-muted">—</span>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </template>

        <!-- Не предлагать -->
        <template v-else-if="l.id === 'suppress'">
          <div v-if="!suppress.length" class="panel mem__empty">
            <BookOpen :size="20" aria-hidden="true" class="mem__empty-icon" />
            <div>
              <p class="t-h2">{{ l.empty }}</p>
              <p class="t-muted">
                Отклонённое человеком агент не предлагает снова, пока не наступит условие возврата:
                выросла частота неудач или вышел новый каталог.
              </p>
            </div>
          </div>
          <div v-else class="data-scroll" role="region" aria-label="Не предлагать" tabindex="0">
            <table class="data-table mem__table">
              <thead>
                <tr>
                  <th scope="col">Кластер или правка</th>
                  <th scope="col">Почему</th>
                  <th scope="col">Условие возврата</th>
                  <th scope="col">Сейчас</th>
                  <th scope="col">Запуск</th>
                  <th scope="col"><span class="sr-only">Действие</span></th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="s in suppress"
                  :key="s.id"
                  :class="{ 'mem__row--off': s.status !== 'active' }"
                >
                  <th scope="row">
                    {{ s.cluster_label }}
                    <span class="t-xs t-muted mem__sub">
                      {{ s.scope === 'patch' ? 'правка' : 'кластер' }} ·
                      {{ SUPPRESS_STATUS[s.status] ?? s.status }}
                    </span>
                  </th>
                  <td class="mem__long">
                    {{ s.why || s.reason }}
                    <span v-if="s.comment" class="t-xs t-muted mem__sub">«{{ s.comment }}»</span>
                    <span class="t-xs t-muted mem__sub">{{ formatDateTime(s.decided_at) }}</span>
                  </td>
                  <td class="mem__long">{{ s.return_condition || '—' }}</td>
                  <td class="mem__long" data-testid="condition-now">
                    {{ conditionNow(s) }}
                  </td>
                  <td class="mem__nowrap">
                    <RouterLink v-if="runHref(s.run_id)" :to="runHref(s.run_id)!" class="link"
                      >откуда</RouterLink
                    >
                    <template v-if="runHref(s.checked_run_id)">
                      ·
                      <RouterLink :to="runHref(s.checked_run_id)!" class="link"
                        >проверено</RouterLink
                      >
                    </template>
                    <template v-if="runHref(s.lifted_run_id)">
                      ·
                      <RouterLink :to="runHref(s.lifted_run_id)!" class="link">вернул</RouterLink>
                    </template>
                  </td>
                  <td>
                    <button
                      v-if="s.status === 'active'"
                      type="button"
                      class="btn btn--secondary btn--sm"
                      :disabled="!!denyFor(s)"
                      :title="denyFor(s) || undefined"
                      @click="openRemove(s)"
                    >
                      <Lock v-if="denyFor(s)" :size="12" aria-hidden="true" />
                      Снять
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </template>

        <!-- Эпизоды -->
        <template v-else>
          <div v-if="!episodes.length" class="panel mem__empty">
            <BookOpen :size="20" aria-hidden="true" class="mem__empty-icon" />
            <div>
              <p class="t-h2">{{ l.empty }}</p>
              <RouterLink to="/searchops?tab=runs" class="btn btn--secondary">
                Открыть запуски
              </RouterLink>
            </div>
          </div>
          <div v-else class="data-scroll" role="region" aria-label="Эпизоды" tabindex="0">
            <table class="data-table mem__table">
              <thead>
                <tr>
                  <th scope="col">Что разобрано</th>
                  <th scope="col">Итог</th>
                  <th scope="col">Эффект после применения</th>
                  <th scope="col">Проверка</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="e in episodes" :key="e.id">
                  <th scope="row">
                    {{ e.cluster_label }}
                    <span class="t-xs t-muted mem__sub">
                      {{ e.kind === 'effect_check' ? 'проверка эффекта' : 'разбор' }} ·
                      {{ formatDateTime(e.ts) }}
                    </span>
                  </th>
                  <td class="mem__long">
                    {{ episodeOutcome(e) }}
                    <span
                      v-if="e.note && e.kind !== 'effect_check'"
                      class="t-xs t-muted mem__sub"
                      >{{ e.note }}</span
                    >
                  </td>
                  <td>
                    <span
                      v-if="e.effect && EFFECT_LABELS[e.effect]"
                      :class="['badge', 'badge--tone', EFFECT_LABELS[e.effect]!.tone]"
                      >{{ EFFECT_LABELS[e.effect]!.label }}</span
                    >
                    <span v-else class="t-muted">—</span>
                    <span
                      v-if="e.kind === 'effect_check' && e.note"
                      class="t-xs t-muted mem__sub"
                      >{{ e.note }}</span
                    >
                  </td>
                  <td class="mem__nowrap">
                    <RouterLink v-if="runHref(e.run_id)" :to="runHref(e.run_id)!" class="link">
                      {{ triggerLabel(e.trigger) }}
                    </RouterLink>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </template>
      </template>
    </TabsContent>

    <UiDialog v-model:open="dialogOpen" title="Снять запись «не предлагать»?">
      <form id="suppress-form" class="mem__form" @submit.prevent="confirmRemove">
        <p v-if="target">
          «{{ target.cluster_label }}». Агент снова сможет предложить это на следующем запуске.
          Снятие — тоже решение, оно попадёт в «Решения».
        </p>
        <label class="mem__label">
          <span class="t-sm">Причина (обязательно)</span>
          <textarea v-model="reason" class="mem__textarea" rows="3" maxlength="500" required />
        </label>
        <p v-if="err" class="mem__err t-sm" role="alert">
          <CircleAlert :size="14" aria-hidden="true" /> {{ err }}
        </p>
      </form>
      <template #footer>
        <button type="button" class="btn btn--secondary" :disabled="busy" @click="target = null">
          Отмена
        </button>
        <button
          type="submit"
          form="suppress-form"
          class="btn btn--primary"
          :disabled="busy || !reason.trim()"
        >
          {{ busy ? 'Снимаем…' : 'Снять запись' }}
        </button>
      </template>
    </UiDialog>
  </TabsRoot>
</template>

<style scoped>
.mem {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.mem__skel {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
}
.mem__empty {
  display: flex;
  gap: var(--space-3);
  padding: var(--space-5);
}
.mem__empty > div {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--space-2);
  max-width: 40rem;
}
.mem__empty-icon {
  flex: none;
  color: var(--feed-memory);
}
.mem__sub {
  display: block;
  font-weight: var(--fw-regular);
}
.mem__long {
  min-width: 12rem;
  max-width: 26rem;
}
.mem__long > span:first-child {
  display: -webkit-box;
  overflow: hidden;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 3;
  line-clamp: 3;
}
.mem__nowrap {
  white-space: nowrap;
}
.mem__row--off {
  color: var(--text-2);
}
.mem__form {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.mem__label {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}
.mem__textarea {
  box-sizing: border-box;
  width: 100%;
  padding: var(--space-2);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-control);
  background: var(--bg);
  color: var(--text);
  font: inherit;
  resize: vertical;
}
.mem__err {
  display: flex;
  gap: var(--space-2);
  color: var(--danger);
}
</style>
