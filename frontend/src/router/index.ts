import { createRouter, createWebHistory } from 'vue-router'
import SearchPage from '@/pages/SearchPage.vue'
import { PRODUCT_NAME } from '@/lib/brand'

declare module 'vue-router' {
  interface RouteMeta {
    /** Название раздела: заголовок вкладки и подпись в шапке. */
    section?: string
    /** Бэк-офисный раздел: в шапке виден переключатель роли. */
    backoffice?: boolean
  }
}

// Шесть разделов («Навигация»). Входа нет: все страницы публичные.
export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/search' },
    { path: '/search', name: 'search', component: SearchPage, meta: { section: 'Поиск' } },
    {
      path: '/compare',
      name: 'compare',
      component: () => import('@/pages/ComparePage.vue'),
      meta: { section: 'До / После' },
    },
    {
      path: '/analytics',
      name: 'analytics',
      component: () => import('@/pages/AnalyticsPage.vue'),
      meta: { section: 'Аналитика', backoffice: true },
    },
    {
      path: '/searchops',
      name: 'searchops',
      component: () => import('@/pages/SearchOpsPage.vue'),
      meta: { section: 'SearchOps', backoffice: true },
    },
    {
      path: '/searchops/runs/:id',
      name: 'agent-run',
      component: () => import('@/pages/AgentRunPage.vue'),
      meta: { section: 'Запуск агента', backoffice: true },
    },
    {
      path: '/changes',
      name: 'changes',
      component: () => import('@/pages/ChangesPage.vue'),
      meta: { section: 'Изменения', backoffice: true },
    },
    {
      path: '/costs',
      name: 'costs',
      component: () => import('@/pages/CostsPage.vue'),
      meta: { section: 'Экономика', backoffice: true },
    },
    {
      path: '/rules',
      name: 'rules',
      component: () => import('@/pages/RulesPage.vue'),
      meta: { section: 'Правила рекомендаций' },
    },
    {
      path: '/margin',
      name: 'margin',
      component: () => import('@/pages/MarginPage.vue'),
      meta: { section: 'Маржа', backoffice: true },
    },
    {
      path: '/:pathMatch(.*)*',
      name: 'not-found',
      component: () => import('@/pages/NotFoundPage.vue'),
      meta: { section: 'Страница не найдена' },
    },
  ],
  scrollBehavior(to, from, saved) {
    if (saved) return saved
    // Смена запроса на той же странице не прыгает наверх.
    if (to.path === from.path) return false
    return { top: 0 }
  },
})

router.afterEach((to) => {
  document.title = to.meta.section ? `${to.meta.section} — ${PRODUCT_NAME}` : PRODUCT_NAME
})
