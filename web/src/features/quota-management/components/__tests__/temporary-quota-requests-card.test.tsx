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
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'

import type { TemporaryQuotaRequest } from '@/features/temporary-quota/types'

const mocks = vi.hoisted(() => ({
  listTemporaryQuotaRequests: vi.fn(),
  approveTemporaryQuotaRequest: vi.fn(),
  rejectTemporaryQuotaRequest: vi.fn(),
}))

vi.mock('../../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../api')>()),
  listTemporaryQuotaRequests: mocks.listTemporaryQuotaRequests,
  approveTemporaryQuotaRequest: mocks.approveTemporaryQuotaRequest,
  rejectTemporaryQuotaRequest: mocks.rejectTemporaryQuotaRequest,
}))

const { TemporaryQuotaRequestsCard } =
  await import('../temporary-quota-requests-card')

const request: TemporaryQuotaRequest = {
  id: 7,
  user_id: 11,
  username: 'review-user',
  display_name: 'Review User',
  requested_quota: '1000000',
  project: 'Project A',
  reason: 'Need more quota',
  status: 'pending',
  review_reason: '',
  created_at: 1_786_886_400,
}

test('associates local approval errors with the reviewed fields', async () => {
  mocks.listTemporaryQuotaRequests.mockResolvedValue([request])
  const user = userEvent.setup()
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <TemporaryQuotaRequestsCard />
    </QueryClientProvider>
  )

  expect(await screen.findByText('Review User')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Approve' }))

  const reason = screen.getByLabelText('Review reason')
  const amount = screen.getByLabelText(/Approved quota/)
  const submit = screen.getByRole('button', { name: 'Approve and grant' })
  await user.click(submit)

  expect(reason).toHaveAttribute('aria-invalid', 'true')
  expect(reason).toHaveAttribute(
    'aria-describedby',
    'temporary-quota-review-reason-error'
  )
  expect(
    document.querySelector('#temporary-quota-review-reason-error')
  ).toHaveAttribute('role', 'alert')

  await user.type(reason, 'Reviewed')
  await user.clear(amount)
  await user.type(amount, '999999999')
  await user.click(submit)

  expect(amount).toHaveAttribute('aria-invalid', 'true')
  expect(amount).toHaveAttribute(
    'aria-describedby',
    'temporary-quota-approved-amount-error'
  )
  expect(
    document.querySelector('#temporary-quota-approved-amount-error')
  ).toHaveAttribute('role', 'alert')
  expect(mocks.approveTemporaryQuotaRequest).not.toHaveBeenCalled()
})

test('accepts a 500-character review reason by Unicode characters', async () => {
  mocks.listTemporaryQuotaRequests.mockResolvedValue([request])
  mocks.approveTemporaryQuotaRequest.mockResolvedValue({})
  const user = userEvent.setup()
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <TemporaryQuotaRequestsCard />
    </QueryClientProvider>
  )

  expect(await screen.findByText('Review User')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Approve' }))
  const reason = screen.getByLabelText('Review reason')
  const unicodeReason = '😀'.repeat(300)
  expect(reason).not.toHaveAttribute('maxlength')
  fireEvent.change(reason, { target: { value: unicodeReason } })
  await user.click(screen.getByRole('button', { name: 'Approve and grant' }))

  await waitFor(() =>
    expect(mocks.approveTemporaryQuotaRequest).toHaveBeenCalledWith(
      request.id,
      request.requested_quota,
      unicodeReason
    )
  )
})
