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

Object.defineProperty(Element.prototype, 'getAnimations', {
  configurable: true,
  value: () => [],
})

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { ModelsFilter } = await import('../models-filter-dialog')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        Filter: 'Filter',
        Today: 'Today',
        '6 Hours': '6 Hours',
        'Apply Filters': 'Apply Filters',
      },
    },
  },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

const preferences = {
  consumptionDistributionChart: 'bar' as const,
  modelAnalyticsChart: 'trend' as const,
  defaultTimeRange: 'today' as const,
  defaultTimeGranularity: 'hour' as const,
}

async function applyQuickRange(label: string) {
  const applied: Array<{ start_timestamp?: Date; end_timestamp?: Date }> = []
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <ModelsFilter
          preferences={preferences}
          currentFilters={{
            start_timestamp: new Date(2026, 6, 29),
            end_timestamp: new Date(2026, 6, 30, 23, 59, 59, 999),
            time_granularity: 'hour',
            username: '',
          }}
          onFilterChange={(filters) => applied.push(filters)}
          onReset={() => undefined}
        />
      </I18nextProvider>
    )
  })

  const findButton = (text: string) =>
    [...document.querySelectorAll('button')].find(
      (button) => button.textContent === text
    )

  await act(async () => findButton('Filter')?.click())
  const rangeButton = findButton(label)
  assert.ok(rangeButton)
  await act(async () => rangeButton.click())
  await act(async () => findButton('Apply Filters')?.click())

  await act(async () => root.unmount())
  container.remove()
  return applied.at(-1)
}

describe('model dashboard quick ranges', () => {
  test('applies an exact rolling duration for an hour preset', async () => {
    const filters = await applyQuickRange('6 Hours')

    assert.ok(filters?.start_timestamp)
    assert.ok(filters.end_timestamp)
    assert.equal(
      filters.end_timestamp.getTime() - filters.start_timestamp.getTime(),
      6 * 3_600_000
    )
  })

  test('applies today to complete calendar-day boundaries', async () => {
    const filters = await applyQuickRange('Today')

    assert.ok(filters?.start_timestamp)
    assert.ok(filters.end_timestamp)
    assert.deepEqual(
      [
        filters.start_timestamp.getHours(),
        filters.start_timestamp.getMinutes(),
        filters.end_timestamp.getHours(),
        filters.end_timestamp.getMinutes(),
      ],
      [0, 0, 23, 59]
    )
  })
})
