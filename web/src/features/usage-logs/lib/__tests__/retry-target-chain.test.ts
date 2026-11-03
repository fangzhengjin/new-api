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
import { describe, expect, test } from 'vitest'

import { getRetryTargetChain } from '../format'

describe('retry target chain', () => {
  test('shows ordered channel, key, and failed-attempt details', () => {
    expect(
      getRetryTargetChain(
        {
          retry_targets: [
            {
              channel_id: 2,
              multi_key_index: 0,
              status_code: 429,
              error: 'rate limited',
            },
            { channel_id: 2, multi_key_index: 7 },
            { channel_id: 5 },
          ],
        },
        'Key'
      )
    ).toEqual(['#2 / Key 0 · 429: rate limited', '#2 / Key 7', '#5'])
  })

  test('marks truncated chains and falls back for historical logs', () => {
    expect(
      getRetryTargetChain(
        {
          retry_targets: [{ channel_id: 2 }],
          retry_targets_truncated: true,
        },
        'Key'
      )
    ).toEqual(['#2', '…'])
    expect(getRetryTargetChain({ use_channel: [2, '5'] }, 'Key')).toEqual([
      '2',
      '5',
    ])
    expect(
      getRetryTargetChain(
        { retry_targets: [{ channel_id: Number.NaN }], use_channel: ['8'] },
        'Key'
      )
    ).toEqual(['8'])
  })
})
