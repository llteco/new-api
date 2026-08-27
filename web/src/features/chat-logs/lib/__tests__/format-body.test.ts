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

You should have received a copy of the GNU Affero General License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { describe, expect, test } from 'vitest'

import { formatChatLogBody } from '../format-body'

const claudeSse = [
  'event: message_start',
  'data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"glm-5.3","content":[],"usage":{"input_tokens":10,"output_tokens":0}}}',
  '',
  'event: ping',
  'data: {"type":"ping"}',
  '',
  'event: content_block_start',
  'data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}',
  '',
  'event: content_block_delta',
  'data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"continue"}}',
  '',
  'event: content_block_stop',
  'data: {"type":"content_block_stop","index":0}',
  '',
  'event: message_delta',
  'data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":3}}',
  '',
  'event: message_stop',
  'data: {"type":"message_stop"}',
  '',
].join('\n')

const claudeSseFullBlocks = [
  'event: message_start',
  'data: {"type":"message_start","message":{"id":"msg_2","type":"message","role":"assistant","model":"glm-5.3","content":[],"usage":{"input_tokens":5,"output_tokens":0}}}',
  '',
  'event: content_block_start',
  'data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}',
  '',
  'event: content_block_delta',
  'data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"let me think"}}',
  '',
  'event: content_block_delta',
  'data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig123"}}',
  '',
  'event: content_block_stop',
  'data: {"type":"content_block_stop","index":0}',
  '',
  'event: content_block_start',
  'data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{}}}',
  '',
  'event: content_block_delta',
  'data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\\"city\\":"}}',
  '',
  'event: content_block_delta',
  'data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":" \\"Beijing\\"}"}}',
  '',
  'event: content_block_stop',
  'data: {"type":"content_block_stop","index":1}',
  '',
  'event: message_delta',
  'data: {"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":50}}',
  '',
  'event: message_stop',
  'data: {"type":"message_stop"}',
  '',
].join('\n')

const openAiSse = [
  'data: {"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"Hi"}}]}',
  '',
  'data: {"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":" there"}}]}',
  '',
  'data: [DONE]',
  '',
].join('\n')

describe('formatChatLogBody', () => {
  test('pretty-prints valid JSON bodies', () => {
    const out = formatChatLogBody('{"a":  1,   "b":[1, 2]}')
    expect(JSON.parse(out)).toEqual({ a: 1, b: [1, 2] })
    expect(out).toContain('\n')
  })

  test('returns empty body unchanged', () => {
    expect(formatChatLogBody('')).toBe('')
  })

  test('reconstructs a Claude SSE stream into a single message', () => {
    const out = formatChatLogBody(claudeSse)
    const msg = JSON.parse(out)
    expect(msg).toMatchObject({
      id: 'msg_1',
      type: 'message',
      role: 'assistant',
      model: 'glm-5.3',
      stop_reason: 'end_turn',
      content: [{ type: 'text', text: 'continue' }],
      usage: { input_tokens: 10, output_tokens: 3 },
    })
  })

  test('reassembles Claude thinking, signature and tool_use json deltas', () => {
    const out = formatChatLogBody(claudeSseFullBlocks)
    const msg = JSON.parse(out)
    expect(msg.stop_reason).toBe('tool_use')
    expect(msg.content[0]).toEqual({
      type: 'thinking',
      thinking: 'let me think',
      signature: 'sig123',
    })
    expect(msg.content[1]).toEqual({
      type: 'tool_use',
      id: 'toolu_1',
      name: 'get_weather',
      input: { city: 'Beijing' },
    })
  })

  test('tolerates a Claude stream truncated mid-event', () => {
    const truncated =
      'event: message_start\n' +
      'data: {"type":"message_start","message":{"id":"msg_t","role":"assistant","model":"m","content":[]}}\n\n' +
      'event: content_block_start\n' +
      'data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}\n\n' +
      'event: content_block_delta\n' +
      'data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial answ'
    const msg = JSON.parse(formatChatLogBody(truncated))
    expect(msg.id).toBe('msg_t')
    expect(msg.content[0].text).toBe('partial answ')
  })

  test('pretty-prints non-Claude SSE chunks as an array', () => {
    const out = formatChatLogBody(openAiSse)
    const chunks = JSON.parse(out)
    expect(Array.isArray(chunks)).toBe(true)
    expect(chunks).toHaveLength(2)
    expect(chunks[0].choices[0].delta.content).toBe('Hi')
  })

  test('re-indents truncated JSON instead of returning one huge line', () => {
    const truncated = '{"model":"glm-5.3","messages":[{"role":"user","content":[{"type":"text","text":"a very long body that got cut of'
    const out = formatChatLogBody(truncated)
    expect(out.split('\n').length).toBeGreaterThan(3)
    expect(out).toContain('"model": "glm-5.3"')
  })

  test('re-indent never breaks string contents containing braces', () => {
    const truncated = '{"text":"keep {this} [intact]","n":1'
    const out = formatChatLogBody(truncated)
    expect(out).toContain('"keep {this} [intact]"')
  })

  test('returns non-JSON plain text unchanged', () => {
    expect(formatChatLogBody('upstream said: bad gateway')).toBe(
      'upstream said: bad gateway'
    )
  })
})
