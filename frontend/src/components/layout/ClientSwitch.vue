<script setup lang="ts">
// «Смотреть глазами клиента» — только сотруднику: список из /api/meta, без него — только «Гость».
// Нужен для флоу F5: один запрос от разных клиентов даёт разную выдачу. У покупателя клиент — из входа.
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import UiSelect, { type UiSelectOption } from '@/components/ui/UiSelect.vue'
import { useAppStore } from '@/stores/app'

const app = useAppStore()
const { clients, clientId } = storeToRefs(app)

const options = computed<UiSelectOption[]>(() =>
  clients.value.map((c) => ({ value: c.id, label: c.name, hint: c.hint })),
)
const model = computed({
  get: () => clientId.value,
  set: (v: string) => app.setClient(v),
})
</script>

<template>
  <UiSelect v-model="model" :options="options" label="Клиент" prefix="Клиент" />
</template>
