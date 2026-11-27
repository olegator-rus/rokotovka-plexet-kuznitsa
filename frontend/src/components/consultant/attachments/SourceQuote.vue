<script setup lang="ts">
// Источник (CONSULTANT.md §6.7): название, до трёх строк дословного фрагмента (длинный раскрывается), «Открыть
// документ». Фрагмента нет — только ссылка.
import { computed, ref } from 'vue'
import { ExternalLink, FileText } from 'lucide-vue-next'
import type { Attachment } from '@/api/typesConsultant'

const props = defineProps<{ data: Extract<Attachment, { type: 'source' }> }>()
const more = ref(false)
const url = computed(() =>
  props.data.url && /^https?:\/\//.test(props.data.url) ? props.data.url : '',
)
const long = computed(() => (props.data.quote ?? '').length > 220)
</script>

<template>
  <section class="sq" data-testid="consultant-source">
    <div class="sq__head">
      <FileText :size="14" aria-hidden="true" />
      <span class="sq__title">{{ data.title || 'Документация вендора' }}</span>
    </div>
    <blockquote v-if="data.quote" class="sq__quote" :class="{ 'sq__quote--clamp': long && !more }">
      {{ data.quote }}
    </blockquote>
    <div class="sq__actions">
      <button v-if="long" type="button" class="sq__link" @click="more = !more">
        {{ more ? 'Свернуть' : 'Показать полностью' }}
      </button>
      <a v-if="url" class="sq__link" :href="url" target="_blank" rel="noopener noreferrer">
        Открыть документ <ExternalLink :size="12" aria-hidden="true" />
      </a>
    </div>
  </section>
</template>

<style scoped>
.sq {
  min-width: 0;
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--border);
  border-radius: 0.75rem;
  background: var(--surface);
  font-size: var(--fs-sm);
}
.sq__head {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  color: var(--text-2);
}
.sq__title {
  font-weight: var(--fw-medium);
  color: var(--text);
}
.sq__quote {
  margin: var(--space-1) 0 0;
  padding-left: var(--space-2);
  border-left: 2px solid var(--border);
  color: var(--text-2);
  overflow-wrap: anywhere;
}
.sq__quote--clamp {
  display: -webkit-box;
  overflow: hidden;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 3;
  line-clamp: 3;
}
.sq__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
}
.sq__link {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  min-height: 2rem;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--text);
  font: inherit;
  text-decoration: underline;
  text-underline-offset: 2px;
  cursor: pointer;
}
</style>
