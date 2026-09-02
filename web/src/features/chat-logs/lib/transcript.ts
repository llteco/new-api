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
import type { ChatTurn } from '../types'
import { formatChatLogBody } from './format-body'

export interface TurnView {
  turn: ChatTurn
  displayMessages: unknown[]
  responseText: string
}

function parseMessages(raw: string): unknown[] {
  try {
    const parsed: unknown = JSON.parse(raw)
    return Array.isArray(parsed) ? parsed : []
  } catch {
    return []
  }
}

function dropLeadingAssistantEchoes(messages: unknown[]): unknown[] {
  let first = 0
  while (first < messages.length) {
    const role = (messages[first] as { role?: string } | undefined)?.role
    if (role !== 'assistant' && role !== 'model') {
      break
    }
    first++
  }
  return messages.slice(first)
}

/**
 * Build the display transcript for a session: each turn keeps its new
 * messages (minus the leading assistant echoes of the previous response)
 * and its formatted response body.
 */
export function buildTranscript(turns: ChatTurn[]): TurnView[] {
  return turns.map((turn) => ({
    turn,
    displayMessages: dropLeadingAssistantEchoes(parseMessages(turn.new_messages)),
    responseText: formatChatLogBody(turn.response_body),
  }))
}
