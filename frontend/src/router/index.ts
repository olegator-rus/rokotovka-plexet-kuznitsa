import { createRouter, createWebHistory } from 'vue-router'
import HomePage from '@/pages/HomePage.vue'
import { SHOP_NAME } from '@/lib/brand'

// Маршруты витрины. Входа нет: все страницы публичные. Страница с title в meta меняет заголовок вкладки.
export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'home', component: HomePage, meta: { title: SHOP_NAME } },
    {
      path: '/:pathMatch(.*)*',
      name: 'not-found',
      component: () => import('@/pages/NotFoundPage.vue'),
      meta: { title: `Страница не найдена — ${SHOP_NAME}` },
    },
  ],
  scrollBehavior(to, from, saved) {
    if (saved) return saved
    // Смена ?q= и ?cat= на той же странице не прыгает наверх.
    if (to.path === from.path) return false
    return { top: 0 }
  },
})

router.afterEach((to) => {
  if (typeof to.meta.title === 'string') document.title = to.meta.title
})
