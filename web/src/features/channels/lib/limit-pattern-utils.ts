import type { LimitPattern } from '../types'

export function validateLimitPatternRegex(regex: string): { valid: boolean; error?: string } {
  // JS RegExp cannot compile Go's (?P<name>...) named-group syntax, so full
  // syntax validation is left to the backend. A pattern without a reset
  // capture group is valid: it cools the key down until the reset cycle (or
  // the fallback minutes) instead of a captured timestamp.
  if (!regex.trim()) {
    return { valid: false, error: 'Regex is required' }
  }
  return { valid: true }
}

// Built-in limit-pattern presets offered in the editor's "Load preset" menu.
export const LIMIT_PATTERN_PRESETS: LimitPattern[] = [
  {
    name: '7-day quota (Chinese)',
    regex: '已达到 \\d+ 天使用上限，(?P<reset>\\d{4}-\\d{2}-\\d{2} \\d{2}:\\d{2}:\\d{2}) 后可继续使用',
    date_layout: '2006-01-02 15:04:05',
    default_minutes: 10,
  },
  {
    // Aliyun DashScope free-token-plan exhaustion; the error carries no reset
    // timestamp, so the key cools down until the next monthly cycle (day 1).
    name: 'Aliyun token-plan quota exhausted',
    regex: 'Your token-plan quota has been exhausted\\.',
    date_layout: '',
    default_minutes: 10,
    reset_cycle: 'monthly:1',
  },
]

export const PREDEFINED_DATE_LAYOUTS = [
  { value: '2006-01-02 15:04:05', label: 'YYYY-MM-DD HH:mm:ss' },
  { value: '2006-01-02T15:04:05', label: 'YYYY-MM-DDTHH:mm:ss' },
  { value: '2006-01-02T15:04:05Z07:00', label: 'RFC3339' },
  { value: '2006/01/02 15:04:05', label: 'YYYY/MM/DD HH:mm:ss' },
  { value: 'custom', label: 'Custom' },
] as const

export type ResetCycleType = 'none' | 'daily' | 'weekly' | 'monthly'

export const resetCycleMaxDay: Record<'weekly' | 'monthly', number> = {
  weekly: 7,
  monthly: 31,
}

export function parseResetCycle(cycle: string | undefined): {
  type: ResetCycleType
  day: number
} {
  if (!cycle) return { type: 'none', day: 1 }
  if (cycle === 'daily') return { type: 'daily', day: 1 }
  const sep = cycle.indexOf(':')
  if (sep === -1) return { type: 'none', day: 1 }
  const type = cycle.slice(0, sep)
  if (type !== 'weekly' && type !== 'monthly') return { type: 'none', day: 1 }
  const day = Number(cycle.slice(sep + 1))
  if (!Number.isInteger(day) || day < 1 || day > resetCycleMaxDay[type]) {
    return { type: 'none', day: 1 }
  }
  return { type, day }
}

export function formatResetCycle(type: ResetCycleType, day: number): string {
  if (type === 'daily') return 'daily'
  if (type === 'weekly' || type === 'monthly') {
    const clamped = Math.min(resetCycleMaxDay[type], Math.max(1, Math.round(day) || 1))
    return `${type}:${clamped}`
  }
  return ''
}
