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

import type React from 'react'
import { describe, test } from 'vitest'

import type { UsageLog } from '../../../data/schema'

Object.defineProperty(Element.prototype, 'getAnimations', {
  configurable: true,
  value: () => [],
})

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { TokenBreakdown } = await import('../details-dialog')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        'Token Breakdown': 'Token Breakdown',
        'Total Input Tokens': 'Total Input Tokens',
        'Input Breakdown': 'Input Breakdown',
        'Uncached Input': 'Uncached Input',
        'Cache Read': 'Cache Read',
        'Cache Write': 'Cache Write',
        'Cache Write TTL': 'Cache Write TTL',
        '5 minutes': '5 minutes',
        '1 hour': '1 hour',
        Unknown: 'Unknown',
        'Recorded Cache': 'Recorded Cache',
        'Output Tokens': 'Output Tokens',
      },
    },
  },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

function createLog(promptTokens: number): UsageLog {
  return {
    id: 1,
    user_id: 1,
    created_at: 1000,
    type: 2,
    content: '',
    username: '',
    display_name: '',
    token_name: '',
    model_name: '',
    quota: 0,
    prompt_tokens: promptTokens,
    completion_tokens: 10,
    use_time: 0,
    is_stream: false,
    channel: 0,
    channel_name: '',
    token_id: 0,
    group: '',
    ip: '',
    other: '',
    request_id: '',
    upstream_request_id: '',
  }
}

async function renderBreakdown(
  log: UsageLog,
  other: React.ComponentProps<typeof TokenBreakdown>['other']
) {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <TokenBreakdown log={log} other={other} />
      </I18nextProvider>
    )
  })

  return { container, root }
}

async function unmountBreakdown(
  rendered: Awaited<ReturnType<typeof renderBreakdown>>
) {
  await act(async () => rendered.root.unmount())
  rendered.container.remove()
}

function definitionValue(container: ParentNode, label: string): string | null {
  const term = [...container.querySelectorAll('dt')].find(
    (element) => element.textContent === label
  )
  return term?.parentElement?.querySelector('dd')?.textContent ?? null
}

describe('token breakdown layout', () => {
  test('keeps zero cache-write details visible in aligned flat groups', async () => {
    const rendered = await renderBreakdown(createLog(1000), {
      usage_semantic: 'openai',
      cache_tokens: 800,
    })

    const inputBreakdown = rendered.container.querySelector(
      '[aria-label="Input Breakdown"]'
    )
    assert.ok(inputBreakdown)
    assert.equal(inputBreakdown.classList.contains('sm:grid-cols-3'), true)
    assert.equal(definitionValue(rendered.container, 'Uncached Input'), '200')
    assert.equal(definitionValue(rendered.container, 'Cache Read'), '800')
    assert.equal(definitionValue(rendered.container, 'Cache Write'), '0')

    const cacheWriteTtl = rendered.container.querySelector(
      '[aria-label="Cache Write TTL"]'
    )
    assert.ok(cacheWriteTtl)
    assert.equal(definitionValue(cacheWriteTtl, '5 minutes'), '0')
    assert.equal(definitionValue(cacheWriteTtl, '1 hour'), '0')
    assert.equal(definitionValue(cacheWriteTtl, 'Unknown'), '0')
    for (const item of cacheWriteTtl.children) {
      assert.equal(item.classList.contains('items-baseline'), true)
    }

    await unmountBreakdown(rendered)
  })

  test('uses compact sans numerals while preserving tabular alignment', async () => {
    const rendered = await renderBreakdown(createLog(1000), {
      usage_semantic: 'openai',
      cache_tokens: 800,
    })

    const totalInput = [...rendered.container.querySelectorAll('dt')].find(
      (element) => element.textContent === 'Total Input Tokens'
    )
    const totalInputValue = totalInput?.parentElement?.querySelector('dd')
    const cacheReadValue = [...rendered.container.querySelectorAll('dt')]
      .find((element) => element.textContent === 'Cache Read')
      ?.parentElement?.querySelector('dd')

    assert.ok(totalInput)
    assert.ok(totalInputValue)
    assert.ok(cacheReadValue)
    assert.equal(totalInput.classList.contains('text-xs'), true)
    assert.equal(totalInputValue.classList.contains('text-sm'), true)
    assert.equal(cacheReadValue.classList.contains('text-xs'), true)
    for (const value of [totalInputValue, cacheReadValue]) {
      assert.equal(value.classList.contains('tabular-nums'), true)
      assert.equal(value.classList.contains('font-mono'), false)
    }

    await unmountBreakdown(rendered)
  })

  test('keeps summary rows denser than the input breakdown', async () => {
    const rendered = await renderBreakdown(createLog(1000), {
      usage_semantic: 'openai',
      cache_tokens: 800,
    })

    const totalInput = [...rendered.container.querySelectorAll('dt')].find(
      (element) => element.textContent === 'Total Input Tokens'
    )
    const output = [...rendered.container.querySelectorAll('dt')].find(
      (element) => element.textContent === 'Output Tokens'
    )
    const inputBreakdown = rendered.container.querySelector(
      '[aria-label="Input Breakdown"]'
    )?.parentElement

    assert.ok(totalInput?.parentElement)
    assert.ok(output?.parentElement?.parentElement)
    assert.ok(inputBreakdown)
    assert.equal(totalInput.parentElement.classList.contains('py-2'), true)
    assert.equal(
      output.parentElement.parentElement.classList.contains('py-2'),
      true
    )
    assert.equal(inputBreakdown.classList.contains('py-2.5'), true)

    await unmountBreakdown(rendered)
  })

  test('does not invent an input composition for inconsistent historical data', async () => {
    const rendered = await renderBreakdown(createLog(40), {
      usage_semantic: 'openai',
      cache_tokens: 50,
      cache_creation_tokens: 20,
    })

    assert.equal(definitionValue(rendered.container, 'Recorded Cache'), '70')
    assert.equal(definitionValue(rendered.container, 'Uncached Input'), null)
    assert.equal(
      rendered.container.querySelector('[aria-label="Cache Write TTL"]'),
      null
    )

    await unmountBreakdown(rendered)
  })
})
