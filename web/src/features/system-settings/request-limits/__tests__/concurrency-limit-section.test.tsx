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

import { ConcurrencyLimitSection } from '../concurrency-limit-section'

const { mutateAsync } = vi.hoisted(() => ({
  mutateAsync: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('../../hooks/use-update-option', () => ({
  useUpdateOption: () => ({ mutateAsync, isPending: false }),
}))

const defaultValues = {
  ModelRequestConcurrencyLimitEnabled: true,
  ModelRequestConcurrencyLimit: 3,
  ModelRequestIPConcurrencyLimit: 5,
  ModelRequestConcurrencyAccountErrorTemplate: '',
  ModelRequestConcurrencyIPErrorTemplate: '',
}

const defaultTemplates = JSON.stringify({
  ModelRequestConcurrencyAccountErrorTemplate:
    'Account concurrency limit {{.Limit}}\n账号并发限制 {{.Limit}}',
  ModelRequestConcurrencyIPErrorTemplate:
    'IP concurrency limit {{.Limit}}\nIP 并发限制 {{.Limit}}',
})

describe('model request concurrency limit settings', () => {
  beforeEach(() => mutateAsync.mockClear())

  test('shows account and IP limits together without a period field', () => {
    render(
      <ConcurrencyLimitSection
        defaultValues={defaultValues}
        defaultTemplates={defaultTemplates}
      />
    )

    expect(
      screen.getByRole('switch', { name: '启用并发限制' })
    ).toBeChecked()
    const accountLimit = screen.getByRole('spinbutton', {
      name: '每账户最大并发请求数',
    })
    const ipLimit = screen.getByRole('spinbutton', {
      name: '每 IP 最大并发请求数',
    })
    expect(accountLimit).toHaveValue(3)
    expect(ipLimit).toHaveValue(5)
    expect(accountLimit).toHaveAttribute('max', '10000')
    expect(ipLimit).toHaveAttribute('max', '10000')
    expect(screen.queryByRole('spinbutton', { name: /period/i })).toBeNull()
    expect(
      screen.getByRole('button', { name: '账号并发请求数' })
    ).toHaveAttribute('aria-pressed', 'true')
    expect(
      screen.getByRole('button', { name: 'IP 并发请求数' })
    ).toBeVisible()
    expect(
      screen.queryByRole('button', { name: /unavailable/i })
    ).not.toBeInTheDocument()
    ipLimit.focus()
    expect(ipLimit).toHaveFocus()
  })

  test('disables concurrency limits without clearing configured values', async () => {
    const { container } = render(
      <ConcurrencyLimitSection
        defaultValues={defaultValues}
        defaultTemplates={defaultTemplates}
      />
    )
    fireEvent.click(
      screen.getByRole('switch', { name: '启用并发限制' })
    )
    const form = container.querySelector('form')
    if (!form) throw new Error('concurrency limit form was not rendered')
    fireEvent.submit(form)

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith({
        key: 'ModelRequestConcurrencyLimitEnabled',
        value: false,
      })
    })
    expect(
      screen.getByRole('spinbutton', {
        name: '每账户最大并发请求数',
      })
    ).toHaveValue(3)
  })

  test('saves changed limits through the existing option path', async () => {
    const { container } = render(
      <ConcurrencyLimitSection
        defaultValues={defaultValues}
        defaultTemplates={defaultTemplates}
      />
    )
    fireEvent.change(
      screen.getByRole('spinbutton', {
        name: '每账户最大并发请求数',
      }),
      { target: { value: '4' } }
    )
    const form = container.querySelector('form')
    if (!form) throw new Error('concurrency limit form was not rendered')
    fireEvent.submit(form)

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith({
        key: 'ModelRequestConcurrencyLimit',
        value: 4,
      })
    })
  })

  test('rejects values above the supported maximum', async () => {
    const { container } = render(
      <ConcurrencyLimitSection
        defaultValues={defaultValues}
        defaultTemplates={defaultTemplates}
      />
    )
    fireEvent.change(
      screen.getByRole('spinbutton', {
        name: '每 IP 最大并发请求数',
      }),
      { target: { value: '10001' } }
    )
    const form = container.querySelector('form')
    if (!form) throw new Error('concurrency limit form was not rendered')
    fireEvent.submit(form)

    expect(
      await screen.findByText('请输入 0 到 10000 之间的整数')
    ).toBeVisible()
    expect(mutateAsync).not.toHaveBeenCalled()
  })
})
