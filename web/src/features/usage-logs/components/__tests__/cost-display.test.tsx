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
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import type React from 'react'
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  test,
} from 'vitest'

import { useSystemConfigStore } from '@/stores/system-config-store'

import { LOG_TYPE_ENUM } from '../../constants'
import { LogCostDisplay } from '../log-cost-display'

function renderCost(
  props: React.ComponentProps<typeof LogCostDisplay>
): ReturnType<typeof render> {
  return render(<LogCostDisplay {...props} />)
}

describe('log cost display', () => {
  beforeAll(() => {
    i18next.addResourceBundle('en', 'translation', {
      'Deducted by subscription': 'Deducted by subscription',
      Subscription: 'Subscription',
      Wallet: 'Wallet',
      'Includes tool-call surcharge': 'Includes tool-call surcharge',
      'Request failed, no fee was charged':
        'Request failed, no fee was charged',
    })
  })

  beforeEach(() => {
    useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  })

  afterEach(() => {
    useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
    localStorage.clear()
  })

  test.each([
    { consumed: 12500, expected: '$0.025' },
    { consumed: 0, expected: '$0' },
    { consumed: 1, expected: '$0.000002' },
    { consumed: undefined, expected: '$0.01' },
  ])(
    'shows subscription deduction $consumed without hover, falling back only when absent',
    ({ consumed, expected }) => {
      const rendered = renderCost({
        logType: LOG_TYPE_ENUM.CONSUME,
        quota: 5000,
        other: {
          billing_source: 'subscription',
          subscription_consumed: consumed,
        },
        showBillingSource: true,
      })

      expect(rendered.container.textContent).toContain(expected)
      expect(screen.getByRole('img', { name: 'Subscription' })).toBeVisible()
      expect(screen.queryByText('Subscription')).not.toBeInTheDocument()
      expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
    }
  )

  test('shows wallet cost and source icon without using subscription metadata', () => {
    const rendered = renderCost({
      logType: LOG_TYPE_ENUM.CONSUME,
      quota: 5000,
      other: { billing_source: 'wallet', subscription_consumed: 12500 },
      showBillingSource: true,
    })

    expect(rendered.container.textContent).toContain('$0.01')
    expect(screen.getByRole('img', { name: 'Wallet' })).toBeVisible()
    expect(screen.queryByText('Wallet')).not.toBeInTheDocument()
    expect(screen.queryByText('Subscription')).not.toBeInTheDocument()
  })

  test('hides the wallet icon when subscriptions are unavailable', () => {
    const rendered = renderCost({
      logType: LOG_TYPE_ENUM.CONSUME,
      quota: 5000,
      other: { billing_source: 'wallet' },
      showBillingSource: false,
    })

    expect(rendered.container.textContent).toContain('$0.01')
    expect(screen.queryByText('Wallet')).not.toBeInTheDocument()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })

  test('keeps the subscription icon on a subscription-billed log when billing sources are hidden', () => {
    const rendered = renderCost({
      logType: LOG_TYPE_ENUM.CONSUME,
      quota: 5000,
      other: { billing_source: 'subscription', subscription_consumed: 12500 },
      showBillingSource: false,
    })

    expect(rendered.container.textContent).toContain('$0.025')
    expect(screen.getByRole('img', { name: 'Subscription' })).toBeVisible()
    expect(screen.queryByRole('img', { name: 'Wallet' })).not.toBeInTheDocument()
  })

  test('keeps legacy cost visible without inventing a funding source', () => {
    const rendered = renderCost({ logType: LOG_TYPE_ENUM.CONSUME, quota: 5000, other: null })

    expect(rendered.container.textContent).toContain('$0.01')
    expect(screen.queryByText('Wallet')).not.toBeInTheDocument()
    expect(screen.queryByText('Subscription')).not.toBeInTheDocument()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })

  test('keeps a large amount unabridged in a single-line cost bubble', () => {
    const rendered = renderCost({
      logType: LOG_TYPE_ENUM.CONSUME,
      quota: 2147483647,
      other: { billing_source: 'subscription' },
      showBillingSource: true,
    })

    expect(rendered.container.textContent).toContain('$4,294.9673')
    const badge = rendered.container.querySelector(
      '[data-cost-tone="subscription"]'
    )
    expect(badge).toHaveClass(
      'border',
      'rounded-md',
      'tabular-nums',
      'whitespace-nowrap'
    )
    expect(rendered.container.firstElementChild).toHaveClass('inline-flex')
    expect(rendered.container.firstElementChild).not.toHaveClass('flex-col')
    expect(screen.getByRole('img', { name: 'Subscription' })).toBeVisible()
  })

  test.each([
    { source: 'subscription', label: 'Subscription' },
    { source: 'wallet', label: 'Wallet' },
  ])(
    'reveals the $source label on hover and keyboard focus',
    async ({ source, label }) => {
      const user = userEvent.setup()
      renderCost({
        logType: LOG_TYPE_ENUM.CONSUME,
        quota: 5000,
        other: { billing_source: source },
        showBillingSource: true,
      })

      const marker = screen.getByRole('img', { name: label })
      expect(screen.queryByText(label)).not.toBeInTheDocument()

      await user.hover(marker)
      expect(await screen.findByText(label)).toBeVisible()
      await user.unhover(marker)
      await waitFor(() =>
        expect(screen.queryByText(label)).not.toBeInTheDocument()
      )
      act(() => marker.focus())
      expect(marker).toHaveFocus()
      expect(await screen.findByText(label)).toBeVisible()
    }
  )

  test('keeps the regular cost visible and adds an accessible surcharge marker', () => {
    const rendered = renderCost({
      logType: LOG_TYPE_ENUM.CONSUME,
      quota: 12500,
      other: {
        tool_surcharges: [{ name: 'lookup_customer', count: 1, price: 5 }],
      },
    })

    expect(rendered.container.textContent).toContain('$0.025')
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

    expect(rendered.container.textContent).toContain('$0.01')
    const costBadge = screen.getByLabelText(
      `Deducted by subscription: $0.01`
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

    screen.getByLabelText(`Deducted by subscription: $0.01`).click()

    expect(await screen.findByText('Deducted by subscription')).toBeVisible()
  })

  test('opens the failed-request tooltip from the keyboard', async () => {
    const user = userEvent.setup()
    renderCost({
      logType: LOG_TYPE_ENUM.ERROR,
      quota: 0,
      other: null,
    })

    const costBadge = screen.getByLabelText(
      `Request failed, no fee was charged: $0`
    )
    expect(costBadge).toHaveAttribute('data-cost-tone', 'error')

    await user.tab()

    expect(costBadge).toHaveFocus()
    expect(
      await screen.findByText('Request failed, no fee was charged')
    ).toBeVisible()
  })

  test('shows subscription cost and source alongside the legacy surcharge marker', () => {
    const rendered = renderCost({
      logType: LOG_TYPE_ENUM.CONSUME,
      quota: 5000,
      other: {
        billing_source: 'subscription',
        web_search: true,
        web_search_call_count: 1,
        web_search_price: 10,
      },
      showBillingSource: true,
    })

    expect(rendered.container.textContent).toContain('$0.01')
    expect(screen.getByRole('img', { name: 'Subscription' })).toBeVisible()
    expect(
      screen.getByRole('img', { name: 'Includes tool-call surcharge' })
    ).toHaveAttribute('data-tool-surcharge-indicator', 'true')
  })
})
