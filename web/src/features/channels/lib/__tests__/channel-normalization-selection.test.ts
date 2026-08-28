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

import type { ChannelNormalizationItem } from '../../channel-setup'
import {
  countNormalizationChanges,
  createDefaultNormalizationSelection,
  createEmptyNormalizationSelection,
  createFullNormalizationSelection,
  hasNormalizationChanges,
  isNormalizationSelectionValid,
  reconcileNormalizationSelection,
} from '../channel-normalization-selection'

const item: ChannelNormalizationItem = {
  channel_id: 7,
  snapshot_hash: 'snapshot',
  add_models: ['manual-alias'],
  remove_models: ['legacy-alias'],
  mapping_set: {
    'manual-alias': 'gpt-5.4',
    'existing-alias': 'new-target',
  },
  mapping_remove: ['legacy-alias'],
  sort_changed: true,
}

const metadata = {
  models: ['gpt-5.4', 'legacy-alias', 'existing-alias'],
  model_mapping: {
    'legacy-alias': 'legacy-target',
    'existing-alias': 'old-target',
  },
}

describe('channel normalization selection', () => {
  test('selects safe additions and sorting but leaves removals and overwrites off', () => {
    const selection = createDefaultNormalizationSelection(item, metadata)

    assert.deepEqual(selection.add_models, ['manual-alias'])
    assert.deepEqual(selection.remove_models, [])
    assert.deepEqual(selection.mapping_set, {
      'manual-alias': 'gpt-5.4',
    })
    assert.deepEqual(selection.mapping_remove, [])
    assert.equal(selection.sort_models, true)
  })

  test('selects every previewed change while preserving dependencies', () => {
    const selection = createFullNormalizationSelection(item, metadata)

    assert.deepEqual(selection.add_models, ['manual-alias'])
    assert.deepEqual(selection.remove_models, ['legacy-alias'])
    assert.deepEqual(selection.mapping_set, {
      'manual-alias': 'gpt-5.4',
      'existing-alias': 'new-target',
    })
    assert.deepEqual(selection.mapping_remove, ['legacy-alias'])
    assert.equal(selection.sort_models, true)
    assert.equal(countNormalizationChanges(selection), 6)
  })

  test('clears every previewed change', () => {
    const selection = createEmptyNormalizationSelection(item)

    assert.equal(hasNormalizationChanges(selection), false)
    assert.equal(countNormalizationChanges(selection), 0)
  })

  test('removing a mapped model also selects its mapping removal', () => {
    const selection = reconcileNormalizationSelection(item, metadata, {
      ...createDefaultNormalizationSelection(item, metadata),
      remove_models: ['legacy-alias'],
    })

    assert.deepEqual(selection.mapping_remove, ['legacy-alias'])
  })

  test('rejects a selected mapping with an empty target', () => {
    const selection = createDefaultNormalizationSelection(item, metadata)
    selection.mapping_set['manual-alias'] = ' '

    assert.equal(isNormalizationSelectionValid(selection), false)
  })

  test('restores a required mapping when its model is selected again', () => {
    const selection = reconcileNormalizationSelection(item, metadata, {
      ...createDefaultNormalizationSelection(item, metadata),
      add_models: ['manual-alias'],
      mapping_set: {},
    })

    assert.deepEqual(selection.mapping_set, {
      'manual-alias': 'gpt-5.4',
    })
  })

  test('requires sorting exactly while a model addition or removal is selected', () => {
    const modelChangeItem = { ...item, sort_changed: false }
    const addition = createDefaultNormalizationSelection(
      modelChangeItem,
      metadata
    )
    assert.equal(addition.sort_models, true)

    const noModelChange = reconcileNormalizationSelection(
      modelChangeItem,
      metadata,
      {
        ...addition,
        add_models: [],
      }
    )
    assert.equal(noModelChange.sort_models, false)

    const removal = reconcileNormalizationSelection(modelChangeItem, metadata, {
      ...noModelChange,
      remove_models: ['legacy-alias'],
      mapping_remove: ['legacy-alias'],
    })
    assert.equal(removal.sort_models, true)
  })
})
