<script setup lang="ts">
// Источник у строки о функции (SPEC §8): маленькая подпись «источник», по нажатию — фрагмент со страницы вендора и
// ссылка на неё. Даты не пишем.
import { PopoverContent, PopoverPortal, PopoverRoot, PopoverTrigger } from 'reka-ui'
import { ExternalLink } from 'lucide-vue-next'
import type { GuideClaim } from '@/api/typesGuide'
import { claimMark, sourceName } from '@/lib/guide'

const props = defineProps<{ claim: GuideClaim; label: string }>()
const name = () => sourceName(props.claim.source)
</script>

<template>
  <PopoverRoot v-if="claim.source">
    <PopoverTrigger
      class="gsrc__btn"
      :aria-label="`Источник: ${label} — ${name()}`"
      data-testid="guide-source"
    >
      <span aria-hidden="true">источник</span>
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
        <p class="t-xs t-muted gsrc__head">
          {{ name() }}
          <span v-if="claimMark(claim)" class="gsrc__warn">· {{ claimMark(claim) }}</span>
        </p>
        <blockquote v-if="claim.source.fragment" class="gsrc__quote">
          {{ claim.source.fragment }}
        </blockquote>
        <p v-if="claim.text" class="t-sm gsrc__text">{{ claim.text }}</p>
        <a
          v-if="claim.source.url"
          :href="claim.source.url"
          class="link t-sm gsrc__link"
          target="_blank"
          rel="noopener noreferrer"
        >
          Открыть страницу <ExternalLink :size="12" aria-hidden="true" />
          <span class="sr-only">(откроется в новой вкладке)</span>
        </a>
      </PopoverContent>
    </PopoverPortal>
  </PopoverRoot>
</template>

<style>
.gsrc__btn {
  display: inline;
  margin-left: var(--space-1);
  padding: 0;
  border: 0;
  background: none;
  color: var(--text-3);
  font: inherit;
  font-size: var(--fs-xs);
  line-height: inherit;
  text-decoration: underline dotted;
  text-underline-offset: 3px;
  white-space: nowrap;
  cursor: pointer;
}
.gsrc__btn:hover,
.gsrc__btn[data-state='open'] {
  color: var(--text);
  text-decoration-style: solid;
}
.gsrc__pop {
  z-index: 60;
  box-sizing: border-box;
  width: min(22rem, calc(100vw - 1.5rem));
  padding: var(--space-3);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-control);
  background: var(--surface);
  box-shadow: var(--shadow-overlay);
  color: var(--text);
}
.gsrc__head {
  margin: 0 0 var(--space-2);
}
.gsrc__warn {
  color: var(--status-pending);
}
.gsrc__quote {
  margin: 0 0 var(--space-2);
  padding-left: var(--space-3);
  border-left: 3px solid var(--border-control);
  font-size: var(--fs-sm);
  line-height: var(--lh-sm);
  overflow-wrap: anywhere;
}
.gsrc__text {
  margin: 0 0 var(--space-2);
}
.gsrc__link {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
}
</style>
