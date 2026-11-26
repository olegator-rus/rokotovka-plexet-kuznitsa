<script setup lang="ts">
// «Дополните решение» (контура фида §5): под топом выдачи, не над ней. По решению
// владельца блок выделен как одна карточка «Рекомендуем»; позиции внутри равны между собой: у каждой
// подпись класса, бейдж уровня основания (A — по документу вендора, B — из каталога, C — проверьте
// совместимость), строка «почему» и источник; у уровня A — ссылка на документ в новой вкладке. Заголовок — из
// `companions.title`. Ничего не выбрано заранее. Пусто — компонент не рисуется вовсе (решает страница).
import { Check, ExternalLink, FileText, Plus, ShieldQuestion, Sparkles } from 'lucide-vue-next'
import VendorLogo from '@/components/common/VendorLogo.vue'
import type { Companion } from '@/api/typesP2'
import CompanionSource from '@/components/search/CompanionSource.vue'
import PromoLine from '@/components/results/PromoLine.vue'
import {
  BLOCK_TITLE,
  classLabel,
  companionBadges,
  evidenceView,
  reasonText,
} from '@/lib/companions'
import { priceModeLabel } from '@/lib/spec'
import { useSpecStore } from '@/stores/spec'

// title — заголовок из ответа фида (§16.7): тогда якорь отдельной строкой под ним. Без title (справочник
// связей mock) — прежний заголовок «Дополните решение к <якорь>».
defineProps<{ items: Companion[]; anchor: string; title?: string }>()
const emit = defineEmits<{
  open: [item: Companion, index: number]
  add: [item: Companion, index: number]
  search: [text: string]
}>()

const spec = useSpecStore()
</script>

<template>
  <section class="cmp" aria-labelledby="cmp-title">
    <header class="cmp__head">
      <p class="cmp__ribbon t-xs"><Sparkles :size="14" aria-hidden="true" /> Рекомендуем</p>
      <template v-if="title">
        <h2 id="cmp-title" class="t-h2">{{ title }}</h2>
        <p v-if="anchor" class="cmp__anchor t-sm">к {{ anchor }}</p>
      </template>
      <h2 v-else id="cmp-title" class="t-h2">{{ BLOCK_TITLE }} к {{ anchor }}</h2>
    </header>
    <ul class="cmp__list">
      <li v-for="(c, i) in items" :key="c.product_id">
        <div
          class="cmp-card"
          role="group"
          :aria-label="c.title"
          data-testid="companion"
          :data-evidence="evidenceView(c)?.level ?? ''"
        >
          <VendorLogo :vendor="c.vendor" class="cmp-card__logo" />
          <div class="cmp-card__main">
            <p class="cmp-card__class t-xs">
              <span class="badge">{{ classLabel(c) }}</span>
              <span
                v-if="evidenceView(c)"
                :class="['cmp-ev', `cmp-ev--${evidenceView(c)?.level}`]"
                data-testid="evidence-badge"
                :title="`Уровень основания ${evidenceView(c)?.level}`"
              >
                <span class="cmp-ev__level" aria-hidden="true">{{ evidenceView(c)?.level }}</span>
                <span class="sr-only">Основание:</span>
                <span
                  class="cmp-ev__tag"
                  :data-testid="evidenceView(c)?.reasonInTag ? 'companion-reason' : undefined"
                  >{{ evidenceView(c)?.tag }}</span
                >
              </span>
              <span v-if="c.category" class="t-muted">{{ c.category }}</span>
            </p>
            <h3 class="cmp-card__title">
              <button type="button" class="cmp-card__open" @click="emit('open', c, i)">
                {{ c.title }}
              </button>
            </h3>
            <p class="t-sm t-muted">
              {{ c.vendor }}<template v-if="c.license_line"> · {{ c.license_line }}</template>
            </p>
            <p v-if="companionBadges(c).length" class="cmp-card__badges">
              <span
                v-for="b in companionBadges(c)"
                :key="b.text"
                :class="['badge', { 'badge--tone cmp-card__discount': b.discount }]"
                data-testid="companion-badge"
                >{{ b.text }}</span
              >
            </p>
            <p
              v-if="reasonText(c) && !evidenceView(c)?.reasonInTag"
              class="t-sm cmp-card__why"
              data-testid="companion-reason"
            >
              {{ reasonText(c) }}
            </p>
            <PromoLine :promos="c.promos" />
            <p
              v-if="c.compatibility === 'unknown' && evidenceView(c)?.level !== 'C'"
              class="t-xs cmp-card__unknown"
            >
              <ShieldQuestion :size="12" aria-hidden="true" />
              Совместимость не подтверждена — уточните у менеджера
            </p>
            <p class="cmp-card__foot">
              <a
                v-if="evidenceView(c)?.docUrl"
                class="link t-xs cmp-card__doc"
                :href="evidenceView(c)?.docUrl"
                target="_blank"
                rel="noopener noreferrer"
                data-testid="evidence-doc"
              >
                <FileText :size="12" aria-hidden="true" />
                Открыть документ
                <ExternalLink :size="11" aria-hidden="true" />
                <span class="sr-only"
                  >: {{ evidenceView(c)?.docTitle }} (откроется в новой вкладке)</span
                >
              </a>
              <CompanionSource v-else :source="c.source" :url="c.source_url" />
              <button
                v-if="c.category"
                type="button"
                class="link t-xs"
                @click="emit('search', c.category)"
              >
                все: {{ c.category.toLowerCase() }}
              </button>
            </p>
          </div>
          <p class="cmp-card__price t-num">{{ priceModeLabel(c) }}</p>
          <div class="cmp-card__actions">
            <span v-if="spec.has(c.product_id)" class="badge badge--strong">
              <Check :size="12" aria-hidden="true" /> В спецификации
            </span>
            <button
              v-else
              type="button"
              class="btn btn--primary btn--sm"
              :aria-label="`В спецификацию: ${c.title}`"
              @click="emit('add', c, i)"
            >
              <Plus :size="14" aria-hidden="true" />
              В спецификацию
            </button>
          </div>
        </div>
      </li>
    </ul>
  </section>
