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

import { describe, test } from 'vitest'

import { getCalendarDateRange } from '@/lib/time'

describe('dashboard calendar ranges', () => {
  test('resolves each calendar preference to exact period boundaries', () => {
    const now = new Date(2026, 6, 30, 12, 34, 56)
    const expected = [
      ['today', new Date(2026, 6, 30), new Date(2026, 6, 30, 23, 59, 59, 999)],
      [
        'thisWeek',
        new Date(2026, 6, 27),
        new Date(2026, 7, 2, 23, 59, 59, 999),
      ],
      [
        'thisMonth',
        new Date(2026, 6, 1),
        new Date(2026, 6, 31, 23, 59, 59, 999),
      ],
      [
        'lastMonth',
        new Date(2026, 5, 1),
        new Date(2026, 5, 30, 23, 59, 59, 999),
      ],
    ] as const

    for (const [range, start, end] of expected) {
      assert.deepEqual(getCalendarDateRange(range, now), { start, end })
    }
  })
})
