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

import { afterEach, test } from 'vitest'

import type { QuotaRecoveryRequest } from '@/features/quota-recovery/types'

import type { QuotaAlgorithmStatus } from '../../types'
import { queryKeys } from '../../utils'

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
const { AlgorithmSwitchCard } = await import('../algorithm-switch-card')
const { RecoveryRequestsCard } = await import('../recovery-requests-card')

const i18n = createInstance()
await i18n.use(initReactI18next).init({ lng: 'en' })

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

const mounted: Array<{ root: ReturnType<typeof createRoot>; node: Element }> =
  []

async function renderWithData(key: readonly unknown[], data: unknown) {
  const node = document.createElement('div')
  document.body.append(node)
  const root = createRoot(node)
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  queryClient.setQueryData(key, data)
  mounted.push({ root, node })
  return { root, node, queryClient }
}

function findButton(text: string): HTMLButtonElement {
  const button = [
    ...document.querySelectorAll<HTMLButtonElement>('button'),
  ].find((candidate) => candidate.textContent?.trim() === text)
  assert.ok(button, `Expected button "${text}"`)
  return button
}

async function click(element: HTMLElement) {
  await act(async () => {
    element.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  })
}

afterEach(async () => {
  for (const item of mounted.splice(0)) {
    await act(async () => item.root.unmount())
    item.node.remove()
  }
})

test('keeps the candidate switch disabled until every persisted gate passes', async () => {
  const status: QuotaAlgorithmStatus = {
    legacy_version: '1.8.0',
    current_version: '1.8.0',
    candidate_version: '2.0.0',
    enable_confirmation_phrase: '确认启用候选额度算法',
    rollback_confirmation_phrase: '确认回退现行额度算法',
    required_qualified_cycles: 2,
    qualified_cycle_ids: [7],
    active_cycle_id: 9,
    recovery_ready: true,
    draft_count: 1,
    can_switch: false,
    rollback_allowed: false,
    can_record_evidence: true,
    blockers: [],
  }
  const { root, queryClient } = await renderWithData(
    queryKeys.algorithm,
    status
  )

  await act(async () => {
    root.render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <AlgorithmSwitchCard />
        </I18nextProvider>
      </QueryClientProvider>
    )
  })

  const switchButton = findButton('Enable candidate algorithm')
  assert.equal(switchButton.disabled, true)
  assert.match(
    document.body.textContent ?? '',
    /1 more qualified closed cycles/
  )
  assert.match(document.body.textContent ?? '', /all 1 pending drafts/)
})

test('requires a review reason before a pending recovery can be granted', async () => {
  const request: QuotaRecoveryRequest = {
    id: 3,
    cycle_id: 9,
    user_id: 7,
    username: 'alice',
    display_name: 'Alice',
    requested_quota: '1000000',
    reason: 'continue priority work',
    status: 'pending',
    decision: '',
    approved_quota: '0',
    plan_id: null,
    reviewed_by: '',
    review_reason: '',
    created_at: 1_786_886_400,
    reviewed_at: null,
    executed_at: null,
  }
  const { root, queryClient } = await renderWithData(
    queryKeys.recoveryRequests,
    [request]
  )

  await act(async () => {
    root.render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <RecoveryRequestsCard />
        </I18nextProvider>
      </QueryClientProvider>
    )
  })
  await click(findButton('Approve'))

  assert.equal(findButton('Approve and grant').disabled, true)
  assert.ok(document.querySelector('#recovery-approved-amount'))
  assert.ok(document.querySelector('#recovery-review-reason'))
})
