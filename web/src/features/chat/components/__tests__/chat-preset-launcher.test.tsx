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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, test, vi } from 'vitest'

import { Button } from '@/components/ui/button'
import { useChatPresetLauncher } from '@/features/chat/components/chat-preset-launcher'
import type { ChatPreset } from '@/features/chat/lib/chat-links'

const mocks = vi.hoisted(() => ({
  loadAllApiKeys: vi.fn(),
}))

vi.mock('@/context/theme-provider', () => ({
  useTheme: () => ({ resolvedTheme: 'light' }),
}))

vi.mock('@/features/chat/api', () => ({
  isApiKeyUsable: () => true,
  launchChatPreset: vi.fn(),
  loadAllApiKeys: mocks.loadAllApiKeys,
}))

vi.mock('@/features/chat/hooks/use-active-chat-key', () => ({
  fetchChatKey: vi.fn(),
}))

vi.mock('@/stores/auth-store', () => ({
  useAuthStore: (
    selector: (state: { auth: { user: { id: number } } }) => unknown
  ) => selector({ auth: { user: { id: 1 } } }),
}))

const preset: ChatPreset = {
  id: '0',
  name: 'Canvas',
  url: 'https://canvas.example/?text={textModels}',
  type: 'web',
  openMode: 'embedded',
}

function LauncherHarness(props: { onDismiss: () => void }) {
  const launcher = useChatPresetLauncher(
    vi.fn(),
    'https://api.example.test',
    undefined,
    props.onDismiss
  )
  return (
    <>
      <Button type='button' onClick={() => void launcher.prepareLaunch(preset)}>
        Open
      </Button>
      {launcher.dialog}
    </>
  )
}

describe('chat preset Token chooser', () => {
  test('reports keyboard dismissal to the direct route', async () => {
    mocks.loadAllApiKeys.mockResolvedValue([
      {
        id: 1,
        name: 'Key 1',
        key: 'sk-a****1',
        status: 1,
        expired_time: -1,
      },
      {
        id: 2,
        name: 'Key 2',
        key: 'sk-a****2',
        status: 1,
        expired_time: -1,
      },
    ])
    const onDismiss = vi.fn()
    const user = userEvent.setup()
    render(<LauncherHarness onDismiss={onDismiss} />)

    await user.click(screen.getByRole('button', { name: 'Open' }))
    await screen.findByRole('dialog')
    await user.keyboard('{Escape}')

    await waitFor(() => expect(onDismiss).toHaveBeenCalledTimes(1))
  })

  test('leaves the direct route when Token loading fails', async () => {
    mocks.loadAllApiKeys.mockRejectedValue(new Error('request failed'))
    const onDismiss = vi.fn()
    const user = userEvent.setup()
    render(<LauncherHarness onDismiss={onDismiss} />)

    await user.click(screen.getByRole('button', { name: 'Open' }))

    await waitFor(() => expect(onDismiss).toHaveBeenCalledTimes(1))
  })
})
