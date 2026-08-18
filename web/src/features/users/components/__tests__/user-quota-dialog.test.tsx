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
import { useState } from 'react'
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
  useSystemConfigStore.getState().setConfig({ companyQuotaModeEnabled: true })
})

test('company mode submits once when the server reports no stage overage', async () => {
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
  expect(screen.getByLabelText('Reason (optional)')).toBeInTheDocument()
  fireEvent.change(screen.getByLabelText(/Amount/), {
    target: { value: '2' },
  })

  expect(screen.getByText(/Balance after increase:/)).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Confirm increase' }))
  await waitFor(() =>
    expect(mocks.adjustUserQuota).toHaveBeenCalledWith({
      id: 7,
      action: 'add_quota',
      mode: 'add',
      value: 1_000_000,
      reason: '',
      confirmed: false,
    })
  )
  expect(screen.queryByRole('alertdialog')).toBeNull()
})

test('company mode opens a second dialog only for the reported stage overage', async () => {
  mocks.adjustUserQuota
    .mockResolvedValueOnce({
      success: false,
      data: {
        stage_cap_quota: '50000000',
        occupied_after_quota: '52000000',
        stage_overage_quota: '9223372036854775807',
      },
    })
    .mockResolvedValueOnce({ success: true })

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
  const confirmation = await screen.findByRole('alertdialog')
  expect(confirmation).toHaveTextContent(
    'This increase exceeds the current stage company quota limit'
  )
  expect(confirmation).toHaveTextContent('Current stage company quota limit')
  expect(confirmation).toHaveTextContent(
    'Cycle consumption plus all managed account balances after increase'
  )
  expect(confirmation).toHaveTextContent('Amount over the limit')
  expect(confirmation).toHaveTextContent('9223372036854775807')
  expect(mocks.adjustUserQuota).toHaveBeenNthCalledWith(1, {
    id: 7,
    action: 'add_quota',
    mode: 'add',
    value: 1_000_000,
    reason: '',
    confirmed: false,
  })

  fireEvent.click(screen.getByRole('button', { name: /Increase anyway by/ }))
  await waitFor(() =>
    expect(mocks.adjustUserQuota).toHaveBeenNthCalledWith(2, {
      id: 7,
      action: 'add_quota',
      mode: 'add',
      value: 1_000_000,
      reason: '',
      confirmed: true,
    })
  )
})

test('closing the dialog clears the pending stage confirmation', async () => {
  mocks.adjustUserQuota.mockResolvedValue({
    success: false,
    data: {
      stage_cap_quota: '5000000',
      occupied_after_quota: '6000000',
      stage_overage_quota: '1000000',
    },
  })

  function Harness() {
    const [open, setOpen] = useState(true)
    return (
      <>
        <button type='button' onClick={() => setOpen(true)}>
          Reopen
        </button>
        <UserQuotaDialog
          open={open}
          onOpenChange={setOpen}
          userId={7}
          currentQuota={5_000_000}
          onSuccess={() => {}}
        />
      </>
    )
  }
  render(
    <I18nextProvider i18n={i18n}>
      <Harness />
    </I18nextProvider>
  )

  fireEvent.change(screen.getByLabelText(/Amount/), {
    target: { value: '2' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Confirm increase' }))
  expect(await screen.findByRole('alertdialog')).toBeInTheDocument()

  fireEvent.keyDown(document, { key: 'Escape' })
  await waitFor(() =>
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  )
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
  fireEvent.click(screen.getByRole('button', { name: 'Reopen' }))

  expect(screen.queryByRole('alertdialog')).toBeNull()
})

test('normal mode keeps override and submits without company confirmation', async () => {
  useSystemConfigStore.getState().setConfig({ companyQuotaModeEnabled: false })
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
  expect(screen.queryByLabelText('Reason (optional)')).toBeNull()
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

test('company mode rejects empty, zero, and negative amounts before submission', () => {
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
