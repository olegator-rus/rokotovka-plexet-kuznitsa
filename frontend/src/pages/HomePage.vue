<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { SearchX } from 'lucide-vue-next'
import ShopHeader from '@/components/shop/ShopHeader.vue'
import SearchBar from '@/components/shop/SearchBar.vue'
import CategoryChips from '@/components/shop/CategoryChips.vue'
import ProductCard from '@/components/shop/ProductCard.vue'
import ApiStatus from '@/components/shop/ApiStatus.vue'
import { filterProducts, parseCategory, type CategoryId } from '@/lib/catalog'
import { DEMO_CATEGORIES, DEMO_PRODUCTS } from '@/mocks/catalog'

// Стартовая страница = витрина магазина, вход не нужен. Запрос и категория живут в адресе
// (?q=…&cat=…): ссылку на выдачу можно отправить, «назад» не теряет поиск.
const route = useRoute()
const router = useRouter()

function setQuery(patch: Record<string, string | undefined>): void {
  const next = { ...route.query, ...patch }
  for (const k of Object.keys(next)) if (!next[k]) delete next[k]
  void router.replace({ query: next })
}

const query = computed({
  get: () => (typeof route.query.q === 'string' ? route.query.q : ''),
  set: (v: string) => setQuery({ q: v || undefined }),
})
const category = computed({
  get: () => parseCategory(route.query.cat, DEMO_CATEGORIES),
  set: (v: CategoryId | null) => setQuery({ cat: v ?? undefined }),
})

const categoryById = new Map(DEMO_CATEGORIES.map((c) => [c.id, c]))
const results = computed(() =>
  filterProducts(DEMO_PRODUCTS, DEMO_CATEGORIES, query.value, category.value),
)
const heading = computed(() => {
  if (query.value.trim()) return `Нашлось: ${results.value.length}`
  if (category.value) return categoryById.get(category.value)?.title ?? 'Каталог'
  return 'Популярное'
})

function reset(): void {
  void router.replace({ query: {} })
}
</script>

<template>
  <div class="flex min-h-dvh flex-col">
    <ShopHeader />

    <main class="flex-1">
      <section class="hero">
        <div class="container-page hero__inner">
          <h1 class="t-hero m-0">Что ищем сегодня?</h1>
          <p class="hero__lead m-0">Введите название товара или выберите категорию.</p>
          <SearchBar v-model="query" class="hero__search" />
          <CategoryChips v-model="category" :categories="DEMO_CATEGORIES" />
        </div>
      </section>

      <section class="container-page pb-12" aria-labelledby="catalog-title">
        <h2 id="catalog-title" class="t-h2 mt-0 mb-4" aria-live="polite">{{ heading }}</h2>

        <ul v-if="results.length" class="grid">
          <li v-for="p in results" :key="p.id">
            <ProductCard :product="p" :category="categoryById.get(p.category)" />
          </li>
        </ul>

        <div v-else class="empty">
          <SearchX :size="40" :stroke-width="1.5" class="text-text-2" aria-hidden="true" />
          <p class="t-h2 m-0">Ничего не нашлось</p>
          <p class="m-0 text-text-2">Попробуйте другое слово или посмотрите все товары.</p>
          <Button label="Показать все товары" @click="reset" />
        </div>
      </section>
    </main>

    <footer class="footer">
      <div class="container-page footer__inner">
        <p class="t-caption m-0 text-text-2">Демо-витрина: товары и цены вымышлены.</p>
        <ApiStatus />
      </div>
    </footer>
  </div>
</template>

<style scoped>
.hero {
  padding: 2rem 0 1.5rem;
  background:
    radial-gradient(60% 120% at 100% 0%, var(--accent-soft) 0%, transparent 70%), var(--bg);
}
@media (min-width: 768px) {
  .hero {
    padding: 3.5rem 0 2rem;
  }
}
.hero__inner {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}
.hero__lead {
  color: var(--text-2);
}
.hero__search {
  max-width: 44rem;
  margin: 0.5rem 0 0.25rem;
}
.grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.75rem;
  margin: 0;
  padding: 0;
  list-style: none;
}
.grid > li {
  display: flex;
}
.grid > li > * {
  flex: 1;
}
@media (min-width: 768px) {
  .grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 1rem;
  }
}
@media (min-width: 1024px) {
  .grid {
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 1.25rem;
  }
}
.empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.5rem;
  padding: 3rem 1rem;
  border: 1px dashed var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
  text-align: center;
}
.empty :deep(.p-button) {
  margin-top: 0.75rem;
}
.footer {
  border-top: 1px solid var(--border);
  background: var(--surface);
}
.footer__inner {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem 1.5rem;
  padding-block: 1.25rem;
}
</style>
