<script setup lang="ts">
import { GitCompareArrows } from 'lucide-vue-next'
import SectionPending from '@/components/common/SectionPending.vue'
</script>

<template>
  <SectionPending
    title="До / После"
    :icon="GitCompareArrows"
    lead="Прогон оценки ещё не запускался. Здесь будет доказательство «стало лучше» на тестовом наборе."
    :items="[
      'Метрики по конфигурациям B0–B4: значение и изменение к предыдущему столбцу',
      'Один запрос в двух конфигурациях рядом, со сдвигом позиций',
      'Запросы, где новая конфигурация проиграла, — открытым списком',
      'Строка воспроизводимости: хэш набора, версия конфигурации, коммит',
    ]"
    source="GET /api/evaluations, POST /api/evaluations/run"
  />
</template>
