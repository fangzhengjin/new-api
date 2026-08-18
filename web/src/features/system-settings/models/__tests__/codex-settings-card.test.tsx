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
    },
  }
  const builtInDefaults = JSON.stringify({
    ...defaultValues.codex,
    request_header_model_patterns: ['^gpt-.*$', '^codex-.*$'],
  })

  test('renders version validation before request header completion', () => {
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
      'Codex Client Version Validation',
      'Codex Client Identity Simulation',
    ])

    const cliSwitch = screen.getByRole('switch', {
      name: 'Codex CLI Client Version Validation',
    })
    const desktopSwitch = screen.getByRole('switch', {
      name: 'Codex Desktop Client Version Validation',
    })
    expect(
      cliSwitch.compareDocumentPosition(desktopSwitch) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
  })

  test('enables the minimum version field only with client validation', async () => {
    const { container } = render(
      <CodexSettingsCard
        defaultValues={defaultValues}
        builtInDefaults={builtInDefaults}
      />
    )

    const minimumVersion = screen.getByRole('textbox', {
      name: 'Minimum Codex CLI Version',
    })
    expect(minimumVersion).toBeDisabled()
    fireEvent.click(
      screen.getByRole('switch', {
        name: 'Codex CLI Client Version Validation',
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
      name: 'Minimum Codex CLI Version',
    })
    const desktopMinimum = screen.getByRole('textbox', {
      name: 'Minimum Codex Desktop Version',
    })
    expect(cliMinimum).toBeDisabled()
    expect(desktopMinimum).toBeDisabled()

    fireEvent.click(
      screen.getByRole('switch', {
        name: 'Codex Desktop Client Version Validation',
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
      name: 'Client Version',
    })
    expect(simulatedVersion).toBeDisabled()
    expect(
      screen.getByText(
        'codex-tui/0.147.0 (Mac OS 26.5.2; arm64) ghostty/1.3.1 (codex-tui; 0.147.0)'
      )
    ).toBeInTheDocument()
    expect(
      screen.getByRole('textbox', { name: 'Operating System' })
    ).toBeDisabled()
    expect(
      screen.getByRole('textbox', { name: 'System Version' })
    ).toBeDisabled()
    expect(screen.getByRole('textbox', { name: 'Architecture' })).toBeDisabled()
    expect(screen.getByRole('textbox', { name: 'Terminal' })).toBeDisabled()
    fireEvent.click(
      screen.getByRole('switch', {
        name: 'Codex Client Identity Simulation',
      })
    )
    expect(simulatedVersion).toBeEnabled()
    expect(
      screen.getByRole('textbox', { name: 'Operating System' })
    ).toBeEnabled()
    expect(screen.getByRole('textbox', { name: 'Terminal' })).toBeEnabled()
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
        name: 'Codex Client Identity Simulation',
      })
    )
    fireEvent.change(
      screen.getByRole('textbox', { name: 'Client Identifier' }),
      {
        target: { value: 'custom-cli' },
      }
    )
    fireEvent.change(screen.getByRole('textbox', { name: 'Terminal' }), {
      target: { value: 'Apple_Terminal/2.14' },
    })
    expect(
      screen.getByText(
        'custom-cli/0.147.0 (Mac OS 26.5.2; arm64) Apple_Terminal/2.14 (custom-cli; 0.147.0)'
      )
    ).toBeInTheDocument()
    fireEvent.submit(container.querySelector('form') as HTMLFormElement)

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith([
        {
          key: 'codex.request_header_fallback_enabled',
          value: true,
        },
        {
          key: 'codex.request_header_fallback_client',
          value: 'custom-cli',
        },
        {
          key: 'codex.request_header_fallback_terminal',
          value: 'Apple_Terminal/2.14',
        },
      ])
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
      screen.getByRole('textbox', { name: 'Client Identifier' }),
      {
        target: { value: 'codex tui' },
      }
    )
    fireEvent.submit(container.querySelector('form') as HTMLFormElement)

    expect(
      await screen.findByText('Enter a valid User-Agent component')
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
        name: 'Codex CLI Client Version Validation',
      })
    )
    fireEvent.change(
      screen.getByRole('textbox', { name: 'Minimum Codex CLI Version' }),
      { target: { value: '0.147.0-beta..1' } }
    )
    fireEvent.submit(container.querySelector('form') as HTMLFormElement)

    expect(
      await screen.findByText('Enter a valid semantic version')
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
      screen.getByRole('textbox', { name: 'Minimum Codex CLI Version' })
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
