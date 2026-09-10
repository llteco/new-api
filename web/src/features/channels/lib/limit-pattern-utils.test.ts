import { describe, expect, test } from 'vitest'

import {
  LIMIT_PATTERN_PRESETS,
  formatResetCycle,
  parseResetCycle,
  validateLimitPatternRegex,
} from './limit-pattern-utils'

describe('validateLimitPatternRegex', () => {
  test('accepts a regex with reset group', () => {
    expect(validateLimitPatternRegex('limit (?P<reset>.+)').valid).toBe(true)
  })

  test('accepts a regex without reset group', () => {
    // e.g. the Aliyun quota error carries no reset timestamp
    const result = validateLimitPatternRegex(
      'Your token-plan quota has been exhausted\\.'
    )
    expect(result.valid).toBe(true)
  })

  test('rejects empty regex', () => {
    const result = validateLimitPatternRegex('   ')
    expect(result.valid).toBe(false)
    expect(result.error ?? '').toMatch(/required/i)
  })
})

describe('parseResetCycle / formatResetCycle', () => {
  test('parses supported cycle specs', () => {
    expect(parseResetCycle(undefined)).toEqual({ type: 'none', day: 1 })
    expect(parseResetCycle('')).toEqual({ type: 'none', day: 1 })
    expect(parseResetCycle('daily')).toEqual({ type: 'daily', day: 1 })
    expect(parseResetCycle('weekly:3')).toEqual({ type: 'weekly', day: 3 })
    expect(parseResetCycle('monthly:1')).toEqual({ type: 'monthly', day: 1 })
  })

  test('rejects malformed cycle specs', () => {
    expect(parseResetCycle('garbage').type).toBe('none')
    expect(parseResetCycle('weekly:0').type).toBe('none')
    expect(parseResetCycle('weekly:8').type).toBe('none')
    expect(parseResetCycle('monthly:32').type).toBe('none')
    expect(parseResetCycle('monthly:x').type).toBe('none')
  })

  test('formats and round-trips cycle specs', () => {
    expect(formatResetCycle('none', 5)).toBe('')
    expect(formatResetCycle('daily', 1)).toBe('daily')
    expect(formatResetCycle('weekly', 4)).toBe('weekly:4')
    expect(formatResetCycle('monthly', 15)).toBe('monthly:15')
    // out-of-range days are clamped instead of producing invalid specs
    expect(formatResetCycle('weekly', 99)).toBe('weekly:7')
    expect(formatResetCycle('monthly', 0)).toBe('monthly:1')
    expect(parseResetCycle(formatResetCycle('monthly', 31))).toEqual({
      type: 'monthly',
      day: 31,
    })
  })
})

describe('LIMIT_PATTERN_PRESETS', () => {
  test('aliyun preset cools down by monthly cycle without reset capture', () => {
    const preset = LIMIT_PATTERN_PRESETS.find(
      (pattern) => pattern.name === 'Aliyun token-plan quota exhausted'
    )
    expect(preset).toBeDefined()
    expect(preset?.regex).not.toContain('(?P<reset>')
    expect(preset?.date_layout).toBe('')
    expect(preset?.reset_cycle).toBe('monthly:1')
    expect(validateLimitPatternRegex(preset?.regex ?? '').valid).toBe(true)
  })
})
