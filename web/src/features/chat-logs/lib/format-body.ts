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

type SseChunk = Record<string, unknown>

interface ClaudeBlock {
  type: string
  text?: string
  thinking?: string
  signature?: string
  id?: string
  name?: string
  input?: unknown
  inputRaw?: string
}

interface ClaudeMessage {
  id?: string
  type: string
  role?: string
  model?: string
  content: ClaudeBlock[]
  stop_reason?: string
  stop_sequence?: string | null
  usage?: Record<string, unknown>
}

/**
 * Close a JSON object whose capture was cut mid-text (close open string,
 * dangling key/comma, open brackets) so the last partial chunk stays readable.
 */
function repairTruncatedJson(text: string): SseChunk | null {
  const stack: string[] = []
  let inString = false
  let escaped = false
  for (const ch of text) {
    if (inString) {
      if (escaped) {
        escaped = false
      } else if (ch === '\\') {
        escaped = true
      } else if (ch === '"') {
        inString = false
      }
    } else if (ch === '"') {
      inString = true
    } else if (ch === '{' || ch === '[') {
      stack.push(ch)
    } else if (ch === '}' || ch === ']') {
      stack.pop()
    }
  }
  let repaired = inString ? `${text}"` : text
  const trimmedEnd = repaired.trimEnd()
  if (trimmedEnd.endsWith(':')) {
    repaired = `${trimmedEnd}null`
  } else if (trimmedEnd.endsWith(',')) {
    repaired = trimmedEnd.slice(0, -1)
  }
  for (let i = stack.length - 1; i >= 0; i--) {
    repaired += stack[i] === '{' ? '}' : ']'
  }
  try {
    const parsed: unknown = JSON.parse(repaired)
    return isRecord(parsed) ? parsed : null
  } catch {
    return null
  }
}

/**
 * Parse `data:` payload lines of an SSE capture. Unparseable lines (e.g. the
 * capture was truncated mid-event) and `[DONE]` markers are skipped.
 */
