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

import { parseGroupList } from './group-list'

describe('parseGroupList', () => {
  test('returns an empty array for missing values', () => {
    expect(parseGroupList(undefined)).toEqual([])
    expect(parseGroupList(null)).toEqual([])
    expect(parseGroupList('')).toEqual([])
  })

  test('splits a comma-separated list and trims whitespace', () => {
    expect(parseGroupList('group0,group1')).toEqual(['group0', 'group1'])
    expect(parseGroupList(' group0 , group1 ')).toEqual(['group0', 'group1'])
  })

  test('drops empty entries', () => {
    expect(parseGroupList('group0,,group1,')).toEqual(['group0', 'group1'])
    expect(parseGroupList(' , ')).toEqual([])
  })

  test('keeps order so the first entry is the primary group', () => {
    expect(parseGroupList('group1,group0')[0]).toBe('group1')
  })

  test('does not split substrings into separate groups', () => {
    expect(parseGroupList('group10')).toEqual(['group10'])
  })
})
