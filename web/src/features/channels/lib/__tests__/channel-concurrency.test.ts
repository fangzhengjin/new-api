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

import type { Channel } from '../../types'
import {
  buildSettingJSON,
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformChannelToFormDefaults,
} from '../channel-form'
import {
  formatChannelConcurrency,
  shouldFetchChannelConcurrency,
} from '../channel-utils'

function channelWithSetting(setting: string): Channel {
  return {
    id: 1,
    type: 1,
    key: '',
    status: 1,
    name: 'limited',
    created_time: 0,
    test_time: 0,
    response_time: 0,
    other: '',
    balance: 0,
    balance_updated_time: 0,
    models: 'gpt-5',
    group: 'default',
    used_quota: 0,
    other_info: '',
    remark: '',
    max_input_tokens: 0,
    channel_info: {
      is_multi_key: false,
      multi_key_size: 0,
      multi_key_polling_index: 0,
      multi_key_mode: 'random',
    },
    settings: '{}',
    setting,
  }
}

describe('channel concurrency settings', () => {
  test('preserves an explicit zero wait and omits disabled limits', () => {
    const parsed = transformChannelToFormDefaults(
      channelWithSetting(
        JSON.stringify({
          max_concurrency: 3,
          concurrency_wait_timeout_seconds: 0,
        })
      )
    )
    assert.equal(parsed.max_concurrency, 3)
    assert.equal(parsed.concurrency_wait_timeout_seconds, 0)

    const enabled = JSON.parse(buildSettingJSON(parsed))
    assert.equal(enabled.max_concurrency, 3)
    assert.equal(enabled.concurrency_wait_timeout_seconds, 0)

    const disabled = JSON.parse(
      buildSettingJSON({ ...parsed, max_concurrency: 0 })
    )
    assert.equal('max_concurrency' in disabled, false)
    assert.equal('concurrency_wait_timeout_seconds' in disabled, false)
  })

  test('uses a 90 second default and rejects negative limits', () => {
    const parsed = transformChannelToFormDefaults(channelWithSetting('{}'))
    assert.equal(parsed.concurrency_wait_timeout_seconds, 90)
    assert.equal(
      channelFormSchema.safeParse({
        ...CHANNEL_FORM_DEFAULT_VALUES,
        name: 'limited',
        models: 'gpt-5',
        max_concurrency: -1,
      }).success,
      false
    )
  })

  test('formats current requests with limited and unlimited capacities', () => {
    assert.equal(formatChannelConcurrency(2, 3, true), '2 / 3')
    assert.equal(formatChannelConcurrency(2, 0, true), '2 / ∞')
    assert.equal(formatChannelConcurrency(undefined, 0, false), '—')
  })

  test('polls only while the concurrency column is visible and has channels', () => {
    assert.equal(shouldFetchChannelConcurrency([1, 2], true), true)
    assert.equal(shouldFetchChannelConcurrency([1, 2], false), false)
    assert.equal(shouldFetchChannelConcurrency([], true), false)
  })
})
