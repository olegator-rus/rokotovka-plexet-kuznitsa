<script setup lang="ts">
// «Изменения» → «Политика ранжирования»: версии политики маржи и откат одним действием (применение
// на вкладке «Политика ранжирования» создаёт версию, откат — во «Изменениях»). Данные — GET /api/margin/policy.
import { getMarginPolicy } from '@/api/margin'
import BlockError from '@/components/common/BlockError.vue'
import MarginHistory from '@/components/margin/MarginHistory.vue'
import { useToast } from '@/components/ui/useToast'
import { useResource } from '@/composables/useResource'

const toast = useToast()
const policy = useResource((opts) => getMarginPolicy(opts))
void policy.load()

async function onChanged(version: number) {
  await policy.load({ quiet: true })
  toast.add({
    tone: 'ok',
    title: `Политика ранжирования v${version}`,
    detail: 'Действует новая версия — копия выбранной.',
  })
}
</script>

<template>
  <div
    v-if="policy.status.value === 'loading' && !policy.data.value"
    class="panel"
    aria-busy="true"
  >
    <span class="skeleton" style="width: 40%" />
  </div>
  <BlockError
    v-else-if="policy.status.value === 'error' && !policy.data.value"
    :error="policy.error.value!"
    title="Версии политики не загрузились"
    @retry="policy.load()"
  />
  <MarginHistory
    v-else-if="policy.data.value"
    :versions="policy.data.value.versions ?? []"
    :active="policy.data.value.version"
    @changed="onChanged"
  />
</template>
