<script setup lang="ts">
// Значок совместимости в карточке выдачи: «Подтверждено», «При условии: …» или
// «Совместимость не подтверждена». Цвет не несёт смысла — смысл в иконке и тексте.
import { computed } from 'vue'
import { ShieldAlert, ShieldCheck, ShieldQuestion } from 'lucide-vue-next'
import type { ItemCompatibility } from '@/api/typesP2'
import { compatView, modelTitle } from '@/lib/companions'

const props = defineProps<{ compat: ItemCompatibility | null | undefined }>()
const view = computed(() =>
  compatView(props.compat, props.compat ? modelTitle(props.compat.target_model) : ''),
)
const ICONS = { ok: ShieldCheck, cond: ShieldAlert, unknown: ShieldQuestion } as const
</script>

<template>
  <span v-if="view" :class="['badge compat', `compat--${view.tone}`]">
    <component :is="ICONS[view.tone]" :size="12" aria-hidden="true" />
    {{ view.text }}
  </span>
</template>

<style scoped>
.compat {
  white-space: normal;
}
.compat--ok {
  color: var(--text);
  border-color: var(--border-control);
}
.compat--unknown {
  border-style: dashed;
}
</style>
