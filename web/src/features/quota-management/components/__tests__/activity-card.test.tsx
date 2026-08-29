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

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, test } from 'vitest'

import type { QuotaActivity } from '../../types'
import { queryKeys } from '../../utils'
import { ActivityCard } from '../activity-card'

Object.defineProperty(Element.prototype, 'getAnimations', {
  configurable: true,
  value: () => [],
})

const i18n = createInstance()
await i18n.use(initReactI18next).init({ lng: 'en' })

const activities: QuotaActivity[] = [
  {
    id: 2,
    action: 'user.quota_adjustment_plan',
    params: {
      target_user_id: 11,
      plan_id: 73,
      adjustment_quota: '250000',
      reason: '项目补充额度',
    },
    operator_id: 8,
    operator: 'quota-admin',
    operator_display_name: '王小明',
    target: { id: 11, username: 'zhangsan', display_name: '张三(北京)' },
    created_at: 1_787_800_000,
  },
  {
    id: 1,
    action: 'quota.plan.execute',
    params: {
      plan_id: 72,
      affected_users: 3,
      issued_quota: '500000',
      reclaimed_quota: '100000',
      net_change_quota: '400000',
    },
    operator_id: 8,
    operator: 'quota-admin',
    operator_display_name: '王小明',
    created_at: 1_787_700_000,
  },
  {
    id: 0,
    action: 'user.quota_adjustment_plan',
    params: { target_user_id: 12, adjustment_quota: '-50000' },
    operator_id: 8,
    operator: 'quota-admin',
    operator_display_name: '王小明',
    target: { id: 12, username: 'lisi', display_name: '李四' },
    created_at: 1_787_600_000,
  },
]

describe('quota activity card', () => {
  test('shows mixed event summaries and opens one consistent detail dialog', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    })
    queryClient.setQueryData(queryKeys.activities, activities)
    const rootRoute = createRootRoute({
      component: () => (
        <QueryClientProvider client={queryClient}>
          <I18nextProvider i18n={i18n}>
            <ActivityCard />
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

    assert.match(container.textContent ?? '', /张三\(北京\)/)
    assert.match(container.textContent ?? '', /zhangsan/)
    assert.match(container.textContent ?? '', /王小明/)
    assert.match(container.textContent ?? '', /quota-admin/)
    assert.match(container.textContent ?? '', /Plan #72 executed/)
    assert.doesNotMatch(container.textContent ?? '', /User quota adjusted/)

    const adjustment = [...container.querySelectorAll('button')].find(
      (button) => button.textContent?.includes('张三(北京)')
    )
    assert.ok(adjustment)
    const user = userEvent.setup()
    adjustment.focus()
    await user.keyboard('{Enter}')

    const dialog = document.querySelector('[role="dialog"]')
    assert.ok(dialog)
    assert.match(dialog.textContent ?? '', /User quota adjusted/)
    assert.match(dialog.textContent ?? '', /Adjustment target/)
    assert.match(dialog.textContent ?? '', /Operator/)
    assert.match(dialog.textContent ?? '', /View plan details/)

    await user.keyboard('{Escape}')
    const historical = [...container.querySelectorAll('button')].find(
      (button) => button.textContent?.includes('李四')
    )
    assert.ok(historical)
    historical.focus()
    await user.keyboard('{Enter}')

    const historicalDialog = document.querySelector('[role="dialog"]')
    assert.ok(historicalDialog)
    assert.match(
      historicalDialog.textContent ?? '',
      /Only the recorded audit information/
    )
    assert.doesNotMatch(historicalDialog.textContent ?? '', /View plan details/)

    await user.keyboard('{Escape}')
    const execution = [...container.querySelectorAll('button')].find((button) =>
      button.textContent?.includes('Plan #72 executed')
    )
    assert.ok(execution)
    execution.focus()
    await user.keyboard('{Enter}')

    const executionDialog = document.querySelector('[role="dialog"]')
    assert.ok(executionDialog)
    assert.match(executionDialog.textContent ?? '', /Issued in this plan/)
    assert.match(executionDialog.textContent ?? '', /Reclaimed in this plan/)
    assert.match(executionDialog.textContent ?? '', /Net change/)
    assert.match(executionDialog.textContent ?? '', /Affected users/)

    await act(async () => root.unmount())
    container.remove()
  })
})
