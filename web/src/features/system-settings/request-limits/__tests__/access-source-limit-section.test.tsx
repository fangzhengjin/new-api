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
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { AccessSourceLimitSection } from '../access-source-limit-section'

const { mutateAsync } = vi.hoisted(() => ({
  mutateAsync: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('../../hooks/use-update-option', () => ({
  useUpdateOption: () => ({ mutateAsync, isPending: false }),
}))

const defaultValues = {
  AccessSourceLimitEnabled: true,
  AccessSourceAssociationWindowHours: 24,
  AccessSourceMaxIPsPerUser: 4,
  AccessSourceSwitchCooldownMinutes: 30,
  AccessSourceMaxUsersPerIP: 5,
  AccessSourceSwitchCooldownErrorTemplate: '',
  AccessSourceAccountIPLimitErrorTemplate: '',
  AccessSourceIPAccountLimitErrorTemplate: '',
}

const defaultTemplates = JSON.stringify({
  AccessSourceSwitchCooldownErrorTemplate:
    'Retry in {{.RetryAfter}}\n请在 {{.RetryAfter}} 后重试',
  AccessSourceAccountIPLimitErrorTemplate:
    'Account IP limit {{.Limit}} within {{.Period}}\n账号在 {{.Period}} 内最多关联 {{.Limit}} 个 IP',
  AccessSourceIPAccountLimitErrorTemplate:
    'IP account limit {{.Limit}} within {{.Period}}\nIP 在 {{.Period}} 内最多关联 {{.Limit}} 个账号',
})

describe('access source limit settings', () => {
  beforeEach(() => mutateAsync.mockClear())

  test('exposes every source rule with its supported range', () => {
    render(
      <AccessSourceLimitSection
        defaultValues={defaultValues}
        defaultTemplates={defaultTemplates}
      />
    )

    expect(
      screen.getByRole('switch', { name: 'Enable access source limits' })
    ).toBeChecked()
    expect(
      screen.getByRole('spinbutton', { name: 'Association period (hours)' })
    ).toHaveValue(24)
    expect(
      screen.getByRole('spinbutton', { name: 'IP switch wait (minutes)' })
    ).toHaveValue(30)
    expect(
      screen.getByRole('spinbutton', { name: 'Association period (hours)' })
    ).toHaveAttribute('max', '168')
    expect(
      screen.getByRole('spinbutton', { name: 'IP switch wait (minutes)' })
    ).toHaveAttribute('max', '1440')
    expect(
      screen.getByRole('spinbutton', { name: 'Max IPs per account' })
    ).toHaveAttribute('max', '1000')
    expect(
      screen.getByRole('spinbutton', { name: 'Max accounts per IP' })
    ).toHaveAttribute('max', '1000')

    fireEvent.click(
      screen.getByRole('button', { name: 'Account associated IPs' })
    )
    expect(screen.getByText(/Account IP limit 4 within 24h/)).toBeVisible()
    expect(
      screen.getByRole('button', { name: 'Insert variable {{.Period}}' })
    ).toBeVisible()
  })

  test('saves changed rules in one batch', async () => {
    const { container } = render(
      <AccessSourceLimitSection
        defaultValues={defaultValues}
        defaultTemplates={defaultTemplates}
      />
    )
    fireEvent.change(
      screen.getByRole('spinbutton', { name: 'IP switch wait (minutes)' }),
      { target: { value: '60' } }
    )
    const form = container.querySelector('form')
    if (!form) throw new Error('access source form was not rendered')
    fireEvent.submit(form)

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith([
        { key: 'AccessSourceSwitchCooldownMinutes', value: 60 },
      ])
    })
  })

  test('rejects an association period longer than seven days', async () => {
    const { container } = render(
      <AccessSourceLimitSection
        defaultValues={defaultValues}
        defaultTemplates={defaultTemplates}
      />
    )
    fireEvent.change(
      screen.getByRole('spinbutton', { name: 'Association period (hours)' }),
      { target: { value: '169' } }
    )
    const form = container.querySelector('form')
    if (!form) throw new Error('access source form was not rendered')
    fireEvent.submit(form)

    expect(
      await screen.findByText('Enter an integer from 1 to 168')
    ).toBeVisible()
    expect(mutateAsync).not.toHaveBeenCalled()
  })
})
