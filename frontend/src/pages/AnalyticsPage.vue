<script setup lang="ts">
import { ChartColumn } from 'lucide-vue-next'
import SectionPending from '@/components/common/SectionPending.vue'
</script>

<template>
  <SectionPending
    title="Аналитика"
    :icon="ChartColumn"
    lead="Аналитика строится из событий поиска; сводка по ним ещё не подключена."
    :items="[
      'Доля пустых выдач, поисков без клика и конверсия поиска в заказ',
      'Воронка: поиски → клики → корзины → заказы',
      'Проблемные запросы: что произошло, почему и что делать',
      'Блоки под роль из шапки: менеджер e-com, продукт, контент, руководитель',
    ]"
    source="GET /api/analytics"
  />
</template>
