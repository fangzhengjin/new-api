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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test, vi } from 'vitest'

import { useSystemConfigStore } from '@/stores/system-config-store'

import type { User } from '../../types'

const mocks = vi.hoisted(() => ({
  getUsers: vi.fn(),
  searchUsers: vi.fn(),
  navigate: vi.fn(),
  search: {},
}))

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  getRouteApi: () => ({
    useSearch: () => mocks.search,
    useNavigate: () => mocks.navigate,
  }),
}))

vi.mock('../../api', () => ({
  getUsers: mocks.getUsers,
  searchUsers: mocks.searchUsers,
}))

const { UsersProvider } = await import('../users-provider')
const { UsersTable } = await import('../users-table')

const whitelistUser: User = {
  id: 7,
  username: 'whitelist-user',
  display_name: 'Whitelist User',
  quota: 1_000_000,
  quota_whitelist: true,
  used_quota: 0,
  request_count: 0,
  group: 'default',
  status: 1,
  role: 1,
}

function renderUsersTable() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <UsersProvider>
        <UsersTable />
      </UsersProvider>
    </QueryClientProvider>
  )
}

beforeEach(() => {
  mocks.getUsers.mockResolvedValue({
    success: true,
    data: { items: [whitelistUser], total: 1, page: 1, page_size: 20 },
  })
  mocks.searchUsers.mockResolvedValue({
    success: true,
    data: { items: [], total: 0, page: 1, page_size: 20 },
  })
  useSystemConfigStore.getState().setConfig({ companyQuotaModeEnabled: true })
})

test('company mode shows the whitelist badge and filters managed users from the toolbar', async () => {
  const user = userEvent.setup()
  renderUsersTable()

  expect(await screen.findByText('whitelist-user')).toBeInTheDocument()
  expect(screen.getByText('Whitelist')).toBeInTheDocument()

  const filterButton = screen.getByRole('button', {
    name: /Quota Management/,
  })
  filterButton.focus()
  await user.keyboard('{Enter}')
  await user.click(await screen.findByText('Managed users'))

  await waitFor(() =>
    expect(mocks.searchUsers).toHaveBeenCalledWith(
      expect.objectContaining({ quota_whitelist: false })
    )
  )
})

test('normal mode hides company quota whitelist controls', async () => {
  useSystemConfigStore.getState().setConfig({ companyQuotaModeEnabled: false })
  renderUsersTable()

  expect(await screen.findByText('whitelist-user')).toBeInTheDocument()
  expect(screen.queryByText('Whitelist')).toBeNull()
  expect(screen.queryByRole('button', { name: /Quota Management/ })).toBeNull()
})
