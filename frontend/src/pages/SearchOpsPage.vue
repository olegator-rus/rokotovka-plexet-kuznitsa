<script setup lang="ts">
// SearchOps («Навигация»; контур автономии — «Память»): вкладки «Запросы без ответа», «Запуски»,
// «Очередь», «Память», «Изменения», «Политика ранжирования» (reka Tabs, вкладка в адресе:
// /searchops?tab=failed|runs|queue|memory|changes|policy). «Запустить разбор» → POST /api/agent/runs по периоду →
// лента запуска. «Разобрать» в строке запроса — запуск на одном запросе (useAnalyseQuery); старый адрес
// ?tab=runs&q=<запрос> показывает плашку с той же кнопкой.
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { TabsContent, TabsList, TabsRoot, TabsTrigger } from 'reka-ui'
import { Play } from 'lucide-vue-next'
import { getAgentSettings, getRuns, startRun } from '@/api/agent'
import ChangesPanel from '@/components/searchops/ChangesPanel.vue'
import FailedQueries from '@/components/searchops/FailedQueries.vue'
import MemoryPanel from '@/components/searchops/MemoryPanel.vue'
import PolicyPanel from '@/components/searchops/PolicyPanel.vue'
import ProposalQueue from '@/components/searchops/ProposalQueue.vue'
import RunsList from '@/components/searchops/RunsList.vue'
import { useToast } from '@/components/ui/useToast'
import { useAnalyseQuery } from '@/composables/useAnalyseQuery'
import { useResource } from '@/composables/useResource'
import { isApiError } from '@/lib/apiError'
import { isRunActive } from '@/lib/agentFeed'
import { useAppStore } from '@/stores/app'

const TABS = ['failed', 'runs', 'queue', 'memory', 'changes', 'policy'] as const
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

/** Старый адрес «Разобрать» (?tab=runs&q=): плашка с запуском на одном запросе. */
const askedQuery = computed(() =>
  tab.value === 'runs' && typeof route.query.q === 'string' ? route.query.q.trim() : '',
)

const { analyse, busy: analysing } = useAnalyseQuery()

const runs = useResource((opts) => getRuns(opts))
const settings = useResource((opts) => getAgentSettings(opts))
watch(
  tab,
  (t) => {
    if (t === 'runs' || t === 'failed') void runs.load({ quiet: !!runs.data.value })
    if (t === 'runs') void settings.load({ quiet: !!settings.data.value })
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
        <TabsTrigger value="memory" class="tabs__trigger">Память</TabsTrigger>
        <TabsTrigger value="changes" class="tabs__trigger">Изменения</TabsTrigger>
        <TabsTrigger value="policy" class="tabs__trigger">Политика ранжирования</TabsTrigger>
      </TabsList>

      <TabsContent value="failed" class="sops__panel">
        <FailedQueries />
      </TabsContent>
      <TabsContent value="runs" class="sops__panel">
        <div v-if="askedQuery" class="panel sops__asked t-sm" role="status">
          <span
            >Разобрать запрос «<strong>{{ askedQuery }}</strong
            >» — агент построит план и проверит гипотезы при вас.</span
          >
          <button
            type="button"
            class="btn btn--primary btn--sm"
            :disabled="!!analysing"
            @click="analyse(askedQuery)"
          >
            {{ analysing ? 'Запускаем…' : 'Разобрать' }}
          </button>
        </div>
        <RunsList
          :runs="runs.data.value?.items ?? null"
          :status="runs.status.value"
          :error="runs.error.value"
          :starting="starting"
          :settings="settings.data.value ?? null"
          @start="onStart"
          @retry="(runs.load(), settings.load())"
        />
      </TabsContent>
      <TabsContent value="queue" class="sops__panel">
        <ProposalQueue @start="onStart" />
      </TabsContent>
      <TabsContent value="memory" class="sops__panel">
        <MemoryPanel />
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
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2) var(--space-3);
  padding: var(--space-3) var(--space-4);
  color: var(--text-2);
}
.sops__panel:focus-visible {
  outline-offset: 4px;
}
</style>
