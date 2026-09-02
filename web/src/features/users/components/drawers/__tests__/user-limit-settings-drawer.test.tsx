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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import type { UserLimitsData } from '../../../types'
import { UserLimitSettingsDrawer } from '../user-limit-settings-drawer'

const apiMocks = vi.hoisted(() => ({
  getUserLimits: vi.fn(),
  updateUserLimits: vi.fn(),
  removeUserAccessSource: vi.fn(),
  allowLatestUserAccessSource: vi.fn(),
}))

vi.mock('../../../api', () => apiMocks)

const user = {
  id: 7,
  username: 'source-user',
  display_name: 'Source User',
  quota: 0,
  used_quota: 0,
  request_count: 0,
  group: 'default',
  status: 1,
  role: 1,
}

const limits: UserLimitsData = {
  user: { id: 7, username: 'source-user' },
  overrides: {
    model_request_rate_limit_count: null,
    model_request_rate_limit_success_count: 50,
    model_request_concurrency_limit: null,
    access_source_max_ips: null,
    access_source_switch_cooldown_minutes: 30,
  },
  effective: {
    model_request_rate_limit_count: 200,
    model_request_rate_limit_count_source: 'group',
    model_request_rate_limit_success_count: 50,
    model_request_rate_limit_success_source: 'user',
    model_request_concurrency_limit: 3,
    model_request_concurrency_limit_source: 'global',
    access_source_max_ips: 4,
    access_source_max_ips_source: 'global',
    access_source_switch_cooldown_minutes: 30,
    access_source_switch_cooldown_source: 'user',
  },
  global: {
    model_request_rate_limit_enabled: true,
    model_request_rate_limit_duration_minutes: 1,
    model_request_ip_rate_limit_count: 200,
    model_request_ip_rate_limit_success_count: 100,
    model_request_concurrency_limit_enabled: true,
    model_request_ip_concurrency_limit: 5,
    access_source_limit_enabled: true,
    access_source_association_window_hours: 24,
    access_source_max_users_per_ip: 5,
  },
  access_source_state: {
    current_ip: '203.0.113.1',
    current_last_seen_at: 1_800_000_000,
    associated_count: 2,
    cooldown_remaining_seconds: 600,
    associations: [
      { ip: '203.0.113.1', last_seen_at: 1_800_000_000, is_current: true },
      { ip: '192.0.2.1', last_seen_at: 1_799_999_000, is_current: false },
    ],
    pending: {
      event_id: 'latest-event',
      ip: '198.51.100.1',
      reason: 'switch_cooldown',
      occurred_at: 1_800_000_001,
    },
    recent_rejections: [
      {
        event_id: 'latest-event',
        ip: '198.51.100.1',
        reason: 'switch_cooldown',
        occurred_at: 1_800_000_001,
      },
      {
        event_id: 'older-event',
        ip: '198.51.100.2',
        reason: 'account_ip_limit',
        occurred_at: 1_799_999_500,
      },
    ],
  },
}

function renderDrawer() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <UserLimitSettingsDrawer
        open
        onOpenChange={() => undefined}
        user={user}
      />
    </QueryClientProvider>
  )
}

