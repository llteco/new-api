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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { CHANNEL_FORM_DEFAULT_VALUES } from '../../lib/channel-form'
import type { Channel } from '../../types'
import { useChannelMutateForm } from '../use-channel-mutate-form'

vi.mock('../../api', () => ({
  updateChannel: vi.fn(),
  createChannel: vi.fn(),
}))
vi.mock('@/stores/auth-store', () => ({
  useAuthStore: (selector: (state: unknown) => unknown) =>
    selector({ auth: { user: { id: 1, role: 100 } } }),
}))
vi.mock('@/lib/admin-permissions', () => ({
  ADMIN_PERMISSION_ACTIONS: { SENSITIVE_WRITE: 'sensitive_write' },
  ADMIN_PERMISSION_RESOURCES: { CHANNEL: 'channel' },
  hasPermission: () => true,
}))

const { updateChannel } = await import('../../api')

function channelFixture(isMultiKey: boolean): Channel {
  return {
    id: 7,
    channel_info: { is_multi_key: isMultiKey },
  } as Channel
}

function formValues(isMultiKey: boolean) {
  return { ...CHANNEL_FORM_DEFAULT_VALUES, is_multi_key: isMultiKey }
}

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
}

beforeEach(() => {
  vi.mocked(updateChannel).mockResolvedValue({ success: true, message: '' })
})

describe('useChannelMutateForm key-mode conversion baseline', () => {
  test('detects conversion against the detail snapshot when the table row is stale', async () => {
    const { result } = renderHook(
      () =>
        useChannelMutateForm({
          // 表格行过期：仍显示单 key；详情快照：已被其他管理员转为多 key
          currentRow: channelFixture(false),
          detailChannel: channelFixture(true),
          isEditing: true,
          onSuccess: () => {},
        }),
      { wrapper: createWrapper() }
    )

    await result.current.mutateAsync(formValues(false))

    expect(updateChannel).toHaveBeenCalledWith(
      7,
      expect.objectContaining({ is_multi_key: false })
    )
  })

  test('falls back to the table row when no detail snapshot is loaded', async () => {
    const { result } = renderHook(
      () =>
        useChannelMutateForm({
          currentRow: channelFixture(true),
          isEditing: true,
          onSuccess: () => {},
        }),
      { wrapper: createWrapper() }
    )

    await result.current.mutateAsync(formValues(false))

    expect(updateChannel).toHaveBeenCalledWith(
      7,
      expect.objectContaining({ is_multi_key: false })
    )
  })
})