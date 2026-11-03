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
  channelFormSchema,
  transformChannelToFormDefaults,
} from '../channel-form'

function channelWithSetting(setting: string): Channel {
  return {
    id: 1,
    type: 1,
    key: '',
    status: 1,
    name: 'prompt strategy',
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

describe('channel system prompt strategy', () => {
  test('defaults to disabled and maps the legacy switch to prepend', () => {
    expect(
      transformChannelToFormDefaults(channelWithSetting('{}'))
        .system_prompt_mode
    ).toBe('none')
    expect(
      transformChannelToFormDefaults(
        channelWithSetting('{"system_prompt_override":true}')
      ).system_prompt_mode
    ).toBe('prepend')
  })

  test.each(['none', 'prepend', 'append', 'override'] as const)(
    'serializes %s and only dual-writes the legacy prepend flag',
    (mode) => {
      const setting = JSON.parse(
        buildSettingJSON({
          ...CHANNEL_FORM_DEFAULT_VALUES,
          system_prompt: 'extra',
          system_prompt_mode: mode,
        })
      )

      expect(setting.system_prompt_mode).toBe(mode)
      expect(setting.system_prompt_override).toBe(mode === 'prepend')
    }
  )

  test('rejects unknown modes', () => {
    expect(
      channelFormSchema.safeParse({
        ...CHANNEL_FORM_DEFAULT_VALUES,
        system_prompt_mode: 'replace',
      }).success
    ).toBe(false)
  })
})
