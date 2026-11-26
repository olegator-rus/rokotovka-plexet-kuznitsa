<script setup lang="ts">
import { Coins } from 'lucide-vue-next'
import SectionPending from '@/components/common/SectionPending.vue'
</script>

<template>
  <SectionPending
    title="Экономика"
    :icon="Coins"
    lead="Вызовов модели ещё не было в журнале. Стоимость одного запроса уже видна в трассе поиска."
    :items="[
      'Средняя стоимость запроса, доля запросов без LLM, доля попаданий в кэш',
      'Таблица по маршрутам A–D: доля, среднее время, средняя стоимость',
      'Месяц на 20 и 200 тыс. запросов со стоимостью сервера как допущением',
      'Журнал вызовов модели со ссылкой на трассу',
    ]"
    source="GET /api/costs"
  />
</template>
