<script setup lang="ts">
// SearchOps («Навигация»): вкладки «Запросы без ответа», «Запуски», «Очередь», «Изменения»,
// «Политика ранжирования» (reka Tabs, вкладка в адресе: /searchops?tab=failed|runs|queue|changes|policy).
// «Запустить разбор» → POST /api/agent/runs → страница запуска с лентой. «Разобрать» в «Запросах без ответа» ведёт
// сюда с ?tab=runs&q=<запрос>: запуск на одном запросе подключит трек агента.
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { TabsContent, TabsList, TabsRoot, TabsTrigger } from 'reka-ui'
import { Play } from 'lucide-vue-next'
import { getRuns, startRun } from '@/api/agent'
import ChangesPanel from '@/components/searchops/ChangesPanel.vue'
import FailedQueries from '@/components/searchops/FailedQueries.vue'
import PolicyPanel from '@/components/searchops/PolicyPanel.vue'
import ProposalQueue from '@/components/searchops/ProposalQueue.vue'
import RunsList from '@/components/searchops/RunsList.vue'
import { useToast } from '@/components/ui/useToast'
import { useResource } from '@/composables/useResource'
import { isApiError } from '@/lib/apiError'
import { isRunActive } from '@/lib/agentFeed'
import { useAppStore } from '@/stores/app'

const TABS = ['failed', 'runs', 'queue', 'changes', 'policy'] as const
type Tab = (typeof TABS)[number]

const route = useRoute()
const router = useRouter()
const toast = useToast()
const { pendingProposals } = storeToRefs(useAppStore())

const tab = computed<Tab>({
  get: () => {
    const t = typeof route.query.tab === 'string' ? route.query.tab : ''
    return (TABS as readonly string[]).includes(t) ? (t as Tab) : 'failed'
  },
  set: (t) => void router.replace({ query: { tab: t } }),
})

/** Запрос из «Разобрать» (?q=): запуск на одном запросе подключит трек агента. */
const askedQuery = computed(() =>
  tab.value === 'runs' && typeof route.query.q === 'string' ? route.query.q.trim() : '',
)

const runs = useResource((opts) => getRuns(opts))
watch(
  tab,
  (t) => {
    if (t === 'runs' || t === 'failed') void runs.load({ quiet: !!runs.data.value })
  },
  { immediate: true },
)

const activeRun = computed(() => (runs.data.value?.items ?? []).find((r) => isRunActive(r.status)))

const starting = ref(false)
async function onStart() {
  if (activeRun.value) {
    void router.push(`/searchops/runs/${encodeURIComponent(activeRun.value.id)}`)
    return
  }
  starting.value = true
  try {
    const { run_id } = await startRun({ days: 30, max_clusters: 5, trigger: 'manual' })
    void router.push(`/searchops/runs/${encodeURIComponent(run_id)}`)
  } catch (e) {
    toast.add({
      tone: 'error',
      title: 'Разбор не запустился',
      detail: isApiError(e)
        ? e.status === 404
          ? 'Сервер пока не отдаёт запуск агента — эндпоинт ещё не подключён.'
          : e.message
        : undefined,
    })
  } finally {
    starting.value = false
  }
}
</script>

<template>
  <section class="page sops" aria-labelledby="sops-title">
    <header class="sops__head">
      <div>
        <h1 id="sops-title" class="t-h1">SearchOps</h1>
        <p class="t-sm t-muted">
          Агент разбирает неудачные поиски, проверяет правку прогоном и приносит человеку. Сам он
          ничего не применяет.
        </p>
      </div>
      <button type="button" class="btn btn--primary" :disabled="starting" @click="onStart">
        <Play :size="14" aria-hidden="true" />
        {{ activeRun ? 'Открыть идущий запуск' : starting ? 'Запускаем…' : 'Запустить разбор' }}
      </button>
    </header>

    <TabsRoot v-model="tab" class="sops__tabs">
      <TabsList class="tabs" aria-label="Разделы SearchOps">
        <TabsTrigger value="failed" class="tabs__trigger">Запросы без ответа</TabsTrigger>
        <TabsTrigger value="runs" class="tabs__trigger">Запуски</TabsTrigger>
        <TabsTrigger value="queue" class="tabs__trigger">
          Очередь
          <span v-if="pendingProposals" class="badge badge--tone status-pending t-num">
            {{ pendingProposals }}
          </span>
        </TabsTrigger>
        <TabsTrigger value="changes" class="tabs__trigger">Изменения</TabsTrigger>
        <TabsTrigger value="policy" class="tabs__trigger">Политика ранжирования</TabsTrigger>
      </TabsList>

      <TabsContent value="failed" class="sops__panel">
        <FailedQueries />
      </TabsContent>
      <TabsContent value="runs" class="sops__panel">
        <p v-if="askedQuery" class="panel sops__asked t-sm" role="status">
          Разбор запроса «<strong>{{ askedQuery }}</strong
          >»: запуск на одном запросе ещё не подключён. «Запустить разбор» разберёт неудачные
          запросы за 30 дней (до 5 самых частых кластеров).
        </p>
        <RunsList
          :runs="runs.data.value?.items ?? null"
          :status="runs.status.value"
          :error="runs.error.value"
          :starting="starting"
          @start="onStart"
          @retry="runs.load()"
        />
      </TabsContent>
      <TabsContent value="queue" class="sops__panel">
        <ProposalQueue @start="onStart" />
      </TabsContent>
      <TabsContent value="changes" class="sops__panel">
        <ChangesPanel />
      </TabsContent>
      <TabsContent value="policy" class="sops__panel">
        <PolicyPanel />
      </TabsContent>
    </TabsRoot>
  </section>
</template>

<style scoped>
.sops {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
.sops__head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-3);
}
.sops__head > div {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  max-width: 44rem;
}
.sops__tabs {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
.sops__asked {
  padding: var(--space-3) var(--space-4);
  color: var(--text-2);
}
.sops__panel:focus-visible {
  outline-offset: 4px;
}
</style>
