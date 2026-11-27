<script setup lang="ts">
// Строка над обычной выдачей, открытой из позиции ТЗ (TZ-UI.md §8): «← Вернуться к ТЗ «…» · позиция 3 из 8». Возврат —
// на /search?tz=<хеш>: то же ТЗ без повторного разбора, раскрытые блоки и место прокрутки восстанавливаются.
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import { ArrowLeft } from 'lucide-vue-next'
import type { TzDocument } from '@/api/typesTz'
import { backLabel } from '@/lib/tz'

const props = defineProps<{ hash: string; item: number | null; doc: TzDocument | null }>()

const text = computed(() =>
  props.doc
    ? backLabel(props.doc.file_name, props.item, props.doc.positions.length)
    : 'Вернуться к ТЗ',
)
</script>

<template>
  <RouterLink v-slot="{ href, navigate }" :to="{ path: '/search', query: { tz: hash } }" custom>
    <a :href="href" class="tzback link t-sm" data-testid="tz-back" @click="navigate">
      <ArrowLeft :size="16" aria-hidden="true" class="tzback__icon" />
      <span>{{ text }}</span>
    </a>
  </RouterLink>
</template>

<style scoped>
.tzback {
  display: inline-flex;
  align-items: flex-start;
  align-self: flex-start;
  gap: var(--space-1);
  min-height: 28px;
  overflow-wrap: anywhere;
}
.tzback__icon {
  flex: none;
  margin-top: 1px;
}
</style>
