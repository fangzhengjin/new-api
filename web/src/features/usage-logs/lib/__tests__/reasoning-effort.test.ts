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

import { getReasoningEffortDisplay } from '../format'

describe('usage log reasoning effort', () => {
  test('keeps legacy strings as a single final value', () => {
    assert.deepEqual(getReasoningEffortDisplay('high'), {
      final: 'high',
    })
  })

  test('exposes structured transitions including a missing original value', () => {
    assert.deepEqual(
      getReasoningEffortDisplay({ original: 'xhigh', final: 'high' }),
      { original: 'xhigh', final: 'high' }
    )
    assert.deepEqual(getReasoningEffortDisplay({ final: 'medium' }), {
      original: 'none',
      final: 'medium',
    })
  })

  test('uses upstream none semantics for missing values', () => {
    assert.deepEqual(getReasoningEffortDisplay(), {
      final: 'none',
    })
    assert.deepEqual(getReasoningEffortDisplay({}), {
      original: 'none',
      final: 'none',
    })
  })
})
