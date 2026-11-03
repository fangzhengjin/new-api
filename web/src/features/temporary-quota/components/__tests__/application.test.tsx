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
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { createTemporaryQuotaFormSchema } from '../../lib/temporary-quota-form'
import type { TemporaryQuotaOverview } from '../../types'
import { TemporaryQuotaDialog } from '../temporary-quota-dialog'
import { TemporaryQuotaPrimaryAction } from '../temporary-quota-primary-action'

const availableOverview: TemporaryQuotaOverview = {
  current_quota: '1240000',
  can_request: true,
  unavailable_reason: '',
  projects: ['Project A'],
}

afterEach(() => vi.unstubAllGlobals())

test('the request form counts Unicode characters like the backend', () => {
  const schema = createTemporaryQuotaFormSchema({
    amountPattern: /^\d+$/,
    amountError: 'invalid amount',
    amountIsValid: () => true,
    projectError: 'project required',
    reasonRequiredError: 'reason required',
    reasonLengthError: 'reason too long',
  })

  expect(
    schema.safeParse({ amount: '1', project: 'A', reason: '😀'.repeat(300) })
      .success
  ).toBe(true)
  expect(
    schema.safeParse({ amount: '1', project: 'A', reason: '审'.repeat(501) })
      .success
  ).toBe(false)
})

test('the request dialog works when randomUUID is unavailable', () => {
  vi.stubGlobal('crypto', {
    getRandomValues: (bytes: Uint8Array) => bytes.fill(7),
  })
  const queryClient = new QueryClient()

  render(
    <QueryClientProvider client={queryClient}>
      <TemporaryQuotaDialog
        open
        overview={availableOverview}
        onOpenChange={() => undefined}
      />
    </QueryClientProvider>
  )

  expect(
    screen.getByRole('heading', { name: 'Request temporary quota' })
  ).toBeInTheDocument()
})

test('an unavailable application remains visible and disabled', () => {
  render(
    <TemporaryQuotaPrimaryAction
      overview={{
        ...availableOverview,
        can_request: false,
        unavailable_reason: 'No active cycle',
      }}
      onOpen={() => undefined}
    />
  )

  expect(
    screen.getByRole('button', { name: 'Request temporary quota' })
  ).toBeDisabled()
})

test('the application action remains disabled while availability is loading', () => {
  render(<TemporaryQuotaPrimaryAction onOpen={() => undefined} />)
  expect(
    screen.getByRole('button', { name: 'Request temporary quota' })
  ).toBeDisabled()
})

test('an exhausted reserve keeps the application action disabled with an accessible reason', async () => {
  const user = userEvent.setup()
  render(
    <TemporaryQuotaPrimaryAction
      overview={{
        ...availableOverview,
        can_request: false,
        unavailable_reason: 'Temporary quota reserve is fully used',
      }}
      onOpen={() => undefined}
    />
  )

  expect(
    screen.getByRole('button', { name: 'Request temporary quota' })
  ).toBeDisabled()
  expect(
    screen.getByLabelText(
      'Request temporary quota: Temporary quota reserve is fully used'
    )
  ).toBeInTheDocument()
  await user.tab()
  expect(
    screen.getByLabelText(
      'Request temporary quota: Temporary quota reserve is fully used'
    )
  ).toHaveFocus()
})

test('an available application action opens the request flow with the keyboard', async () => {
  const user = userEvent.setup()
  const onOpen = vi.fn()
  render(
    <TemporaryQuotaPrimaryAction overview={availableOverview} onOpen={onOpen} />
  )

  await user.tab()
  expect(
    screen.getByRole('button', { name: 'Request temporary quota' })
  ).toHaveFocus()
  await user.keyboard('{Enter}')
  expect(onOpen).toHaveBeenCalledOnce()
})

test('the request dialog shows account balance without internal approval rules', () => {
  const onOpenChange = vi.fn()
  const queryClient = new QueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <TemporaryQuotaDialog
        open
        overview={availableOverview}
        onOpenChange={onOpenChange}
      />
    </QueryClientProvider>
  )

  expect(
    screen.getByRole('heading', { name: 'Request temporary quota' })
  ).toBeInTheDocument()
  expect(screen.getByText('Current account balance')).toBeInTheDocument()
  expect(screen.getByLabelText('Project')).toBeInTheDocument()
  expect(screen.getByLabelText(/Requested quota/)).toBeInTheDocument()
  expect(screen.getByLabelText('Reason')).toBeInTheDocument()
  expect(screen.getByLabelText('Reason')).not.toHaveAttribute('maxlength')
  expect(document.body).not.toHaveTextContent('Automatic')

  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  expect(onOpenChange).toHaveBeenCalledWith(false)
})

test('the request dialog associates validation errors with each field', async () => {
  const queryClient = new QueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <TemporaryQuotaDialog
        open
        overview={availableOverview}
        onOpenChange={() => undefined}
      />
    </QueryClientProvider>
  )

  fireEvent.click(screen.getByRole('button', { name: 'Submit' }))
  await screen.findByText('Project is required')

  for (const [label, errorId] of [
    ['Project', 'temporary-quota-project-error'],
    [/Requested quota/, 'temporary-quota-amount-error'],
    ['Reason', 'temporary-quota-reason-error'],
  ] as const) {
    const field = screen.getByLabelText(label)
    expect(field).toHaveAttribute('aria-invalid', 'true')
    expect(field).toHaveAttribute('aria-describedby', errorId)
    expect(document.querySelector(`#${errorId}`)).toHaveAttribute(
      'role',
      'alert'
    )
  }
})
