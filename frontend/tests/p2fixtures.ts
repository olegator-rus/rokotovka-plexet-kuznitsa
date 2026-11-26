// Фикстуры контура коммерции (пока бэкенд-треки не отдают поля).
import type { Companion } from '../src/api/typesP2.ts'

export function companion(id: string, over: Partial<Companion> = {}): Companion {
  return {
    product_id: id,
    family_id: `${id}-fam`,
    title: id,
    vendor: 'Microsoft',
    category: 'Лицензии доступа CAL',
    license_line: 'Бессрочная · 1 пользователь',
    price_rub: 3000,
    price_mode: 'fixed',
    class: 'conditional_required',
    compatibility: 'verified',
    reason: 'Нужны для доступа пользователей к серверу',
    conditions: ['access_license_required', 'no_existing_equivalent_license'],
    evidence_level: 'E4',
    source: 'vendor_documentation',
    source_url: 'https://example.com/licensing',
    suggested_quantity: 50,
    label: 'Проверьте необходимость',
    upgrade_adds: [],
    ...over,
  }
}
