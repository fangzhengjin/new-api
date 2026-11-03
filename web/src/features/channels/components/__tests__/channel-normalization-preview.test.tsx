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

import { describe, test, vi } from 'vitest'

import type {
  ChannelNormalizationData,
  ChannelNormalizationSelection,
} from '../../channel-setup'

const { fireEvent, render, screen } = await import('@testing-library/react')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { ChannelNormalizationPreview } =
  await import('../channel-normalization-preview')

const i18n = createInstance()
await i18n.use(initReactI18next).init({ lng: 'en' })

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

const data: ChannelNormalizationData = {
  task: {
    task_id: 'task',
    status: 'succeeded',
    result: {
      summary: { scanned: 1, changed: 1, skipped: 0, failed: 0 },
      items: [
        {
          channel_id: 7,
          snapshot_hash: 'snapshot',
          add_models: ['gpt-5.6-sol'],
          sort_changed: false,
        },
      ],
    },
  },
  channels: {
    '7': {
      name: 'example',
      models: ['gpt-5.5'],
      model_mapping: {},
    },
  },
}

const selections: Record<number, ChannelNormalizationSelection> = {
  7: {
    channel_id: 7,
    add_models: ['gpt-5.6-sol'],
    remove_models: [],
    mapping_set: {},
    mapping_remove: [],
    sort_models: true,
  },
}

const bulkData: ChannelNormalizationData = {
  task: {
    task_id: 'bulk-task',
    status: 'succeeded',
    result: {
      summary: { scanned: 2, changed: 2, skipped: 0, failed: 0 },
      items: [
        {
          channel_id: 7,
          snapshot_hash: 'bulk-snapshot',
          add_models: ['new-alias'],
          remove_models: ['legacy-alias'],
          mapping_set: {
            'new-alias': 'gpt-5.6-sol',
            'existing-alias': 'new-target',
          },
          mapping_remove: ['legacy-alias'],
          mapping_warnings: { 'existing-alias': 'old-target' },
          sort_changed: true,
        },
        {
          channel_id: 8,
          snapshot_hash: 'second-snapshot',
          add_models: ['gpt-5.6-terra'],
          sort_changed: false,
        },
      ],
    },
  },
  channels: {
    '7': {
      name: 'example',
      models: ['legacy-alias', 'existing-alias'],
      model_mapping: {
        'legacy-alias': 'legacy-target',
        'existing-alias': 'old-target',
      },
    },
    '8': {
      name: 'second',
      models: ['gpt-5.5'],
      model_mapping: {},
    },
  },
}

const bulkSelections: Record<number, ChannelNormalizationSelection> = {
  7: {
    channel_id: 7,
    add_models: ['new-alias'],
    remove_models: [],
    mapping_set: { 'new-alias': 'gpt-5.6-sol' },
    mapping_remove: [],
    sort_models: true,
  },
  8: {
    channel_id: 8,
    add_models: ['gpt-5.6-terra'],
    remove_models: [],
    mapping_set: {},
    mapping_remove: [],
    sort_models: true,
  },
}

function renderPreview(
  previewData: ChannelNormalizationData,
  previewSelections: Record<number, ChannelNormalizationSelection>,
  onSelectionChange = vi.fn()
) {
  render(
    <I18nextProvider i18n={i18n}>
      <ChannelNormalizationPreview
        data={previewData}
        selections={previewSelections}
        onSelectionChange={onSelectionChange}
        onResetSafeDefaults={() => undefined}
      />
    </I18nextProvider>
  )
  return onSelectionChange
}

describe('channel normalization preview', () => {
  test('shows mandatory sorting without a switch when a model addition is selected', () => {
    const { container } = render(
      <I18nextProvider i18n={i18n}>
        <ChannelNormalizationPreview
          data={data}
          selections={selections}
          onSelectionChange={() => undefined}
          onResetSafeDefaults={() => undefined}
        />
      </I18nextProvider>
    )

    assert.match(container.textContent ?? '', /Sort model names/)
    assert.match(
      container.textContent ?? '',
      /Model names are sorted automatically when models change\./
    )
    assert.equal(container.querySelector('[role="switch"]'), null)
  })

  test('global bulk controls select and clear every previewed change', () => {
    const onSelectionChange = renderPreview(bulkData, bulkSelections)

    fireEvent.click(screen.getAllByRole('button', { name: 'Select all' })[0])
    assert.deepEqual(onSelectionChange.mock.calls.slice(-2), [
      [
        7,
        {
          channel_id: 7,
          add_models: ['new-alias'],
          remove_models: ['legacy-alias'],
          mapping_set: {
            'new-alias': 'gpt-5.6-sol',
            'existing-alias': 'new-target',
          },
          mapping_remove: ['legacy-alias'],
          sort_models: true,
        },
      ],
      [
        8,
        {
          channel_id: 8,
          add_models: ['gpt-5.6-terra'],
          remove_models: [],
          mapping_set: {},
          mapping_remove: [],
          sort_models: true,
        },
      ],
    ])

    fireEvent.click(
      screen.getAllByRole('button', { name: 'Clear selection' })[0]
    )
    assert.deepEqual(onSelectionChange.mock.calls.slice(-2), [
      [
        7,
        {
          channel_id: 7,
          add_models: [],
          remove_models: [],
          mapping_set: {},
          mapping_remove: [],
          sort_models: false,
        },
      ],
      [
        8,
        {
          channel_id: 8,
          add_models: [],
          remove_models: [],
          mapping_set: {},
          mapping_remove: [],
          sort_models: false,
        },
      ],
    ])
  })

  test('per-channel controls reuse the same bulk selection semantics', () => {
    const onSelectionChange = renderPreview(bulkData, bulkSelections)

    fireEvent.click(screen.getAllByRole('button', { name: 'Select all' })[1])
    assert.deepEqual(onSelectionChange.mock.calls.at(-1)?.[1], {
      channel_id: 7,
      add_models: ['new-alias'],
      remove_models: ['legacy-alias'],
      mapping_set: {
        'new-alias': 'gpt-5.6-sol',
        'existing-alias': 'new-target',
      },
      mapping_remove: ['legacy-alias'],
      sort_models: true,
    })

    fireEvent.click(
      screen.getAllByRole('button', { name: 'Clear selection' })[1]
    )
    assert.deepEqual(onSelectionChange.mock.calls.at(-1)?.[1], {
      channel_id: 7,
      add_models: [],
      remove_models: [],
      mapping_set: {},
      mapping_remove: [],
      sort_models: false,
    })
  })

  test('shows selection summaries and exposes an accessible collapse control', () => {
    renderPreview(bulkData, bulkSelections)

    assert.ok(screen.getByText('Selected changes: 3'))
    assert.ok(screen.getByText('Models: 2 → 3'))
    assert.ok(screen.getByText('Warnings: 1'))

    const trigger = screen.getByRole('button', { name: 'example: Details' })
    assert.equal(trigger.getAttribute('aria-expanded'), 'true')
    fireEvent.click(trigger)
    assert.equal(trigger.getAttribute('aria-expanded'), 'false')
  })
})
