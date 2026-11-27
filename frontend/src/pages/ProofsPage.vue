<script setup lang="ts">
// «Доказательства» (S5; редизайн): один уровень навигации — девять экранов в меню слева, группы —
// только подписи. Раздел открыт гостю (жюри): действия бэк-офиса внутри видит только сотрудник.
// Адреса прежние: /proofs?tab=compare&view=metrics|one|regressions|panel|commerce, tab=contribution|cost|theory.
// Контур эталонов (): «На данных заказчика» — tab=feed, первым пунктом меню и экраном по умолчанию: эталоны,
// закрытая часть, «≥4 товара» и стоимость запроса на фиде. Прежние экраны с mock-прогонами остаются и подписаны.
import {
  type Component,
  type ComponentPublicInstance,
  computed,
  nextTick,
  onMounted,
  ref,
  useTemplateRef,
  watch,
} from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { TabsContent, TabsList, TabsRoot, TabsTrigger } from 'reka-ui'
import {
  BadgeCheck,
  GraduationCap,
  ChartColumn,
  Columns2,
  Coins,
  Layers,
  PanelTop,
  TrendingDown,
  Wallet,
} from 'lucide-vue-next'
import CompareTab from '@/components/proofs/CompareTab.vue'
import ContributionTab from '@/components/proofs/ContributionTab.vue'
import CostTab from '@/components/proofs/CostTab.vue'
import FeedSummaryTab from '@/components/proofs/FeedSummaryTab.vue'
import TheoryTab from '@/components/proofs/TheoryTab.vue'

type ViewId =
  | 'feed'
  | 'theory'
  | 'metrics'
  | 'one'
  | 'regressions'
  | 'panel'
  | 'contribution'
  | 'cost'
  | 'commerce'

interface View {
  id: ViewId
  label: string
  short: string
  about: string
  icon: Component
}

const GROUPS: readonly { title: string; views: readonly View[] }[] = [
  {
    title: 'Основы',
    views: [
      {
        id: 'theory',
        label: 'Теоретические основы',
        short: 'Как работает решение',
        about: 'Как устроен поиск и на каких методах он держится — простыми словами.',
        icon: GraduationCap,
      },
    ],
  },
  {
    title: 'Качество поиска',
    views: [
      {
        id: 'metrics',
        label: 'Метрики',
        short: 'Насколько стало лучше',
        about:
          'Одни и те же запросы прогоняются через версии поиска: от точного совпадения в названии (B0) до полной.',
        icon: ChartColumn,
      },
      {
        id: 'one',
        label: 'Один запрос',
        short: 'Две выдачи рядом',
        about: 'Введите запрос и сравните, что найдут две версии поиска.',
        icon: Columns2,
      },
      {
        id: 'regressions',
        label: 'Ухудшения',
        short: 'Где стало хуже',
        about: 'Запросы, на которых следующая версия поиска отвечает хуже предыдущей.',
        icon: TrendingDown,
      },
      {
        id: 'panel',
        label: 'Подсказки',
        short: 'Панель под строкой',
        about: 'Насколько панель под строкой поиска угадывает товар, пока запрос ещё набирают.',
        icon: PanelTop,
      },
      {
        id: 'contribution',
        label: 'Вклад компонентов',
        short: 'Что даёт каждая часть',
        about:
          'Что меняется в качестве, скорости и стоимости, если добавить или убрать часть поиска.',
        icon: Layers,
      },
    ],
  },
  {
    title: 'Деньги',
    views: [
      {
        id: 'cost',
        label: 'Стоимость',
        short: 'Сколько стоит запрос',
        about: 'Сколько стоит поиск и откуда берётся каждая цифра.',
        icon: Coins,
      },
      {
        id: 'commerce',
        label: 'Коммерция',
        short: 'Дополнения и маржа',
        about: 'Как дополнения и политика ранжирования меняют выдачу и то, что в ней продаётся.',
        icon: Wallet,
      },
    ],
  },
  {
    title: 'Итог',
    views: [
      {
        id: 'feed',
        label: 'На данных заказчика',
        short: 'Главное одним экраном',
        about:
          'Эталоны заказчика, закрытая часть набора, доля ответов с четырьмя товарами и стоимость запроса — на фиде магазина, только из прогонов.',
        icon: BadgeCheck,
      },
    ],
  },
]
const VIEWS = GROUPS.flatMap((g) => g.views)
/** Экраны со своим адресом tab=<id>; остальные — экраны сравнения tab=compare&view=<id>. */
const STANDALONE: readonly ViewId[] = ['feed', 'theory', 'contribution', 'cost']

