/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { describe, expect, test } from 'vitest'

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  applyMultiKeyUpdateFields,
  type ChannelFormValues,
} from '../channel-form'

function formValues(overrides: Partial<ChannelFormValues>): ChannelFormValues {
  return { ...CHANNEL_FORM_DEFAULT_VALUES, ...overrides }
}

describe('applyMultiKeyUpdateFields', () => {
  test('switches a single-key channel to multi-key with appended keys', () => {
    const payload = applyMultiKeyUpdateFields(
      { id: 1, models: 'gpt-4o' },
      formValues({
        is_multi_key: true,
        key: 'sk-new-1\nsk-new-2',
        key_mode: 'append',
        multi_key_type: 'polling',
      }),
      { currentIsMultiKey: false, canEditSensitive: true }
    )

    expect(payload.is_multi_key).toBe(true)
    expect(payload.key_mode).toBe('append')
    expect(payload.multi_key_mode).toBe('polling')
  })

  test('switches to multi-key without a key and keeps the default strategy', () => {
    const payload = applyMultiKeyUpdateFields(
      { id: 1 },
      formValues({ is_multi_key: true }),
      { currentIsMultiKey: false, canEditSensitive: true }
    )

    expect(payload.is_multi_key).toBe(true)
    expect(payload).not.toHaveProperty('key_mode')
    expect(payload.multi_key_mode).toBe('random')
  })

  test('switches a multi-key channel back to single-key without multi-key fields', () => {
    const payload = applyMultiKeyUpdateFields(
      { id: 1 },
      formValues({ is_multi_key: false, multi_key_type: 'polling' }),
      { currentIsMultiKey: true, canEditSensitive: true }
    )

    expect(payload.is_multi_key).toBe(false)
    expect(payload).not.toHaveProperty('key_mode')
    expect(payload).not.toHaveProperty('multi_key_mode')
  })

  test('editing an existing multi-key channel sends the strategy but not a conversion', () => {
    const payload = applyMultiKeyUpdateFields(
      { id: 1 },
      formValues({
        is_multi_key: true,
        key: 'sk-new',
        key_mode: 'replace',
        multi_key_type: 'polling',
      }),
      { currentIsMultiKey: true, canEditSensitive: true }
    )

    expect(payload).not.toHaveProperty('is_multi_key')
    expect(payload.key_mode).toBe('replace')
    expect(payload.multi_key_mode).toBe('polling')
  })

  test('omits conversion and strategy fields without sensitive permission', () => {
    const payload = applyMultiKeyUpdateFields(
      { id: 1 },
      formValues({
        is_multi_key: true,
        key: 'sk-new',
        key_mode: 'append',
        multi_key_type: 'polling',
      }),
      { currentIsMultiKey: false, canEditSensitive: false }
    )

    expect(payload).not.toHaveProperty('is_multi_key')
    expect(payload).not.toHaveProperty('key_mode')
    expect(payload).not.toHaveProperty('multi_key_mode')
  })
})
