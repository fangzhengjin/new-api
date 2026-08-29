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

import type { PlanDetailData } from '../../types'

const mocks = vi.hoisted(() => ({ executePlan: vi.fn() }))

vi.mock('../../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../api')>()),
  executePlan: mocks.executePlan,
}))

Object.defineProperty(Element.prototype, 'getAnimations', {
  configurable: true,
  value: () => [],
})

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { QueryClient, QueryClientProvider } =
  await import('@tanstack/react-query')
const { createMemoryHistory, createRootRoute, createRouter, RouterProvider } =
  await import('@tanstack/react-router')
const { PlanActions } = await import('../plan-actions')

const i18n = createInstance()
await i18n.use(initReactI18next).init({ lng: 'en' })

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

const confirmationPhrase = 'EXECUTE PLAN 42'
const detail = {
  plan: { id: 42, status: 'draft' },
  cycle: { id: 7 },
  items: [],
  summary: {
    increase: { count: 2, total: '1200000' },
    decrease: { count: 1, total: '300000' },
  },
  confirmation_phrase: confirmationPhrase,
} as unknown as PlanDetailData

function findButton(text: string): HTMLButtonElement {
  const button = [
    ...document.querySelectorAll<HTMLButtonElement>('button'),
  ].find((candidate) => candidate.textContent?.trim() === text)
  assert.ok(button, `Expected button "${text}"`)
  return button
}

async function click(element: HTMLElement): Promise<void> {
  await act(async () => {
    element.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  })
}

async function changeInput(input: HTMLInputElement, value: string) {
  await act(async () => {
    const valueSetter = Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      'value'
    )?.set
    assert.ok(valueSetter)
    valueSetter.call(input, value)
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })
}

describe('quota plan execution confirmation', () => {
  test('enables execution only after the exact server phrase is entered', async () => {
    mocks.executePlan.mockImplementation(() => new Promise(() => {}))
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const rootRoute = createRootRoute({
      component: () => (
        <QueryClientProvider client={queryClient}>
          <I18nextProvider i18n={i18n}>
            <PlanActions detail={detail} />
          </I18nextProvider>
        </QueryClientProvider>
      ),
    })
    const router = createRouter({
      routeTree: rootRoute,
      history: createMemoryHistory({ initialEntries: ['/'] }),
    })

    await act(async () => {
      await router.load()
      root.render(<RouterProvider router={router} />)
    })
    await click(findButton('Execute plan'))

    const executeNow = findButton('Execute now')
    const input = document.querySelector<HTMLInputElement>(
      '#execute-plan-confirmation'
    )
    assert.ok(input)
    assert.equal(executeNow.disabled, true)

    await changeInput(input, 'EXECUTE PLAN')
    assert.equal(executeNow.disabled, true)
    assert.match(document.body.textContent ?? '', /does not match/)
    assert.equal(input.getAttribute('aria-invalid'), 'true')
    const describedBy = input.getAttribute('aria-describedby')?.split(' ') ?? []
    assert.ok(describedBy.includes('execute-plan-confirmation-description'))
    assert.ok(describedBy.includes('execute-plan-confirmation-error'))
    assert.match(
      document.querySelector('#execute-plan-confirmation-error')?.textContent ??
        '',
      /does not match/
    )

    await changeInput(input, confirmationPhrase)
    assert.equal(executeNow.disabled, false)

    await click(findButton('Cancel'))
    await click(findButton('Execute plan'))
    const reopenedInput = document.querySelector<HTMLInputElement>(
      '#execute-plan-confirmation'
    )
    assert.ok(reopenedInput)
    assert.equal(reopenedInput.value, '')
    assert.equal(reopenedInput.getAttribute('aria-invalid'), 'false')
    assert.equal(
      reopenedInput.getAttribute('aria-describedby'),
      'execute-plan-confirmation-description'
    )
    assert.equal(findButton('Execute now').disabled, true)

    await changeInput(reopenedInput, confirmationPhrase)
    await click(findButton('Execute now'))
    await vi.waitFor(() => {
      assert.equal(findButton('Execute now').disabled, true)
    })
    const closeButton = document.querySelector<HTMLButtonElement>(
      '[data-slot="dialog-close"]'
    )
    assert.ok(closeButton)
    await click(closeButton)
    assert.equal(
      document.querySelector('#execute-plan-confirmation'),
      reopenedInput
    )
    assert.equal(findButton('Execute now').disabled, true)

    await act(async () => root.unmount())
    container.remove()
  })
})
