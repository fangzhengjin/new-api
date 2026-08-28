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
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
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

const cdnRuleGroups = JSON.stringify([
  {
    vendor: 'Cloudflare',
    rules: [{ name: 'CF-*', record: false, forward: false }],
  },
  {
    vendor: 'Tencent Cloud EdgeOne',
    rules: [{ name: 'EO-*', record: false, forward: false }],
  },
])

const cdnRules = [
  { name: 'CF-*', record: false, forward: false },
  { name: 'EO-*', record: false, forward: false },
]

describe('Request header audit settings', () => {
  test('renders general, CDN, and system rules as separate layers', () => {
    render(
      <LogSettingsSection
        defaultEnabled
        defaultRules={JSON.stringify([
          { name: 'X-Request-ID', record: false, forward: false },
          ...cdnRules,
        ])}
        builtInRules={JSON.stringify([
          { name: 'X-Request-ID', record: false, forward: false },
          ...cdnRules,
        ])}
        cdnRuleGroups={cdnRuleGroups}
        systemRules='[{"name":"Content-Type","record":false,"forward":true}]'
        capacityBytes={16 * 1024}
      />
    )

    const auditHeading = screen.getByRole('heading', {
      name: 'Request header processing rules',
    })
    const auditSection = auditHeading.closest('[data-slot="card"]')
    const serverLogHeading = screen.getByRole('heading', {
      name: 'Server Log Management',
    })
    if (!auditSection) throw new Error('audit section was not rendered')

    expect(
      serverLogHeading.compareDocumentPosition(auditSection) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    const generalHeading = screen.getByRole('heading', {
      name: 'General request headers',
    })
    const cdnHeading = screen.getByRole('heading', {
      name: 'CDN request headers',
    })
    const systemHeading = screen.getByRole('heading', {
      name: 'System-protected request headers',
    })
    expect(
      generalHeading.compareDocumentPosition(cdnHeading) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    expect(
      cdnHeading.compareDocumentPosition(systemHeading) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Add rule' })).toBeVisible()
    expect(
      screen.getByRole('textbox', { name: 'Header rule: X-Request-ID' })
    ).toBeVisible()
    expect(screen.getByText('Cloudflare')).toBeVisible()
    expect(screen.getByText('CF-*', { selector: 'code' })).toBeVisible()
    expect(
      screen.queryByRole('textbox', { name: 'Header rule: CF-*' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Delete CF-* rule' })
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole('button', {
        name: 'View system-managed request headers',
      })
    ).toHaveAttribute('aria-expanded', 'false')
    expect(
      screen.queryByText('Content-Type', { selector: 'code' })
    ).not.toBeInTheDocument()
  })

  test('edits general and CDN policies while system rules stay read-only', async () => {
    const user = userEvent.setup()
    const { container } = render(
      <LogSettingsSection
        defaultEnabled
        defaultRules={JSON.stringify([
          { name: 'X-Request-ID', record: false, forward: false },
          ...cdnRules,
        ])}
        builtInRules={JSON.stringify([
          { name: 'X-Request-ID', record: false, forward: false },
          ...cdnRules,
        ])}
        cdnRuleGroups={cdnRuleGroups}
        systemRules='[{"name":"Connection","record":false,"forward":false},{"name":"Content-Type","record":false,"forward":true}]'
        capacityBytes={16 * 1024}
      />
    )

    const protectionsButton = screen.getByRole('button', {
      name: 'View system-managed request headers',
    })
    protectionsButton.focus()
    expect(protectionsButton).toHaveFocus()
    await user.keyboard('{Enter}')
    expect(protectionsButton).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('Content-Type', { selector: 'code' })).toBeVisible()
    expect(
      screen.getByRole('heading', { name: 'Not recorded · Not forwarded' })
    ).toBeVisible()
    expect(
      screen.getByRole('heading', { name: 'Not recorded · Forwardable' })
    ).toBeVisible()
    expect(
      screen.queryByRole('textbox', { name: 'Header rule: Content-Type' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('checkbox', { name: 'Record Content-Type' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Delete Content-Type rule' })
    ).not.toBeInTheDocument()

    const cdnSection = screen
      .getByRole('heading', { name: 'CDN request headers' })
      .closest('section')
    if (!cdnSection) throw new Error('CDN section was not rendered')
    await user.click(
      within(cdnSection).getByRole('checkbox', { name: 'Forward CF-*' })
    )
    await user.click(screen.getByRole('button', { name: 'Add rule' }))
    const newRule = screen.getByRole('textbox', { name: 'Header rule: 4' })
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
            '[{"name":"X-Request-ID","record":false,"forward":false},{"name":"CF-*","record":false,"forward":true},{"name":"EO-*","record":false,"forward":false},{"name":"X-Debug-*","record":true,"forward":false}]',
        },
      ])
    )
  })

  test('rejects invalid, duplicate, and system-owned rule names', async () => {
    const user = userEvent.setup()
    const { container } = render(
      <LogSettingsSection
        defaultEnabled
        defaultRules={JSON.stringify([
          { name: 'X-Debug-*', record: false, forward: false },
          ...cdnRules,
        ])}
        builtInRules={JSON.stringify(cdnRules)}
        cdnRuleGroups={cdnRuleGroups}
        systemRules='[{"name":"*authorization","record":false,"forward":true}]'
        capacityBytes={16 * 1024}
      />
    )
    const form = container.querySelector('form')
    if (!form) throw new Error('settings form was not rendered')

    await user.click(screen.getByRole('button', { name: 'Add rule' }))
    const newRule = screen.getByRole('textbox', { name: 'Header rule: 4' })

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
    await user.type(newRule, 'Authorization')
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
        defaultRules={JSON.stringify([
          { name: 'X-Debug-*', record: false, forward: false },
          ...cdnRules,
        ])}
        builtInRules={JSON.stringify([
          { name: 'X-Request-ID', record: false, forward: false },
          { name: 'CF-*', record: true, forward: false },
          { name: 'EO-*', record: false, forward: false },
        ])}
        cdnRuleGroups={cdnRuleGroups}
        systemRules='[{"name":"Content-Type","record":false,"forward":true}]'
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
      screen.getByRole('textbox', { name: 'Header rule: X-Request-ID' })
    ).toHaveValue('X-Request-ID')
    expect(screen.getByRole('checkbox', { name: 'Record CF-*' })).toBeChecked()
    expect(mocks.mutateAsync).not.toHaveBeenCalled()
    expect(
      screen.getByRole('button', { name: 'Restore defaults' })
    ).toBeDisabled()

    fireEvent.submit(container.querySelector('form') as HTMLFormElement)
    await waitFor(() =>
      expect(mocks.mutateAsync).toHaveBeenCalledWith([
        {
          key: 'RequestHeaderRules',
          value:
            '[{"name":"X-Request-ID","record":false,"forward":false},{"name":"CF-*","record":true,"forward":false},{"name":"EO-*","record":false,"forward":false}]',
        },
      ])
    )
  })
})
