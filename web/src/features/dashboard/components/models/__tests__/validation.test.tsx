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

import type { DashboardFilters } from '@/features/dashboard/types'

Object.defineProperty(Element.prototype, 'getAnimations', {
  configurable: true,
  value: () => [],
})

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { toast } = await import('sonner')
const { ModelsFilter } = await import('../models-filter-dialog')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        Filter: 'Filter',
        'Apply Filters': 'Apply Filters',
        'Please select both start and end times':
          'Please select both start and end times',
        'Start time must be earlier than end time':
          'Start time must be earlier than end time',
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

async function submitFilters(currentFilters: DashboardFilters) {
  const applied: DashboardFilters[] = []
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <ModelsFilter
          preferences={preferences}
          currentFilters={currentFilters}
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
  await act(async () => findButton('Apply Filters')?.click())

  const latestToast = toast.getHistory().at(-1)

  const result = {
    applied,
    dialogOpen: findButton('Apply Filters') !== undefined,
    message:
      latestToast && 'title' in latestToast ? latestToast.title : undefined,
  }

  await act(async () => root.unmount())
  container.remove()
  return result
}

describe('model dashboard filter validation', () => {
  test('keeps the dialog open and prompts when either boundary is missing', async () => {
    const result = await submitFilters({
      start_timestamp: undefined,
      end_timestamp: new Date(2026, 6, 30, 23, 59, 59, 999),
      time_granularity: 'hour',
      username: '',
    })

    assert.equal(result.applied.length, 0)
    assert.equal(result.dialogOpen, true)
    assert.equal(result.message, 'Please select both start and end times')
  })

  test('keeps the dialog open and prompts when the range is reversed', async () => {
    const result = await submitFilters({
      start_timestamp: new Date(2026, 6, 31),
      end_timestamp: new Date(2026, 6, 30, 23, 59, 59, 999),
      time_granularity: 'hour',
      username: '',
    })

    assert.equal(result.applied.length, 0)
    assert.equal(result.dialogOpen, true)
    assert.equal(result.message, 'Start time must be earlier than end time')
  })
})
