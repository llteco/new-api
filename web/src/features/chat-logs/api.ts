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
import { api } from '@/lib/api'

import type {
  GetChatSessionsParams,
  SessionDetailResponse,
  SessionListResponse,
} from './types'

export async function getChatSessions(
  params: GetChatSessionsParams
): Promise<SessionListResponse> {
  const res = await api.get('/api/chat_logs/sessions', { params })
  return res.data
}

export async function getChatSessionDetail(
  id: number
): Promise<SessionDetailResponse> {
  const res = await api.get(`/api/chat_logs/sessions/${id}`)
  return res.data
}

export const chatSessionsQueryKeys = {
  all: ['chat-sessions'] as const,
  list: (params: Record<string, unknown>) =>
    [...chatSessionsQueryKeys.all, 'list', params] as const,
  detail: (id: number) => [...chatSessionsQueryKeys.all, 'detail', id] as const,
}
