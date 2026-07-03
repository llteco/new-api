export function validateLimitPatternRegex(regex: string): { valid: boolean; error?: string } {
  // ponytail: JS RegExp cannot compile Go's (?P<name>...) named-group syntax,
  // so the substring check is the real validator; new RegExp is best-effort only.
  if (!regex.includes('(?P<reset>')) {
    return { valid: false, error: 'Regex must contain a (?P<reset>...) named capture group' }
  }
  try {
    new RegExp(regex)
  } catch {
    // Tolerate Go-specific syntax that JS RegExp cannot parse (e.g. (?P<reset>...)).
  }
  return { valid: true }
}

export const PREDEFINED_DATE_LAYOUTS = [
  { value: '2006-01-02 15:04:05', label: 'YYYY-MM-DD HH:mm:ss' },
  { value: '2006-01-02T15:04:05', label: 'YYYY-MM-DDTHH:mm:ss' },
  { value: '2006-01-02T15:04:05Z07:00', label: 'RFC3339' },
  { value: '2006/01/02 15:04:05', label: 'YYYY/MM/DD HH:mm:ss' },
  { value: 'custom', label: 'Custom' },
] as const
