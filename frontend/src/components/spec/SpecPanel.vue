<script setup lang="ts">
// Панель «Спецификация» справа: позиция — количество — цена, пометка
// происхождения, итог («N позиций · у M цена по запросу · известная часть от …»), «Запросить КП» и
// «Скопировать спецификацию». Пусто — «Добавьте позиции из поиска»; ошибка запроса — позиции сохранены,
// «Повторить». Сумму не считает сервер — фронт складывает только известные цены из выдачи. Гость оставляет контакт для
// ответа (покупатель и сотрудник известны по входу).
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  DialogClose,
  DialogContent,
  DialogOverlay,
  DialogPortal,
  DialogRoot,
  DialogTitle,
} from 'reka-ui'
import { CircleAlert, CircleCheck, Copy, Minus, Plus, Trash2, X } from 'lucide-vue-next'
import SpecCompanions from '@/components/spec/SpecCompanions.vue'
import { useToast } from '@/components/ui/useToast'
import { formatPrice } from '@/lib/format'
import { hasKnownPrice, originLabel, priceModeLabel, specText } from '@/lib/spec'
import { useSpecStore } from '@/stores/spec'
import { useAuthStore } from '@/stores/auth'
import { contactProblem } from '@/lib/auth'

const spec = useSpecStore()
const auth = useAuthStore()
/** Гость: контакт для ответа на КП (почта или телефон) — уходит в комментарий запроса. */
const needContact = computed(() => !auth.signedIn)
const contact = ref('')
const contactTried = ref(false)
const contactErr = computed(() => (contactTried.value ? contactProblem(contact.value) : null))
function requestQuote() {
  if (needContact.value) {
    contactTried.value = true
    if (contactProblem(contact.value)) return
    void spec.sendQuote(`Контакт: ${contact.value.trim()}`)
    return
  }
  void spec.sendQuote()
}
const toast = useToast()
const route = useRoute()
const router = useRouter()

const lineTotal = (l: (typeof spec.lines)[number]) =>
  hasKnownPrice(l) ? formatPrice((l.price_rub as number) * l.quantity) : ''
const sent = computed(() => spec.quoteStatus === 'sent' && spec.lastQuote)

async function copy() {
  const text = specText(spec.lines)
  try {
    await navigator.clipboard.writeText(text)
    toast.add({ tone: 'ok', title: 'Спецификация скопирована' })
  } catch {
    toast.add({
      tone: 'error',
      title: 'Не удалось скопировать',
      detail: 'Буфер обмена недоступен.',
    })
  }
}

function toSearch() {
  spec.open = false
  if (route.path !== '/search') void router.push('/search')
}

function startOver() {
  spec.clear()
  spec.quoteStatus = 'idle'
}
</script>

