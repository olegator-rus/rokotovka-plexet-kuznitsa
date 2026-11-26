<script setup lang="ts">
// Акция у позиции: строка «Акция: …» и ссылка «условия» на страницу акции. Это не бейдж
// «Скидка N%»: скидка — уже в цене по фиду, акция — условия, которые покупатель проверяет сам. Срок не выводится.
import { computed } from 'vue'
import { ExternalLink, Tag } from 'lucide-vue-next'
import type { Promo } from '@/api/typesP2'
import { promoViews } from '@/lib/offerNotes'

const props = defineProps<{ promos?: Promo[] | null }>()
const list = computed(() => promoViews(props.promos))
</script>

<template>
  <ul v-if="list.length" class="promo t-xs" data-testid="promo">
    <li v-for="p in list" :key="p.id" class="promo__row">
      <Tag :size="12" aria-hidden="true" class="promo__icon" />
      <span>
        {{ p.text
        }}<template v-if="p.url"
          >,
          <a :href="p.url" target="_blank" rel="noopener noreferrer" class="link promo__link">
            условия<ExternalLink :size="11" aria-hidden="true" />
            <span class="sr-only"> акции (откроется в новой вкладке)</span>
          </a></template
        >
      </span>
    </li>
  </ul>
</template>

<style scoped>
.promo {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  padding: 0;
  list-style: none;
  color: var(--text-2);
}
.promo__row {
  display: flex;
  align-items: baseline;
  gap: var(--space-1);
  min-width: 0;
  overflow-wrap: anywhere;
}
.promo__icon {
  flex: none;
  align-self: flex-start;
  margin-top: 2px;
}
.promo__link {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  white-space: nowrap;
}
</style>
