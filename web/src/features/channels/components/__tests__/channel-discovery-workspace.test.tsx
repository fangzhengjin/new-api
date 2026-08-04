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

import type { DiscoveryFormValues } from '../../lib/channel-discovery-form'

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { useForm } = await import('react-hook-form')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { ChannelDiscoveryWorkspace } =
  await import('../channel-discovery-workspace')
const { ChannelDiscoverySourcePanel } =
  await import('../channel-discovery-source-panel')

const i18n = createInstance()
await i18n.use(initReactI18next).init({ lng: 'en' })

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

function Harness(props: {
  probing: boolean
  probeStatus?: 'idle' | 'probing' | 'success' | 'error'
  routes?: Record<string, string>
  groupOptions?: string[]
}) {
  const form = useForm<DiscoveryFormValues>({
    defaultValues: {
      text: 'https://api.example.test\nsecret',
      operation: 'create',
      channelId: 0,
      baseUrl: 'https://api.example.test',
      name: 'example',
      tag: '',
      priority: 0,
      keyMode: 'append',
      syncConfiguration: true,
      enabled: true,
      reenable: false,
    },
  })

  return (
    <I18nextProvider i18n={i18n}>
      <ChannelDiscoveryWorkspace
        form={form}
        connection={{
          block_index: 0,
          base_url: 'https://api.example.test',
          suggested_name: 'example',
          usable: true,
          models: ['gpt-5.4'],
          choices: [
            {
              source: 'gpt-5.4',
              model: 'gpt-5.4',
              target: 'gpt-5.4',
              recommended: true,
            },
          ],
          usable_key_indexes: [0],
          rejected_key_count: 0,
          matches: [],
        }}
        groupOptions={props.groupOptions ?? ['default']}
        groups={['default']}
        selectedModels={['gpt-5.4']}
        mapping={{}}
        routes={props.routes ?? { responses: '/v1/responses' }}
        acceptedKeyIndexes={[0]}
        probeModel='gpt-5.4'
        probing={props.probing}
        probeStatus={props.probeStatus ?? 'idle'}
        probeDisplayModel='upstream-model'
        probeKeyIndex={1}
        onGroupsChange={() => undefined}
        onAcceptedKeysChange={() => undefined}
        onProbeModelChange={() => undefined}
        onProbe={() => undefined}
        onToggleModel={() => undefined}
        onModelsChange={() => undefined}
        onSetMapping={() => undefined}
        onRemoveMapping={() => undefined}
        onSetRoute={() => undefined}
      />
    </I18nextProvider>
  )
}

function SourceHarness() {
  const form = useForm<DiscoveryFormValues>({
    defaultValues: {
      text: 'https://first.example\nsecret\n\nhttps://second.example\nsecret',
      operation: 'create',
      channelId: 0,
      baseUrl: '',
      name: '',
      tag: '',
      priority: 0,
      keyMode: 'append',
      syncConfiguration: true,
      enabled: true,
      reenable: false,
    },
  })

  return (
    <I18nextProvider i18n={i18n}>
      <ChannelDiscoverySourcePanel
        form={form}
        connections={[
          {
            block_index: 1,
            base_url: 'https://second.example',
            suggested_name: 'second',
            usable: true,
            models: ['gpt-5.4'],
            choices: [],
            usable_key_indexes: [0],
            rejected_key_count: 0,
            matches: [],
          },
        ]}
        totalCount={2}
        completedCount={1}
        draftedBlockIndexes={[1]}
        selectedBlockIndex={1}
        discovering={false}
        onTextChange={() => undefined}
        onReset={() => undefined}
        onDiscover={() => undefined}
        onSelect={() => undefined}
      />
    </I18nextProvider>
  )
}

describe('channel discovery workspace pending state', () => {
  test('locks all draft controls while protocol probing is pending', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () => root.render(<Harness probing />))
    const fieldset = container.querySelector('fieldset')
    const baseUrlInput = container.querySelector('#discovery-base-url')
    assert.ok(fieldset)
    assert.ok(baseUrlInput)
    assert.equal(fieldset.disabled, true)
    assert.equal(fieldset.getAttribute('aria-busy'), 'true')
    assert.equal(baseUrlInput.closest('fieldset'), fieldset)

    await act(async () => root.render(<Harness probing={false} />))
    assert.equal(fieldset.disabled, false)

    await act(async () => root.unmount())
    container.remove()
  })

  test('identifies the model and key while a protocol probe is running', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () =>
      root.render(<Harness probing probeStatus='probing' />)
    )
    const status = container.querySelector('[role="status"][aria-live="polite"]')
    assert.ok(status)
    assert.match(
      status.textContent ?? '',
      /Probing upstream-model with key 2/
    )

    await act(async () => root.unmount())
    container.remove()
  })

  test('shows detected protocols after the probe completes', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () =>
      root.render(
        <Harness
          probing={false}
          probeStatus='success'
          routes={{
            models: '/v1/models',
            responses: '/v1/responses',
            messages: '/v1/messages',
          }}
        />
      )
    )
    const status = container.querySelector('[role="status"][aria-live="polite"]')
    assert.ok(status)
    assert.match(status.textContent ?? '', /Probe completed for upstream-model/)
    assert.match(status.textContent ?? '', /responses/)
    assert.match(status.textContent ?? '', /messages/)

    await act(async () => root.unmount())
    container.remove()
  })

  test('keeps probe failure visible with a recovery instruction', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () =>
      root.render(<Harness probing={false} probeStatus='error' />)
    )
    const status = container.querySelector('[role="status"][aria-live="polite"]')
    assert.ok(status)
    assert.match(status.textContent ?? '', /Probe failed for upstream-model/)
    assert.match(
      status.textContent ?? '',
      /checking the Base URL, API key, and upstream protocol support/
    )

    await act(async () => root.unmount())
    container.remove()
  })

  test('lays out long group names in an aligned responsive grid', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () =>
      root.render(
        <Harness
          probing={false}
          groupOptions={['default', 'c:events:free:claude:very-long-name']}
        />
      )
    )
    const group = container.querySelector('[role="group"][aria-label="Groups"]')
    const longName = container.querySelector(
      'label[for="discovery-group-c:events:free:claude:very-long-name"] span.break-all'
    )
    assert.ok(group)
    assert.ok(longName)
    assert.match(group.className, /\bgrid\b/)
    assert.match(longName.className, /break-all/)

    await act(async () => root.unmount())
    container.remove()
  })

  test('keeps source credentials and pending drafts while processing multiple connections', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () => root.render(<SourceHarness />))
    const source = container.querySelector<HTMLTextAreaElement>(
      '#channel-discovery-input'
    )
    assert.ok(source)
    assert.equal(source.readOnly, true)
    assert.match(source.value, /first\.example/)
    assert.match(container.textContent ?? '', /1 of 2 completed/)
    assert.match(container.textContent ?? '', /Draft/)

    await act(async () => root.unmount())
    container.remove()
  })
})
