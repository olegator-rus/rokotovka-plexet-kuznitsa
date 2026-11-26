<script setup lang="ts">
// Переключатель роли бэк-офиса: меняет набор блоков в Аналитике, не права доступа.
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import UiSelect from '@/components/ui/UiSelect.vue'
import { ROLES, isRoleId } from '@/lib/roles'
import { useAppStore } from '@/stores/app'

const app = useAppStore()
const { role } = storeToRefs(app)
const options = ROLES.map((r) => ({ value: r.id, label: r.label }))
const model = computed({
  get: () => role.value as string,
  set: (v: string) => {
    if (isRoleId(v)) app.setRole(v)
  },
})
</script>

<template>
  <UiSelect v-model="model" :options="options" label="Роль" prefix="Роль" />
</template>