<template>
  <DialogRoot v-model:open="spec.open">
    <DialogPortal>
      <DialogOverlay class="ui-scrim" />
      <DialogContent class="spec-sheet" :aria-describedby="undefined">
        <header class="spec-sheet__head">
          <DialogTitle class="t-h2">Спецификация</DialogTitle>
          <DialogClose class="btn btn--ghost btn--icon btn--sm" aria-label="Закрыть спецификацию">
            <X :size="16" aria-hidden="true" />
          </DialogClose>
        </header>

        <div v-if="sent" class="spec-sheet__done" role="status">
          <CircleCheck :size="20" aria-hidden="true" />
          <div>
            <p class="t-h2">Запрос КП отправлен</p>
            <p class="t-sm t-muted">
              Номер запроса <span class="t-mono">{{ spec.lastQuote!.id }}</span> ·
              {{ spec.summary.text }}. Менеджер пришлёт коммерческое предложение.
            </p>
            <div class="spec-sheet__done-actions">
              <button type="button" class="btn btn--secondary btn--sm" @click="startOver">
                Новая спецификация
              </button>
              <button
                type="button"
                class="btn btn--ghost btn--sm"
                @click="spec.quoteStatus = 'idle'"
              >
                Вернуться к позициям
              </button>
            </div>
          </div>
        </div>

        <div v-else-if="!spec.lines.length" class="spec-sheet__empty">
          <p class="t-h2">Добавьте позиции из поиска</p>
          <p class="t-sm t-muted">
            Кнопка «В спецификацию» есть у каждой позиции выдачи и в блоке «Дополните решение».
          </p>
          <div>
            <button type="button" class="btn btn--secondary btn--sm" @click="toSearch">
              К поиску
            </button>
          </div>
        </div>

        <template v-else>
          <ul class="spec-lines" aria-label="Позиции спецификации">
            <li v-for="l in spec.lines" :key="l.product_id" class="spec-line">
              <div class="spec-line__main">
                <p class="spec-line__title">{{ l.title }}</p>
                <p class="t-xs t-muted">
                  {{ l.vendor }}<template v-if="l.license_line"> · {{ l.license_line }}</template>
                </p>
                <p class="t-xs">
                  <span class="badge">{{ originLabel(l) }}</span>
                  <span v-if="l.anchor" class="t-muted"> к «{{ l.anchor }}»</span>
                </p>
                <SpecCompanions
                  v-if="l.origin === 'search'"
                  :product-id="l.product_id"
                  :title="l.title"
                />
              </div>
              <div class="spec-line__qty" role="group" :aria-label="`Количество: ${l.title}`">
                <button
                  type="button"
                  class="btn btn--ghost btn--icon btn--sm"
                  aria-label="Меньше"
                  :disabled="l.quantity <= 1"
                  @click="spec.setQty(l.product_id, l.quantity - 1)"
                >
                  <Minus :size="14" aria-hidden="true" />
                </button>
                <input
                  class="spec-line__input t-num"
                  type="number"
                  inputmode="numeric"
                  min="1"
                  :value="l.quantity"
                  :aria-label="`Количество: ${l.title}`"
                  @change="spec.setQty(l.product_id, ($event.target as HTMLInputElement).value)"
                />
                <button
                  type="button"
                  class="btn btn--ghost btn--icon btn--sm"
                  aria-label="Больше"
                  @click="spec.setQty(l.product_id, l.quantity + 1)"
                >
                  <Plus :size="14" aria-hidden="true" />
                </button>
              </div>
              <div class="spec-line__price t-num">
                <span>{{ lineTotal(l) || priceModeLabel(l) }}</span>
                <span v-if="lineTotal(l) && l.quantity > 1" class="t-xs t-muted">
                  {{ priceModeLabel(l) }} × {{ l.quantity }}
                </span>
              </div>
              <button
                type="button"
                class="btn btn--ghost btn--icon btn--sm spec-line__remove"
                :aria-label="`Удалить: ${l.title}`"
                @click="spec.remove(l.product_id)"
              >
                <Trash2 :size="14" aria-hidden="true" />
              </button>
            </li>
          </ul>

          <p class="spec-sheet__total t-num" role="status">{{ spec.summary.text }}</p>

          <div v-if="spec.quoteStatus === 'error'" class="spec-sheet__error" role="alert">
            <CircleAlert :size="16" aria-hidden="true" />
            <span>
              Запрос КП не отправлен: {{ spec.quoteError?.message }}. Позиции сохранены.
            </span>
          </div>

          <div v-if="needContact" class="spec-sheet__contact">
            <label for="spec-contact" class="t-sm">Контакт для ответа — почта или телефон</label>
            <input
              id="spec-contact"
              v-model="contact"
              class="spec-sheet__input"
              autocomplete="email"
              maxlength="120"
              :aria-invalid="contactErr ? true : undefined"
              :aria-describedby="contactErr ? 'spec-contact-err' : undefined"
            />
            <p v-if="contactErr" id="spec-contact-err" class="spec-sheet__field-err t-xs">
              {{ contactErr }}
            </p>
            <p v-else class="t-xs t-muted spec-sheet__hint">
              Стенд демонстрационный — подойдёт вымышленный контакт. Покупателю после входа контакт
              не нужен.
            </p>
          </div>

          <footer class="spec-sheet__actions">
            <button
              type="button"
              class="btn btn--primary"
              :disabled="spec.quoteStatus === 'sending'"
              @click="requestQuote"
            >
              {{
                spec.quoteStatus === 'sending'
                  ? 'Отправляем…'
                  : spec.quoteStatus === 'error'
                    ? 'Повторить запрос КП'
                    : 'Запросить КП'
              }}
            </button>
            <button type="button" class="btn btn--secondary" @click="copy">
              <Copy :size="16" aria-hidden="true" />
              Скопировать спецификацию
            </button>
          </footer>
        </template>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>

