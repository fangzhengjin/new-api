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
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { OverviewDashboard } from '../overview-dashboard'

const dashboardState = vi.hoisted(() => ({
  user: {
    request_count: 1,
    quota: 1,
    used_quota: 0,
    role: 1,
  },
  apiKeys: [
    {
      id: 1,
      name: 'Primary key',
      key: 'test-key',
      status: 1,
      remain_quota: 1,
      used_quota: 0,
      unlimited_quota: false,
      expired_time: -1,
      created_time: 0,
      accessed_time: 0,
      group: '',
      auto_groups: null,
      cross_group_retry: false,
      model_limits_enabled: false,
      model_limits: '',
      allow_ips: '',
    },
  ],
}))

vi.mock('@tanstack/react-query', () => ({
  useQuery: (options: { queryKey: string[] }) =>
    options.queryKey.includes('api-keys')
      ? { data: dashboardState.apiKeys, isFetched: true }
      : { data: ['gpt-4.1'], isFetched: true },
}))

vi.mock('@tanstack/react-router', () => ({
  Link: (props: { children?: ReactNode; className?: string; to: string }) => (
    <a className={props.className} href={props.to}>
      {props.children}
    </a>
  ),
}))

vi.mock('@/stores/auth-store', () => ({
  useAuthStore: (
    selector: (state: { auth: { user: typeof dashboardState.user } }) => unknown
  ) => selector({ auth: { user: dashboardState.user } }),
}))

vi.mock('../../../hooks/use-status-data', () => ({
  useApiInfo: () => ({ items: [] }),
  useDashboardContentVisibility: () => ({
    apiInfo: false,
    announcements: false,
    faq: false,
    uptimeKuma: false,
    panelLayout: [
      { id: 'api-info', span: 1 },
      { id: 'announcements', span: 1 },
      { id: 'faq', span: 1 },
      { id: 'uptime-kuma', span: 1 },
    ],
  }),
}))

vi.mock('../summary-cards', () => ({
  SummaryCards: () => <div>Usage summary</div>,
}))

describe('dashboard setup guide', () => {
  beforeEach(() => {
    window.localStorage.clear()
    dashboardState.user.request_count = 1
  })

  test('hides the guide after every setup step is complete', () => {
    const view = render(<OverviewDashboard />)

    expect(
      screen.getByText('Setup guide complete').closest('.hidden')
    ).not.toBeNull()
    expect(screen.getByText('Usage summary')).toBeInTheDocument()

    view.unmount()
    window.localStorage.setItem(
      'dashboard_overview_setup_guide_expanded',
      'expanded'
    )
    render(<OverviewDashboard />)

    expect(screen.getByText('Get started').closest('.hidden')).not.toBeNull()
  })

  test('keeps the guide available while a setup step is incomplete', () => {
    dashboardState.user.request_count = 0

    render(<OverviewDashboard />)

    const hideGuide = screen.getByRole('button', {
      name: 'Hide setup guide',
    })
    hideGuide.focus()
    expect(hideGuide).toHaveFocus()
  })
})
