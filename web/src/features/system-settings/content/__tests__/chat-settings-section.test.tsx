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
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { ChatSettingsSection } from '../chat-settings-section'

const { mutateAsync } = vi.hoisted(() => ({
  mutateAsync: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('../../hooks/use-update-option', () => ({
  useUpdateOption: () => ({ mutateAsync, isPending: false }),
}))

describe('chat preset settings', () => {
  beforeEach(() => {
    mutateAsync.mockClear()
  })

  test('shows variable notes without implementation details or URL placement rules', () => {
    render(
      <SettingsPageProvider actionsContainer={document.createElement('div')}>
        <ChatSettingsSection
          defaultValue='[]'
          defaultThreshold={3}
          builtInValue='[]'
          builtInThreshold={3}
        />
      </SettingsPageProvider>
    )

    expect(screen.getByRole('columnheader', { name: 'Variable' })).toBeVisible()
    expect(screen.getByRole('columnheader', { name: 'Notes' })).toBeVisible()
    expect(
      screen.queryByRole('columnheader', { name: 'Replaced by' })
    ).not.toBeInTheDocument()
    expect(screen.getByText('{authCode}')).toBeVisible()
    expect(screen.getByText('{theme}')).toBeVisible()
    expect(
      screen.getByText('One-time authorization code, valid for one minute.')
    ).toBeVisible()
  })

  test('restores the preset list and collapse threshold before saving', async () => {
    const actions = document.createElement('div')
    document.body.append(actions)
    const currentPresets = JSON.stringify([
      { name: 'Custom', url: 'https://custom.example', enabled: true },
    ])
    const builtInPresets = JSON.stringify([
      { name: 'Default', url: 'https://default.example', enabled: true },
    ])
    const { container } = render(
      <SettingsPageProvider actionsContainer={actions}>
        <ChatSettingsSection
          defaultValue={currentPresets}
          defaultThreshold={9}
          builtInValue={builtInPresets}
          builtInThreshold={3}
        />
      </SettingsPageProvider>
    )

    fireEvent.click(screen.getByRole('button', { name: 'Restore defaults' }))
    expect(screen.getByRole('spinbutton')).toHaveValue(3)
    expect(mutateAsync).not.toHaveBeenCalled()
    expect(
      screen.getByRole('button', { name: 'Restore defaults' })
    ).toBeDisabled()

    fireEvent.submit(container.querySelector('form') as HTMLFormElement)
    await waitFor(() =>
      expect(mutateAsync).toHaveBeenCalledWith([
        { key: 'ChatMenuCollapseThreshold', value: 3 },
        { key: 'Chats', value: builtInPresets },
      ])
    )
    actions.remove()
  })
})
