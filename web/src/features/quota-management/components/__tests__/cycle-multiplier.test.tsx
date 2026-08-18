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

import { test } from 'vitest'

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { QueryClient, QueryClientProvider } =
  await import('@tanstack/react-query')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { CreateCycleDialog } = await import('../create-cycle-dialog')

const i18n = createInstance()
await i18n.use(initReactI18next).init({ lng: 'en' })

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

test('requires one automatic allocation limit when creating a cycle', async () => {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })

  await act(async () => {
    root.render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <CreateCycleDialog
            open
            onOpenChange={() => {}}
            defaults={{ start_at: 1_786_886_400, end_at: 1_789_478_400 }}
            recommendation={null}
          />
        </I18nextProvider>
      </QueryClientProvider>
    )
  })

  const select = document.querySelector<HTMLSelectElement>(
    '#quota-cycle-concentration-multiplier'
  )
  assert.ok(select)
  assert.equal(select.value, '')
  const createButton = [...document.querySelectorAll('button')].find(
    (button) => button.textContent?.trim() === 'Create'
  )
  assert.ok(createButton)
  await act(async () => createButton.click())
  assert.match(
    document.body.textContent ?? '',
    /Select an automatic allocation limit/
  )
  assert.equal(select.getAttribute('aria-invalid'), 'true')

  await act(async () => root.unmount())
  queryClient.clear()
  container.remove()
})
