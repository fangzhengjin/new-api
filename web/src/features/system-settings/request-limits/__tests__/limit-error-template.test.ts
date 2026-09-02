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

import {
  createLimitErrorTemplateSchema,
  formatRequestLimitDuration,
} from '../limit-error-template'

describe('request limit error duration', () => {
  test.each([
    [45, '45s'],
    [135, '2m 15s'],
    [3600, '1h'],
    [3900, '1h 5m'],
    [3930, '1h 5m'],
  ])('formats %s seconds as %s', (seconds, expected) => {
    expect(formatRequestLimitDuration(seconds)).toBe(expected)
  })
})

describe('request limit error template validation', () => {
  const schema = createLimitErrorTemplateSchema((key) => key)

  test('counts Unicode code points consistently with the backend', () => {
    expect(schema.safeParse('😀'.repeat(2000)).success).toBe(true)
    expect(schema.safeParse('😀'.repeat(2001)).success).toBe(false)
  })
})