function parseSseData(body: string): SseChunk[] {
  const chunks: SseChunk[] = []
  for (const line of body.split('\n')) {
    const trimmed = line.trim()
    if (!trimmed.startsWith('data:')) {
      continue
    }
    const payload = trimmed.slice(5).trim()
    if (!payload || payload === '[DONE]') {
      continue
    }
    try {
      const parsed: unknown = JSON.parse(payload)
      if (isRecord(parsed)) {
        chunks.push(parsed)
      }
    } catch {
      const repaired = repairTruncatedJson(payload)
      if (repaired) {
        chunks.push(repaired)
      }
    }
  }
  return chunks
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function str(value: unknown): string | undefined {
  return typeof value === 'string' ? value : undefined
}

/**
 * Reassemble Claude streaming events into the equivalent non-stream message.
 * Returns null when the chunks are not a Claude event stream.
 */
function reassembleClaudeMessage(chunks: SseChunk[]): ClaudeMessage | null {
  if (!chunks.some((c) => c.type === 'message_start')) {
    return null
  }
  let message: ClaudeMessage | null = null
  const blocks = new Map<number, ClaudeBlock>()
  for (const chunk of chunks) {
    switch (chunk.type) {
      case 'message_start': {
        const src = isRecord(chunk.message) ? chunk.message : {}
        message = {
          id: str(src.id),
          type: 'message',
          role: str(src.role),
          model: str(src.model),
          content: [],
          usage: isRecord(src.usage) ? { ...src.usage } : undefined,
        }
        break
      }
      case 'content_block_start': {
        const src = isRecord(chunk.content_block) ? chunk.content_block : {}
        blocks.set(Number(chunk.index) || 0, {
          type: str(src.type) ?? 'text',
          text: str(src.text),
          thinking: str(src.thinking),
          signature: str(src.signature),
          id: str(src.id),
          name: str(src.name),
          inputRaw: '',
        })
        break
      }
      case 'content_block_delta': {
        const block = blocks.get(Number(chunk.index) || 0)
        const delta = isRecord(chunk.delta) ? chunk.delta : {}
        if (!block) {
          break
        }
        if (delta.type === 'text_delta' && typeof delta.text === 'string') {
          block.text = (block.text ?? '') + delta.text
        } else if (
          delta.type === 'thinking_delta' &&
          typeof delta.thinking === 'string'
        ) {
          block.thinking = (block.thinking ?? '') + delta.thinking
        } else if (
          delta.type === 'signature_delta' &&
          typeof delta.signature === 'string'
        ) {
          block.signature = (block.signature ?? '') + delta.signature
        } else if (
          delta.type === 'input_json_delta' &&
          typeof delta.partial_json === 'string'
        ) {
          block.inputRaw = (block.inputRaw ?? '') + delta.partial_json
        }
        break
      }
      case 'message_delta': {
        if (!message) {
          break
        }
        const delta = isRecord(chunk.delta) ? chunk.delta : {}
        message.stop_reason = str(delta.stop_reason)
        message.stop_sequence =
          delta.stop_sequence === null ? null : str(delta.stop_sequence)
        if (isRecord(chunk.usage)) {
          message.usage = { ...message.usage, ...chunk.usage }
        }
        break
      }
      default:
        break
    }
  }
  if (!message) {
    return null
  }
  const finalizeBlock = (block: ClaudeBlock): ClaudeBlock => {
    if (block.type === 'tool_use') {
      const raw = block.inputRaw ?? ''
      delete block.inputRaw
      try {
        block.input = raw ? JSON.parse(raw) : {}
      } catch {
        block.input = raw // truncated mid-JSON; keep the raw fragment
      }
    } else {
      delete block.inputRaw
    }
    return block
  }
  message.content = [...blocks.entries()]
    .sort(([a], [b]) => a - b)
    .map(([, block]) => finalizeBlock(block))
  return message
}

/**
 * Line-break and indent a JSON-like body that failed to parse (capture was
 * truncated mid-JSON). Whitespace outside strings is re-emitted by structure;
 * string contents are kept verbatim so truncation never loses text.
 */
function reindentJsonLike(text: string): string {
  const out: string[] = []
  let indent = 0
  let inString = false
  let escaped = false
  const newline = () => `\n${'  '.repeat(indent)}`
  for (const ch of text) {
    if (inString) {
      out.push(ch)
      if (escaped) {
        escaped = false
      } else if (ch === '\\') {
        escaped = true
      } else if (ch === '"') {
        inString = false
      }
      continue
    }
    if (ch === '"') {
      inString = true
      out.push(ch)
    } else if (ch === '{' || ch === '[') {
      indent += 1
      out.push(ch, newline())
    } else if (ch === '}' || ch === ']') {
      indent = Math.max(0, indent - 1)
      out.push(newline(), ch)
    } else if (ch === ',') {
      out.push(ch, newline())
    } else if (ch === ':') {
      out.push(ch, ' ')
    } else if (ch !== ' ' && ch !== '\t' && ch !== '\n' && ch !== '\r') {
      out.push(ch)
    }
  }
  return out.join('').trim()
}

/**
 * Format a captured chat-log request/response body for display:
 * - valid JSON is pretty-printed
 * - Claude SSE streams are reassembled into a single non-stream message
 * - other SSE streams are shown as an array of parsed chunks
 * - truncated JSON is re-indented instead of shown as one huge line
 */
export function formatChatLogBody(body: string): string {
  const trimmed = body.trim()
  if (!trimmed) {
    return body
  }
  if (trimmed.startsWith('{') || trimmed.startsWith('[')) {
    try {
      return JSON.stringify(JSON.parse(trimmed), null, 2)
    } catch {
      return reindentJsonLike(trimmed)
    }
  }
  if (/^data:/m.test(trimmed)) {
    const chunks = parseSseData(trimmed)
    const claude = chunks.length > 0 ? reassembleClaudeMessage(chunks) : null
    if (claude) {
      return JSON.stringify(claude, null, 2)
    }
    if (chunks.length > 0) {
      return JSON.stringify(chunks, null, 2)
    }
  }
  return body
}