const route = useRoute()
const router = useRouter()
const str = (v: unknown) => (typeof v === 'string' ? v : '')
const current = computed<ViewId>({
  get: () => {
    const tab = str(route.query.tab)
    if (STANDALONE.includes(tab as ViewId)) return tab as ViewId
    const view = str(route.query.view)
    if (VIEWS.some((v) => v.id === view && !STANDALONE.includes(v.id))) return view as ViewId
    // Без адреса — сводка на данных заказчика; прежний адрес tab=compare без view — «Метрики».
    return tab === 'compare' ? 'metrics' : 'feed'
  },
  set: (id) => {
    if (STANDALONE.includes(id)) void router.replace({ query: { tab: id } })
    else {
      // Запрос и выбранные версии сравнения сохраняются при переходе между экранами оценки.
      const keep = str(route.query.tab) === 'compare' ? route.query : {}
      void router.replace({ query: { ...keep, tab: 'compare', view: id } })
    }
  },
})
const view = computed(() => VIEWS.find((v) => v.id === current.value) ?? VIEWS[0]!)
const compare = computed(() => !STANDALONE.includes(current.value))
/** Число ухудшений известно после загрузки прогонов. */
const regressions = ref<number | null>(null)

// На узком экране меню прокручивается строкой: выбранный пункт виден целиком.
const list = useTemplateRef<ComponentPublicInstance>('list')
function revealActive() {
  const el = list.value?.$el as HTMLElement | undefined
  const active = el?.querySelector<HTMLElement>('[data-state="active"]')
  if (!el || !active || el.scrollWidth <= el.clientWidth) return
  const l = el.getBoundingClientRect()
  const a = active.getBoundingClientRect()
  const pad = 16
  if (a.right > l.right) el.scrollLeft += a.right - l.right + pad
  else if (a.left < l.left) el.scrollLeft -= l.left - a.left + pad
}
onMounted(revealActive)
watch(current, () => void nextTick(revealActive))
</script>

<template>
  <section class="page proofs" aria-labelledby="proofs-title">
    <header class="proofs__hero on-hero">
      <h1 id="proofs-title" class="proofs__title">Доказательства</h1>
      <p class="proofs__lead">
        Всё, что мы утверждаем о поиске, измерено. Каждое число взято из прогона или журнала, и у
        каждого есть «как посчитано».
      </p>
    </header>

    <TabsRoot v-model="current" orientation="vertical" class="proofs__layout">
      <TabsList ref="list" class="proofs__nav" aria-label="Разделы доказательств">
        <template v-for="g in GROUPS" :key="g.title">
          <p class="proofs__group" aria-hidden="true">{{ g.title }}</p>
          <TabsTrigger v-for="v in g.views" :key="v.id" :value="v.id" class="proofs__item">
            <component :is="v.icon" :size="18" aria-hidden="true" class="proofs__icon" />
            <span class="proofs__text">
              <span class="proofs__label">{{ v.label }}</span>
              <span class="proofs__short" aria-hidden="true">{{ v.short }}</span>
            </span>
            <span
              v-if="v.id === 'regressions' && regressions !== null"
              class="proofs__badge t-num"
              :aria-label="`: ${regressions}`"
              >{{ regressions }}</span
            >
          </TabsTrigger>
        </template>
      </TabsList>

      <TabsContent :value="current" class="proofs__panel">
        <header class="proofs__view">
          <h2 class="proofs__view-title">{{ view.label }}</h2>
          <p class="t-sm t-muted">{{ view.about }}</p>
        </header>
        <CompareTab v-if="compare" @regressions-total="regressions = $event" />
        <FeedSummaryTab v-else-if="current === 'feed'" />
        <TheoryTab v-else-if="current === 'theory'" />
        <ContributionTab v-else-if="current === 'contribution'" />
        <CostTab v-else />
      </TabsContent>
    </TabsRoot>
  </section>
</template>

