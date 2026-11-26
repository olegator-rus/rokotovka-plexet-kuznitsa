<script setup lang="ts">
// Контент-менеджер: три списка на вкладках — карточки без нужных слов (CONTENT_GAP,
// «было / предлагается»), неизвестные термины (ALIAS_GAP, TYPO) и предложенные алиасы из всех предложений.
// Агент предлагает только слова из запросов и значения справочников; решение — в очереди SearchOps.
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import { TabsContent, TabsList, TabsRoot, TabsTrigger } from 'reka-ui'
import { getProposals } from '@/api/agent'
import type { Proposal, RulePatchItem } from '@/api/types'
import BlockError from '@/components/common/BlockError.vue'
import PatchDiff from '@/components/searchops/PatchDiff.vue'
import { useResource } from '@/composables/useResource'
import { formatInt } from '@/lib/format'
import { clusterFreq, clusterQueries, patchItemText, proposalStatus } from '@/lib/proposals'

const res = useResource((opts) => getProposals({}, opts))
void res.load()

const all = computed(() => res.data.value?.items ?? [])
const cards = computed(() => all.value.filter((p) => p.diagnosis === 'CONTENT_GAP'))
const terms = computed(() =>
  all.value
    .filter((p) => p.diagnosis === 'ALIAS_GAP' || p.diagnosis === 'TYPO')
    .sort((a, b) => clusterFreq(b) - clusterFreq(a)),
)
const aliases = computed(() =>
  all.value.flatMap((p) =>
    (p.patch?.add ?? [])
      .filter((i) => i.kind === 'alias' || i.kind === 'typo')
      .map((i, n) => ({ key: `${p.id}-${n}`, p, ...patchItemText(i) })),
  ),
)

function beforeAfter(i: RulePatchItem): { before: string; after: string } | null {
  if (i.before === undefined && i.after === undefined) return null
  return { before: String(i.before ?? '—'), after: String(i.after ?? '—') }
}
const status = (p: Proposal) => proposalStatus(p.status)
const queueLink = { path: '/searchops', query: { tab: 'queue', role: 'content' } }
</script>

<template>
  <section class="content" aria-labelledby="content-title">
    <h2 id="content-title" class="t-h2">Какие карточки и словари править</h2>

    <div v-if="res.status.value === 'loading' && !res.data.value" class="panel content__skel">
      <span v-for="i in 4" :key="i" class="skeleton" :style="{ width: `${88 - i * 9}%` }" />
    </div>
    <BlockError
      v-else-if="res.status.value === 'error'"
      :error="res.error.value!"
      title="Предложения агента не загрузились"
      @retry="res.load()"
    />

    <TabsRoot v-else default-value="cards" class="content__tabs">
      <TabsList class="tabs" aria-label="Списки контент-менеджера">
        <TabsTrigger value="cards" class="tabs__trigger">
          Карточки без характеристик <span class="badge t-num">{{ cards.length }}</span>
        </TabsTrigger>
        <TabsTrigger value="terms" class="tabs__trigger">
          Неизвестные термины <span class="badge t-num">{{ terms.length }}</span>
        </TabsTrigger>
        <TabsTrigger value="aliases" class="tabs__trigger">
          Предложенные алиасы <span class="badge t-num">{{ aliases.length }}</span>
        </TabsTrigger>
      </TabsList>

      <TabsContent value="cards">
        <p v-if="!cards.length" class="panel content__empty t-muted">
          Карточек, которые не находятся по своим запросам, агент пока не нашёл.
        </p>
        <ul v-else class="content__list">
          <li v-for="p in cards" :key="p.id" class="panel content__item">
            <div class="content__row">
              <span class="content__label">«{{ p.cluster?.label }}»</span>
              <span :class="['badge', 'badge--tone', status(p).tone]">{{ status(p).label }}</span>
            </div>
            <p v-if="p.diagnosis_text" class="t-sm">{{ p.diagnosis_text }}</p>
            <template v-for="(i, n) in p.patch?.add ?? []" :key="n">
              <dl v-if="beforeAfter(i)" class="content__ba t-sm">
                <dt class="t-muted">Было</dt>
                <dd>{{ beforeAfter(i)!.before }}</dd>
                <dt class="t-muted">Предлагается</dt>
                <dd>{{ beforeAfter(i)!.after }}</dd>
              </dl>
            </template>
            <PatchDiff
              v-if="!(p.patch?.add ?? []).some((i) => beforeAfter(i))"
              :patch="p.patch"
              empty="Правка карточки — задача контент-менеджеру"
            />
            <p class="t-xs t-muted">
              Должны находить:
              {{
                clusterQueries(p)
                  .map((q) => q.text)
                  .join(', ')
              }}
            </p>
          </li>
        </ul>
      </TabsContent>

      <TabsContent value="terms">
        <p v-if="!terms.length" class="panel content__empty t-muted">
          Неизвестных терминов агент пока не находил.
        </p>
        <div v-else class="data-scroll" role="region" aria-label="Неизвестные термины" tabindex="0">
          <table class="data-table">
            <thead>
              <tr>
                <th scope="col">Термин</th>
                <th scope="col">Запросы</th>
                <th scope="col" class="num">Частота</th>
                <th scope="col">Статус</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="p in terms" :key="p.id">
                <th scope="row">«{{ p.cluster?.label }}»</th>
                <td class="t-muted">
                  {{
                    clusterQueries(p)
                      .slice(0, 4)
                      .map((q) => q.text)
                      .join(', ')
                  }}
                </td>
                <td class="num">{{ formatInt(clusterFreq(p)) }}</td>
                <td>
                  <span :class="['badge', 'badge--tone', status(p).tone]">{{
                    status(p).label
                  }}</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </TabsContent>

      <TabsContent value="aliases">
        <p v-if="!aliases.length" class="panel content__empty t-muted">
          Алиасов агент пока не предлагал.
        </p>
        <div v-else class="data-scroll" role="region" aria-label="Предложенные алиасы" tabindex="0">
          <table class="data-table">
            <thead>
              <tr>
                <th scope="col">Алиас</th>
                <th scope="col">Вид и цель</th>
                <th scope="col">Статус</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="a in aliases" :key="a.key">
                <th scope="row">{{ a.text }}</th>
                <td class="t-muted">{{ a.note }}</td>
                <td>
                  <span :class="['badge', 'badge--tone', status(a.p).tone]">
                    {{ status(a.p).label }}
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </TabsContent>
    </TabsRoot>
    <RouterLink :to="queueLink" class="link t-sm"
      >Решения по правкам — в очереди SearchOps</RouterLink
    >
  </section>
</template>

<style scoped>
.content {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.content__tabs {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.content__skel {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
}
.content__empty {
  padding: var(--space-4);
}
.content__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.content__item {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
}
.content__row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
}
.content__label {
  font-weight: var(--fw-medium);
}
.content__ba {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: 2px var(--space-3);
  margin: 0;
}
.content__ba dd {
  margin: 0;
}
</style>
