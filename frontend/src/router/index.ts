import { type LocationQuery, createRouter, createWebHistory } from 'vue-router'
import SearchPage from '@/pages/SearchPage.vue'
import { LOGIN_PATH, decideGuard } from '@/lib/auth'
import { PRODUCT_NAME } from '@/lib/brand'
import { useAuthStore } from '@/stores/auth'

/** /compare?tab=one&q=… → /proofs?tab=compare&view=one&q=…; вкладка «Эффект агента» удалена — на «Метрики». */
function compareQuery(q: LocationQuery): Record<string, string> {
  const out: Record<string, string> = { tab: 'compare' }
  const view = typeof q.tab === 'string' ? q.tab : ''
  if (['one', 'regressions', 'commerce', 'panel'].includes(view)) out.view = view
  for (const k of ['q', 'left', 'right', 'cfg']) {
    const v = q[k]
    if (typeof v === 'string' && v) out[k] = v
  }
  return out
}

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

// Разделы («Навигация»): «Поиск» и «Доказательства» открыты всем, SearchOps — после входа
// сотрудника.
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
      path: '/proofs',
      name: 'proofs',
      component: () => import('@/pages/ProofsPage.vue'),
      meta: { section: 'Доказательства' },
    },
    {
      path: '/help',
      name: 'help',
      component: () => import('@/pages/HelpPage.vue'),
      meta: { section: 'Помощь' },
    },
    {
      path: '/rules',
      name: 'rules',
      component: () => import('@/pages/RulesPage.vue'),
      meta: { section: 'Правила рекомендаций' },
    },
    {
      path: '/product/:id',
      name: 'product',
      component: () => import('@/pages/ProductPage.vue'),
      meta: { section: 'Товар' },
    },
    // Старые адреса контуров поиска–3 (контур доводки: три раздела) — на новые места, без ссылок в пустоту.
    { path: '/analytics', redirect: () => ({ path: '/searchops', query: { tab: 'failed' } }) },
    {
      path: '/changes',
      redirect: (to) => ({
        path: '/searchops',
        query: {
          tab: 'changes',
          ...(to.query.tab === 'relations' ? { view: 'relations' } : {}),
        },
      }),
    },
    {
      path: '/margin',
      redirect: (to) => ({
        path: '/searchops',
        query: { tab: 'policy', ...(typeof to.query.w === 'string' ? { w: to.query.w } : {}) },
      }),
    },
    { path: '/costs', redirect: () => ({ path: '/proofs', query: { tab: 'cost' } }) },
    { path: '/compare', redirect: (to) => ({ path: '/proofs', query: compareQuery(to.query) }) },
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
