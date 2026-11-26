import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { router } from '@/router'
import '@/styles/global.css'

const app = createApp(App)

app.use(createPinia())
app.use(router)

app.mount('#app')

// Страница из кэша «назад/вперёд» хранит прежнее состояние входа: после «Выйти» кнопка «Назад» показала бы
// бэк-офис со старой шапкой. Восстановленную из кэша страницу перечитываем — охрана маршрутов сработает заново.
window.addEventListener('pageshow', (e) => {
  if (e.persisted) window.location.reload()
})
