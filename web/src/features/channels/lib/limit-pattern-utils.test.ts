import { describe, expect, test } from 'vitest'

import { validateLimitPatternRegex } from './limit-pattern-utils'

describe('validateLimitPatternRegex', () => {
  test('accepts a regex with reset group', () => {
    expect(validateLimitPatternRegex('limit (?P<reset>.+)').valid).toBe(true)
  })

  test('rejects regex without reset group', () => {
    const result = validateLimitPatternRegex('limit .+')
    expect(result.valid).toBe(false)
    expect(result.error ?? '').toMatch(/reset/)
  })

  test('rejects invalid syntax', () => {
    expect(validateLimitPatternRegex('[').valid).toBe(false)
  })
})
