<script setup lang="ts">
// «Доказательства» → «Теоретические основы» (tab=theory): как устроено решение и на каких методах держится
// поиск — BM25 и векторы, слияние RRF, реранкер, маршруты A–D, кэш разборов, метрики NDCG/MRR, агент.
// Пока заглушка: текст наполняется отдельно.
</script>

<template>
  <div class="panel theory__empty">
    <p class="t-sm">Раздел готовится.</p>
    <p class="t-xs t-muted">
      Здесь будет подробно описано, как работает решение: от разбора запроса до выдачи и оценки.
    </p>
  </div>
</template>

<style scoped>
.theory__empty {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding: var(--space-6) var(--space-4);
  text-align: center;
}
</style>