<style scoped>
.proofs {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}
.proofs__hero {
  position: relative;
  overflow: hidden;
  padding: var(--space-6);
  border-radius: var(--radius-panel);
}
/* Фирменные квадраты из логотипа — тихий фон справа. */
.proofs__hero::after {
  content: '';
  position: absolute;
  right: calc(-1 * var(--space-6));
  top: calc(-1 * var(--space-8));
  width: 13rem;
  height: 13rem;
  background:
    linear-gradient(var(--hero-hover), var(--hero-hover)) 0 0 / 48% 48% no-repeat,
    linear-gradient(var(--hero-hover), var(--hero-hover)) 100% 0 / 48% 48% no-repeat,
    linear-gradient(var(--hero-hover), var(--hero-hover)) 0 100% / 48% 48% no-repeat,
    linear-gradient(var(--hero-hover), var(--hero-hover)) 100% 100% / 48% 48% no-repeat;
  transform: rotate(12deg);
  pointer-events: none;
}
.proofs__title {
  margin: 0 0 var(--space-2);
  font-size: var(--fs-banner);
  line-height: 1.2;
  font-weight: var(--fw-bold);
  letter-spacing: -0.02em;
}
.proofs__lead {
  position: relative;
  z-index: 1;
  max-width: 60ch;
  margin: 0;
  color: var(--on-hero-2);
}
.proofs__layout {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  min-width: 0;
}
/* Узкий экран: меню — строка пилюль с прокруткой. */
.proofs__nav {
  display: flex;
  gap: var(--space-2);
  min-width: 0;
  padding-bottom: var(--space-1);
  overflow-x: auto;
  scrollbar-width: none;
}
.proofs__group,
.proofs__short {
  display: none;
}
.proofs__item {
  display: inline-flex;
  flex: none;
  align-items: center;
  gap: var(--space-3);
  min-height: var(--control-h);
  padding: var(--space-1) var(--space-4);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-pill);
  background: var(--surface);
  color: var(--text);
  font: inherit;
  text-align: left;
  white-space: nowrap;
  cursor: pointer;
}
.proofs__item:hover {
  border-color: var(--accent);
}
.proofs__item[data-state='active'] {
  border-color: var(--accent);
  background: var(--accent);
  color: var(--on-accent);
}
.proofs__icon {
  flex: none;
}
.proofs__text {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-width: 0;
}
.proofs__label {
  font-weight: var(--fw-semibold);
}
.proofs__badge {
  min-width: 1.5rem;
  padding: 0 var(--space-1);
  box-sizing: border-box;
  border-radius: var(--radius-pill);
  background: var(--surface-3);
  color: var(--text);
  font-size: var(--fs-xs);
  line-height: 1.5rem;
  text-align: center;
}
.proofs__panel {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  min-width: 0;
}
.proofs__panel:focus-visible {
  outline-offset: 4px;
}
.proofs__view {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding-bottom: var(--space-3);
  border-bottom: var(--border-w) solid var(--border);
}
.proofs__view-title {
  margin: 0;
  font-size: var(--fs-h1);
  line-height: var(--lh-h1);
  font-weight: var(--fw-bold);
}
.proofs__view p {
  max-width: 70ch;
  margin: 0;
}

@media (min-width: 1024px) {
  .proofs__layout {
    display: grid;
    grid-template-columns: 16rem minmax(0, 1fr);
    align-items: start;
    gap: var(--space-6);
  }
  .proofs__nav {
    position: sticky;
    top: calc(var(--header-actual, var(--header-h)) + var(--space-3));
    flex-direction: column;
    gap: 2px;
    max-height: calc(100dvh - var(--header-actual, var(--header-h)) - 2 * var(--space-3));
    padding: var(--space-2);
    overflow-y: auto;
    border: var(--border-w) solid var(--border);
    border-radius: var(--radius-card);
    background: var(--surface);
  }
  .proofs__group {
    display: block;
    margin: 0;
    padding: var(--space-3) var(--space-3) var(--space-1);
    color: var(--text-3);
    font-size: var(--fs-xs);
    font-weight: var(--fw-bold);
  }
  .proofs__short {
    display: block;
    color: var(--text-3);
    font-size: var(--fs-xs);
    line-height: var(--lh-xs);
  }
  .proofs__item {
    padding: var(--space-2) var(--space-3);
    border-color: transparent;
    border-radius: var(--radius-control);
    white-space: normal;
  }
  .proofs__item:hover {
    border-color: transparent;
    background: var(--surface-2);
  }
  .proofs__item[data-state='active'] {
    border-color: transparent;
    background: var(--accent-soft);
    color: var(--accent);
  }
  .proofs__item[data-state='active'] .proofs__badge {
    background: var(--accent);
    color: var(--on-accent);
  }
}
</style>
