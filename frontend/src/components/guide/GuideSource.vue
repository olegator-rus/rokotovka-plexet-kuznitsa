<script setup lang="ts">
// Источник под утверждением (UI 6.6): текстовая ссылка «Документация вендора» или «Правила лицензирования …»; по
// нажатию — всплывающий блок с дословным фрагментом и ссылкой на документ. Дат нет.
import { PopoverContent, PopoverPortal, PopoverRoot, PopoverTrigger } from 'reka-ui'
import { ExternalLink } from 'lucide-vue-next'
import type { GuideScreenSource } from '@/api/typesGuide'

defineProps<{
  source: GuideScreenSource
  /** Что подтверждает документ: «Шифрование данных». */
  heading?: string
  /** Где это есть: «Есть в Расширенной редакции». */
  sub?: string
}>()
</script>

<template>
  <PopoverRoot>
    <PopoverTrigger class="gsrc__btn" data-testid="guide-source">
      {{ source.label || 'Документация вендора' }}
    </PopoverTrigger>
    <PopoverPortal>
      <PopoverContent
        class="gsrc__pop"
        side="bottom"
        align="start"
        :side-offset="6"
        :collision-padding="12"
        data-testid="guide-source-pop"
      >
        <p v-if="heading" class="gsrc__head">{{ heading }}</p>
        <p v-if="sub" class="gsrc__sub">{{ sub }}</p>
        <blockquote v-if="source.fragment" class="gsrc__quote">
          «{{ source.fragment.replace(/^[«"]+|[»"]+$/g, '') }}»
        </blockquote>
        <a
          v-if="source.url"
          :href="source.url"
          class="link gsrc__link"
          target="_blank"
          rel="noopener noreferrer"
        >
          Открыть документ вендора <ExternalLink :size="12" aria-hidden="true" />
          <span class="sr-only">(откроется в новой вкладке)</span>
        </a>
      </PopoverContent>
    </PopoverPortal>
  </PopoverRoot>
</template>

<style>
.gsrc__btn {
  display: inline;
  padding: 0;
  border: 0;
  background: none;
  color: var(--accent);
  font: inherit;
  font-size: var(--fs-sm);
  line-height: inherit;
  text-decoration: underline;
  text-decoration-thickness: 1px;
  text-underline-offset: 3px;
  cursor: pointer;
}
.gsrc__btn:hover,
.gsrc__btn[data-state='open'] {
  color: var(--accent-hover);
}
.gsrc__pop {
  z-index: 60;
  box-sizing: border-box;
  width: min(24rem, calc(100vw - 1.5rem));
  padding: var(--space-4);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
  background: var(--surface);
  box-shadow: var(--shadow-overlay);
  color: var(--text);
}
.gsrc__head {
  margin: 0;
  font-weight: var(--fw-semibold);
}
.gsrc__sub {
  margin: 0;
  color: var(--text-2);
  font-size: var(--fs-sm);
}
.gsrc__quote {
  margin: var(--space-3) 0;
  padding: var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
  background: var(--surface-2);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
  overflow-wrap: anywhere;
}
.gsrc__link {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  font-size: var(--fs-sm);
}
</style>
