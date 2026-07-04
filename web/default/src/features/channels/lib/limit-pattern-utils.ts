export function validateLimitPatternRegex(regex: string): { valid: boolean; error?: string } {
  // JS RegExp cannot compile Go's (?P<name>...) named-group syntax, so we only
  // verify the reset group is present. The backend re-validates with Go regexp.
  if (!regex.includes('(?P<reset>')) {
    return { valid: false, error: 'Regex must contain a (?P<reset>...) named capture group' }
  }
  return { valid: true }
}

import type { LimitPattern } from '../types'

// Built-in limit-pattern presets offered in the editor's "Load preset" menu.
export const LIMIT_PATTERN_PRESETS: LimitPattern[] = [
  {
    name: '7-day quota (Chinese)',
    regex: '已达到 \\d+ 天使用上限，(?P<reset>\\d{4}-\\d{2}-\\d{2} \\d{2}:\\d{2}:\\d{2}) 后可继续使用',
    date_layout: '2006-01-02 15:04:05',
    default_minutes: 10,
  },
]

export const PREDEFINED_DATE_LAYOUTS = [
  { value: '2006-01-02 15:04:05', label: 'YYYY-MM-DD HH:mm:ss' },
  { value: '2006-01-02T15:04:05', label: 'YYYY-MM-DDTHH:mm:ss' },
  { value: '2006-01-02T15:04:05Z07:00', label: 'RFC3339' },
  { value: '2006/01/02 15:04:05', label: 'YYYY/MM/DD HH:mm:ss' },
  { value: 'custom', label: 'Custom' },
] as const
