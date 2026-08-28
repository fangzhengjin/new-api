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

import {
  isCurrentChannelDiscoveryPreview,
  isCurrentChannelDiscoveryProbe,
  isCurrentChannelDiscoveryText,
  selectChannelDiscoveryProbeModel,
  type ChannelDiscoveryDraft,
  type ChannelDiscoveryProbeInput,
} from '../../channel-setup'

const probe: ChannelDiscoveryProbeInput = {
  text: 'https://api.example.test\nsecret',
  block_index: 0,
  key_index: 1,
  base_url: 'https://api.example.test',
  model: 'upstream-model',
  models_path: '/vendor/v1/models',
}

const draft: ChannelDiscoveryDraft = {
  operation: 'create',
  block_index: 0,
  base_url: 'https://api.example.test',
  accepted_key_indexes: [1],
  selected_models: ['public-model'],
  mapping: { 'public-model': 'upstream-model' },
  routes: { responses: '/v1/responses' },
  name: 'example',
  groups: ['default'],
  tag: '',
  priority: 0,
  enabled: true,
  key_mode: 'append',
  sync_configuration: true,
  reenable: false,
}

describe('channel discovery request identity', () => {
  test('accepts only discovery results for the current normalized source text', () => {
    assert.equal(
      isCurrentChannelDiscoveryText(probe.text, `  ${probe.text}\n`),
      true
    )
    assert.equal(isCurrentChannelDiscoveryText(probe.text, 'changed'), false)
  })

  test('rejects a probe result when any request target changed', () => {
    assert.equal(isCurrentChannelDiscoveryProbe(probe, { ...probe }), true)
    for (const current of [
      { ...probe, text: 'changed' },
      { ...probe, block_index: 2 },
      { ...probe, key_index: 3 },
      { ...probe, base_url: 'https://api.example.test/changed' },
      { ...probe, model: 'changed-model' },
      { ...probe, models_path: '/models' },
    ]) {
      assert.equal(isCurrentChannelDiscoveryProbe(probe, current), false)
    }
  })

  test('rejects a preview result after its editable draft changed', () => {
    const request = { text: probe.text, draft }
    assert.equal(
      isCurrentChannelDiscoveryPreview(request, {
        text: probe.text,
        draft: { ...draft },
      }),
      true
    )
    assert.equal(
      isCurrentChannelDiscoveryPreview(request, {
        text: probe.text,
        draft: { ...draft, selected_models: ['changed-model'] },
      }),
      false
    )
  })

  test('accepts a preview result after equivalent form values are trimmed', () => {
    assert.equal(
      isCurrentChannelDiscoveryPreview(
        { text: probe.text.trim(), draft },
        {
          text: `  ${probe.text}\n`,
          draft: {
            ...draft,
            base_url: ` ${draft.base_url} `,
            name: ` ${draft.name} `,
            tag: ' ',
          },
        }
      ),
      true
    )
  })

  test('chooses a representative model only from the selected models', () => {
    assert.equal(
      selectChannelDiscoveryProbeModel([
        'gpt-image-2',
        'gpt-5.4',
        'gpt-5.6-luna',
      ]),
      'gpt-5.6-luna'
    )
    assert.equal(
      selectChannelDiscoveryProbeModel(['gpt-image-2', 'gpt-5.4']),
      'gpt-5.4'
    )
    assert.equal(
      selectChannelDiscoveryProbeModel(['gpt-image-2']),
      'gpt-image-2'
    )
    assert.equal(selectChannelDiscoveryProbeModel([]), '')
  })
})
