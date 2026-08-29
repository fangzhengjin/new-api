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
import { fireEvent, render, screen } from '@testing-library/react'
import { expect, test, vi } from 'vitest'

import type { SelfTemporaryQuotaRequest } from '../../types'
import { TemporaryQuotaDetailSheet } from '../temporary-quota-detail-sheet'

const request: SelfTemporaryQuotaRequest = {
  id: 7,
  requested_quota: '2000000',
  project: 'Project A',
  reason: 'Keep production work running',
  status: 'executed',
  approved_quota: '1500000',
  review_reason: 'Approved for current delivery',
  created_at: 1_800_000_000,
  reviewed_at: 1_800_000_100,
  executed_at: 1_800_000_120,
}

test('the detail sheet presents the application and result without internal decision data', () => {
  const onOpenChange = vi.fn()
  render(
    <TemporaryQuotaDetailSheet request={request} onOpenChange={onOpenChange} />
  )

  expect(
    screen.getByRole('heading', { name: 'Temporary quota request details' })
  ).toBeInTheDocument()
  expect(screen.getByText('Project A')).toBeInTheDocument()
  expect(screen.getByText('Granted')).toBeInTheDocument()
  expect(screen.getByText('Approved for current delivery')).toBeInTheDocument()
  expect(document.body).not.toHaveTextContent('auto')
  expect(document.body).not.toHaveTextContent('manual')

  fireEvent.click(screen.getByRole('button', { name: 'Close' }))
  expect(onOpenChange).toHaveBeenCalledWith(false)
})
