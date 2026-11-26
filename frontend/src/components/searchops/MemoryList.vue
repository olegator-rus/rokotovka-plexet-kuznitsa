<script setup lang="ts">
// Вкладка «Память» (ARCHITECTURE §3.4): решения человека, которые агент читает перед
// разбором кластера. Отклонённое с причиной агент больше не предлагает.
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import { BookMarked } from 'lucide-vue-next'
import { getMemory } from '@/api/agent'
import BlockError from '@/components/common/BlockError.vue'
import { useResource } from '@/composables/useResource'
import { formatDateTime } from '@/lib/format'
import { diagnosisLabel, proposalStatus, rejectReasonLabel } from '@/lib/proposals'

const res = useResource((opts) => getMemory(opts))
void res.load()

const items = computed(() =>
  [...(res.data.value?.items ?? [])].sort((a, b) => String(b.ts).localeCompare(String(a.ts))),
)

/** decision может прийти как статус предложения (accepted/rejected/…) или словом — показываем подпись. */
const decision = (d: string) => proposalStatus(d)
</script>

<template>
  <div
    v-if="res.status.value === 'loading' && !res.data.value"
    class="panel mem__skel"
    aria-busy="true"
  >
    <span v-for="i in 4" :key="i" class="skeleton" :style="{ width: `${90 - i * 9}%` }" />
  </div>

  <BlockError
    v-else-if="res.status.value === 'error'"
    :error="res.error.value!"
    title="Память агента не загрузилась"
    @retry="res.load()"
  />

  <div v-else-if="!items.length" class="panel mem__empty">
    <BookMarked :size="20" aria-hidden="true" class="mem__icon" />
    <div>
      <p class="t-h2">Память пуста</p>
      <p class="t-muted">
        Здесь появятся решения по предложениям: что принято, что отклонено и почему. Перед разбором
        кластера агент читает память и не предлагает то, что уже отклонили.
      </p>
    </div>
  </div>

  <ul v-else class="mem">
    <li v-for="(m, i) in items" :key="`${m.proposal_id}-${i}`" class="panel mem__item">
      <div class="mem__head">
        <span :class="['badge', 'badge--tone', decision(m.decision).tone]">
          {{ decision(m.decision).label }}
        </span>
        <span class="mem__label">Кластер «{{ m.cluster_label }}»</span>
        <span class="t-xs t-muted t-num mem__ts">{{ formatDateTime(m.ts) }}</span>
      </div>
      <p class="t-sm">
        <template v-if="m.diagnosis">{{ diagnosisLabel(m.diagnosis) }}</template>
        <template v-if="m.reason && decision(m.decision).id !== 'accepted'">
          <template v-if="m.diagnosis"> · </template>причина:
          {{ rejectReasonLabel(m.reason).toLowerCase() }}
        </template>
      </p>
      <p v-if="m.comment" class="t-sm mem__comment">«{{ m.comment }}»</p>
      <p class="t-xs t-muted">
        {{
          decision(m.decision).id === 'rejected'
            ? 'Агент прочитает это перед разбором похожего кластера и не будет предлагать то же самое.'
            : 'Агент проверит, держится ли эффект правки, в следующем запуске.'
        }}
        <RouterLink
          v-if="m.proposal_id"
          :to="{ path: '/searchops', query: { tab: 'queue', status: 'all', p: m.proposal_id } }"
          class="link"
        >
          Открыть предложение
        </RouterLink>
      </p>
    </li>
  </ul>
</template>

<style scoped>
.mem__skel {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
}
.mem__empty {
  display: flex;
  gap: var(--space-3);
  padding: var(--space-5);
}
.mem__empty > div {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  max-width: 40rem;
}
.mem__icon {
  flex: none;
  color: var(--text-2);
}
.mem {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.mem__item {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding: var(--space-3) var(--space-4);
}
.mem__head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
}
.mem__label {
  font-weight: var(--fw-medium);
}
.mem__comment {
  color: var(--text-2);
}
.mem__ts {
  margin-left: auto;
}
</style>
