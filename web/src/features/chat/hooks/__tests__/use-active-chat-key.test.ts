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
import { beforeEach, expect, test, vi } from 'vitest'

import { getServerErrorMessage } from '@/lib/handle-server-error'

import { fetchChatKey } from '../use-active-chat-key'

const mocks = vi.hoisted(() => ({
  fetchTokenKey: vi.fn(),
  getApiKeys: vi.fn(),
}))

vi.mock('@/features/keys/api', () => mocks)

beforeEach(() => {
  mocks.fetchTokenKey.mockReset()
  mocks.getApiKeys.mockReset()
})

test('uses the selected Token ID without falling back to another Token', async () => {
  mocks.fetchTokenKey.mockResolvedValue({
    success: true,
    data: { key: 'selected-key' },
  })

  await expect(fetchChatKey(7)).resolves.toBe('sk-selected-key')
  expect(mocks.fetchTokenKey).toHaveBeenCalledWith(7)
  expect(mocks.getApiKeys).not.toHaveBeenCalled()
})

test('preserves a backend Token rejection for the visible error message', async () => {
  mocks.fetchTokenKey.mockResolvedValue({
    success: false,
    message: 'Token 不存在或不属于当前用户',
  })

  let keyError: unknown
  try {
    await fetchChatKey(7)
  } catch (error) {
    keyError = error
  }

  expect(getServerErrorMessage(keyError)).toBe('Token 不存在或不属于当前用户')
})
