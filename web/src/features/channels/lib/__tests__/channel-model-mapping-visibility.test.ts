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

import type { Channel } from '../../types'
import {
  buildSettingJSON,
  CHANNEL_FORM_DEFAULT_VALUES,
  transformChannelToFormDefaults,
} from '../channel-form'

function channelWithSetting(setting: string): Channel {
  return {
    id: 1,
    type: 1,
    key: '',
    status: 1,
    name: 'mapped channel',
    created_time: 0,
    test_time: 0,
    response_time: 0,
    other: '',
    balance: 0,
    balance_updated_time: 0,
    models: 'public-model',
    group: 'default',
    used_quota: 0,
    model_mapping: '{"public-model":"upstream-model"}',
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

describe('channel model mapping visibility', () => {
  test('omits the hidden list when every mapping is user visible', () => {
    const setting = JSON.parse(
      buildSettingJSON({
        ...CHANNEL_FORM_DEFAULT_VALUES,
        model_mapping: '{"public-model":"upstream-model"}',
      })
    )

    expect(setting).not.toHaveProperty('user_hidden_model_mappings')
  })

  test('persists only unique hidden source models from the current mapping', () => {
    const setting = JSON.parse(
      buildSettingJSON({
        ...CHANNEL_FORM_DEFAULT_VALUES,
        model_mapping: JSON.stringify({
          'public-model': 'upstream-a',
          'private-model': 'upstream-b',
        }),
        user_hidden_model_mappings: [
          'public-model',
          'stale-model',
          'public-model',
        ],
      })
    )

    expect(setting.user_hidden_model_mappings).toEqual(['public-model'])
  })

  test('restores the models configured as hidden from users', () => {
    const form = transformChannelToFormDefaults(
      channelWithSetting(
        JSON.stringify({ user_hidden_model_mappings: ['public-model'] })
      )
    )

    expect(form.user_hidden_model_mappings).toEqual(['public-model'])
  })

  test('preserves settings not managed by the channel form when saving visibility', () => {
    const form = transformChannelToFormDefaults(
      channelWithSetting(
        JSON.stringify({
          future_flag: { enabled: true },
          user_hidden_model_mappings: ['public-model'],
        })
      )
    )

    const setting = JSON.parse(buildSettingJSON(form))

    expect(setting.future_flag).toEqual({ enabled: true })
    expect(setting.user_hidden_model_mappings).toEqual(['public-model'])
  })
})
