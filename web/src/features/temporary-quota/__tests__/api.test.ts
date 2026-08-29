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
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { getServerErrorMessage } from '@/lib/handle-server-error'

import { submitSelfTemporaryQuota } from '../api'

const mocks = vi.hoisted(() => ({ post: vi.fn() }))

vi.mock('@/lib/api', () => ({ api: { post: mocks.post } }))

describe('temporary quota API', () => {
  beforeEach(() => mocks.post.mockReset())

  test('preserves a backend application rejection for the visible error message', async () => {
    mocks.post.mockResolvedValue({
      data: {
        success: false,
        message: '当前没有活跃周期',
      },
    })

    let submittedError: unknown
    try {
      await submitSelfTemporaryQuota({
        idempotency_key: 'request-1',
        requested_quota: '1000',
        project: 'Project A',
        reason: 'Need additional quota',
      })
    } catch (error) {
      submittedError = error
    }

    expect(getServerErrorMessage(submittedError)).toBe('当前没有活跃周期')
  })
})
