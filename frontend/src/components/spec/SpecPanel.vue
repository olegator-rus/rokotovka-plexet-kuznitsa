<script setup lang="ts">
// Панель «Спецификация» справа: позиция — количество — цена, пометка
// происхождения, итог («N позиций · у M цена по запросу · известная часть от …»), «Запросить КП» и
// «Скопировать спецификацию». Пусто — «Добавьте позиции из поиска»; ошибка запроса — позиции сохранены,
// «Повторить». Сумму не считает сервер — фронт складывает только известные цены из выдачи. Гость оставляет контакт для
// ответа (покупатель и сотрудник известны по входу).
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  DialogClose,
  DialogContent,
  DialogOverlay,
  DialogPortal,
  DialogRoot,
  DialogTitle,
} from 'reka-ui'
import {
  CircleAlert,
  CircleCheck,
  ClipboardList,
  Copy,
  Minus,
  Plus,
  Trash2,
  X,
} from 'lucide-vue-next'
import VendorLogo from '@/components/common/VendorLogo.vue'
import SpecCompanions from '@/components/spec/SpecCompanions.vue'
import { useToast } from '@/components/ui/useToast'
import { formatPrice } from '@/lib/format'
import { anchorOf, hasKnownPrice, originLabel, priceModeLabel, specText } from '@/lib/spec'
import { purchaseNote } from '@/lib/procurement'
import { useSpecStore } from '@/stores/spec'
import { useAuthStore } from '@/stores/auth'
import { contactProblem } from '@/lib/auth'

const spec = useSpecStore()
const auth = useAuthStore()
/** Гость: контакт для ответа на КП (почта или телефон) — уходит в комментарий запроса. */
const needContact = computed(() => !auth.signedIn)
const contact = ref('')
const contactTried = ref(false)
const contactInput = ref<HTMLInputElement | null>(null)
const contactErr = computed(() => (contactTried.value ? contactProblem(contact.value) : null))
function requestQuote() {
  if (needContact.value) {
    contactTried.value = true
    if (contactProblem(contact.value)) {
      // Ошибка видна и с клавиатуры: фокус — в поле контакта.
      contactInput.value?.focus()
      return
    }
    void spec.sendQuote(`Контакт: ${contact.value.trim()}`)
    return
  }
  void spec.sendQuote()
}
const toast = useToast()
// На узком экране тост встаёт поверх первой строки панели: открытая спецификация уже показывает добавленное.
watch(
  () => spec.open,
  (open) => {
    if (open) toast.dismissAll()
  },
)
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
          <DialogTitle class="spec-sheet__title">Спецификация</DialogTitle>
          <span v-if="spec.count && !sent" class="spec-sheet__count t-num" aria-hidden="true">
            {{ spec.count }}
          </span>
          <DialogClose
            class="btn btn--secondary btn--icon spec-sheet__close"
            aria-label="Закрыть спецификацию"
          >
            <X :size="18" aria-hidden="true" />
          </DialogClose>
        </header>

        <div v-if="sent" class="spec-sheet__done" role="status">
          <span class="spec-sheet__ill spec-sheet__ill--ok" aria-hidden="true">
            <CircleCheck :size="32" />
          </span>
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
          <span class="spec-sheet__ill" aria-hidden="true"><ClipboardList :size="32" /></span>
          <p class="t-h2">Добавьте позиции из поиска</p>
          <p class="t-sm t-muted">
            Кнопка «В спецификацию» есть у каждой позиции выдачи и в блоке «Дополните решение».
          </p>
          <div>
            <button type="button" class="btn btn--primary" @click="toSearch">К поиску</button>
          </div>
        </div>

        <template v-else>
          <div class="spec-sheet__body">
            <ul class="spec-lines" aria-label="Позиции спецификации">
              <li v-for="l in spec.lines" :key="l.product_id" class="spec-line">
                <div class="spec-line__top">
                  <VendorLogo :vendor="l.vendor" />
                  <div class="spec-line__main">
                    <p class="spec-line__title">{{ l.title }}</p>
                    <p class="t-xs t-muted">
                      {{ l.vendor
                      }}<template v-if="l.license_line"> · {{ l.license_line }}</template>
                    </p>
                    <p class="t-xs spec-line__origin">
                      <span class="badge">{{ originLabel(l) }}</span>
                      <span v-if="anchorOf(l)" class="t-muted"> к «{{ anchorOf(l) }}»</span>
                    </p>
                    <p v-if="l.purchase" class="t-xs t-muted" data-testid="spec-purchase">
                      {{ purchaseNote(l.purchase) }}
                    </p>
                  </div>
                  <button
                    type="button"
                    class="btn btn--ghost btn--icon btn--sm spec-line__remove"
                    :aria-label="`Удалить: ${l.title}`"
                    @click="spec.remove(l.product_id)"
                  >
                    <Trash2 :size="16" aria-hidden="true" />
                  </button>
                </div>
                <div class="spec-line__bottom">
                  <div class="spec-line__qty" role="group" :aria-label="`Количество: ${l.title}`">
                    <button
                      type="button"
                      class="spec-line__step"
                      aria-label="Меньше"
                      :disabled="l.quantity <= 1"
                      @click="spec.setQty(l.product_id, l.quantity - 1)"
                    >
                      <Minus :size="16" aria-hidden="true" />
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
                      class="spec-line__step"
                      aria-label="Больше"
                      @click="spec.setQty(l.product_id, l.quantity + 1)"
                    >
                      <Plus :size="16" aria-hidden="true" />
                    </button>
                  </div>
                  <div class="spec-line__price t-num">
                    <span class="spec-line__sum">{{ lineTotal(l) || priceModeLabel(l) }}</span>
                    <span v-if="lineTotal(l) && l.quantity > 1" class="t-xs t-muted">
                      {{ priceModeLabel(l) }} × {{ l.quantity }}
                    </span>
                  </div>
                </div>
                <SpecCompanions
                  v-if="l.origin === 'search'"
                  :product-id="l.product_id"
                  :title="l.title"
                />
              </li>
            </ul>
            <button type="button" class="btn btn--ghost btn--sm spec-sheet__copy" @click="copy">
              <Copy :size="16" aria-hidden="true" />
              Скопировать спецификацию
            </button>
          </div>

          <footer class="spec-sheet__foot">
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
                ref="contactInput"
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
                Стенд демонстрационный — подойдёт вымышленный контакт. Покупателю после входа
                контакт не нужен.
              </p>
            </div>

            <button
              type="button"
              class="btn btn--primary spec-sheet__send"
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
  width: min(34rem, 100vw);
  overflow: hidden;
  background: var(--surface-2);
  box-shadow: var(--shadow-overlay);
  animation: spec-slide-left var(--dur-panel) var(--ease-standard);
}
@media (min-width: 640px) {
  .spec-sheet {
    border-radius: var(--radius-panel) 0 0 var(--radius-panel);
  }
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
  flex: none;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
  border-bottom: var(--border-w) solid var(--border);
  background: var(--surface);
}
.spec-sheet__title {
  margin: 0;
  font-size: var(--fs-h1);
  line-height: var(--lh-h1);
  font-weight: var(--fw-bold);
}
.spec-sheet__count {
  min-width: 1.5rem;
  padding: 0 var(--space-2);
  box-sizing: border-box;
  border-radius: var(--radius-pill);
  background: var(--accent);
  color: var(--on-accent);
  font-size: var(--fs-xs);
  line-height: 1.5rem;
  font-weight: var(--fw-bold);
  text-align: center;
}
.spec-sheet__close {
  margin-left: auto;
}
/* Пусто и «отправлено» — по центру, с крупным значком */
.spec-sheet__empty,
.spec-sheet__done {
  display: flex;
  flex: 1;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--space-3);
  padding: var(--space-8) var(--space-6);
  overflow-y: auto;
  text-align: center;
}
.spec-sheet__ill {
  display: grid;
  flex: none;
  place-items: center;
  width: 4.5rem;
  height: 4.5rem;
  border-radius: var(--radius-pill);
  background: var(--accent-soft);
  color: var(--accent);
}
.spec-sheet__ill--ok {
  background: var(--status-accepted-bg);
  color: var(--status-accepted);
}
.spec-sheet__done-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: var(--space-2);
  margin-top: var(--space-4);
}

