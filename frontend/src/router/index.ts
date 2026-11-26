import { createRouter, createWebHistory } from 'vue-router'
import SearchPage from '@/pages/SearchPage.vue'
import { LOGIN_PATH, decideGuard } from '@/lib/auth'
import { PRODUCT_NAME } from '@/lib/brand'
import { useAuthStore } from '@/stores/auth'

declare module 'vue-router' {
  interface RouteMeta {
    /** Название раздела: заголовок вкладки и подпись в шапке. */
    section?: string
    /** Раздел бэк-офиса: только сотрудникам, гость и покупатель уходят на /login. */
    backoffice?: boolean
    /** Страница без оболочки (экран входа). */
    bare?: boolean
  }
}

// Разделы («Навигация»): витрина открыта всем, бэк-офис — после входа сотрудника.
export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/search' },
    { path: '/search', name: 'search', component: SearchPage, meta: { section: 'Поиск' } },
    {
      path: LOGIN_PATH,
      name: 'login',
      component: () => import('@/pages/LoginPage.vue'),
      meta: { section: 'Вход', bare: true },
    },
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

// Охрана: бэк-офис без роли сотрудника → /login?returnTo=…; вошедший на /login → свой стартовый экран.
router.beforeEach(async (to) => {
  const auth = useAuthStore()
  await auth.ensure()
  const d = decideGuard({
    fullPath: to.fullPath,
    isLogin: to.name === 'login',
    backoffice: to.meta.backoffice === true,
    role: auth.role,
    returnTo: to.query.returnTo,
  })
  return d.kind === 'redirect' ? d.to : true
})

router.afterEach((to) => {
  document.title = to.meta.section ? `${to.meta.section} — ${PRODUCT_NAME}` : PRODUCT_NAME
})
