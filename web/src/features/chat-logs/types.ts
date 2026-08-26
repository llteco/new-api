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
export interface ChatLogMeta {
  id: number
  token_id: number
  user_id: number
  channel_id: number
  model_name: string
  request_id: string
  is_stream: boolean
  truncated: boolean
  status_code: number
  use_time: number
  created_at: number
}

export interface ChatLogFull extends ChatLogMeta {
  request_body: string
  response_body: string
}

export interface GetChatLogsParams {
  page?: number
  page_size?: number
  token_id?: number
  user_id?: number
  channel_id?: number
  model_name?: string
}

export interface ListResponse {
  success: boolean
  message?: string
  data?: ChatLogMeta[]
  total: number
}

export interface DetailResponse {
  success: boolean
  message?: string
  data?: ChatLogFull
}
