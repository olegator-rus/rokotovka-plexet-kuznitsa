<script setup lang="ts">
// «Аналитика» → блок «Допродажи» для роли: плитки роли из GET /api/analytics
// (commerce). Каждая плитка кликабельна: открывает список за ней (менеджер e-com — запросы с пустым блоком,
// продукт-менеджер — находки агента, контент — позиции на заполнение) или ведёт на экран («Эффект»).
// Поведенческие числа подписаны «события стенда»: это не трафик магазина.
import { computed, ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { ArrowRight, Bot } from 'lucide-vue-next'
import type { AnalyticsCommerce, CommerceTile } from '@/api/typesKiller'
import SyntheticBadge from '@/components/common/SyntheticBadge.vue'
import UiDialog from '@/components/ui/UiDialog.vue'
import { formatInt } from '@/lib/format'
import { commerceSourceLabel, tileValue } from '@/lib/killer'

const props = defineProps<{ commerce: AnalyticsCommerce; role: string }>()
const router = useRouter()

const open = ref<CommerceTile | null>(null)
const dialogOpen = computed({
  get: () => open.value !== null,
  set: (v) => {
    if (!v) open.value = null
  },
})
const list = computed(() =>
  open.value?.list ? (props.commerce.lists.find((l) => l.key === open.value?.list) ?? null) : null,
)
const LINKS: Record<string, { path: string; query?: Record<string, string> }> = {
  effect: { path: '/costs', query: { tab: 'effect' } },
}
function onTile(t: CommerceTile) {
  const link = t.link ? LINKS[t.link] : null
  if (link && !t.list) {
    void router.push(link)
    return
  }
  open.value = t
}
const ACTION_HINT: Record<string, string> = {
  empty_block: 'Откройте запрос в поиске с трассой: шаг допродажи покажет, почему блок пуст.',
  types_without_complements:
    'Отправьте агенту на проверку: запуск в режиме «Коммерция» ищет пары по покупкам и предлагает связи.',
  copurchase_without_relation: 'Находки агента ждут решения в очереди SearchOps.',
  relations_without_basis: 'Список на заполнение: основание и уровень доказательности связи.',
  hardware_without_mpn: 'Список на заполнение: парт-номер позиции.',
  components_without_compat:
    'Список на заполнение: правило совместимости или атрибут compatible_with.',
}
const isProposalList = (key: string) => key === 'copurchase_without_relation'
</script>

<template>
  <section class="cb" :aria-labelledby="`cb-${role}`">
    <header class="cb__head">
      <h2 :id="`cb-${role}`" class="t-h2">{{ commerce.title }}</h2>
      <span class="t-xs t-muted">{{ commerce.note }}</span>
      <SyntheticBadge v-if="commerce.synthetic" />
    </header>

    <p v-if="!commerce.tiles.length" class="panel cb__empty t-sm t-muted">
      Для этой роли показателей допродаж нет<template v-if="commerce.missing.length"
        >: {{ commerce.missing.join('; ') }}</template
      >.
    </p>

    <ul v-else class="cb__tiles" :aria-label="`${commerce.title}: показатели`">
      <li v-for="t in commerce.tiles" :key="t.key">
        <button
          type="button"
          class="panel cb__tile"
          :data-testid="`tile-${t.key}`"
          :aria-label="`${t.label}: ${tileValue(t) ?? 'нет данных'}. ${t.list || t.link ? 'Открыть' : 'Как посчитано'}`"
          @click="onTile(t)"
        >
          <span class="cb__label">{{ t.label }}</span>
          <span v-if="tileValue(t) !== null" class="cb__value t-num">{{ tileValue(t) }}</span>
          <span v-else class="cb__value cb__value--missing">нет данных</span>
          <span class="cb__src t-xs">
            {{ commerceSourceLabel(t.source) }}
            <ArrowRight v-if="t.list || t.link" :size="12" aria-hidden="true" />
          </span>
        </button>
      </li>
    </ul>
    <ul v-if="commerce.missing.length && commerce.tiles.length" class="cb__missing">
      <li v-for="m in commerce.missing" :key="m" class="t-xs t-muted">{{ m }}</li>
    </ul>

    <UiDialog v-model:open="dialogOpen" :title="open?.label ?? ''" wide>
      <div v-if="open" class="cb__dlg">
        <p>
          <strong class="t-num">{{ tileValue(open) ?? 'нет данных' }}</strong>
          <span class="t-sm t-muted"> · {{ commerceSourceLabel(open.source) }}</span>
        </p>
        <p class="t-sm">{{ open.note }}</p>
        <template v-if="list">
          <p class="t-sm t-muted">
            {{ list.label }}: {{ formatInt(list.total)
            }}<template v-if="list.total > list.items.length">
              (показаны первые {{ formatInt(list.items.length) }})</template
            >
          </p>
          <p v-if="ACTION_HINT[list.key]" class="t-sm">{{ ACTION_HINT[list.key] }}</p>
          <p v-if="!list.items.length" class="t-sm t-muted">Список пуст — делать нечего.</p>
          <ul v-else class="cb__list">
            <li v-for="r in list.items" :key="r.id" class="cb__row">
              <RouterLink
                v-if="isProposalList(list.key)"
                :to="{ path: '/searchops', query: { tab: 'queue', status: 'all', p: r.id } }"
                class="link"
                >{{ r.title }}</RouterLink
              >
              <RouterLink
                v-else-if="r.query"
                :to="{ path: '/search', query: { q: r.query, trace: '1' } }"
                class="link"
                >{{ r.title }}</RouterLink
              >
              <span v-else>{{ r.title }}</span>
              <span class="t-xs t-muted">{{ r.detail }}</span>
            </li>
          </ul>
        </template>
      </div>
      <template #footer>
        <RouterLink
          v-if="open?.list === 'types_without_complements'"
          :to="{ path: '/searchops', query: { tab: 'runs' } }"
          class="btn btn--secondary"
        >
          <Bot :size="14" aria-hidden="true" /> Отправить агенту на проверку
        </RouterLink>
        <RouterLink
          v-if="open?.link && LINKS[open.link]"
          :to="LINKS[open.link]"
          class="btn btn--secondary"
          >Открыть «Эффект»</RouterLink
        >
        <button type="button" class="btn btn--primary" @click="open = null">Закрыть</button>
      </template>
    </UiDialog>
  </section>
</template>

<style scoped>
.cb {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.cb__head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1) var(--space-3);
}
.cb__empty {
  padding: var(--space-4);
}
.cb__tiles {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 14rem), 1fr));
  gap: var(--space-3);
  margin: 0;
  padding: 0;
  list-style: none;
}
.cb__tile {
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--space-1);
  width: 100%;
  height: 100%;
  min-width: 0;
  padding: var(--space-3) var(--space-4);
  text-align: left;
  font: inherit;
  color: var(--text);
  cursor: pointer;
  transition: border-color var(--dur-fast) var(--ease-standard);
}
.cb__tile:hover {
  border-color: var(--border-control);
}
.cb__label {
  color: var(--text-2);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
  overflow-wrap: anywhere;
}
.cb__value {
  font-size: var(--fs-h1);
  line-height: var(--lh-h1);
  font-weight: var(--fw-semibold);
}
.cb__value--missing {
  color: var(--text-3);
  font-size: var(--fs-body);
  font-weight: var(--fw-medium);
}
.cb__src {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  color: var(--text-3);
}
.cb__missing {
  margin: 0;
  padding: 0;
  list-style: none;
}
.cb__dlg {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  overflow-wrap: anywhere;
}
.cb__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  max-height: 22rem;
  overflow-y: auto;
  margin: 0;
  padding: 0;
  list-style: none;
}
.cb__row {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-sm);
  background: var(--surface-2);
}
</style>
