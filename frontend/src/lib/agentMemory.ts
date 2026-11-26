// Память агента («Экран «Память»»): подписи решений, статусов «не предлагать»,
// эффекта эпизода и текущего значения условия возврата. Без DOM: tests/agentRuns.test.ts.
import type { MemoryEpisode, MemorySuppress } from '@/api/typesAgent'
import { formatDateTime, formatInt } from './format.ts'

export const DECISION_LABELS: Record<string, { label: string; tone: string }> = {
  accepted: { label: 'принято', tone: 'status-accepted' },
  rejected: { label: 'отклонено', tone: 'status-rejected' },
  rolled_back: { label: 'откачено', tone: 'status-rolled-back' },
  suppress_removed: { label: 'снято «не предлагать»', tone: 'status-pending' },
}

export const SUPPRESS_STATUS: Record<string, string> = {
  active: 'действует',
  lifted: 'условие наступило — агент вернулся',
  removed: 'снято человеком',
}

export const EFFECT_LABELS: Record<string, { label: string; tone: string }> = {
  helped: { label: 'помогло', tone: 'status-accepted' },
  no_effect: { label: 'без эффекта', tone: 'status-rolled-back' },
  harmed: { label: 'навредило', tone: 'status-rejected' },
}

const OUTCOMES: Record<string, string> = {
  proposal: 'правка в очереди',
  ticket: 'задача владельцу',
  closed: 'правкой не решается',
  rejected: 'гипотезы отклонены',
  skipped: 'пропущено по памяти',
  draft: 'черновик (только чтение)',
  unfinished: 'не закончено',
  error: 'ошибка',
  helped: 'проверено: помогло',
  no_effect: 'проверено: без эффекта',
  harmed: 'проверено: навредило — предложен откат',
}

export function episodeOutcome(e: Pick<MemoryEpisode, 'outcome' | 'cause_label'>): string {
  const o = OUTCOMES[e.outcome] ?? e.outcome
  return e.cause_label && !o.includes(e.cause_label) ? `${o} · ${e.cause_label}` : o
}

/** Текущее значение условия возврата: «неудачных поисков 3 из 6 нужных · проверено 27.05, 03:00». */
export function conditionNow(s: MemorySuppress): string {
  if (s.status === 'removed') {
    return `снято${s.removed_reason ? `: «${s.removed_reason}»` : ''}${s.removed_at ? ` · ${formatDateTime(s.removed_at)}` : ''}`
  }
  if (s.status === 'lifted') return s.lifted_what || 'условие наступило'
  const parts: string[] = []
  if (s.return_failures > 0)
    parts.push(
      `неудачных поисков ${formatInt(s.current_failures ?? 0)} из ${formatInt(s.return_failures)} нужных`,
    )
  if (s.checked_at) parts.push(`проверено ${formatDateTime(s.checked_at)}`)
  else parts.push('агент ещё не проверял')
  return parts.join(' · ')
}