describe('user limit settings drawer', () => {
  beforeEach(() => {
    apiMocks.getUserLimits.mockResolvedValue({ success: true, data: limits })
    apiMocks.updateUserLimits.mockResolvedValue({ success: true })
    apiMocks.removeUserAccessSource.mockResolvedValue({ success: true })
    apiMocks.allowLatestUserAccessSource.mockResolvedValue({ success: true })
  })

  test('shows actions only for old associations and the latest rejection', async () => {
    renderDrawer()

    expect(
      await screen.findByRole('button', { name: 'Remove association' })
    ).toBeVisible()
    expect(screen.getByRole('button', { name: 'Allow switch' })).toBeVisible()
    expect(screen.getAllByText('198.51.100.2')).toHaveLength(1)
    expect(
      screen.getAllByRole('button', { name: /Allow (switch|use)/ })
    ).toHaveLength(1)
  })

  test('shows when global concurrency limiting is disabled', async () => {
    apiMocks.getUserLimits.mockResolvedValue({
      success: true,
      data: {
        ...limits,
        global: {
          ...limits.global,
          model_request_concurrency_limit_enabled: false,
        },
      },
    })

    renderDrawer()

    expect(
      await screen.findByText('Global concurrency limiting is disabled')
    ).toBeVisible()
  })

  test('revalidates and submits the latest rejected source', async () => {
    renderDrawer()
    fireEvent.click(await screen.findByRole('button', { name: 'Allow switch' }))
    fireEvent.click(screen.getByRole('button', { name: 'Allow source' }))

    await waitFor(() => {
      expect(apiMocks.allowLatestUserAccessSource).toHaveBeenCalledWith(
        7,
        'latest-event',
        '198.51.100.1'
      )
    })
  })

  test('distinguishes switch waiting from association limits', async () => {
    apiMocks.getUserLimits.mockResolvedValue({
      success: true,
      data: {
        ...limits,
        effective: { ...limits.effective, access_source_max_ips: 0 },
        global: { ...limits.global, access_source_max_users_per_ip: 0 },
        access_source_state: {
          ...limits.access_source_state,
          cooldown_remaining_seconds: 0,
        },
      },
    })

    renderDrawer()

    expect(
      await screen.findByText(
        'Association period: 24 hours · Max accounts per IP: Unlimited'
      )
    ).toBeVisible()
    expect(
      screen.getByText(
        'IP switch wait only controls switching; association limits are checked separately'
      )
    ).toBeVisible()
    expect(
      screen.getByText(
        'IP switch wait has ended; retrying will be checked against association limits'
      )
    ).toBeVisible()
    expect(screen.getByText('2 / Unlimited')).toBeVisible()
  })

  test('explains the lifetime of an allowed source', async () => {
    renderDrawer()
    fireEvent.click(await screen.findByRole('button', { name: 'Allow switch' }))

    expect(screen.getByRole('alertdialog')).toHaveTextContent(
      'The normal association expires after 24 hours without activity and is not a permanent whitelist.'
    )
  })

  test('keeps inherited values empty and exposes their effective value', async () => {
    renderDrawer()

    const accountLimit = await screen.findByRole('spinbutton', {
      name: 'Max requests per account per period',
    })
    expect(accountLimit).toHaveValue(null)
    expect(accountLimit).toHaveAttribute('placeholder', '200')
    accountLimit.focus()
    expect(accountLimit).toHaveFocus()

    const concurrencyLimit = screen.getByRole('spinbutton', {
      name: 'Max concurrent requests per account',
    })
    expect(concurrencyLimit).toHaveValue(null)
    expect(concurrencyLimit).toHaveAttribute('placeholder', '3')
    expect(
      screen.getByText('Global max concurrent requests per IP: 5')
    ).toBeVisible()
  })

  test('keeps unsaved limits while refreshing source records', async () => {
    renderDrawer()

    const accountLimit = await screen.findByRole('spinbutton', {
      name: 'Max requests per account per period',
    })
    fireEvent.change(accountLimit, { target: { value: '321' } })
    fireEvent.click(screen.getByRole('button', { name: 'Allow switch' }))
    fireEvent.click(screen.getByRole('button', { name: 'Allow source' }))

    await waitFor(() => expect(apiMocks.getUserLimits).toHaveBeenCalledTimes(2))
    expect(accountLimit).toHaveValue(321)
  })

  test('saves an account concurrency override with the existing limit settings', async () => {
    renderDrawer()

    const concurrencyLimit = await screen.findByRole('spinbutton', {
      name: 'Max concurrent requests per account',
    })
    fireEvent.change(concurrencyLimit, { target: { value: '4' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => {
      expect(apiMocks.updateUserLimits).toHaveBeenCalledWith(
        7,
        expect.objectContaining({ model_request_concurrency_limit: 4 })
      )
    })
  })
})
