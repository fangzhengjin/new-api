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

import type { QuotaCycle } from '../../types'

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { createMemoryHistory, createRootRoute, createRouter, RouterProvider } =
  await import('@tanstack/react-router')
const { CycleTable } = await import('../cycle-table')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        'Balance at cycle end': 'Balance at cycle end',
        'Carry forward': 'Carry forward',
        'Reset to zero': 'Reset to zero',
      },
    },
  },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

function cycle(id: number, balancePolicy: 'reset' | 'carry'): QuotaCycle {
  return {
    id,
    cycle_start_at: 1_786_886_400,
    cycle_end_at: 1_789_478_400,
    budget_quota: '1000000',
    initial_grant_quota: '100000',
    balance_policy: balancePolicy,
    status: 'scheduled',
    settlement_plan_id: null,
    settled_at: null,
    restored_at: null,
    restored_by: '',
    created_at: 1_786_886_400,
    created_by: 'root',
    updated_at: 1_786_886_400,
    updated_by: 'root',
  }
}

test('shows reset and carry policies as text instead of color alone', async () => {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)
  const rootRoute = createRootRoute({
    component: () => (
      <I18nextProvider i18n={i18n}>
        <CycleTable cycles={[cycle(1, 'reset'), cycle(2, 'carry')]} />
      </I18nextProvider>
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

  assert.match(container.textContent ?? '', /Reset to zero/)
  assert.match(container.textContent ?? '', /Carry forward/)

  await act(async () => root.unmount())
  container.remove()
})
