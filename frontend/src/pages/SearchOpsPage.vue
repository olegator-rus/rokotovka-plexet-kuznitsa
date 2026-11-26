<script setup lang="ts">
// SearchOps («Навигация»; контур автономии — «Память»; контур фида — первый экран «Отчёт» для маркетолога):
// вкладки «Отчёт», «Запросы без ответа», «Запуски», «Очередь», «Память», «Изменения», «Политика ранжирования» (reka
// Tabs, вкладка в адресе: /searchops?tab=report|failed|runs|queue|memory|changes|policy; без вкладки — «Отчёт»). «Запустить разбор» → POST /api/agent/runs по периоду →
// лента запуска. «Разобрать» в строке запроса — запуск на одном запросе (useAnalyseQuery); старый адрес
// ?tab=runs&q=<запрос> показывает плашку с той же кнопкой.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { TabsContent, TabsList, TabsRoot, TabsTrigger } from 'reka-ui'
import { Play } from 'lucide-vue-next'
import { getAgentSettings, getRuns, startRun } from '@/api/agent'
import ChangesPanel from '@/components/searchops/ChangesPanel.vue'
import FailedQueries from '@/components/searchops/FailedQueries.vue'
import MarketerReport from '@/components/searchops/MarketerReport.vue'
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

const TABS = ['report', 'failed', 'runs', 'queue', 'memory', 'changes', 'policy'] as const
type Tab = (typeof TABS)[number]

const route = useRoute()
const router = useRouter()
const toast = useToast()
const { pendingProposals } = storeToRefs(useAppStore())

const tab = computed<Tab>({
  get: () => {
    const t = typeof route.query.tab === 'string' ? route.query.tab : ''
    return (TABS as readonly string[]).includes(t) ? (t as Tab) : 'report'
  },
  set: (t) => void router.replace({ query: { tab: t } }),
})

// 360 px: вкладок семь, видно три — строка прокручивается; край с продолжением гаснет (намёк на прокрутку),
// выбранная вкладка прокручивается в видимую часть (QA r2 №17).
const tabList = ref<{ $el: HTMLElement } | null>(null)
const tabEdge = ref({ left: false, right: false })
function measureTabs() {
  const el = tabList.value?.$el
  if (!el) return
  tabEdge.value = {
    left: el.scrollLeft > 2,
    right: el.scrollLeft + el.clientWidth < el.scrollWidth - 2,
  }
}
function revealActiveTab() {
  const el = tabList.value?.$el
  const active = el?.querySelector<HTMLElement>('[data-state="active"]')
  if (el && active) {
    // Не под гаснущим краем (48 px): вкладка с бейджем видна целиком.
    const FADE = 48
    const box = el.getBoundingClientRect()
    const r = active.getBoundingClientRect()
    const left = r.left - box.left + el.scrollLeft
    const right = left + r.width
    if (right > el.scrollLeft + el.clientWidth - FADE)
      el.scrollLeft = Math.min(el.scrollWidth - el.clientWidth, right - el.clientWidth + FADE)
    else if (left < el.scrollLeft + FADE) el.scrollLeft = Math.max(0, left - FADE)
  }
  measureTabs()
}
let tabsObserver: ResizeObserver | null = null
onMounted(() => {
  const el = tabList.value?.$el
  if (!el) return
  el.addEventListener('scroll', measureTabs, { passive: true })
  if (typeof ResizeObserver !== 'undefined') {
    tabsObserver = new ResizeObserver(measureTabs)
    tabsObserver.observe(el)
  }
  revealActiveTab()
  // Шрифт и бейдж очереди меняют ширину вкладок после монтирования — повторить, когда всё встанет.
  void document.fonts?.ready.then(() => revealActiveTab())
})
onBeforeUnmount(() => {
  tabList.value?.$el?.removeEventListener('scroll', measureTabs)
  tabsObserver?.disconnect()
})
watch(tab, () => void nextTick(revealActiveTab))
watch(pendingProposals, () => void nextTick(revealActiveTab))

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
      <TabsList
        ref="tabList"
        :class="[
          'tabs',
          'sops__tablist',
          { 'is-more-left': tabEdge.left, 'is-more-right': tabEdge.right },
        ]"
        aria-label="Разделы SearchOps"
      >
        <TabsTrigger value="report" class="tabs__trigger">Отчёт</TabsTrigger>
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

      <TabsContent value="report" class="sops__panel">
        <MarketerReport @start="onStart" />
      </TabsContent>
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
.sops__tablist {
  scrollbar-width: none;
}
.sops__tablist::-webkit-scrollbar {
  display: none;
}
.sops__tablist.is-more-right {
  mask-image: linear-gradient(to right, #000 calc(100% - 48px), transparent);
}
.sops__tablist.is-more-left {
  mask-image: linear-gradient(to right, transparent, #000 48px);
}
.sops__tablist.is-more-left.is-more-right {
  mask-image: linear-gradient(
    to right,
    transparent,
    #000 48px,
    #000 calc(100% - 48px),
    transparent
  );
}
@media (max-width: 480px) {
  .sops__tablist > .tabs__trigger {
    padding: 0 var(--space-2);
  }
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
