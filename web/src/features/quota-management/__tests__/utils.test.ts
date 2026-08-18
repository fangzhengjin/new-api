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

import { beforeEach, describe, test } from 'vitest'

import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import {
  formatQuota,
  fromShanghaiAdjustmentDate,
  fromShanghaiInput,
  quotaFromDisplayAmount,
  quotaToDisplayAmount,
  toLocalInput,
  toShanghaiDateInput,
} from '../utils'

describe('quota management formatting', () => {
  beforeEach(() => {
    useSystemConfigStore.getState().setConfig({
      currency: { ...DEFAULT_CURRENCY_CONFIG },
    })
  })

  test('uses the configured new-api quota display and conversion', () => {
    assert.equal(formatQuota('-750000', true), '-$1.5')
    assert.equal(formatQuota('750000', true), '+$1.5')
    assert.equal(formatQuota('2500'), '$0.005')
    assert.equal(quotaToDisplayAmount('750000'), '1.5')
    assert.equal(quotaFromDisplayAmount('1.5'), '750000')
    assert.equal(quotaFromDisplayAmount('0', true), '0')

    useSystemConfigStore.getState().setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaPerUnit: 100000,
      },
    })
    assert.equal(formatQuota('100000'), '$1')
    assert.equal(quotaFromDisplayAmount('1'), '100000')

    useSystemConfigStore.getState().setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaDisplayType: 'CNY',
        usdExchangeRate: 7,
      },
    })
    assert.equal(formatQuota('500000'), '¥7')
    assert.equal(quotaToDisplayAmount('500000'), '7')
    assert.equal(quotaFromDisplayAmount('7'), '500000')

    useSystemConfigStore.getState().setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaDisplayType: 'TOKENS',
      },
    })
    assert.equal(formatQuota('750000'), '750k')
    assert.equal(quotaToDisplayAmount('750000'), '750000')
    assert.equal(quotaFromDisplayAmount('750000'), '750000')
    assert.equal(quotaFromDisplayAmount('1.5'), null)
  })

  test('round-trips datetime-local values in Asia/Shanghai', () => {
    const input = '2026-08-06T15:30'
    assert.equal(toLocalInput(fromShanghaiInput(input)), input)
  })

  test('normalizes adjustment dates to 09:00 or the exact cycle end', () => {
    const cycleEnd = fromShanghaiInput('2026-08-20T18:30')
    assert.equal(
      fromShanghaiAdjustmentDate('2026-08-10', cycleEnd),
      fromShanghaiInput('2026-08-10T09:00')
    )
    assert.equal(toShanghaiDateInput(cycleEnd), '2026-08-20')
    assert.equal(fromShanghaiAdjustmentDate('2026-08-20', cycleEnd), cycleEnd)
  })
})
