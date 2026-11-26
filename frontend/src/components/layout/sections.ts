// Разделы левой панели («Навигация»). backoffice — раздел бэк-офиса: гостю и покупателю не показывается.
import type { Component } from 'vue'
import {
  BadgePercent,
  Bot,
  ChartColumn,
  Coins,
  GitCompareArrows,
  History,
  Search,
} from 'lucide-vue-next'

export interface Section {
  path: string
  name: string
  label: string
  icon: Component
  backoffice: boolean
}

export const SECTIONS: readonly Section[] = [
  { path: '/search', name: 'search', label: 'Поиск', icon: Search, backoffice: false },
  {
    path: '/compare',
    name: 'compare',
    label: 'До / После',
    icon: GitCompareArrows,
    backoffice: false,
  },
  {
    path: '/analytics',
    name: 'analytics',
    label: 'Аналитика',
    icon: ChartColumn,
    backoffice: true,
  },
  { path: '/searchops', name: 'searchops', label: 'SearchOps', icon: Bot, backoffice: true },
  { path: '/changes', name: 'changes', label: 'Изменения', icon: History, backoffice: true },
  { path: '/costs', name: 'costs', label: 'Экономика', icon: Coins, backoffice: true },
  { path: '/margin', name: 'margin', label: 'Маржа', icon: BadgePercent, backoffice: true },
]
