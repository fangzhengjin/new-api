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

import { launchChatPreset } from '../api'

const mocks = vi.hoisted(() => ({ post: vi.fn() }))

vi.mock('@/lib/api', () => ({ api: { post: mocks.post } }))

describe('chat preset launch API', () => {
  beforeEach(() => mocks.post.mockReset())

  test('sends the selected preset name and Token id', async () => {
    mocks.post.mockResolvedValue({
      data: {
        success: true,
        data: { launch_url: 'https://canvas.example/#code=one-time' },
      },
    })

    const launchURL = await launchChatPreset('Canvas', 42)

    expect(launchURL).toBe('https://canvas.example/#code=one-time')
    expect(mocks.post).toHaveBeenCalledWith(
      '/api/integrations/launch',
      { preset_name: 'Canvas', token_id: 42 },
      { skipBusinessError: true, skipErrorHandler: true }
    )
  })
})
