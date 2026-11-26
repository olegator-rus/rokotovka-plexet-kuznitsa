<script setup lang="ts">
// «Доказательства» («Навигация», S5): вкладки «До / После», «Вклад компонентов», «Стоимость».
// Раздел открыт гостю (жюри): действия бэк-офиса внутри вкладок видит только сотрудник. Вкладка в адресе:
// /proofs?tab=compare|contribution|cost; старые /compare и /costs ведут сюда (router).
import {
  type ComponentPublicInstance,
  computed,
  nextTick,
  onMounted,
  useTemplateRef,
  watch,
} from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { TabsContent, TabsList, TabsRoot, TabsTrigger } from 'reka-ui'
import CompareTab from '@/components/proofs/CompareTab.vue'
import ContributionTab from '@/components/proofs/ContributionTab.vue'
import CostTab from '@/components/proofs/CostTab.vue'

const TABS = ['compare', 'contribution', 'cost'] as const
type Tab = (typeof TABS)[number]

const route = useRoute()
const router = useRouter()
const tab = computed<Tab>({
  get: () => {
    const t = typeof route.query.tab === 'string' ? route.query.tab : ''
    return (TABS as readonly string[]).includes(t) ? (t as Tab) : 'compare'
  },
  set: (t) => void router.replace({ query: { tab: t } }),
})

// На узком экране список вкладок прокручивается: выбранная («Стоимость» по ссылке) видна целиком.
const tabsList = useTemplateRef<ComponentPublicInstance>('tabsList')
function revealActiveTab() {
  const list = tabsList.value?.$el as HTMLElement | undefined
  const active = list?.querySelector<HTMLElement>('[data-state="active"]')
  if (!list || !active || list.scrollWidth <= list.clientWidth) return
  const l = list.getBoundingClientRect()
  const a = active.getBoundingClientRect()
  const pad = 16
  if (a.right > l.right) list.scrollLeft += a.right - l.right + pad
  else if (a.left < l.left) list.scrollLeft -= l.left - a.left + pad
}
onMounted(revealActiveTab)
watch(tab, () => void nextTick(revealActiveTab))
</script>

<template>
  <section class="page proofs" aria-labelledby="proofs-title">
    <header class="proofs__head">
      <h1 id="proofs-title" class="t-h1">Доказательства</h1>
      <p class="t-sm t-muted">
        Что измерено: качество до и после, вклад каждого компонента и стоимость запроса. Каждое
        число — из прогона или журнала, у каждого есть «как посчитано».
      </p>
    </header>

    <TabsRoot v-model="tab" class="proofs__tabs">
      <TabsList ref="tabsList" class="tabs" aria-label="Разделы доказательств">
        <TabsTrigger value="compare" class="tabs__trigger">До / После</TabsTrigger>
        <TabsTrigger value="contribution" class="tabs__trigger">Вклад компонентов</TabsTrigger>
        <TabsTrigger value="cost" class="tabs__trigger">Стоимость</TabsTrigger>
      </TabsList>
      <TabsContent value="compare" class="proofs__panel">
        <CompareTab />
      </TabsContent>
      <TabsContent value="contribution" class="proofs__panel">
        <ContributionTab />
      </TabsContent>
      <TabsContent value="cost" class="proofs__panel">
        <CostTab />
      </TabsContent>
    </TabsRoot>
  </section>
</template>

<style scoped>
.proofs,
.proofs__tabs {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
.proofs__head {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  max-width: 44rem;
}
.proofs__panel:focus-visible {
  outline-offset: 4px;
}
</style>
