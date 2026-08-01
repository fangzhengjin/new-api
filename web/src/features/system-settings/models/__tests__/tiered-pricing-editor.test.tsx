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

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { TieredPricingEditor } = await import('../tiered-pricing-editor')

const i18n = createInstance()
await i18n.use(initReactI18next).init({ lng: 'en', resources: { en: {} } })

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

type EditorProps = React.ComponentProps<typeof TieredPricingEditor>

async function renderEditor(props: EditorProps) {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <TieredPricingEditor {...props} />
      </I18nextProvider>
    )
  })

  return { container, root }
}

async function unmountEditor(
  rendered: Awaited<ReturnType<typeof renderEditor>>
) {
  await act(async () => rendered.root.unmount())
  rendered.container.remove()
}

async function selectEditorMode(
  rendered: Awaited<ReturnType<typeof renderEditor>>,
  label: string
) {
  const trigger =
    rendered.container.querySelector<HTMLElement>('[role="combobox"]')
  assert.ok(trigger)
  await act(async () => trigger.click())

  const option = [
    ...document.querySelectorAll<HTMLElement>('[role="option"]'),
  ].find((item) => item.textContent === label)
  assert.ok(option)
  await act(async () => option.click())
}

async function changeTextareaValue(
  textarea: HTMLTextAreaElement,
  value: string
) {
  const valueSetter = Object.getOwnPropertyDescriptor(
    HTMLTextAreaElement.prototype,
    'value'
  )?.set
  assert.ok(valueSetter)

  await act(async () => {
    valueSetter.call(textarea, value)
    textarea.dispatchEvent(new Event('input', { bubbles: true }))
  })
}

describe('tiered pricing editor expression preservation', () => {
  test('does not replace an unsupported billing expression on mount', async () => {
    const billingExpr =
      '(len <= 272000 ? tier("standard", p * 5 + c * 30 + cr * 0.5) : tier("long_context", p * 10 + c * 45 + cr * 1)) * (param("service_tier") == "priority" || param("service_tier") == "fast" ? 2 : 1)'
    const billingChanges: string[] = []
    const requestRuleChanges: string[] = []
    const rendered = await renderEditor({
      billingExpr,
      requestRuleExpr: '',
      onBillingExprChange: (value) => billingChanges.push(value),
      onRequestRuleExprChange: (value) => requestRuleChanges.push(value),
    })

    assert.equal(
      rendered.container.querySelector('textarea')?.value,
      billingExpr
    )
    assert.deepEqual(billingChanges, [])
    assert.deepEqual(requestRuleChanges, [])

    await unmountEditor(rendered)
  })

  test('does not clear unsupported request rules on mount', async () => {
    const billingExpr = 'tier("base", p * 5 + c * 30)'
    const requestRuleExpr =
      '(param("service_tier") == "priority" || param("service_tier") == "fast" ? 2 : 1)'
    const billingChanges: string[] = []
    const requestRuleChanges: string[] = []
    const rendered = await renderEditor({
      billingExpr,
      requestRuleExpr,
      onBillingExprChange: (value) => billingChanges.push(value),
      onRequestRuleExprChange: (value) => requestRuleChanges.push(value),
    })

    assert.equal(
      rendered.container.querySelector('textarea')?.value,
      `(${billingExpr}) * ${requestRuleExpr}`
    )
    assert.deepEqual(billingChanges, [])
    assert.deepEqual(requestRuleChanges, [])

    await unmountEditor(rendered)
  })

  test('writes visual edits back to the billing expression', async () => {
    const billingChanges: string[] = []
    const rendered = await renderEditor({
      billingExpr: 'tier("base", p * 5 + c * 30)',
      requestRuleExpr: '',
      onBillingExprChange: (value) => billingChanges.push(value),
      onRequestRuleExprChange: () => {},
    })

    const addTierButton = [
      ...rendered.container.querySelectorAll('button'),
    ].find((button) => button.textContent === 'Add tier')
    assert.ok(addTierButton)
    await act(async () => addTierButton.click())

    assert.equal(
      billingChanges.at(-1),
      'len < 200000 ? tier("base", p * 5 + c * 30) : tier("tier_2", p * 0 + c * 0)'
    )

    await unmountEditor(rendered)
  })

  test('stays in expression mode when visual conversion would lose data', async () => {
    const billingExpr = 'tier("base", p * 5 + c * 30)'
    const billingChanges: string[] = []
    const rendered = await renderEditor({
      billingExpr,
      requestRuleExpr: '',
      onBillingExprChange: (value) => billingChanges.push(value),
      onRequestRuleExprChange: () => {},
    })

    await selectEditorMode(rendered, 'Expression editor')
    const textarea = rendered.container.querySelector('textarea')
    assert.ok(textarea)
    const unsupportedExpr = `${billingExpr} * 2`
    await changeTextareaValue(textarea, unsupportedExpr)
    const changeCount = billingChanges.length

    await selectEditorMode(rendered, 'Visual editor')

    assert.equal(
      rendered.container.querySelector('textarea')?.value,
      unsupportedExpr
    )
    assert.equal(billingChanges.length, changeCount)

    await unmountEditor(rendered)
  })
})
