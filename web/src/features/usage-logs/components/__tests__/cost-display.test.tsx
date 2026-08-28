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
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import type React from 'react'
import { beforeAll, describe, expect, test } from 'vitest'

import { formatLogQuota } from '@/lib/format'

import { LOG_TYPE_ENUM } from '../../constants'
import { LogCostDisplay } from '../log-cost-display'

function renderCost(
  props: React.ComponentProps<typeof LogCostDisplay>
): ReturnType<typeof render> {
  return render(<LogCostDisplay {...props} />)
}

function normalizedText(value: string | null): string {
  return (value ?? '').replaceAll(/\s/g, '')
}

describe('log cost display', () => {
  beforeAll(() => {
    i18next.addResourceBundle('en', 'translation', {
      'Deducted by subscription': 'Deducted by subscription',
      'Includes tool-call surcharge': 'Includes tool-call surcharge',
      'Request failed, no fee was charged':
        'Request failed, no fee was charged',
    })
  })

  test('keeps the regular cost visible and adds an accessible surcharge marker', () => {
    const rendered = renderCost({
      logType: LOG_TYPE_ENUM.CONSUME,
      quota: 12500,
      other: {
        tool_surcharges: [{ name: 'lookup_customer', count: 1, price: 5 }],
      },
    })

    expect(
      normalizedText(rendered.container.textContent).includes(
        normalizedText(formatLogQuota(12500))
      )
    ).toBe(true)
    const costBadge = rendered.container.querySelector(
      '[data-cost-tone="metered"]'
    )
    expect(costBadge).not.toHaveAttribute('title')
    const marker = screen.getByRole('img', {
      name: 'Includes tool-call surcharge',
    })
    expect(marker).toHaveAttribute('data-tool-surcharge-indicator', 'true')
    expect(marker).toHaveAttribute('tabindex', '0')
  })

  test('opens the subscription tooltip on hover while keeping the amount visible', async () => {
    const user = userEvent.setup()
    const rendered = renderCost({
      logType: LOG_TYPE_ENUM.CONSUME,
      quota: 5000,
      other: {
        billing_source: 'subscription',
        web_search: true,
        web_search_call_count: 1,
        web_search_price: 10,
      },
    })

    const formattedQuota = formatLogQuota(5000)
    expect(
      normalizedText(rendered.container.textContent).includes(
        normalizedText(formattedQuota)
      )
    ).toBe(true)
    const costBadge = screen.getByLabelText(
      `Deducted by subscription: ${formattedQuota}`
    )
    expect(costBadge).toHaveAttribute('data-cost-tone', 'subscription')
    expect(costBadge).toHaveAttribute('tabindex', '0')
    expect(
      screen.getByRole('img', { name: 'Includes tool-call surcharge' })
    ).toHaveAttribute('data-tool-surcharge-indicator', 'true')

    await user.hover(costBadge)

    expect(await screen.findByText('Deducted by subscription')).toBeVisible()
  })

  test('opens the subscription tooltip on click without hover or focus', async () => {
    renderCost({
      logType: LOG_TYPE_ENUM.CONSUME,
      quota: 5000,
      other: { billing_source: 'subscription' },
    })

    fireEvent.click(
      screen.getByLabelText(`Deducted by subscription: ${formatLogQuota(5000)}`)
    )

    expect(await screen.findByText('Deducted by subscription')).toBeVisible()
  })

  test('opens the failed-request tooltip from the keyboard', async () => {
    const user = userEvent.setup()
    renderCost({
      logType: LOG_TYPE_ENUM.ERROR,
      quota: 0,
      other: null,
    })

    const formattedQuota = formatLogQuota(0)
    const costBadge = screen.getByLabelText(
      `Request failed, no fee was charged: ${formattedQuota}`
    )
    expect(costBadge).toHaveAttribute('data-cost-tone', 'error')

    await user.tab()

    expect(costBadge).toHaveFocus()
    expect(
      await screen.findByText('Request failed, no fee was charged')
    ).toBeVisible()
  })
})
