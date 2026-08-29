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
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { CycleQuotaSettingsPage } from '../index'

const { mutateAsync } = vi.hoisted(() => ({
  mutateAsync: vi.fn().mockResolvedValue({ success: true }),
}))

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useParams: () => ({}),
    useBlocker: () => ({ status: 'idle' }),
  }
})

vi.mock('../../hooks/use-system-options', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('../../hooks/use-system-options')>()
  return {
    ...actual,
    useSystemOptions: () => ({ data: undefined, isLoading: false }),
  }
})

vi.mock('../../hooks/use-update-option', () => ({
  useUpdateOption: () => ({ mutateAsync, isPending: false }),
}))

describe('temporary quota project settings', () => {
  beforeEach(() => {
    mutateAsync.mockClear()
  })

  test('focuses one new draft without showing a required error before submit', async () => {
    const user = userEvent.setup()
    render(<CycleQuotaSettingsPage />)

    const addButton = screen.getByRole('button', { name: 'Add project' })
    await user.click(addButton)

    const projectName = screen.getByRole('textbox', { name: 'Project name' })
    expect(projectName).toHaveFocus()
    expect(addButton).toBeDisabled()
    expect(
      screen.getByRole('button', { name: 'Switch to JSON' })
    ).toBeDisabled()
    expect(
      screen.queryByText('Project name is required')
    ).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Save Changes' }))

    expect(await screen.findByText('Project name is required')).toBeVisible()
    expect(projectName).toHaveAttribute('aria-invalid', 'true')
    expect(mutateAsync).not.toHaveBeenCalled()
  })

  test('keeps every completed row in a non-shrinking project section', async () => {
    const user = userEvent.setup()
    render(<CycleQuotaSettingsPage />)

    const addButton = screen.getByRole('button', { name: 'Add project' })
    for (const name of ['alpha', 'beta', 'gamma', 'delta', 'epsilon']) {
      await user.click(addButton)
      const projectNames = screen.getAllByRole('textbox', {
        name: 'Project name',
      })
      const projectName = projectNames.at(-1)
      if (!projectName) throw new Error('Project draft was not rendered')
      await user.type(projectName, name)
    }

    expect(
      screen.getAllByRole('textbox', { name: 'Project name' })
    ).toHaveLength(5)

    const projectCard = screen
      .getByText('Temporary quota projects')
      .closest('[data-slot=card]')
    expect(projectCard?.closest('section')).toHaveClass('shrink-0')
  })
})
