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
const { DataTableRowActions } = await import('../data-table-row-actions')

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

// 当前登录管理员 id=1；other-key 属于用户 2
const ADMIN_ID = 1
const ownKey = {
  id: 11,
  user_id: ADMIN_ID,
  name: 'own-key',
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
const otherUserKey = { ...ownKey, id: 22, user_id: 2, name: 'other-key' }

function signInAsAdmin() {
  useAuthStore.setState((state) => ({
    auth: {
      ...state.auth,
      user: { id: ADMIN_ID, username: 'admin', role: 100 },
    },
  }))
}

async function renderRowActions(adminMode: boolean, apiKey: typeof ownKey) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const freshAt = Date.now() + 60_000
  queryClient.setQueryData(['status'], { data: {} }, { updatedAt: freshAt })

  render(
    <QueryClientProvider client={queryClient}>
      <I18nextProvider i18n={i18n}>
        <ApiKeysProvider adminMode={adminMode}>
          <DataTableRowActions row={{ original: apiKey } as never} />
        </ApiKeysProvider>
      </I18nextProvider>
    </QueryClientProvider>
  )
  return queryClient
}

async function openRowMenu() {
  fireEvent.click(screen.getByRole('button', { name: 'Open menu' }))
  // 菜单内容挂在 body 门户下，等它出现
  await waitFor(
    () => {
      expect(screen.getByText('Delete')).toBeTruthy()
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

describe('admin mode row actions', () => {
  test("hides plaintext key actions for another user's key in admin mode", async () => {
    signInAsAdmin()
    await renderRowActions(true, otherUserKey)
    await openRowMenu()

    expect(screen.queryByText('Copy Key')).toBe(null)
    expect(screen.queryByText('Copy Connection Info')).toBe(null)
    expect(screen.queryByText('CC Switch')).toBe(null)
    // 管理操作保留
    expect(screen.getByText('Delete')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Edit' })).toBeTruthy()
  })

  test("keeps plaintext key actions for the admin's own key in admin mode", async () => {
    signInAsAdmin()
    await renderRowActions(true, ownKey)
    await openRowMenu()

    expect(screen.getByText('Copy Key')).toBeTruthy()
    expect(screen.getByText('Copy Connection Info')).toBeTruthy()
    expect(screen.getByText('CC Switch')).toBeTruthy()
  })

  test("routes the status toggle through the admin endpoint for another user's key", async () => {
    signInAsAdmin()
    const putUrls: string[] = []
    apiClient.put = async (url) => {
      putUrls.push(url)
      return { data: { success: true, data: {} } }
    }

    await renderRowActions(true, otherUserKey)
    fireEvent.click(screen.getByRole('button', { name: 'Disable' }))

    await waitFor(() => expect(putUrls).toHaveLength(1))
    expect(putUrls[0]).toBe('/api/token/admin/?status_only=true')
  })

  test('routes the status toggle through the user endpoint for own keys', async () => {
    signInAsAdmin()
    const putUrls: string[] = []
    apiClient.put = async (url) => {
      putUrls.push(url)
      return { data: { success: true, data: {} } }
    }

    await renderRowActions(false, ownKey)
    fireEvent.click(screen.getByRole('button', { name: 'Disable' }))

    await waitFor(() => expect(putUrls).toHaveLength(1))
    expect(putUrls[0]).toBe('/api/token/?status_only=true')
  })
})
