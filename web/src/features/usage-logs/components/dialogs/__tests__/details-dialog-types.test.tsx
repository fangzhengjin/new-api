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
import { render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, test } from 'vitest'

import { LOG_TYPES } from '../../../constants'
import type { UsageLog } from '../../../data/schema'
import { DetailsDialog } from '../details-dialog'

Object.defineProperty(Element.prototype, 'getAnimations', {
  configurable: true,
  value: () => [],
})

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: false } },
})

beforeEach(() => {
  const freshAt = Date.now() + 60_000
  queryClient.setQueryData(['status'], {}, { updatedAt: freshAt })
  queryClient.setQueryData(
    ['pricing'],
    { data: [], vendors: [] },
    { updatedAt: freshAt }
  )
})

afterEach(() => queryClient.clear())

function renderDetails(log: UsageLog, isAdmin: boolean) {
  return render(
    <QueryClientProvider client={queryClient}>
      <DetailsDialog
        log={log}
        isAdmin={isAdmin}
        isRoot={false}
        open
        onOpenChange={() => undefined}
      />
    </QueryClientProvider>
  )
}

function createLog(type: number, overrides: Partial<UsageLog> = {}): UsageLog {
  return {
    id: type + 1,
    user_id: 1,
    created_at: 1000,
    type,
    content: '',
    username: 'test',
    display_name: '',
    token_name: '',
    model_name: '',
    quota: 0,
    prompt_tokens: 0,
    completion_tokens: 0,
    use_time: 0,
    is_stream: false,
    channel: 0,
    channel_name: '',
    token_id: 0,
    group: '',
    ip: '',
    other: '',
    request_id: `request-${type}`,
    upstream_request_id: '',
    ...overrides,
  }
}

describe('Log details dialog types', () => {
  test.each(LOG_TYPES)('shows the $label label', ({ value, label }) => {
    renderDetails(createLog(value), true)

    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getByText(label)).toBeInTheDocument()
  })

  test('keeps management details free of request billing fields and duplicate export text', () => {
    renderDetails(
      createLog(3, {
        content: 'raw export fallback',
        token_name: 'irrelevant-token',
        group: 'irrelevant-group',
        channel: 99,
        other: JSON.stringify({
          op: {
            action: 'user.update',
            params: { username: 'target', id: 42 },
          },
          admin_info: {
            admin_username: 'operator',
            admin_id: 7,
            auth_method: 'session',
          },
        }),
      }),
      true
    )

    expect(screen.getByText('Operation Audit Info')).toBeInTheDocument()
    expect(screen.getByText('Operator Admin')).toBeInTheDocument()
    expect(screen.queryByText('Billing Path')).not.toBeInTheDocument()
    expect(screen.queryByText('irrelevant-token')).not.toBeInTheDocument()
    expect(screen.queryByText('irrelevant-group')).not.toBeInTheDocument()
    expect(screen.queryByText('raw export fallback')).not.toBeInTheDocument()
  })

  test('shows localized management operation without admin-only details to regular users', () => {
    renderDetails(
      createLog(3, {
        content: 'raw export fallback',
        other: JSON.stringify({
          op: {
            action: 'user.update',
            params: { username: 'target', id: 42 },
          },
          admin_info: {
            admin_username: 'operator',
            admin_id: 7,
            auth_method: 'session',
          },
          audit_info: {
            method: 'PUT',
            route: '/api/user',
            status: 200,
            success: true,
          },
        }),
      }),
      false
    )

    expect(screen.getByText('Updated user target (ID: 42)')).toBeInTheDocument()
    expect(screen.queryByText('raw export fallback')).not.toBeInTheDocument()
    expect(screen.queryByText('Operator Admin')).not.toBeInTheDocument()
    expect(screen.queryByText('Authentication Method')).not.toBeInTheDocument()
    expect(screen.queryByText('PUT /api/user')).not.toBeInTheDocument()
  })

  test('keeps request diagnostics on error logs when billing-path data exists', () => {
    renderDetails(
      createLog(5, {
        token_name: 'request-token',
        group: 'request-group',
        channel: 9,
        other: JSON.stringify({
          admin_info: { usage_billing_path: 'local' },
        }),
      }),
      true
    )

    expect(screen.getByText('Billing Path')).toBeInTheDocument()
    expect(screen.getByText('Local Billing')).toBeInTheDocument()
    expect(screen.getByText('request-token')).toBeInTheDocument()
    expect(screen.getByText('request-group')).toBeInTheDocument()
  })
})
