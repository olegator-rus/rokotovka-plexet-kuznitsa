// Оформление категории на витрине: иконка Lucide и пара токенов плитки (tokens.css, --tile-N).
// Пока у товаров нет фото, плитка с иконкой заменяет картинку.
import type { Component } from 'vue'
import { BookOpen, CookingPot, Dumbbell, Headphones, Puzzle, Shirt } from 'lucide-vue-next'
import type { CategoryId } from '@/lib/catalog'

export interface CategoryVisual {
  icon: Component
  /** CSS-переменные фона и цвета иконки. */
  bg: string
  ink: string
}

const VISUALS: Record<CategoryId, CategoryVisual> = {
  electronics: { icon: Headphones, bg: 'var(--tile-2)', ink: 'var(--tile-2-ink)' },
  home: { icon: CookingPot, bg: 'var(--tile-1)', ink: 'var(--tile-1-ink)' },
  sport: { icon: Dumbbell, bg: 'var(--tile-3)', ink: 'var(--tile-3-ink)' },
  clothes: { icon: Shirt, bg: 'var(--tile-4)', ink: 'var(--tile-4-ink)' },
  books: { icon: BookOpen, bg: 'var(--tile-5)', ink: 'var(--tile-5-ink)' },
  kids: { icon: Puzzle, bg: 'var(--tile-6)', ink: 'var(--tile-6-ink)' },
}

export function categoryVisual(id: CategoryId): CategoryVisual {
  return VISUALS[id]
}
