import { createApp } from 'vue'
import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import App from './App.vue'
import { router } from '@/router'
import { ShopPreset } from '@/lib/primevuePreset'
import { ruLocale } from '@/lib/primevueLocale'
import '@/styles/global.css'

const app = createApp(App)

app.use(createPinia())
app.use(router)
app.use(PrimeVue, {
  locale: ruLocale,
  theme: {
    preset: ShopPreset,
    options: {
      // Тема одна, светлая: тёмную не включаем ни по системе, ни по атрибуту.
      darkModeSelector: false,
      // Слой primevue встаёт в порядок, объявленный в styles/global.css.
      cssLayer: { name: 'primevue', order: 'theme, base, primevue' },
    },
  },
})

app.mount('#app')
