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

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const i18next = (await import('i18next')).default
const { initReactI18next } = await import('react-i18next')
await i18next.use(initReactI18next).init({ lng: 'en' })
const { ChannelDiscoveryEndpoints, ChannelDiscoveryOptions } =
  await import('../channel-discovery-options')

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

function connection(models: string[]) {
  return {
    block_index: 0,
    base_url: 'https://api.example.test',
    suggested_name: 'example',
    usable: true,
    models,
    choices: models.map((model) => ({
      source: model,
      model,
      target: model,
      recommended: true,
    })),
    usable_key_indexes: [0],
    rejected_key_count: 0,
    matches: [],
  }
}

describe('channel discovery options', () => {
  test('uses a current upstream model when discovery refreshes the same connection', async () => {
    const mappings: Array<[string, string]> = []
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    const render = (models: string[]) => (
      <ChannelDiscoveryOptions
        connection={connection(models)}
        selectedModels={models}
        mapping={{}}
        onToggleModel={() => undefined}
        onModelsChange={() => undefined}
        onSetMapping={(source, target) => mappings.push([source, target])}
        onRemoveMapping={() => undefined}
      />
    )

    await act(async () => root.render(render(['old-model'])))
    const mappingTab = [...container.querySelectorAll('button')].find(
      (button) => button.textContent === 'Mapping'
    )
    assert.ok(mappingTab)
    await act(async () => mappingTab.click())

    const sourceInput = container.querySelector<HTMLInputElement>(
      'input[aria-label="Exposed model name"]'
    )
    assert.ok(sourceInput)
    await act(async () => {
      const valueSetter = Object.getOwnPropertyDescriptor(
        HTMLInputElement.prototype,
        'value'
      )?.set
      assert.ok(valueSetter)
      valueSetter.call(sourceInput, 'public-model')
      sourceInput.dispatchEvent(new Event('input', { bubbles: true }))
    })

    await act(async () => root.render(render(['new-model'])))
    const targetSelect = container.querySelector<HTMLSelectElement>(
      'select[aria-label="Upstream model"]'
    )
    assert.ok(targetSelect)
    assert.equal(targetSelect.value, 'new-model')

    const addButton = [...container.querySelectorAll('button')].find((button) =>
      button.textContent?.includes('Add Mapping')
    )
    assert.ok(addButton)
    await act(async () => addButton.click())
    assert.deepEqual(mappings, [['public-model', 'new-model']])

    await act(async () => root.unmount())
    container.remove()
  })

  test('keeps endpoints outside the model and mapping tabs', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () =>
      root.render(
        <>
          <ChannelDiscoveryEndpoints
            routes={{ responses: '/v1/responses' }}
            onSetRoute={() => undefined}
          />
          <ChannelDiscoveryOptions
            connection={connection(['gpt-5.4'])}
            selectedModels={['gpt-5.4']}
            mapping={{}}
            onToggleModel={() => undefined}
            onModelsChange={() => undefined}
            onSetMapping={() => undefined}
            onRemoveMapping={() => undefined}
          />
        </>
      )
    )

    const tabLabels = new Set(
      [...container.querySelectorAll('[role="tab"]')].map(
        (tab) => tab.textContent
      )
    )
    assert.deepEqual(tabLabels, new Set(['Models', 'Mapping']))
    assert.ok(container.textContent?.includes('Endpoints'))

    await act(async () => root.unmount())
    container.remove()
  })

  test('offers select all, invert, and clear actions for the visible models', async () => {
    const updates: string[][] = []
    const models = ['model-a', 'model-b', 'model-c']
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () =>
      root.render(
        <ChannelDiscoveryOptions
          connection={connection(models)}
          selectedModels={['model-a']}
          mapping={{}}
          onToggleModel={() => undefined}
          onModelsChange={(choices) =>
            updates.push(choices.map((choice) => choice.model))
          }
          onSetMapping={() => undefined}
          onRemoveMapping={() => undefined}
        />
      )
    )

    const buttons = [...container.querySelectorAll('button')]
    const selectAll = buttons.find(
      (button) => button.textContent === 'Select all'
    )
    const invert = buttons.find(
      (button) => button.textContent === 'Invert selection'
    )
    const clear = buttons.find(
      (button) => button.textContent === 'Clear selection'
    )
    assert.ok(selectAll)
    assert.ok(invert)
    assert.ok(clear)

    await act(async () => selectAll.click())
    await act(async () => invert.click())
    await act(async () => clear.click())
    assert.deepEqual(updates, [
      ['model-a', 'model-b', 'model-c'],
      ['model-b', 'model-c'],
      [],
    ])

    await act(async () => root.unmount())
    container.remove()
  })

  test('keeps manually mapped models when changing visible selections', async () => {
    const updates: string[][] = []
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () =>
      root.render(
        <ChannelDiscoveryOptions
          connection={connection(['model-a', 'model-b'])}
          selectedModels={['manual-model']}
          mapping={{ 'manual-model': 'model-a' }}
          onToggleModel={() => undefined}
          onModelsChange={(choices) =>
            updates.push(choices.map((choice) => choice.model))
          }
          onSetMapping={() => undefined}
          onRemoveMapping={() => undefined}
        />
      )
    )

    const selectAll = [...container.querySelectorAll('button')].find(
      (button) => button.textContent === 'Select all'
    )
    assert.ok(selectAll)
    await act(async () => selectAll.click())
    assert.deepEqual(updates, [['manual-model', 'model-a', 'model-b']])

    await act(async () => root.unmount())
    container.remove()
  })

  test('keeps every endpoint available for manual overrides', async () => {
    const updates: Array<[string, string | null]> = []
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () =>
      root.render(
        <ChannelDiscoveryEndpoints
          routes={{ models: '/provider/models' }}
          onSetRoute={(protocol, path) => updates.push([protocol, path])}
        />
      )
    )

    const generations = container.querySelector<HTMLButtonElement>(
      '#route-image_generations'
    )
    const edits =
      container.querySelector<HTMLButtonElement>('#route-image_edits')
    const models = container.querySelector<HTMLButtonElement>('#route-models')
    const compact = container.querySelector<HTMLButtonElement>('#route-compact')
    assert.ok(generations)
    assert.ok(edits)
    assert.ok(models)
    assert.ok(compact)
    assert.equal(generations.disabled, false)
    assert.equal(edits.disabled, false)
    assert.equal(models.disabled, false)
    assert.equal(compact.disabled, false)

    await act(async () => models.click())
    await act(async () => compact.click())
    await act(async () => generations.click())
    await act(async () => edits.click())
    assert.deepEqual(updates, [
      ['models', null],
      ['compact', '/v1/responses/compact'],
      ['image_generations', '/v1/images/generations'],
      ['image_edits', '/v1/images/edits'],
    ])

    await act(async () => root.unmount())
    container.remove()
  })
})
