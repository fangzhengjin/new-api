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

import { SettingsPageProvider } from '../../components/settings-page-context'
import { ClaudeSettingsCard } from '../claude-settings-card'

const { mutateAsync } = vi.hoisted(() => ({
  mutateAsync: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('../../hooks/use-update-option', () => ({
  useUpdateOption: () => ({ mutateAsync, isPending: false }),
}))

describe('Claude Code client version settings', () => {
  beforeEach(() => {
    mutateAsync.mockClear()
  })

  const defaultValues = {
    claude: {
      client_version_check_enabled: false,
      minimum_client_version: '2.1.233',
      request_header_fallback_enabled: false,
      request_header_fallback_version: '2.1.233',
      request_header_model_patterns: '["^claude-.*$"]',
      model_headers_settings: '{}',
      default_max_tokens: '{"default":8192}',
      thinking_adapter_enabled: true,
      thinking_adapter_budget_tokens_percentage: 0.8,
    },
  }
  const builtInDefaults = JSON.stringify({
    ...defaultValues.claude,
    request_header_model_patterns: ['^claude-.*$'],
    model_headers_settings: {},
    default_max_tokens: { default: 8192 },
  })

  test('renders each settings responsibility in reading order', () => {
    render(
      <ClaudeSettingsCard
        defaultValues={defaultValues}
        builtInDefaults={builtInDefaults}
      />
    )

    expect(
      screen
        .getAllByRole('heading', { level: 3 })
        .map((heading) => heading.textContent)
    ).toEqual([
      'Claude',
      'Claude Code Client Version Validation',
      'Claude Code Client Identity Simulation',
      'Model Request Configuration',
      'Thinking Suffix Adaptation',
    ])
  })

  test('enables and saves the minimum version validation independently', async () => {
    const { container } = render(
      <ClaudeSettingsCard
        defaultValues={defaultValues}
        builtInDefaults={builtInDefaults}
      />
    )

    const minimumVersion = screen.getByRole('textbox', {
      name: 'Minimum Allowed Version',
    })
    expect(minimumVersion).toBeDisabled()
    fireEvent.click(
      screen.getByRole('switch', {
        name: 'Claude Code Client Version Validation',
      })
    )
    expect(minimumVersion).toBeEnabled()
    fireEvent.submit(container.querySelector('form') as HTMLFormElement)

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith({
        key: 'claude.client_version_check_enabled',
        value: true,
      })
    })
  })

  test('enables and saves Claude Code identity simulation independently', async () => {
    const { container } = render(
      <ClaudeSettingsCard
        defaultValues={defaultValues}
        builtInDefaults={builtInDefaults}
      />
    )

    const identityVersion = screen.getByRole('textbox', {
      name: 'Client Version',
    })
    const modelPatterns = screen.getByRole('textbox', {
      name: 'Target Model Patterns',
    })
    expect(identityVersion).toBeDisabled()
    expect(modelPatterns).toBeDisabled()
    expect(modelPatterns).toHaveValue('^claude-.*$')

    fireEvent.click(
      screen.getByRole('switch', {
        name: 'Claude Code Client Identity Simulation',
      })
    )
    expect(identityVersion).toBeEnabled()
    expect(modelPatterns).toBeEnabled()
    fireEvent.submit(container.querySelector('form') as HTMLFormElement)

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith({
        key: 'claude.request_header_fallback_enabled',
        value: true,
      })
    })
  })

  test('restores the built-in draft before it is saved', async () => {
    const actions = document.createElement('div')
    document.body.append(actions)
    const customizedValues = {
      claude: { ...defaultValues.claude, minimum_client_version: '9.9.9' },
    }
    const { container } = render(
      <SettingsPageProvider actionsContainer={actions}>
        <ClaudeSettingsCard
          defaultValues={customizedValues}
          builtInDefaults={builtInDefaults}
        />
      </SettingsPageProvider>
    )

    fireEvent.click(screen.getByRole('button', { name: 'Restore defaults' }))
    expect(
      screen.getByRole('textbox', { name: 'Minimum Allowed Version' })
    ).toHaveValue('2.1.233')
    expect(mutateAsync).not.toHaveBeenCalled()
    expect(
      screen.getByRole('button', { name: 'Restore defaults' })
    ).toBeDisabled()

    fireEvent.submit(container.querySelector('form') as HTMLFormElement)
    await waitFor(() =>
      expect(mutateAsync).toHaveBeenCalledWith({
        key: 'claude.minimum_client_version',
        value: '2.1.233',
      })
    )
    actions.remove()
  })
})
