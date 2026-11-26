// Тема PrimeVue поверх Aura: значения — ссылки на CSS-переменные (src/styles/tokens.css), поэтому
// цвета меняются в одном месте, без пересборки темы.
import { definePreset } from '@primeuix/themes'
import Aura from '@primeuix/themes/aura'

const scheme = {
  primary: {
    color: 'var(--accent)',
    contrastColor: 'var(--on-accent)',
    hoverColor: 'var(--accent-hover)',
    activeColor: 'var(--accent-hover)',
  },
  highlight: {
    background: 'var(--accent-soft)',
    focusBackground: 'var(--accent-soft)',
    color: 'var(--accent)',
    focusColor: 'var(--accent)',
  },
  focusRing: { color: 'var(--focus-ring)' },
  formField: {
    background: 'var(--surface)',
    disabledBackground: 'var(--surface-2)',
    filledBackground: 'var(--surface-2)',
    borderColor: 'var(--border-control)',
    hoverBorderColor: 'var(--text-2)',
    focusBorderColor: 'var(--accent)',
    invalidBorderColor: 'var(--danger)',
    color: 'var(--text)',
    disabledColor: 'var(--disabled-text)',
    placeholderColor: 'var(--text-2)',
    iconColor: 'var(--text-2)',
  },
  text: {
    color: 'var(--text)',
    hoverColor: 'var(--text)',
    mutedColor: 'var(--text-2)',
    hoverMutedColor: 'var(--text)',
  },
  content: {
    background: 'var(--surface)',
    hoverBackground: 'var(--surface-2)',
    borderColor: 'var(--border)',
    color: 'var(--text)',
    hoverColor: 'var(--text)',
  },
}

export const ShopPreset = definePreset(Aura, {
  semantic: {
    colorScheme: { light: scheme },
  },
  components: {
    button: {
      root: {
        borderRadius: 'var(--radius-control)',
        paddingX: '1rem',
        paddingY: '0.625rem',
        label: { fontWeight: '600' },
      },
    },
    inputtext: {
      root: {
        borderRadius: 'var(--radius-control)',
        paddingX: '0.875rem',
        paddingY: '0.625rem',
        lg: { fontSize: '1.0625rem', paddingX: '1rem', paddingY: '0.875rem' },
      },
    },
  },
})
