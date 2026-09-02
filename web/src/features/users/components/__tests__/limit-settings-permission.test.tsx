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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { USER_ROLE } from '../../constants'
import type { User } from '../../types'

const mocks = vi.hoisted(() => ({
  getUsers: vi.fn(),
  searchUsers: vi.fn(),
  getUserLimits: vi.fn(),
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

vi.mock('../../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../api')>()),
  getUsers: mocks.getUsers,
  searchUsers: mocks.searchUsers,
  getUserLimits: mocks.getUserLimits,
}))

const { UsersProvider } = await import('../users-provider')
const { UsersTable } = await import('../users-table')

const target: User = {
  id: 1,
  username: 'target',
  display_name: 'Target',
  quota: 0,
  used_quota: 0,
  request_count: 0,
  group: 'default',
  status: 1,
  role: USER_ROLE.USER,
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
  mocks.searchUsers.mockResolvedValue({
    success: true,
    data: { items: [], total: 0, page: 1, page_size: 20 },
  })
  mocks.getUserLimits.mockResolvedValue({
    success: false,
    message: 'Limit fixture is not needed',
  })
  useSystemConfigStore
    .getState()
    .setConfig({ cycleQuotaManagementEnabled: false })
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'root',
    role: USER_ROLE.ROOT,
  })
})

test('root can open limit settings for the root account', async () => {
  mocks.getUsers.mockResolvedValue({
    success: true,
    data: {
      items: [{ ...target, username: 'root', role: USER_ROLE.ROOT }],
      total: 1,
      page: 1,
      page_size: 20,
    },
  })
  const user = userEvent.setup()
  renderUsersTable()

  expect(await screen.findByText('root')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Open menu' }))

  expect(
    screen.getByText('Limit Settings').closest('[role="menuitem"]')
  ).not.toHaveAttribute('aria-disabled', 'true')
  await user.click(screen.getByText('Limit Settings'))
  expect(
    await screen.findByRole('heading', { name: 'User Limit Settings' })
  ).toBeVisible()
})

test('admin cannot open limit settings for an admin peer', async () => {
  useAuthStore.getState().auth.setUser({
    id: 2,
    username: 'admin-operator',
    role: USER_ROLE.ADMIN,
  })
  mocks.getUsers.mockResolvedValue({
    success: true,
    data: {
      items: [
        { ...target, id: 3, username: 'admin-peer', role: USER_ROLE.ADMIN },
      ],
      total: 1,
      page: 1,
      page_size: 20,
    },
  })
  const user = userEvent.setup()
  renderUsersTable()

  expect(await screen.findByText('admin-peer')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Open menu' }))

  expect(
    screen.getByText('Limit Settings').closest('[role="menuitem"]')
  ).toHaveAttribute('aria-disabled', 'true')
})
