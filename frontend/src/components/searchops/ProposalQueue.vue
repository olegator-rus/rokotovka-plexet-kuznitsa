<script setup lang="ts">
// Вкладка «Очередь»: предложения по убыванию потерянного спроса, фильтр по статусу и
// роли-адресату (в адресе: ?status=&role=). Решение — через диалог, итог — тост; после принятия тост
// ведёт в «Поиск» на действующих правилах B4.
import { computed, nextTick, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { Bot, Play } from 'lucide-vue-next'
import { getProposals } from '@/api/agent'
import type { ApproveResponse, Proposal } from '@/api/types'
import BlockError from '@/components/common/BlockError.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import { useToast } from '@/components/ui/useToast'
import { useResource } from '@/composables/useResource'
import { formatInt } from '@/lib/format'
import {
  PROPOSAL_ROLES,
  PROPOSAL_STATUSES,
  isTicket,
  searchCheckUrl,
  sortByLostDemand,
} from '@/lib/proposals'
import { useAppStore } from '@/stores/app'
import DecisionDialogs from './DecisionDialogs.vue'
import ProposalCard from './ProposalCard.vue'

defineEmits<{ start: [] }>()

const ALL = 'all'
const route = useRoute()
const router = useRouter()
const app = useAppStore()
const { meta } = storeToRefs(app)
const toast = useToast()

const STATUS_OPTIONS = [
  ...PROPOSAL_STATUSES.map((s) => ({ value: s.id, label: s.label })),
  { value: ALL, label: 'Все статусы' },
]
const ROLE_OPTIONS = [
  { value: ALL, label: 'Все роли' },
  ...PROPOSAL_ROLES.map((r) => ({ value: r.id, label: r.label })),
]

function queryParam(key: string, allowed: { value: string }[], fallback: string) {
  return computed<string>({
    get: () => {
      const v = typeof route.query[key] === 'string' ? (route.query[key] as string) : ''
      return allowed.some((o) => o.value === v) ? v : fallback
    },
    set: (v) => void router.replace({ query: { ...route.query, [key]: v } }),
  })
}
const status = queryParam('status', STATUS_OPTIONS, 'pending')
const role = queryParam('role', ROLE_OPTIONS, ALL)

const res = useResource((opts) =>
  getProposals(
    {
      status: status.value === ALL ? undefined : status.value,
      role: role.value === ALL ? undefined : role.value,
    },
    opts,
  ),
)
watch([status, role], () => void res.load(), { immediate: true })

const items = computed(() => sortByLostDemand(res.data.value?.items ?? []))

// ?p=<id> — переход из «Изменений» и «Памяти»: прокрутка к карточке предложения, один раз.
let focusedProposal = ''
watch(
  items,
  async (list) => {
    const id = typeof route.query.p === 'string' ? route.query.p : ''
    if (!id || focusedProposal === id || !list.some((x) => x.id === id)) return
    focusedProposal = id
    await nextTick()
    const el = document.getElementById(`proposal-${id}`)
    el?.scrollIntoView({ block: 'start' })
    el?.focus({ preventScroll: true })
  },
  { immediate: true },
)
const filtered = computed(() => status.value !== 'pending' || role.value !== ALL)

const target = ref<{ proposal: Proposal; mode: 'accept' | 'reject' } | null>(null)
const activeVersion = computed(() => meta.value?.rules_version ?? null)

function afterDecision() {
  target.value = null
  void res.load({ quiet: true })
  void app.loadPending()
}

function onAccepted(p: Proposal, r: ApproveResponse) {
  afterDecision()
  void app.loadMeta(true)
  const url = searchCheckUrl(p)
  if (isTicket(p)) {
    toast.add({ tone: 'ok', title: 'Взято в работу', detail: `Кластер «${p.cluster?.label}»` })
    return
  }
  toast.add({
    tone: 'ok',
    title: `Правка принята · версия правил ${r.config_version}`,
    detail: 'Правило уже действует в поиске.',
    action: url ? { label: 'Проверить в поиске', run: () => void router.push(url) } : undefined,
  })
}

function onRejected(p: Proposal) {
  afterDecision()
  toast.add({
    tone: 'ok',
    title: 'Предложение отклонено',
    detail: `Причина по кластеру «${p.cluster?.label}» записана в память агента.`,
  })
}
</script>

<template>
  <div class="queue">
    <div class="queue__filters">
      <UiSelect v-model="status" :options="STATUS_OPTIONS" label="Статус" prefix="Статус" />
      <UiSelect v-model="role" :options="ROLE_OPTIONS" label="Кому" prefix="Кому" />
      <span v-if="res.status.value === 'ready'" class="t-sm t-muted t-num">
        {{ formatInt(items.length) }} · по убыванию потерянного спроса
      </span>
    </div>

    <div
      v-if="res.status.value === 'loading' && !res.data.value"
      class="queue__list"
      aria-busy="true"
    >
      <div v-for="i in 3" :key="i" class="panel queue__skel">
        <span class="skeleton" style="width: 30%" />
        <span class="skeleton" style="width: 70%" />
        <span class="skeleton" style="width: 55%" />
        <span class="skeleton" style="width: 80%" />
      </div>
    </div>

    <BlockError
      v-else-if="res.status.value === 'error'"
      :error="res.error.value!"
      title="Очередь не загрузилась"
      @retry="res.load()"
    />

    <div v-else-if="res.status.value === 'ready' && !items.length" class="panel queue__empty">
      <Bot :size="20" aria-hidden="true" class="queue__empty-icon" />
      <div>
        <template v-if="filtered">
          <p class="t-h2">По этому фильтру предложений нет</p>
          <p class="t-muted">Выберите другой статус или роль.</p>
        </template>
        <template v-else>
          <p class="t-h2">Предложений нет. Запустите разбор</p>
          <p class="t-muted">
            Агент разберёт неудачные запросы, проверит правки прогоном и положит сюда те, что
            прошли.
          </p>
          <button type="button" class="btn btn--primary" @click="$emit('start')">
            <Play :size="14" aria-hidden="true" /> Запустить разбор
          </button>
        </template>
      </div>
    </div>

    <div v-else class="queue__list">
      <ProposalCard
        v-for="p in items"
        :key="p.id"
        :proposal="p"
        @accept="target = { proposal: p, mode: 'accept' }"
        @reject="target = { proposal: p, mode: 'reject' }"
      />
    </div>

    <DecisionDialogs
      :target="target"
      :active-version="activeVersion"
      @close="target = null"
      @accepted="onAccepted"
      @rejected="onRejected"
    />
  </div>
</template>

<style scoped>
.queue {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.queue__filters {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2) var(--space-3);
}
.queue__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.queue__skel {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
}
.queue__empty {
  display: flex;
  gap: var(--space-3);
  padding: var(--space-5);
}
.queue__empty > div {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--space-2);
}
.queue__empty-icon {
  flex: none;
  color: var(--text-2);
}
</style>