</template>

<style scoped>
/* Одна выделенная карточка: красная рамка и подложка шапки, позиции — строки внутри на белом. */
.cmp {
  overflow: hidden;
  margin: var(--space-2) 0;
  border: var(--focus-w) solid var(--accent);
  border-radius: var(--radius-card);
  background: var(--surface);
}
.cmp__head {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
  background: var(--accent-soft);
}
.cmp__ribbon {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  margin: 0;
  padding: var(--space-1) var(--space-3);
  border-radius: var(--radius-pill);
  background: var(--accent);
  color: var(--on-accent);
  font-weight: var(--fw-bold);
}
.cmp__head h2 {
  margin: 0;
  overflow-wrap: anywhere;
}
.cmp__anchor {
  margin: calc(-1 * var(--space-1)) 0 0;
  color: var(--text-2);
  overflow-wrap: anywhere;
}
/* Уровень основания — ненавязчиво: буква в кружке и подпись мелким текстом, без заливки цветом. */
.cmp-ev {
  display: inline-flex;
  align-items: center;
  min-width: 0;
  gap: var(--space-1);
  color: var(--text-2);
  white-space: nowrap;
}
.cmp-ev__tag {
  white-space: normal;
  overflow-wrap: anywhere;
}
.cmp-card__badges {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1);
  margin: 2px 0;
}
.cmp-card__discount {
  --tone: var(--status-accepted);
  --tone-bg: var(--status-accepted-bg);
}
.cmp-ev__level {
  flex: none;
  display: inline-grid;
  place-items: center;
  width: 1.25rem;
  height: 1.25rem;
  border: var(--border-w) solid var(--border-control);
  border-radius: 50%;
  font-size: var(--fs-xs);
  line-height: 1;
  font-weight: var(--fw-bold);
}
.cmp-ev--A .cmp-ev__tag {
  white-space: normal;
  overflow-wrap: anywhere;
}
.cmp-card__badges {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1);
  margin: 2px 0;
}
.cmp-card__discount {
  --tone: var(--status-accepted);
  --tone-bg: var(--status-accepted-bg);
}
.cmp-ev__level {
  border-color: var(--status-accepted);
  color: var(--status-accepted);
}
.cmp-ev--C .cmp-ev__tag {
  white-space: normal;
  overflow-wrap: anywhere;
}
.cmp-card__badges {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1);
  margin: 2px 0;
}
.cmp-card__discount {
  --tone: var(--status-accepted);
  --tone-bg: var(--status-accepted-bg);
}
.cmp-ev__level {
  border-style: dashed;
}
.cmp-card__doc {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
}
.cmp__list {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  list-style: none;
}
.cmp__list li + li {
  border-top: var(--border-w) solid var(--border);
}
.cmp-card {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  grid-template-areas:
    'logo main main'
    'logo price actions';
  align-items: center;
  gap: var(--space-2) var(--space-3);
  padding: var(--space-3) var(--space-4);
}
.cmp-card__logo {
  grid-area: logo;
  align-self: start;
}
.cmp-card__main {
  grid-area: main;
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.cmp-card__class {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
  margin-bottom: 2px;
}
.cmp-card__title {
  margin: 0;
  font-size: var(--fs-body);
  line-height: var(--lh-body);
  font-weight: var(--fw-semibold);
}
.cmp-card__open {
  padding: 0;
  border: 0;
  background: none;
  color: var(--text);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.cmp-card__open:hover {
  text-decoration: underline;
  text-decoration-color: var(--text-3);
  text-underline-offset: 3px;
}
.cmp-card__why {
  color: var(--text);
}
.cmp-card__unknown {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  color: var(--text-2);
}
.cmp-card__foot {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1) var(--space-3);
  margin-top: 2px;
}
.cmp-card__price {
  grid-area: price;
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
  font-weight: var(--fw-bold);
}
.cmp-card__actions {
  grid-area: actions;
  justify-self: end;
}
@media (min-width: 640px) {
  .cmp-card {
    grid-template-columns: auto minmax(0, 1fr) 8.5rem auto;
    grid-template-areas: 'logo main price actions';
  }
  .cmp-card__price {
    text-align: right;
  }
}
</style>
