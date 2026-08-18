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
const { fireEvent } = await import('@testing-library/react')
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
  concentration_variants: [15_000, 20_000, 30_000].map((multiplier, index) => ({
    multiplier_basis_points: multiplier,
    population: 2,
    position_ceiling_quota: String(75_000_000 + index * 25_000_000),
    maximum_position_share_basis_points: 5_200,
    capped_users: 1,
    capped_quota: String(30_000_000 - index * 10_000_000),
    minimum_raw_coverage_basis_points: 3_000 + index * 500,
    p10_raw_coverage_basis_points: 3_000 + index * 500,
    p50_raw_coverage_basis_points: 6_000,
    minimum_effective_coverage_basis_points: 8_000,
    p10_effective_coverage_basis_points: 8_000,
    p50_effective_coverage_basis_points: 10_000,
    minimum_safety_coverage_basis_points: 10_000,
    safety_unmet: 0,
    planned_increase_quota: '40000000',
    reclaimed_quota: '0',
    occupied_after_quota: '140000000',
    unallocated_stage_quota: '30000000',
    items: [
      {
        user_id: 7,
        username: 'alice',
        period_spend_quota: '80000000',
        current_balance_quota: '5000000',
        current_position_quota: '85000000',
        spend_share_basis_points: 4_000,
        safety_target_quota: '5000000',
        raw_target_quota: '100000000',
        effective_target_quota: '75000000',
        position_ceiling_quota: String(75_000_000 + index * 25_000_000),
        adjustment_quota: '10000000',
        after_balance_quota: '15000000',
        after_position_quota: '95000000',
        capped_quota: String(30_000_000 - index * 10_000_000),
        raw_coverage_basis_points: 1_500,
        effective_coverage_basis_points: 2_000,
      },
    ],
  })),
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
  assert.match(
    container.textContent ?? '',
    /Cumulative position concentration comparison/
  )
  assert.match(container.textContent ?? '', /1\.5×/)
  assert.match(container.textContent ?? '', /Period spend/)
  assert.match(container.textContent ?? '', /Raw demand coverage/)
  assert.match(container.textContent ?? '', /Effective coverage/)

  await act(async () => root.unmount())
  container.remove()
})

test('shows the cycle multiplier first and paginates searchable user details', async () => {
  const pagedResult: FairnessShadowComparison = {
    ...result,
    concentration_variants: result.concentration_variants.map((variant) => ({
      ...variant,
      items: Array.from({ length: 60 }, (_, index) => ({
        ...variant.items[0],
        user_id: index + 1,
        username: `user-${index + 1}`,
      })),
    })),
  }
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <FairnessShadowResult
          result={pagedResult}
          selectedMultiplierBasisPoints={20_000}
        />
      </I18nextProvider>
    )
  })

  const hasExactCell = (value: string) =>
    [...container.querySelectorAll('td .font-medium')].some(
      (cell) => cell.textContent?.trim() === value
    )
  assert.match(container.textContent ?? '', /2× · Cycle setting/)
  assert.doesNotMatch(
    container.textContent ?? '',
    /Candidate passes shadow gate/
  )
  assert.equal(hasExactCell('user-50'), true)
  assert.equal(hasExactCell('user-51'), false)

  const nextButton = [...container.querySelectorAll('button')].find(
    (button) => button.textContent?.trim() === 'Next page'
  )
  assert.ok(nextButton)
  await act(async () => nextButton.click())
  assert.equal(hasExactCell('user-51'), true)

  const search = container.querySelector<HTMLInputElement>(
    '#concentration-user-search'
  )
  assert.ok(search)
  await act(async () => {
    fireEvent.change(search, { target: { value: 'user-60' } })
  })
  assert.equal(hasExactCell('user-60'), true)
  assert.equal(hasExactCell('user-51'), false)

  await act(async () => root.unmount())
  container.remove()
})
