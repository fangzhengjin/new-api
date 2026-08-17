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
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { OverviewLayoutSection } from '../overview-layout-section'

const { mutateAsync } = vi.hoisted(() => ({
  mutateAsync: vi.fn().mockResolvedValue({ success: true }),
}))

vi.mock('@tanstack/react-router', () => ({
  Link: (props: { children?: ReactNode; to: string }) => (
    <a href={props.to}>{props.children}</a>
  ),
}))

vi.mock('../../hooks/use-update-option', () => ({
  useUpdateOption: () => ({ mutateAsync, isPending: false }),
}))

describe('overview layout settings', () => {
  beforeEach(() => {
    mutateAsync.mockClear()
  })

  test('keeps disabled panels sortable and saves the new order', async () => {
    const actionsContainer = document.createElement('div')
    document.body.appendChild(actionsContainer)

    render(
      <SettingsPageProvider actionsContainer={actionsContainer}>
        <OverviewLayoutSection
          defaultValue='["api-info","announcements","faq","uptime-kuma"]'
          enabledPanels={{
            'api-info': true,
            announcements: true,
            faq: true,
            'uptime-kuma': false,
          }}
        />
      </SettingsPageProvider>
    )

    expect(screen.getByText('Uptime Kuma').closest('li')).toHaveTextContent(
      'Disabled'
    )

    fireEvent.click(screen.getByRole('button', { name: 'Move API Info down' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }))

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith({
        key: 'console_setting.overview_panel_order',
        value: '["announcements","api-info","faq","uptime-kuma"]',
      })
    })
  })
})
