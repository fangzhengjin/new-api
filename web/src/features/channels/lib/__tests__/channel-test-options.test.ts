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
import assert from 'node:assert/strict'

import { describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import {
  isChannelTestStreamSupported,
  handleTestChannel,
} from '../channel-actions'

describe('channel test stream options', () => {
  test.each([
    { stream: undefined, expected: true },
    { stream: false, expected: false },
  ])('sends stream=$expected when the selected stream is $stream', async ({
    stream,
    expected,
  }) => {
    const get = vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true },
    })

    await handleTestChannel(7, {
      testModel: 'gpt-4.1',
      endpointType: 'chat-completions',
      stream,
      silent: true,
    })

    expect(get).toHaveBeenCalledWith(
      '/api/channel/test/7',
      expect.objectContaining({
        params: {
          model: 'gpt-4.1',
          endpoint_type: 'chat-completions',
          stream: expected,
        },
      })
    )
  })

  test('allows image streaming while keeping non-stream endpoints disabled', () => {
    assert.equal(isChannelTestStreamSupported('image-generation'), true)
    assert.equal(isChannelTestStreamSupported('embeddings'), false)
  })
})
