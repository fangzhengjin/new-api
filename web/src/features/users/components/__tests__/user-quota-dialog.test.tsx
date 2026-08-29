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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { beforeEach, expect, test, vi } from 'vitest'

import { useSystemConfigStore } from '@/stores/system-config-store'

const mocks = vi.hoisted(() => ({ adjustUserQuota: vi.fn() }))

vi.mock('../../api', () => ({ adjustUserQuota: mocks.adjustUserQuota }))

const { createInstance } = await import('i18next')
const { UserQuotaDialog } = await import('../user-quota-dialog')

const i18n = createInstance()
await i18n.use(initReactI18next).init({ lng: 'en' })

beforeEach(() => {
  mocks.adjustUserQuota.mockReset()
  mocks.adjustUserQuota.mockResolvedValue({ success: true })
  useSystemConfigStore
    .getState()
    .setConfig({ cycleQuotaManagementEnabled: true })
})

test('cycle quota management submits one adjustment with a required reason', async () => {
  render(
    <I18nextProvider i18n={i18n}>
      <UserQuotaDialog
        open
        onOpenChange={() => {}}
        userId={7}
        currentQuota={5_000_000}
        onSuccess={() => {}}
      />
    </I18nextProvider>
  )

  expect(screen.queryByRole('button', { name: 'Override' })).toBeNull()
  expect(screen.getByRole('button', { name: 'Increase' })).toHaveAttribute(
    'aria-pressed',
    'true'
  )
  expect(screen.getByRole('button', { name: 'Decrease' })).toHaveAttribute(
    'aria-pressed',
    'false'
  )
  expect(screen.getByLabelText('Reason')).toBeRequired()
  fireEvent.change(screen.getByLabelText(/Amount/), {
    target: { value: '2' },
  })
  fireEvent.change(screen.getByLabelText('Reason'), {
    target: { value: 'Project need' },
  })

  expect(screen.getByText(/Balance after increase:/)).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Confirm increase' }))
  await waitFor(() =>
    expect(mocks.adjustUserQuota).toHaveBeenCalledWith({
      id: 7,
      action: 'add_quota',
      mode: 'add',
      value: 1_000_000,
      reason: 'Project need',
    })
  )
})

test('cycle quota management rejects an empty reason before submission', () => {
  render(
    <I18nextProvider i18n={i18n}>
      <UserQuotaDialog
        open
        onOpenChange={() => {}}
        userId={7}
        currentQuota={5_000_000}
        onSuccess={() => {}}
      />
    </I18nextProvider>
  )
  fireEvent.change(screen.getByLabelText(/Amount/), {
    target: { value: '2' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Confirm increase' }))
  expect(screen.getByRole('alert')).toHaveTextContent('Reason is required')
  expect(mocks.adjustUserQuota).not.toHaveBeenCalled()
})

test('disabled cycle quota management keeps override and submits without a reason', async () => {
  useSystemConfigStore
    .getState()
    .setConfig({ cycleQuotaManagementEnabled: false })
  render(
    <I18nextProvider i18n={i18n}>
      <UserQuotaDialog
        open
        onOpenChange={() => {}}
        userId={8}
        currentQuota={5_000_000}
        onSuccess={() => {}}
      />
    </I18nextProvider>
  )

  expect(screen.getByRole('button', { name: 'Override' })).toBeInTheDocument()
  expect(screen.queryByLabelText('Reason')).toBeNull()
  fireEvent.change(screen.getByLabelText(/Amount/), {
    target: { value: '1' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Confirm increase' }))

  await waitFor(() =>
    expect(mocks.adjustUserQuota).toHaveBeenCalledWith({
      id: 8,
      action: 'add_quota',
      mode: 'add',
      value: 500_000,
      reason: undefined,
    })
  )
})

test('cycle quota management rejects empty, zero, and negative amounts before submission', () => {
  render(
    <I18nextProvider i18n={i18n}>
      <UserQuotaDialog
        open
        onOpenChange={() => {}}
        userId={7}
        currentQuota={5_000_000}
        onSuccess={() => {}}
      />
    </I18nextProvider>
  )

  fireEvent.click(screen.getByRole('button', { name: 'Confirm increase' }))
  expect(screen.getByRole('alert')).toHaveTextContent('Enter a valid amount')

  fireEvent.change(screen.getByLabelText(/Amount/), {
    target: { value: '0' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Confirm increase' }))
  expect(screen.getByRole('alert')).toHaveTextContent(
    'Enter an amount greater than 0'
  )

  fireEvent.change(screen.getByLabelText(/Amount/), {
    target: { value: '-1' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Confirm increase' }))
  expect(screen.getByRole('alert')).toHaveTextContent(
    'Enter an amount greater than 0'
  )
  expect(mocks.adjustUserQuota).not.toHaveBeenCalled()
})
