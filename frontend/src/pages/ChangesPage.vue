<script setup lang="ts">
import { History } from 'lucide-vue-next'
import SectionPending from '@/components/common/SectionPending.vue'
</script>

<template>
  <SectionPending
    title="Изменения"
    :icon="History"
    lead="Действует исходная версия правил. Принятые правки агента появятся здесь версиями."
    :items="[
      'Версии правил: номер, время, кто принял, ссылка на предложение',
      'Разница с предыдущей версией — списком добавленных и удалённых правил',
      'Откат создаёт новую версию, история остаётся полной',
    ]"
    source="GET /api/configs, POST /api/configs/rollback"
  />
</template>
