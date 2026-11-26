<script setup lang="ts">
import { Bot } from 'lucide-vue-next'
import SectionPending from '@/components/common/SectionPending.vue'
</script>

<template>
  <SectionPending
    title="SearchOps"
    :icon="Bot"
    lead="Агент ещё не запускался. Здесь он разбирает неудачные поиски и приносит проверенные правки."
    :items="[
      'Кнопка «Запустить разбор» и история запусков',
      'Лента шагов агента: инструмент, диагноз, гипотеза, результат прогона',
      'Очередь предложений: принять или отклонить с причиной',
    ]"
    source="POST /api/agent/runs, GET /api/agent/runs/{id}, GET /api/proposals"
  />
</template>
