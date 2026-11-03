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
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { CodexSettingsCard } from '../codex-settings-card'

const { mutateAsync } = vi.hoisted(() => ({
  mutateAsync: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('../../hooks/use-update-option', () => ({
  useUpdateOption: () => ({ mutateAsync, isPending: false }),
}))

describe('Codex settings card', () => {
  beforeEach(() => {
    mutateAsync.mockClear()
  })

  const defaultValues = {
    codex: {
      client_version_check_enabled: false,
      minimum_client_version: '0.147.0',
      desktop_client_version_check_enabled: false,
      minimum_desktop_client_version: '26.810.52044',
      request_header_fallback_enabled: false,
      request_header_fallback_client: 'codex-tui',
      request_header_fallback_version: '0.147.0',
      request_header_fallback_os: 'Mac OS',
      request_header_fallback_os_version: '26.5.2',
      request_header_fallback_architecture: 'arm64',
      request_header_fallback_terminal: 'ghostty/1.3.1',
      request_header_model_patterns: '["^gpt-.*$","^codex-.*$"]',
      error_response_mappings: '[]',
    },
  }
  const builtInDefaults = JSON.stringify({
    ...defaultValues.codex,
    request_header_model_patterns: ['^gpt-.*$', '^codex-.*$'],
    error_response_mappings: [],
  })

  test('renders error response mappings last with editor actions above the status', () => {
    render(
      <CodexSettingsCard
        defaultValues={defaultValues}
        builtInDefaults={builtInDefaults}
      />
    )

    expect(
      screen
        .getAllByRole('heading', { level: 3 })
        .map((heading) => heading.textContent)
    ).toEqual([
      'Codex',
      'Codex 客户端版本校验',
      'Codex 客户端身份模拟',
      'Codex 错误响应映射',
    ])

    const identityHeading = screen.getByRole('heading', {
      name: 'Codex 客户端身份模拟',
    })
    const mappingHeading = screen.getByRole('heading', {
      name: 'Codex 错误响应映射',
    })
    expect(
      identityHeading.compareDocumentPosition(mappingHeading) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    const mappingSection = mappingHeading.closest('section')
    expect(mappingSection).toHaveClass('pb-10')
    const actionGroup = within(mappingSection as HTMLElement).getByRole(
      'group',
      { name: '错误响应映射操作' }
    )
    expect(
      within(actionGroup).getByRole('button', { name: '新建映射规则' })
    ).toBeVisible()
    expect(
      within(actionGroup).getByRole('button', { name: '填充模板' })
    ).toBeVisible()
    expect(
      within(actionGroup).getByRole('button', { name: 'Switch to JSON' })
    ).toBeVisible()
    const status = within(mappingSection as HTMLElement).getByText(
      '已配置 · 0 条规则'
    )
    expect(actionGroup).not.toContainElement(status)
    expect(
      actionGroup.compareDocumentPosition(status) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()

    const cliSwitch = screen.getByRole('switch', {
      name: 'Codex CLI 客户端版本校验',
    })
    const desktopSwitch = screen.getByRole('switch', {
      name: 'Codex Desktop 客户端版本校验',
    })
    expect(
      cliSwitch.compareDocumentPosition(desktopSwitch) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
  })

  test('fills a retryable-rate-limit template and saves one mapping', async () => {
    const user = userEvent.setup()
    const { container } = render(
      <CodexSettingsCard
        defaultValues={defaultValues}
        builtInDefaults={builtInDefaults}
      />
    )

    await user.click(screen.getByRole('button', { name: '填充模板' }))
    expect(screen.getByText('已配置 · 1 条规则')).toBeVisible()
    expect(
      screen.getByRole<HTMLTextAreaElement>('textbox', {
        name: '消息匹配条件',
      }).value
    ).toContain('Request rate increased too quickly.')
    expect(screen.getByText('HTTP 503 · application/json')).toBeVisible()

    fireEvent.submit(container.querySelector('form') as HTMLFormElement)

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith({
        key: 'codex.error_response_mappings',
        value:
          '[{"match":{"status_code":429,"message_patterns":["Request rate increased too quickly.","Requests rate limit exceeded","re:(request|token).*rate limit exceeded"]},"rewrite":{"status_code":503,"type":"server_error","code":"server_is_overloaded","message":"Temporary upstream rate limit"}}]',
      })
    })
  })

  test('rejects a non-error response status and associates its message', async () => {
    const user = userEvent.setup()
    const { container } = render(
      <CodexSettingsCard
        defaultValues={defaultValues}
        builtInDefaults={builtInDefaults}
      />
    )

    await user.click(screen.getByRole('button', { name: '新建映射规则' }))
    const targetStatus = screen.getByRole('spinbutton', {
      name: '返回状态码',
    })
    await user.clear(targetStatus)
    await user.type(targetStatus, '200')
    expect(
      screen.getByRole('spinbutton', { name: '返回状态码' })
    ).toHaveValue(200)
    fireEvent.submit(container.querySelector('form') as HTMLFormElement)

    const message = await screen.findByText(
      '请输入 400 到 599 之间的 HTTP 错误状态码'
    )
    const error = message.closest('[role="alert"]')
    expect(error).not.toBeNull()
    expect(targetStatus).toHaveAttribute('aria-describedby', error?.id)
    expect(mutateAsync).not.toHaveBeenCalled()
  })

  test('enables the minimum version field only with client validation', async () => {
    const { container } = render(
      <CodexSettingsCard
        defaultValues={defaultValues}
        builtInDefaults={builtInDefaults}
      />
    )

    const minimumVersion = screen.getByRole('textbox', {
      name: 'Codex CLI 最低版本',
    })
    expect(minimumVersion).toBeDisabled()
    fireEvent.click(
      screen.getByRole('switch', {
        name: 'Codex CLI 客户端版本校验',
      })
    )
    expect(minimumVersion).toBeEnabled()
    const form = container.querySelector('form')
    expect(form).not.toBeNull()
    fireEvent.submit(form as HTMLFormElement)

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith({
        key: 'codex.client_version_check_enabled',
        value: true,
      })
    })
  })

  test('enables and saves Desktop validation independently from CLI', async () => {
    const { container } = render(
      <CodexSettingsCard
        defaultValues={defaultValues}
        builtInDefaults={builtInDefaults}
      />
    )

    const cliMinimum = screen.getByRole('textbox', {
      name: 'Codex CLI 最低版本',
    })
    const desktopMinimum = screen.getByRole('textbox', {
      name: 'Codex Desktop 最低版本',
    })
    expect(cliMinimum).toBeDisabled()
    expect(desktopMinimum).toBeDisabled()

    fireEvent.click(
      screen.getByRole('switch', {
        name: 'Codex Desktop 客户端版本校验',
      })
    )
    expect(cliMinimum).toBeDisabled()
    expect(desktopMinimum).toBeEnabled()
    fireEvent.submit(container.querySelector('form') as HTMLFormElement)

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith({
        key: 'codex.desktop_client_version_check_enabled',
        value: true,
      })
    })
  })

  test('saves the upstream header completion switch independently', async () => {
    const { container } = render(
      <CodexSettingsCard
        defaultValues={defaultValues}
        builtInDefaults={builtInDefaults}
      />
    )

    const simulatedVersion = screen.getByRole('textbox', {
      name: '客户端版本',
    })
    expect(simulatedVersion).toBeDisabled()
    expect(
      screen.getByText(
        'codex-tui/0.147.0 (Mac OS 26.5.2; arm64) ghostty/1.3.1 (codex-tui; 0.147.0)'
      )
    ).toBeInTheDocument()
    expect(
      screen.getByRole('textbox', { name: '操作系统' })
    ).toBeDisabled()
    expect(
      screen.getByRole('textbox', { name: 'System Version' })
    ).toBeDisabled()
    expect(screen.getByRole('textbox', { name: '架构' })).toBeDisabled()
    expect(screen.getByRole('textbox', { name: '终端' })).toBeDisabled()
    fireEvent.click(
      screen.getByRole('switch', {
        name: 'Codex 客户端身份模拟',
      })
    )
    expect(simulatedVersion).toBeEnabled()
    expect(
      screen.getByRole('textbox', { name: '操作系统' })
    ).toBeEnabled()
    expect(screen.getByRole('textbox', { name: '终端' })).toBeEnabled()
    fireEvent.submit(container.querySelector('form') as HTMLFormElement)

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith({
        key: 'codex.request_header_fallback_enabled',
        value: true,
      })
    })
  })

  test('previews and saves a configured terminal identity', async () => {
    const { container } = render(
      <CodexSettingsCard
        defaultValues={defaultValues}
        builtInDefaults={builtInDefaults}
      />
    )

    fireEvent.click(
      screen.getByRole('switch', {
        name: 'Codex 客户端身份模拟',
      })
    )
    fireEvent.change(
      screen.getByRole('textbox', { name: '客户端标识' }),
      {
        target: { value: 'custom-cli' },
      }
    )
    fireEvent.change(screen.getByRole('textbox', { name: '终端' }), {
      target: { value: 'Apple_Terminal/2.14' },
    })
    expect(
      screen.getByText(
        'custom-cli/0.147.0 (Mac OS 26.5.2; arm64) Apple_Terminal/2.14 (custom-cli; 0.147.0)'
      )
    ).toBeInTheDocument()
    fireEvent.submit(container.querySelector('form') as HTMLFormElement)

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith({
        key: 'codex.request_header_fallback_enabled',
        value: true,
      })
    })
    expect(mutateAsync).toHaveBeenCalledWith({
      key: 'codex.request_header_fallback_client',
      value: 'custom-cli',
    })
    expect(mutateAsync).toHaveBeenCalledWith({
      key: 'codex.request_header_fallback_terminal',
      value: 'Apple_Terminal/2.14',
    })
  })

  test('rejects an invalid User-Agent client identifier', async () => {
    const { container } = render(
      <CodexSettingsCard
        defaultValues={defaultValues}
        builtInDefaults={builtInDefaults}
      />
    )

    fireEvent.change(
      screen.getByRole('textbox', { name: '客户端标识' }),
      {
        target: { value: 'codex tui' },
      }
    )
    fireEvent.submit(container.querySelector('form') as HTMLFormElement)

    expect(
      await screen.findByText('请输入有效的 User-Agent 组成字段')
    ).toBeInTheDocument()
    expect(mutateAsync).not.toHaveBeenCalled()
  })

  test('rejects an invalid prerelease version before saving', async () => {
    const { container } = render(
      <CodexSettingsCard
        defaultValues={defaultValues}
        builtInDefaults={builtInDefaults}
      />
    )

    fireEvent.click(
      screen.getByRole('switch', {
        name: 'Codex CLI 客户端版本校验',
      })
    )
    fireEvent.change(
      screen.getByRole('textbox', { name: 'Codex CLI 最低版本' }),
      { target: { value: '0.147.0-beta..1' } }
    )
    fireEvent.submit(container.querySelector('form') as HTMLFormElement)

    expect(
      await screen.findByText('请输入有效的语义化版本号')
    ).toBeInTheDocument()
    expect(mutateAsync).not.toHaveBeenCalled()
  })

  test('restores the built-in draft before it is saved', async () => {
    const actions = document.createElement('div')
    document.body.append(actions)
    const customizedValues = {
      codex: { ...defaultValues.codex, minimum_client_version: '9.9.9' },
    }
    const { container } = render(
      <SettingsPageProvider actionsContainer={actions}>
        <CodexSettingsCard
          defaultValues={customizedValues}
          builtInDefaults={builtInDefaults}
        />
      </SettingsPageProvider>
    )

    fireEvent.click(screen.getByRole('button', { name: 'Restore defaults' }))
    expect(
      screen.getByRole('textbox', { name: 'Codex CLI 最低版本' })
    ).toHaveValue('0.147.0')
    expect(mutateAsync).not.toHaveBeenCalled()
    expect(
      screen.getByRole('button', { name: 'Restore defaults' })
    ).toBeDisabled()

    fireEvent.submit(container.querySelector('form') as HTMLFormElement)
    await waitFor(() =>
      expect(mutateAsync).toHaveBeenCalledWith({
        key: 'codex.minimum_client_version',
        value: '0.147.0',
      })
    )
    actions.remove()
  })
})