<style>
.spec-sheet {
  position: fixed;
  top: 0;
  right: 0;
  bottom: 0;
  z-index: 51;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  width: min(32rem, 100vw);
  overflow-y: auto;
  padding: var(--space-4) var(--gutter) var(--space-6);
  background: var(--surface);
  border-left: var(--border-w) solid var(--border);
  box-shadow: var(--shadow-overlay);
  animation: spec-slide-left var(--dur-panel) var(--ease-standard);
}
.spec-sheet:focus-visible {
  outline: none;
}
@keyframes spec-slide-left {
  from {
    transform: translateX(16px);
    opacity: 0;
  }
  to {
    transform: none;
    opacity: 1;
  }
}
@media (prefers-reduced-motion: reduce) {
  .spec-sheet {
    animation: none;
  }
}
.spec-sheet__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.spec-sheet__empty,
.spec-sheet__done {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-4) 0;
}
.spec-sheet__done {
  flex-direction: row;
  align-items: flex-start;
  gap: var(--space-3);
}
.spec-sheet__done-actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  margin-top: var(--space-3);
}
.spec-lines {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  list-style: none;
  border-top: var(--border-w) solid var(--border);
}
.spec-line {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto auto;
  grid-template-areas:
    'main main remove'
    'qty price price';
  align-items: center;
  gap: var(--space-2) var(--space-3);
  padding: var(--space-3) 0;
  border-bottom: var(--border-w) solid var(--border);
}
.spec-line__main {
  grid-area: main;
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.spec-line__title {
  font-weight: var(--fw-medium);
  overflow-wrap: anywhere;
}
.spec-line__qty {
  grid-area: qty;
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
}
.spec-line__input {
  box-sizing: border-box;
  width: 4.5rem;
  min-height: var(--control-h-sm);
  padding: 0 var(--space-2);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-control);
  background: var(--bg);
  color: var(--text);
  font: inherit;
  text-align: right;
}
.spec-line__price {
  grid-area: price;
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  font-weight: var(--fw-medium);
  white-space: nowrap;
}
.spec-line__remove {
  grid-area: remove;
  align-self: start;
}
.spec-sheet__total {
  font-weight: var(--fw-semibold);
}
.spec-sheet__contact {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}
.spec-sheet__input {
  box-sizing: border-box;
  min-height: var(--control-h);
  padding: 0 var(--space-3);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-control);
  background: var(--bg);
  color: var(--text);
  font: inherit;
  font-size: var(--fs-input);
}
.spec-sheet__input:focus-visible {
  outline: var(--focus-w) solid var(--focus-ring);
  outline-offset: 1px;
}
.spec-sheet__input[aria-invalid='true'] {
  border-color: var(--danger);
}
.spec-sheet__hint {
  margin: 0;
}
.spec-sheet__field-err {
  margin: 0;
  color: var(--danger);
}
.spec-sheet__error {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-control);
  background: var(--danger-bg);
  color: var(--danger);
  font-size: var(--fs-sm);
}
.spec-sheet__error svg {
  flex: none;
  margin-top: 1px;
}
.spec-sheet__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}
</style>
