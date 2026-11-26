<script setup lang="ts">
// Продукт-менеджер: кластеры спроса без предложения в каталоге — из предложений агента с
// диагнозом «нет в ассортименте» / «есть аналог». Частые сверху; раскрытие — запросы и что проверил агент;
// «Взять в работу» — решение по предложению. Рядом — востребованные аналоги.
import { computed, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { CollapsibleContent, CollapsibleRoot, CollapsibleTrigger } from 'reka-ui'
import { ArrowRight, ChevronDown, ChevronRight } from 'lucide-vue-next'
import { getProposals } from '@/api/agent'
import type { Proposal } from '@/api/types'
import BlockError from '@/components/common/BlockError.vue'
import DecisionDialogs from '@/components/searchops/DecisionDialogs.vue'
import { useToast } from '@/components/ui/useToast'
import { useResource } from '@/composables/useResource'
import { formatInt, plural } from '@/lib/format'
import { clusterFreq, clusterQueries, proposalStatus } from '@/lib/proposals'
import { useAppStore } from '@/stores/app'

const DEMAND = new Set(['ASSORTMENT_GAP', 'SUBSTITUTE_AVAILABLE', 'UNKNOWN'])

const app = useAppStore()
const toast = useToast()
const res = useResource((opts) => getProposals({}, opts))
void res.load()

const clusters = computed(() =>
  (res.data.value?.items ?? [])
    .filter((p) => DEMAND.has(p.diagnosis) && (p.role === 'product' || p.diagnosis !== 'UNKNOWN'))
    .sort((a, b) => clusterFreq(b) - clusterFreq(a)),
)
const substitutes = computed(() =>
  clusters.value.filter((p) => p.diagnosis === 'SUBSTITUTE_AVAILABLE').slice(0, 6),
)

const openId = ref<string | null>(null)
const target = ref<{ proposal: Proposal; mode: 'accept' | 'reject' } | null>(null)

function mark(p: Proposal): string {
  if (p.diagnosis === 'SUBSTITUTE_AVAILABLE') return 'есть аналог'
  if (p.diagnosis === 'ASSORTMENT_GAP') return 'аналога нет'
  return 'требует ручной проверки'
}

function onDone(p: Proposal, title: string) {
  target.value = null
  void res.load({ quiet: true })
  void app.loadPending()
  toast.add({ tone: 'ok', title, detail: `Кластер «${p.cluster?.label}»` })
}
</script>

<template>
  <div class="role">
    <section class="role__main" aria-labelledby="demand-title">
      <h2 id="demand-title" class="t-h2">Что ищут и не находят</h2>

      <div v-if="res.status.value === 'loading' && !res.data.value" class="panel role__skel">
        <span v-for="i in 4" :key="i" class="skeleton" :style="{ width: `${88 - i * 9}%` }" />
      </div>
      <BlockError
        v-else-if="res.status.value === 'error'"
        :error="res.error.value!"
        title="Кластеры спроса не загрузились"
        @retry="res.load()"
      />
      <p v-else-if="!clusters.length" class="panel role__empty t-muted">
        Кластеров спроса без предложения пока нет: агент ставит диагноз «нет в каталоге» только
        после расширенного поиска.
        <RouterLink to="/searchops" class="link">Запустить разбор</RouterLink>
      </p>
      <ul v-else class="role__list">
        <CollapsibleRoot
          v-for="p in clusters"
          :key="p.id"
          as="li"
          class="panel role__item"
          :open="openId === p.id"
          @update:open="(v: boolean) => (openId = v ? p.id : null)"
        >
          <CollapsibleTrigger class="role__head">
            <ChevronDown v-if="openId === p.id" :size="14" aria-hidden="true" />
            <ChevronRight v-else :size="14" aria-hidden="true" />
            <span class="role__label">«{{ p.cluster?.label }}»</span>
            <span class="badge">{{ mark(p) }}</span>
            <span class="role__freq t-num t-sm">
              {{ formatInt(clusterFreq(p)) }}
              {{ plural(clusterFreq(p), ['запрос', 'запроса', 'запросов']) }}
            </span>
          </CollapsibleTrigger>
          <CollapsibleContent class="role__body">
            <p v-if="p.diagnosis_text" class="t-sm">{{ p.diagnosis_text }}</p>
            <ul class="role__queries t-sm">
              <li v-for="q in clusterQueries(p)" :key="q.text">
                {{ q.text }} <span class="t-muted t-num">× {{ formatInt(q.freq) }}</span>
              </li>
            </ul>
            <div class="role__actions">
              <RouterLink
                v-if="p.run_id"
                :to="{ path: `/searchops/runs/${p.run_id}`, query: { cluster: p.cluster?.id } }"
                class="link t-sm role__link"
              >
                Что проверил агент <ArrowRight :size="12" aria-hidden="true" />
              </RouterLink>
              <span
                v-if="p.status !== 'pending'"
                :class="['badge', 'badge--tone', proposalStatus(p.status).tone]"
              >
                {{ proposalStatus(p.status).label }}
              </span>
              <button
                v-else
                type="button"
                class="btn btn--primary btn--sm"
                @click="target = { proposal: p, mode: 'accept' }"
              >
                Взять в работу
              </button>
            </div>
          </CollapsibleContent>
        </CollapsibleRoot>
      </ul>
    </section>

    <aside class="panel role__side" aria-labelledby="subst-title">
      <h2 id="subst-title" class="t-h2">Востребованные аналоги</h2>
      <p class="t-xs t-muted">Чего нет в каталоге, но покупатели просят замену</p>
      <ol v-if="substitutes.length" class="role__subst">
        <li v-for="p in substitutes" :key="p.id">
          <span>«{{ p.cluster?.label }}»</span>
          <span class="t-num t-muted">{{ formatInt(clusterFreq(p)) }}</span>
        </li>
      </ol>
      <p v-else class="t-sm t-muted">Агент пока не нашёл таких кластеров.</p>
    </aside>

    <DecisionDialogs
      :target="target"
      :active-version="app.meta?.rules_version ?? null"
      @close="target = null"
      @accepted="(p) => onDone(p, 'Взято в работу')"
      @rejected="(p) => onDone(p, 'Предложение отклонено')"
    />
  </div>
</template>

<style scoped>
.role {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 18rem;
  align-items: start;
  gap: var(--space-4);
}
.role__main {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  min-width: 0;
}
.role__skel,
.role__empty {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
}
.role__empty {
  display: block;
}
.role__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.role__head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
  width: 100%;
  padding: var(--space-2) var(--space-3);
  border: 0;
  background: none;
  color: var(--text);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.role__head:hover {
  background: var(--surface-2);
}
.role__label {
  font-weight: var(--fw-medium);
}
.role__freq {
  margin-left: auto;
  color: var(--text-2);
}
.role__body {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: 0 var(--space-3) var(--space-3) calc(var(--space-3) + 14px + var(--space-2));
}
.role__queries {
  margin: 0;
  padding-left: var(--space-4);
}
.role__link {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
}
.role__actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
}
.role__side {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-4);
}
.role__subst {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin: 0;
  padding: 0;
  list-style: none;
  font-size: var(--fs-sm);
}
.role__subst li {
  display: flex;
  justify-content: space-between;
  gap: var(--space-2);
}
@media (max-width: 1023px) {
  .role {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
