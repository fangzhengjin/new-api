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

import type { FairnessShadowComparison } from '../../types'

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { FairnessShadowResult } = await import('../fairness-shadow-comparison')

const i18n = createInstance()
await i18n.use(initReactI18next).init({ lng: 'en' })

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

const metrics = {
  population: 2,
  minimum_coverage_basis_points: 4_600,
  p10_coverage_basis_points: 4_600,
  p50_coverage_basis_points: 4_600,
  p90_coverage_basis_points: 9_400,
  minimum_safety_coverage_basis_points: 10_000,
  safety_unmet: 0,
  new_user_count: 0,
  new_user_coverage_basis_points: null,
  reclaimed_quota: '0',
  recovery_reserve_quota: '10000000',
  occupied_after_quota: '170000000',
}

const result: FairnessShadowComparison = {
  snapshot_at: 1_786_886_400,
  stage_cap_quota: '170000000',
  current_algorithm_version: '1.8.0',
  candidate_algorithm_version: '2.0.0',
  candidate_qualified: true,
  current: metrics,
  candidate: {
    ...metrics,
    minimum_coverage_basis_points: 5_000,
    p10_coverage_basis_points: 5_000,
  },
  items: [
    {
      user_id: 7,
      username: 'alice',
      current_balance_quota: '5000000',
      safety_target_quota: '0',
      demand_target_quota: '50000000',
      target_quota: '50000000',
      current_adjustment_quota: '18000000',
      candidate_adjustment_quota: '20000000',
      current_after_quota: '23000000',
      candidate_after_quota: '25000000',
      current_coverage_basis_points: 4_600,
      candidate_coverage_basis_points: 5_000,
    },
  ],
}

test('shows a text gate result and both algorithm outcomes', async () => {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <FairnessShadowResult result={result} />
      </I18nextProvider>
    )
  })

  assert.match(container.textContent ?? '', /Candidate passes shadow gate/)
  assert.match(container.textContent ?? '', /1\.8\.0/)
  assert.match(container.textContent ?? '', /2\.0\.0/)
  assert.match(container.textContent ?? '', /alice/)
  assert.match(container.textContent ?? '', /46%/)
  assert.match(container.textContent ?? '', /50%/)

  await act(async () => root.unmount())
  container.remove()
})
