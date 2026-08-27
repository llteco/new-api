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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, test } from 'vitest'

const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { QueryClient, QueryClientProvider } =
  await import('@tanstack/react-query')
const { api } = await import('@/lib/api')
const { useAuthStore } = await import('@/stores/auth-store')
const { ApiKeysProvider } = await import('../api-keys-provider')
const { ApiKeysMutateDrawer } = await import('../api-keys-mutate-drawer')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

type ApiMethod = (url: string, data?: unknown) => Promise<{ data: unknown }>
type MockableApi = {
  get: ApiMethod
  put: ApiMethod
}

const apiClient = api as unknown as MockableApi
const originalGet = apiClient.get
const originalPut = apiClient.put

// 管理员 id=1 编辑用户 2 的令牌
const adminUserId = 1
const ownerUserId = 2
const otherUserToken = {
  id: 33,
  user_id: ownerUserId,
  name: 'other-user-token',
  key: 'sk-****',
  status: 1,
  remain_quota: 100,
  used_quota: 0,
  unlimited_quota: true,
  expired_time: -1,
  created_time: 1,
  accessed_time: 1,
  group: 'default',
  auto_groups: null,
  cross_group_retry: false,
  model_limits_enabled: false,
  model_limits: '',
  allow_ips: '',
  reset_period: 'never',
  reset_quota: 0,
  channel_quota_mode: false,
  chat_log_enabled: false,
}

function installApiFixtures(requestedUrls: string[], putUrls: string[]) {
  apiClient.get = async (url) => {
    requestedUrls.push(url)
    switch (url) {
      case '/api/status':
        return { data: { data: {} } }
      case '/api/user/models':
        return { data: { success: true, data: [] } }
      case '/api/user/self/groups':
        return {
          data: {
            success: true,
            data: {
              default: { desc: 'Standard access', ratio: 1 },
              vip: { desc: 'Priority access', ratio: 2 },
            },
          },
        }
      case `/api/token/admin/${otherUserToken.id}`:
        return { data: { success: true, data: otherUserToken } }
      case `/api/token/admin/auto-groups?user_id=${ownerUserId}`:
        return {
          data: { success: true, data: { groups: ['default'], max_count: 3 } },
        }
      case `/api/token/${otherUserToken.id}/channel_quotas`:
        return { data: { success: true, data: [] } }
      default:
        throw new Error(`Unexpected GET ${url}`)
    }
  }
  apiClient.put = async (url) => {
    putUrls.push(url)
    return { data: { success: true, data: {} } }
  }
}

async function renderAdminEditDrawer(): Promise<void> {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const freshAt = Date.now() + 60_000
  queryClient.setQueryData(['status'], { data: {} }, { updatedAt: freshAt })

  useAuthStore.setState((state) => ({
    auth: {
      ...state.auth,
      user: { id: adminUserId, username: 'admin', role: 100 },
    },
  }))

  render(
    <QueryClientProvider client={queryClient}>
      <I18nextProvider i18n={i18n}>
        <ApiKeysProvider adminMode>
          <ApiKeysMutateDrawer
            open
            onOpenChange={() => undefined}
            currentRow={otherUserToken}
          />
        </ApiKeysProvider>
      </I18nextProvider>
    </QueryClientProvider>
  )
  await waitFor(
    () => {
      const saveButton = screen
        .queryAllByRole<HTMLButtonElement>('button')
        .find((candidate) => candidate.textContent?.includes('Save changes'))
      expect(saveButton).toBeEnabled()
    },
    { timeout: 1500 }
  )
}

afterEach(() => {
  apiClient.get = originalGet
  apiClient.put = originalPut
  useAuthStore.setState((state) => ({ auth: { ...state.auth, user: null } }))
  localStorage.clear()
})

describe('admin mode mutate drawer', () => {
  test("loads another user's token through the admin endpoints and submits the update there", async () => {
    const requestedUrls: string[] = []
    const putUrls: string[] = []
    installApiFixtures(requestedUrls, putUrls)
    await renderAdminEditDrawer()

    expect(requestedUrls).toContain(`/api/token/admin/${otherUserToken.id}`)
    expect(requestedUrls).toContain(
      `/api/token/admin/auto-groups?user_id=${ownerUserId}`
    )

    const nameInput = screen.getByLabelText('Name') as HTMLInputElement
    fireEvent.input(nameInput, { target: { value: 'admin-renamed' } })
    fireEvent.click(
      screen
        .queryAllByRole<HTMLButtonElement>('button')
        .find((candidate) =>
          candidate.textContent?.includes('Save changes')
        ) as HTMLButtonElement
    )

    await waitFor(() => expect(putUrls).toHaveLength(1))
    expect(putUrls[0]).toBe('/api/token/admin/')
  })
})
