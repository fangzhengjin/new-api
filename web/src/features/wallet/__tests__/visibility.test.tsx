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
import { render, screen } from '@testing-library/react'
import { beforeEach, expect, test, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  status: {} as Record<string, unknown>,
  getSelf: vi.fn(),
}))

vi.mock('@/components/layout', () => {
  const SectionPageLayout = (props: { children: React.ReactNode }) => (
    <div>{props.children}</div>
  )
  SectionPageLayout.Title = (props: { children: React.ReactNode }) => (
    <h1>{props.children}</h1>
  )
  SectionPageLayout.Content = (props: { children: React.ReactNode }) => (
    <main>{props.children}</main>
  )
  return { SectionPageLayout }
})

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({ status: mocks.status }),
}))

vi.mock('@/hooks/use-system-config', () => ({
  useSystemConfig: () => ({
    currency: { quotaDisplayType: 'USD', usdExchangeRate: 1 },
  }),
}))

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  getSelf: mocks.getSelf,
}))

vi.mock('../hooks', () => ({
  useTopupInfo: () => ({ topupInfo: null, presetAmounts: [], loading: false }),
  usePayment: () => ({
    amount: 0,
    calculating: false,
    processing: false,
    calculatePaymentAmount: vi.fn(),
    processPayment: vi.fn(),
  }),
  useAffiliate: () => ({
    affiliateLink: '',
    loading: false,
    transferQuota: vi.fn(),
    transferring: false,
  }),
  useRedemption: () => ({ redeeming: false, redeemCode: vi.fn() }),
  useCreemPayment: () => ({ processing: false, processCreemPayment: vi.fn() }),
  useWaffoPayment: () => ({ processing: false, processWaffoPayment: vi.fn() }),
  useWaffoPancakePayment: () => ({
    processing: false,
    processWaffoPancakePayment: vi.fn(),
  }),
}))

vi.mock('../components/wallet-stats-card', () => ({
  WalletStatsCard: () => <div>Wallet stats</div>,
}))
vi.mock('../components/recharge-form-card', () => ({
  RechargeFormCard: () => <div>Add funds section</div>,
}))
vi.mock('../components/subscription-plans-card', () => ({
  SubscriptionPlansCard: () => <div>Subscription section</div>,
}))
vi.mock('../components/affiliate-rewards-card', () => ({
  AffiliateRewardsCard: () => <div>Referral section</div>,
}))
vi.mock('../components/dialogs/billing-history-dialog', () => ({
  BillingHistoryDialog: () => null,
}))
vi.mock('../components/dialogs/creem-confirm-dialog', () => ({
  CreemConfirmDialog: () => null,
}))
vi.mock('../components/dialogs/payment-confirm-dialog', () => ({
  PaymentConfirmDialog: () => null,
}))
vi.mock('../components/dialogs/transfer-dialog', () => ({
  TransferDialog: () => null,
}))

const { Wallet } = await import('../index')

beforeEach(() => {
  mocks.getSelf.mockResolvedValue({ success: true, data: { quota: 0 } })
  mocks.status = {
    cycle_quota_management_enabled: true,
    SidebarModulesAdmin: JSON.stringify({
      personal: {
        enabled: true,
        topup: true,
        wallet_add_funds: true,
        wallet_subscriptions: true,
        wallet_affiliate: true,
      },
    }),
  }
})

test('cycle quota management does not directly hide wallet sections', () => {
  render(<Wallet />)

  expect(screen.getByText('Add funds section')).toBeInTheDocument()
  expect(screen.getByText('Subscription section')).toBeInTheDocument()
  expect(screen.getByText('Referral section')).toBeInTheDocument()
})

test('wallet section switches independently hide their content', () => {
  mocks.status = {
    cycle_quota_management_enabled: false,
    SidebarModulesAdmin: JSON.stringify({
      personal: {
        enabled: true,
        topup: true,
        wallet_add_funds: false,
        wallet_subscriptions: true,
        wallet_affiliate: false,
      },
    }),
  }

  render(<Wallet />)

  expect(screen.queryByText('Add funds section')).toBeNull()
  expect(screen.getByText('Subscription section')).toBeInTheDocument()
  expect(screen.queryByText('Referral section')).toBeNull()
})

test('subscription visibility switch hides the subscription section', () => {
  mocks.status = {
    cycle_quota_management_enabled: true,
    SidebarModulesAdmin: JSON.stringify({
      personal: {
        enabled: true,
        topup: true,
        wallet_add_funds: true,
        wallet_subscriptions: false,
        wallet_affiliate: true,
      },
    }),
  }

  render(<Wallet />)

  expect(screen.getByText('Add funds section')).toBeInTheDocument()
  expect(screen.queryByText('Subscription section')).toBeNull()
  expect(screen.getByText('Referral section')).toBeInTheDocument()
})
