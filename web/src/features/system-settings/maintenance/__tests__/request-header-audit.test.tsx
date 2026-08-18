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
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { LogSettingsSection } from '../log-settings-section'

const mocks = vi.hoisted(() => ({
  mutateAsync: vi.fn(),
  get: vi.fn(),
}))

vi.mock('@/features/system-settings/hooks/use-update-option', () => ({
  useUpdateOption: () => ({
    mutateAsync: mocks.mutateAsync,
    isPending: false,
  }),
}))

vi.mock('@/features/system-settings/api', () => ({
  getCurrentLogCleanupTask: vi.fn().mockResolvedValue({
    success: true,
    data: null,
  }),
  getSystemTask: vi.fn(),
  startLogCleanupTask: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    get: mocks.get,
    delete: vi.fn(),
  },
}))

beforeEach(() => {
  mocks.mutateAsync.mockReset().mockResolvedValue({ success: true })
  mocks.get.mockReset().mockResolvedValue({ data: { success: false } })
})

describe('Request header audit settings', () => {
  test('renders the final section as an inline rule table without source or edit controls', () => {
    render(
      <LogSettingsSection
        defaultEnabled
        defaultRules='[{"name":"Sec-Fetch-*","record":false,"forward":false}]'
        builtInRules='[{"name":"Sec-Fetch-*","record":false,"forward":false}]'
        systemRules='[{"name":"Authorization","record":false,"forward":true}]'
        capacityBytes={16 * 1024}
      />
    )

    const auditHeading = screen.getByRole('heading', {
      name: 'Request header audit',
    })
    const auditSection = auditHeading.closest('section')
    const serverLogHeading = screen.getByRole('heading', {
      name: 'Server Log Management',
    })
    if (!auditSection) throw new Error('audit section was not rendered')

    expect(
      serverLogHeading.compareDocumentPosition(auditSection) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    expect(
      screen.getByRole('columnheader', { name: 'Header rule' })
    ).toBeVisible()
    expect(screen.getByRole('columnheader', { name: 'Record' })).toBeVisible()
    expect(screen.getByRole('columnheader', { name: 'Forward' })).toBeVisible()
    expect(screen.getByRole('columnheader', { name: 'Actions' })).toBeVisible()
    expect(screen.queryByText('Source')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Edit' })
    ).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Add rule' })).toBeVisible()
  })

  test('edits user rules inline while every system control stays disabled', async () => {
    const user = userEvent.setup()
    const { container } = render(
      <LogSettingsSection
        defaultEnabled
        defaultRules='[{"name":"Sec-Fetch-*","record":false,"forward":false}]'
        builtInRules='[{"name":"Content-Length","record":false,"forward":true}]'
        systemRules='[{"name":"Authorization","record":false,"forward":true}]'
        capacityBytes={16 * 1024}
      />
    )

    const systemRule = screen.getByRole('textbox', {
      name: 'Header rule: Authorization',
    })
    expect(systemRule).toBeDisabled()
    expect(
      screen.getByRole('checkbox', { name: 'Record Authorization' })
    ).toHaveAttribute('aria-disabled', 'true')
    expect(
      screen.getByRole('checkbox', { name: 'Forward Authorization' })
    ).toHaveAttribute('aria-disabled', 'true')
    expect(
      screen.getByRole('button', { name: 'Delete Authorization rule' })
    ).toBeDisabled()

    await user.click(screen.getByRole('button', { name: 'Add rule' }))
    const newRule = screen.getByRole('textbox', { name: 'Header rule: 2' })
    await user.type(newRule, 'X-Debug-*')
    await user.click(screen.getByRole('checkbox', { name: 'Record X-Debug-*' }))
    const form = container.querySelector('form')
    if (!form) throw new Error('settings form was not rendered')
    fireEvent.submit(form)
    await waitFor(() =>
      expect(mocks.mutateAsync).toHaveBeenCalledWith([
        {
          key: 'RequestHeaderRules',
          value:
            '[{"name":"Sec-Fetch-*","record":false,"forward":false},{"name":"X-Debug-*","record":true,"forward":false}]',
        },
      ])
    )
  })

  test('rejects invalid, duplicate, and system-owned rule names', async () => {
    const user = userEvent.setup()
    const { container } = render(
      <LogSettingsSection
        defaultEnabled
        defaultRules='[{"name":"X-Debug-*","record":false,"forward":false}]'
        builtInRules='[]'
        systemRules='[{"name":"Forwarded","record":false,"forward":false}]'
        capacityBytes={16 * 1024}
      />
    )
    const form = container.querySelector('form')
    if (!form) throw new Error('settings form was not rendered')

    await user.click(screen.getByRole('button', { name: 'Add rule' }))
    const newRule = screen.getByRole('textbox', { name: 'Header rule: 2' })

    await user.type(newRule, 'Sec*Fetch')
    fireEvent.submit(form)
    expect(
      await screen.findByText(
        'Use an HTTP header name; only a trailing * wildcard is supported'
      )
    ).toBeVisible()
    expect(mocks.mutateAsync).not.toHaveBeenCalled()

    await user.clear(newRule)
    await user.type(newRule, 'x-debug-*')
    fireEvent.submit(form)
    expect(
      await screen.findByText('Duplicate request header rule')
    ).toBeVisible()
    expect(mocks.mutateAsync).not.toHaveBeenCalled()

    await user.clear(newRule)
    await user.type(newRule, 'Forwarded')
    fireEvent.submit(form)
    expect(
      await screen.findByText(
        'This request header rule is managed by the system'
      )
    ).toBeVisible()
    expect(mocks.mutateAsync).not.toHaveBeenCalled()
  })

  test('restores the built-in rule draft without saving immediately', async () => {
    const user = userEvent.setup()
    const { container } = render(
      <LogSettingsSection
        defaultEnabled
        defaultRules='[{"name":"X-Debug-*","record":false,"forward":false}]'
        builtInRules='[{"name":"Content-Length","record":false,"forward":true}]'
        systemRules='[{"name":"Authorization","record":false,"forward":true}]'
        capacityBytes={16 * 1024}
      />
    )

    const restoreButton = screen.getByRole('button', {
      name: 'Restore defaults',
    })
    restoreButton.focus()
    expect(restoreButton).toHaveFocus()
    await user.keyboard('{Enter}')

    expect(
      screen.getByRole('textbox', { name: 'Header rule: Content-Length' })
    ).toHaveValue('Content-Length')
    expect(mocks.mutateAsync).not.toHaveBeenCalled()
    expect(
      screen.getByRole('button', { name: 'Restore defaults' })
    ).toBeDisabled()

    fireEvent.submit(container.querySelector('form') as HTMLFormElement)
    await waitFor(() =>
      expect(mocks.mutateAsync).toHaveBeenCalledWith([
        {
          key: 'RequestHeaderRules',
          value: '[{"name":"Content-Length","record":false,"forward":true}]',
        },
      ])
    )
  })
})
