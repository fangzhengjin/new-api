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

import { RateLimitSection } from '../rate-limit-section'

const { mutateAsync } = vi.hoisted(() => ({
  mutateAsync: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('../../hooks/use-update-option', () => ({
  useUpdateOption: () => ({ mutateAsync, isPending: false }),
}))

vi.mock('../rate-limit-visual-editor', () => ({
  RateLimitVisualEditor: (props: { onChange: (value: string) => void }) => (
    <button type='button' onClick={() => props.onChange('{"default":[200,0]}')}>
      Set unlimited group success limit
    </button>
  ),
}))

const defaultValues = {
  ModelRequestRateLimitEnabled: true,
  ModelRequestRateLimitDurationMinutes: 1,
  ModelRequestRateLimitCount: 200,
  ModelRequestIPRateLimitCount: 0,
  ModelRequestIPRateLimitSuccessCount: 0,
  ModelRequestRateLimitSuccessCount: 100,
  ModelRequestRateLimitGroup: '{"default":[200,100]}',
  ModelRequestRateLimitAccountTotalErrorTemplate: '',
  ModelRequestRateLimitAccountSuccessErrorTemplate: '',
  ModelRequestRateLimitIPTotalErrorTemplate: '',
  ModelRequestRateLimitIPSuccessErrorTemplate: '',
}

const defaultTemplates = JSON.stringify({
  ModelRequestRateLimitAccountTotalErrorTemplate:
    'Account limit {{.Limit}} per {{.Period}}, retry in {{.RetryAfter}}\n账号限制 {{.Limit}} 次，请在 {{.RetryAfter}} 后重试',
  ModelRequestRateLimitAccountSuccessErrorTemplate: 'Account success limit',
  ModelRequestRateLimitIPTotalErrorTemplate: 'IP total limit',
  ModelRequestRateLimitIPSuccessErrorTemplate: 'IP success limit',
})

describe('model request IP rate limit settings', () => {
  beforeEach(() => {
    mutateAsync.mockClear()
  })

  test('shows the IP field and explains how all limits relate', () => {
    render(
      <RateLimitSection
        defaultValues={defaultValues}
        defaultTemplates={defaultTemplates}
      />
    )

    const accountLimits = screen.getByRole('group', {
      name: 'Account Request Limits',
    })
    const ipLimits = screen.getByRole('group', { name: 'IP Request Limits' })
    const period = screen.getByRole('spinbutton', {
      name: 'Limit period (minutes)',
    })
    const ipLimit = screen.getByRole('spinbutton', {
      name: 'Max requests per IP per period',
    })
    const accountSuccessLimit = screen.getByRole('spinbutton', {
      name: 'Max successful requests per account per period',
    })
    const ipSuccessLimit = screen.getByRole('spinbutton', {
      name: 'Max successful requests per IP per period',
    })
    const groupLabel = screen.getByText('Group-based rate limits')

    expect(ipLimit).toHaveValue(0)
    expect(period).toHaveValue(1)
    expect(accountSuccessLimit).toHaveValue(100)
    expect(ipSuccessLimit).toHaveValue(0)
    expect(
      screen.getByRole('button', { name: 'Rejection messages' })
    ).toHaveAttribute('aria-expanded', 'true')
    expect(ipLimit).toHaveAttribute('min', '0')
    expect(ipLimit).toHaveAttribute('max', '100000000')
    expect(ipSuccessLimit).toHaveAttribute('min', '0')
    expect(ipSuccessLimit).toHaveAttribute('max', '100000000')
    expect(accountLimits).toHaveClass('md:grid-cols-2')
    expect(ipLimits).toHaveClass('md:grid-cols-2')
    expect(
      accountLimits.compareDocumentPosition(ipLimits) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    ipSuccessLimit.focus()
    expect(ipSuccessLimit).toHaveFocus()
    expect(
      screen.getByText('Account and IP limits share this period')
    ).toBeVisible()
    expect(screen.queryByText('times')).not.toBeInTheDocument()
    expect(
      screen.getByText(
        'Counts all requests from the same IP, including failures, 0 = unlimited'
      )
    ).toBeVisible()
    expect(
      screen.getByText(
        'Only overrides account request limits, does not affect IP limits, and accounts in the same group do not share counters'
      )
    ).toBeVisible()
    expect(
      ipLimit.compareDocumentPosition(groupLabel) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
  })

  test('saves a valid IP success limit through the existing option path', async () => {
    const { container } = render(
      <RateLimitSection
        defaultValues={defaultValues}
        defaultTemplates={defaultTemplates}
      />
    )
    const form = container.querySelector('form')
    if (!form) throw new Error('rate limit form was not rendered')

    fireEvent.change(
      screen.getByRole('spinbutton', {
        name: 'Max successful requests per IP per period',
      }),
      { target: { value: '25' } }
    )
    fireEvent.submit(form)

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledOnce()
      expect(mutateAsync).toHaveBeenCalledWith([
        { key: 'ModelRequestIPRateLimitSuccessCount', value: 25 },
      ])
    })
  })

  test('accepts zero as an unlimited group success limit', async () => {
    const { container } = render(
      <RateLimitSection
        defaultValues={defaultValues}
        defaultTemplates={defaultTemplates}
      />
    )
    const form = container.querySelector('form')
    if (!form) throw new Error('rate limit form was not rendered')

    fireEvent.click(
      screen.getByRole('button', {
        name: 'Set unlimited group success limit',
      })
    )
    fireEvent.submit(form)

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith([
        { key: 'ModelRequestRateLimitGroup', value: '{"default":[200,0]}' },
      ])
    })
  })

  test('rejects an IP request limit above the supported form range', async () => {
    const { container } = render(
      <RateLimitSection
        defaultValues={defaultValues}
        defaultTemplates={defaultTemplates}
      />
    )
    const form = container.querySelector('form')
    if (!form) throw new Error('rate limit form was not rendered')

    fireEvent.change(
      screen.getByRole('spinbutton', {
        name: 'Max successful requests per IP per period',
      }),
      { target: { value: '100000001' } }
    )
    fireEvent.submit(form)

    expect(
      await screen.findByText('Enter an integer from 0 to 100000000')
    ).toBeVisible()
    expect(mutateAsync).not.toHaveBeenCalled()
  })

  test('edits and saves the selected rejection scenario', async () => {
    const { container } = render(
      <RateLimitSection
        defaultValues={defaultValues}
        defaultTemplates={defaultTemplates}
      />
    )

    expect(
      screen.getByRole('button', { name: 'Account total requests' })
    ).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByText(/Account limit 200 per 1m/)).toBeVisible()
    expect(screen.getByText(/账号限制 200 次/)).toBeVisible()

    fireEvent.click(screen.getByRole('button', { name: 'IP total requests' }))
    const textarea = screen.getByRole('textbox', {
      name: 'Custom rejection message (optional)',
    }) as HTMLTextAreaElement
    fireEvent.change(textarea, {
      target: {
        value: 'IP limit {{.Limit}} per {{.Period}}\nIP 限制 {{.Limit}} 次',
      },
    })
    const form = container.querySelector('form')
    if (!form) throw new Error('rate limit form was not rendered')
    fireEvent.submit(form)

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith([
        {
          key: 'ModelRequestRateLimitIPTotalErrorTemplate',
          value: 'IP limit {{.Limit}} per {{.Period}}\nIP 限制 {{.Limit}} 次',
        },
      ])
    })
  })

  test('inserts an available variable at the current cursor', () => {
    render(
      <RateLimitSection
        defaultValues={defaultValues}
        defaultTemplates={defaultTemplates}
      />
    )

    const textarea = screen.getByRole('textbox', {
      name: 'Custom rejection message (optional)',
    }) as HTMLTextAreaElement
    fireEvent.change(textarea, { target: { value: 'Retry ' } })
    textarea.setSelectionRange(6, 6)
    fireEvent.click(
      screen.getByRole('button', { name: 'Insert variable {{.Limit}}' })
    )

    expect(textarea).toHaveValue('Retry {{.Limit}}')
    expect(textarea).toHaveFocus()
  })

  test('associates the template field with one complete description', () => {
    const { container } = render(
      <RateLimitSection
        defaultValues={defaultValues}
        defaultTemplates={defaultTemplates}
      />
    )

    const textarea = screen.getByRole('textbox', {
      name: 'Custom rejection message (optional)',
    })
    const descriptionId = textarea.getAttribute('aria-describedby')

    expect(descriptionId).toBeTruthy()
    const descriptions = container.querySelectorAll(`[id="${descriptionId}"]`)
    expect(descriptions).toHaveLength(1)
    expect(descriptions[0]).toHaveTextContent('45s, 2m 15s, and 1h 5m')
    expect(descriptions[0]).toHaveTextContent(
      'English and Chinese are returned on separate lines'
    )
  })
})
