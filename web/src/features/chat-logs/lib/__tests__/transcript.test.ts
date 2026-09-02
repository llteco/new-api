/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { describe, expect, test } from 'vitest'

import type { ChatTurn } from '../../types'
import { buildTranscript } from '../transcript'

function makeTurn(overrides: Partial<ChatTurn> = {}): ChatTurn {
  return {
    id: 1,
    session_id: 1,
    turn_index: 0,
    request_id: 'req-1',
    model_name: 'gpt-4',
    channel_id: 1,
    status_code: 200,
    use_time: 1,
    is_stream: false,
    new_messages: '[]',
    response_body: '{}',
    created_at: 1700000000,
    ...overrides,
  }
}

describe('buildTranscript', () => {
  test('drops leading assistant echo messages and stops at first non-assistant', () => {
    const turn = makeTurn({
      new_messages: JSON.stringify([
        { role: 'assistant', content: 'previous response' },
        { role: 'assistant', content: 'another echo' },
        { role: 'user', content: 'hello' },
        { role: 'assistant', content: 'kept: after user' },
      ]),
    })

    const [view] = buildTranscript([turn])

    expect(view?.displayMessages).toEqual([
      { role: 'user', content: 'hello' },
      { role: 'assistant', content: 'kept: after user' },
    ])
  })

  test('drops leading model (Gemini) echo message and stops at first non-echo', () => {
    const turn = makeTurn({
      new_messages: JSON.stringify([
        { role: 'model', content: 'gemini echo of previous response' },
        { role: 'user', content: 'hello' },
      ]),
    })

    const [view] = buildTranscript([turn])

    expect(view?.displayMessages).toEqual([{ role: 'user', content: 'hello' }])
  })

  test('keeps assistant message that appears after a user message', () => {
    const turn = makeTurn({
      new_messages: JSON.stringify([
        { role: 'user', content: 'hi' },
        { role: 'assistant', content: 'tool echo mid-conversation' },
        { role: 'user', content: 'next' },
      ]),
    })

    const [view] = buildTranscript([turn])

    expect(view?.displayMessages).toHaveLength(3)
  })

  test('returns empty displayMessages when new_messages is malformed JSON', () => {
    const turn = makeTurn({ new_messages: '{"broken":' })

    const [view] = buildTranscript([turn])

    expect(view?.displayMessages).toEqual([])
  })

  test('passes response_body through formatChatLogBody', () => {
    const turn = makeTurn({ response_body: '{"role":"assistant","content":"hi"}' })

    const [view] = buildTranscript([turn])

    expect(view?.responseText).toBe(
      JSON.stringify(
        { role: 'assistant', content: 'hi' },
        null,
        2
      )
    )
  })

  test('returns one TurnView per turn preserving order', () => {
    const views = buildTranscript([
      makeTurn({ id: 1, turn_index: 0 }),
      makeTurn({ id: 2, turn_index: 1 }),
    ])

    expect(views.map((v) => v.turn.id)).toEqual([1, 2])
  })
})
