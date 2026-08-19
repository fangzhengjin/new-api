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
import { describe, expect, test, vi } from 'vitest'

import { OverviewDashboard } from '../overview-dashboard'

vi.mock('@tanstack/react-query', () => ({
  useQuery: (options: { queryKey: string[] }) =>
    options.queryKey.includes('api-keys')
      ? { data: [], isFetched: true }
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
  useAuthStore: (selector: (state: { auth: { user: object } }) => unknown) =>
    selector({
      auth: {
        user: { request_count: 1, quota: 1, used_quota: 0, role: 1 },
      },
    }),
}))

vi.mock('../../../hooks/use-status-data', () => ({
  useApiInfo: () => ({ items: [] }),
  useDashboardContentVisibility: () => ({
    apiInfo: true,
    announcements: true,
    faq: true,
    uptimeKuma: true,
    panelLayout: [
      { id: 'faq', span: 2 },
      { id: 'api-info', span: 1 },
      { id: 'announcements', span: 1 },
      { id: 'uptime-kuma', span: 3 },
    ],
  }),
}))

vi.mock('../summary-cards', () => ({
  SummaryCards: () => <div>Usage summary</div>,
}))
vi.mock('../api-info-panel', () => ({
  ApiInfoPanel: () => <div data-testid='overview-content-panel'>API Info</div>,
}))
vi.mock('../announcements-panel', () => ({
  AnnouncementsPanel: () => (
    <div data-testid='overview-content-panel'>Announcements</div>
  ),
}))
vi.mock('../faq-panel', () => ({
  FAQPanel: () => <div data-testid='overview-content-panel'>FAQ</div>,
}))
vi.mock('../uptime-panel', () => ({
  UptimePanel: () => (
    <div data-testid='overview-content-panel'>Uptime Kuma</div>
  ),
}))

describe('dashboard content panel order', () => {
  test('renders enabled panels in the configured order', () => {
    render(<OverviewDashboard />)

    expect(
      screen
        .getAllByTestId('overview-content-panel')
        .map((panel) => panel.textContent)
    ).toEqual(['FAQ', 'API Info', 'Announcements', 'Uptime Kuma'])

    for (const panel of screen.getAllByTestId('overview-content-panel')) {
      expect(panel.parentElement).toHaveClass('h-full')
    }

    expect(
      screen.getByText('FAQ').closest('.lg\\:col-span-2')
    ).toBeInTheDocument()
    expect(
      screen.getByText('Uptime Kuma').closest('.xl\\:col-span-3')
    ).toBeInTheDocument()
  })
})
