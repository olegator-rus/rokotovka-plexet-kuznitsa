// Разделы левой панели («Навигация»): «Поиск», «SearchOps», «Доказательства».
// backoffice — раздел бэк-офиса: гостю и покупателю не показывается.
import type { Component } from 'vue'
import { Bot, FlaskConical, Search } from 'lucide-vue-next'

export interface Section {
  path: string
  name: string
  label: string
  icon: Component
  backoffice: boolean
}

export const SECTIONS: readonly Section[] = [
  { path: '/search', name: 'search', label: 'Поиск', icon: Search, backoffice: false },
  { path: '/searchops', name: 'searchops', label: 'SearchOps', icon: Bot, backoffice: true },
  {
    path: '/proofs',
    name: 'proofs',
    label: 'Доказательства',
    icon: FlaskConical,
    backoffice: false,
  },
]