.spec-sheet__body {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-2);
  min-height: 7rem;
  padding: var(--space-3);
  overflow-y: auto;
  overscroll-behavior: contain;
}
.spec-lines {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.spec-line {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
}
.spec-line__top {
  display: flex;
  align-items: flex-start;
  gap: var(--space-3);
}
.spec-line__main {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 0;
}
.spec-line__main p {
  margin: 0;
}
.spec-line__title {
  font-weight: var(--fw-semibold);
  overflow-wrap: anywhere;
}
.spec-line__remove {
  flex: none;
  color: var(--text-3);
}
.spec-line__remove:hover {
  color: var(--danger);
}
.spec-line__bottom {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
}
.spec-line__qty {
  display: inline-flex;
  align-items: center;
  padding: 2px;
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-pill);
}
.spec-line__qty:focus-within {
  border-color: var(--focus-ring);
}
.spec-line__step {
  display: grid;
  place-items: center;
  width: var(--control-h-sm);
  height: var(--control-h-sm);
  padding: 0;
  border: 0;
  border-radius: var(--radius-pill);
  background: var(--surface-2);
  cursor: pointer;
}
.spec-line__step:hover:not(:disabled) {
  background: var(--accent);
  color: var(--on-accent);
}
.spec-line__step:disabled {
  color: var(--text-3);
  opacity: 0.45;
  cursor: not-allowed;
}
.spec-line__input {
  box-sizing: border-box;
  width: 4rem;
  height: var(--control-h-sm);
  padding: 0 var(--space-1);
  border: 0;
  background: none;
  color: var(--text);
  font: inherit;
  font-weight: var(--fw-semibold);
  text-align: center;
  appearance: textfield;
}
.spec-line__input::-webkit-outer-spin-button,
.spec-line__input::-webkit-inner-spin-button {
  margin: 0;
  appearance: none;
}
.spec-line__price {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  min-width: 0;
  text-align: right;
}
.spec-line__sum {
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
  font-weight: var(--fw-bold);
}
.spec-sheet__copy {
  flex: none;
  align-self: center;
}

/* Низ панели всегда на виду: итог, контакт, отправка. */
.spec-sheet__foot {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  max-height: 60%;
  padding: var(--space-4);
  overflow-y: auto;
  border-top: var(--border-w) solid var(--border);
  background: var(--surface);
}
.spec-sheet__total {
  margin: 0;
  font-size: var(--fs-h2);
  line-height: var(--lh-h2);
  font-weight: var(--fw-bold);
  /* t-num не переносит строку, а итог длинный («… · известная часть от 119 990 ₽», 11-значная сумма). */
  white-space: normal;
  overflow-wrap: anywhere;
}
.spec-sheet__contact {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}
.spec-sheet__input {
  box-sizing: border-box;
  min-height: var(--touch-min);
  padding: 0 var(--space-4);
  border: var(--border-w) solid var(--border-control);
  border-radius: var(--radius-pill);
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
.spec-sheet__send {
  flex: none;
  width: 100%;
  min-height: var(--touch-min);
  font-size: var(--fs-input);
}
</style>
