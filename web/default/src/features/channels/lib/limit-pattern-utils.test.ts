import { test } from 'node:test'
import assert from 'node:assert/strict'
import { validateLimitPatternRegex } from './limit-pattern-utils'

test('validateLimitPatternRegex accepts a regex with reset group', () => {
  assert.equal(validateLimitPatternRegex('limit (?P<reset>.+)').valid, true)
})

test('validateLimitPatternRegex rejects regex without reset group', () => {
  const result = validateLimitPatternRegex('limit .+')
  assert.equal(result.valid, false)
  assert.match(result.error ?? '', /reset/)
})

test('validateLimitPatternRegex rejects invalid syntax', () => {
  assert.equal(validateLimitPatternRegex('[').valid, false)
})
