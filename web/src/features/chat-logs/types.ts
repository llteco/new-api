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
export interface ChatSessionMeta {
  id: number
  token_id: number
  user_id: number
  model_name: string
  turn_count: number
  message_count: number
  created_at: number
  last_active_at: number
}

export interface ChatTurn {
  id: number
  session_id: number
  turn_index: number
  request_id: string
  model_name: string
  channel_id: number
  status_code: number
  use_time: number
  is_stream: boolean
  new_messages: string
  response_body: string
  created_at: number
}

export interface SessionDetail {
  session: ChatSessionMeta
  turns: ChatTurn[]
  has_more: boolean
  next_turn_id: number | null
}

export interface GetChatSessionsParams {
  limit?: number
  cursor?: string
  token_id?: number
  user_id?: number
  model_name?: string
  start_ts?: number
  end_ts?: number
}

export interface SessionListResponse {
  success: boolean
  message?: string
  data?: {
    items: ChatSessionMeta[]
    has_more: boolean
    next_cursor: string | null
  }
}

export interface GetSessionDetailParams {
  limit?: number
  before_id?: number
}

export interface SessionDetailResponse {
  success: boolean
  message?: string
  data?: SessionDetail
}
