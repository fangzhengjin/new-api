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

import type { PlanDetailData } from '../types'
import { formatQuota, queryKeys } from '../utils'

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
const { waitFor } = await import('@testing-library/react')
const { QuotaPlanDetail } = await import('../plan-detail')

const i18n = createInstance()
await i18n.use(initReactI18next).init({ lng: 'en' })

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

const detail = {
  plan: {
    id: 73,
    cycle_id: 8,
    name: 'August adjustment',
    purpose: 'Monthly allocation',
    plan_type: 'adjustment',
    planned_delta_quota: '30000000',
    allocation_after_quota: '25792498448',
    status: 'executed',
    snapshot_at: 1_787_817_600,
    next_adjustment_at: null,
    created_at: 1_787_817_600,
    executed_at: 1_787_817_700,
    cancelled_at: null,
    cancel_reason: '',
  },
  cycle: {
    id: 8,
    cycle_start_at: 1_786_086_000,
    cycle_end_at: 1_788_159_600,
  },
  items: [],
  summary: {
    affected: 3,
    increase: { count: 2, total: '50000000' },
    decrease: { count: 2, total: '20000000' },
    budget: '26250000000',
    occupied_after: '25792498448',
    cycle_pool_issued_quota: '35000000',
    reclaimed_used_for_increases: '15000000',
    reclaimed_used_to_cover_overage: '3000000',
    reclaimed_unused: '2000000',
    pool_remaining: '457501552',
  },
  confirmation_phrase: '',
} as unknown as PlanDetailData

function findLeaf(container: HTMLElement, text: string): HTMLElement {
  const element = [...container.querySelectorAll<HTMLElement>('*')].find(
    (candidate) =>
      candidate.childElementCount === 0 && candidate.textContent === text
  )
  assert.ok(element, `Expected text "${text}"`)
  return element
}

async function renderDetail(data: PlanDetailData) {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  queryClient.setQueryData(queryKeys.plan(data.plan.id), data)
  const rootRoute = createRootRoute({
    component: () => (
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <QuotaPlanDetail planId={data.plan.id} />
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
  await waitFor(() => {
    assert.ok(container.textContent?.includes('Cycle total'))
  })

  return {
    container,
    cleanup: async () => {
      await act(async () => root.unmount())
      queryClient.clear()
      container.remove()
    },
  }
}

describe('quota plan fund flow', () => {
  test('shows the cycle equation and each authoritative flow once', async () => {
    const view = await renderDetail(detail)
    try {
      const expectedMetrics = [
        ['Cycle total', detail.summary.budget],
        ['Total issued after this plan', detail.summary.occupied_after],
        ['Cycle remaining', detail.summary.pool_remaining],
      ] as const
      for (const [label, value] of expectedMetrics) {
        const block = findLeaf(view.container, label).parentElement
        assert.ok(block)
        assert.ok(block.textContent?.includes(formatQuota(value)))
      }
      assert.ok(view.container.textContent?.includes('Issued in this plan'))
      assert.ok(
        view.container.textContent?.includes('Reclaimed from user accounts')
      )
      const reissuedLabels = [
        ...view.container.querySelectorAll<HTMLElement>('*'),
      ].filter(
        (element) =>
          element.childElementCount === 0 &&
          element.textContent === 'Reclaimed and reissued'
      )
      assert.equal(reissuedLabels.length, 1)
    } finally {
      await view.cleanup()
    }
  })

  test('shows every fund destination for non-zero and zero values', async () => {
    const destinations = [
      ['Issued from cycle remaining quota', 'cycle_pool_issued_quota'],
      ['Reclaimed and reissued', 'reclaimed_used_for_increases'],
      ['Reclaimed to cover existing excess', 'reclaimed_used_to_cover_overage'],
      ['Reclaimed and returned to cycle quota pool', 'reclaimed_unused'],
    ] as const
    const view = await renderDetail(detail)
    try {
      for (const [label, key] of destinations) {
        const block = findLeaf(view.container, label).parentElement
        assert.ok(block)
        assert.ok(block.textContent?.includes(formatQuota(detail.summary[key])))
      }
    } finally {
      await view.cleanup()
    }

    const zeroSummary = {
      ...detail.summary,
      cycle_pool_issued_quota: '0',
      reclaimed_used_for_increases: '0',
      reclaimed_used_to_cover_overage: '0',
      reclaimed_unused: '0',
    }
    const zeroView = await renderDetail({ ...detail, summary: zeroSummary })
    try {
      for (const [label] of destinations) {
        const block = findLeaf(zeroView.container, label).parentElement
        assert.ok(block)
        assert.ok(block.textContent?.includes(formatQuota('0')))
      }
    } finally {
      await zeroView.cleanup()
    }
  })
})
