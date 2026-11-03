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
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { SidebarProvider } from '@/components/ui/sidebar'
import type { ChatPreset } from '@/features/chat/lib/chat-links'
import type { ApiKey } from '@/features/keys/types'

import { ChatPresetsItem } from '../chat-presets-item'

const mocks = vi.hoisted(() => ({
  fetchChatKey: vi.fn(),
  loadAllApiKeys: vi.fn(),
  launchChatPreset: vi.fn(),
  navigate: vi.fn(),
  setOpenMobile: vi.fn(),
  chatPresets: [
    {
      id: '0',
      name: 'Canvas',
      url: 'https://canvas.example/#code={authCode}',
      type: 'web' as const,
      openMode: 'new_tab' as const,
    },
  ] as ChatPreset[],
}))

vi.mock('@tanstack/react-router', () => ({
  Link: (props: {
    children?: ReactNode
    to?: string
    onClick?: () => void
  }) => (
    <a href={props.to} onClick={props.onClick}>
      {props.children}
    </a>
  ),
  useLocation: () => '/',
  useNavigate: () => mocks.navigate,
}))

vi.mock('lucide-react/dynamic', () => ({
  iconNames: ['message-square', 'palette'],
  DynamicIcon: ({ name }: { name: string }) => (
    <svg data-testid={`lucide-${name}`} />
  ),
}))

vi.mock('@/components/ui/sidebar', async (importOriginal) => {
  const original =
    await importOriginal<typeof import('@/components/ui/sidebar')>()
  return {
    ...original,
    useSidebar: () => ({
      state: 'expanded',
      isMobile: false,
      setOpenMobile: mocks.setOpenMobile,
    }),
  }
})

vi.mock('@/features/chat/api', () => ({
  isApiKeyUsable: (token: ApiKey) => token.status === 1,
  launchChatPreset: mocks.launchChatPreset,
  loadAllApiKeys: mocks.loadAllApiKeys,
}))

vi.mock('@/features/chat/hooks/use-active-chat-key', () => ({
  fetchChatKey: mocks.fetchChatKey,
}))

vi.mock('@/features/chat/hooks/use-chat-presets', () => ({
  useChatPresets: () => ({
    chatPresets: mocks.chatPresets,
    serverAddress: 'https://api.example.test',
    chatMenuCollapseThreshold: 3,
  }),
}))

vi.mock('@/stores/auth-store', () => ({
  useAuthStore: (
    selector: (state: { auth: { user: { id: number } } }) => unknown
  ) => selector({ auth: { user: { id: 1 } } }),
}))

function apiKey(id: number): ApiKey {
  return {
    id,
    name: `Key ${id}`,
    key: `sk-a****${id}`,
    status: 1,
    remain_quota: 1,
    unlimited_quota: false,
    expired_time: -1,
  } as ApiKey
}

function renderItem() {
  render(
    <SidebarProvider>
      <ChatPresetsItem item={{ title: 'Chat', type: 'chat-presets' }} />
    </SidebarProvider>
  )
}

const storageKey = 'chat-preset-token:v1:1:Canvas'

function popupWindow() {
  return {
    opener: window,
  } as unknown as Window
}

describe('backend chat preset launch', () => {
  beforeEach(() => {
    window.history.replaceState(null, '', '/')
    window.localStorage.clear()
    mocks.fetchChatKey.mockReset()
    mocks.loadAllApiKeys.mockReset()
    mocks.launchChatPreset.mockReset()
    mocks.navigate.mockReset()
    mocks.setOpenMobile.mockReset()
    mocks.chatPresets = [
      {
        id: '0',
        name: 'Canvas',
        url: 'https://canvas.example/#code={authCode}',
        type: 'web',
        openMode: 'new_tab',
      },
    ]
    vi.restoreAllMocks()
  })

  test('opens the token chooser without flashing a blank tab when multiple keys are available', async () => {
    mocks.loadAllApiKeys.mockResolvedValue([apiKey(1), apiKey(2)])
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    const user = userEvent.setup()
    renderItem()

    await user.click(screen.getByRole('button', { name: /Canvas/ }))

    expect(await screen.findByText('Choose a Token')).toBeInTheDocument()
    expect(
      screen.getByRole('checkbox', { name: 'Remember my choice' })
    ).not.toBeChecked()
    expect(open).not.toHaveBeenCalled()
  })

  test('does not save the selected key unless remembering is checked', async () => {
    mocks.loadAllApiKeys.mockResolvedValue([apiKey(1), apiKey(2)])
    mocks.launchChatPreset.mockResolvedValue(
      'https://canvas.example.test?theme={theme}'
    )
    const popup = popupWindow()
    vi.spyOn(window, 'open').mockReturnValue(popup)
    const user = userEvent.setup()
    renderItem()

    await user.click(screen.getByRole('button', { name: /Canvas/ }))
    await user.click(await screen.findByRole('button', { name: /Key 1/ }))

    await waitFor(() =>
      expect(window.open).toHaveBeenCalledWith(
        'https://canvas.example.test?theme=light',
        '_blank',
        'noopener'
      )
    )
    expect(window.localStorage.getItem(storageKey)).toBeNull()
  })

  test('saves the selected key only after remembering is checked', async () => {
    mocks.loadAllApiKeys.mockResolvedValue([apiKey(1), apiKey(2)])
    mocks.launchChatPreset.mockResolvedValue('https://canvas.example.test')
    vi.spyOn(window, 'open').mockReturnValue(popupWindow())
    const user = userEvent.setup()
    renderItem()

    await user.click(screen.getByRole('button', { name: /Canvas/ }))
    await user.click(
      await screen.findByRole('checkbox', { name: 'Remember my choice' })
    )
    await user.click(screen.getByRole('button', { name: /Key 1/ }))

    await waitFor(() =>
      expect(window.localStorage.getItem(storageKey)).toBe('1')
    )
  })

  test('clears an unavailable remembered key and uses the only remaining Token', async () => {
    window.localStorage.setItem(storageKey, '2')
    mocks.loadAllApiKeys.mockResolvedValue([
      apiKey(1),
      { ...apiKey(2), status: 2 },
    ])
    mocks.launchChatPreset.mockResolvedValue('https://canvas.example.test')
    vi.spyOn(window, 'open').mockReturnValue(popupWindow())
    const user = userEvent.setup()
    renderItem()

    await user.click(screen.getByRole('button', { name: /Canvas/ }))

    await waitFor(() =>
      expect(mocks.launchChatPreset).toHaveBeenCalledWith('Canvas', 1)
    )
    expect(screen.queryByText('Choose a Token')).not.toBeInTheDocument()
    expect(window.localStorage.getItem(storageKey)).toBeNull()
  })

  test('uses an available remembered key without prompting', async () => {
    window.localStorage.setItem(storageKey, '2')
    mocks.loadAllApiKeys.mockResolvedValue([apiKey(1), apiKey(2)])
    mocks.launchChatPreset.mockResolvedValue('https://canvas.example.test')
    const popup = popupWindow()
    vi.spyOn(window, 'open').mockReturnValue(popup)
    const user = userEvent.setup()
    renderItem()

    await user.click(screen.getByRole('button', { name: /Canvas/ }))

    await waitFor(() =>
      expect(mocks.launchChatPreset).toHaveBeenCalledWith('Canvas', 2)
    )
    expect(screen.queryByText('Choose a Token')).not.toBeInTheDocument()
    expect(window.localStorage.getItem(storageKey)).toBe('2')
  })

  test('does not reuse a remembered Token from another preset', async () => {
    window.localStorage.setItem(storageKey, '2')
    mocks.chatPresets = [
      {
        id: '0',
        name: 'Other Canvas',
        url: 'https://other.example/#code={authCode}',
        type: 'web',
        openMode: 'new_tab',
      },
    ]
    mocks.loadAllApiKeys.mockResolvedValue([apiKey(1), apiKey(2)])
    const user = userEvent.setup()
    renderItem()

    await user.click(screen.getByRole('button', { name: /Other Canvas/ }))

    expect(await screen.findByText('Choose a Token')).toBeInTheDocument()
    expect(mocks.launchChatPreset).not.toHaveBeenCalled()
  })

  test('uses the only usable Token without showing the chooser', async () => {
    mocks.loadAllApiKeys.mockResolvedValue([apiKey(1)])
    mocks.launchChatPreset.mockResolvedValue('https://canvas.example.test')
    vi.spyOn(window, 'open').mockReturnValue(popupWindow())
    const user = userEvent.setup()
    renderItem()

    await user.click(screen.getByRole('button', { name: /Canvas/ }))

    await waitFor(() =>
      expect(mocks.launchChatPreset).toHaveBeenCalledWith('Canvas', 1)
    )
    expect(screen.queryByText('Choose a Token')).not.toBeInTheDocument()
    expect(window.localStorage.getItem(storageKey)).toBeNull()
  })

  test('does not replace the current page when noopener returns no window handle', async () => {
    mocks.loadAllApiKeys.mockResolvedValue([apiKey(1)])
    mocks.launchChatPreset.mockResolvedValue('#canvas')
    vi.spyOn(window, 'open').mockReturnValue(null)
    const user = userEvent.setup()
    renderItem()

    await user.click(screen.getByRole('button', { name: /Canvas/ }))

    await waitFor(() =>
      expect(window.open).toHaveBeenCalledWith('#canvas', '_blank', 'noopener')
    )
    expect(window.open).toHaveBeenCalledTimes(1)
    expect(window.location.hash).toBe('')
  })

  test('chooses a Token before entering a protected embedded preset', async () => {
    mocks.chatPresets = [
      {
        id: '0',
        name: 'Canvas',
        url: 'https://canvas.example/#code={authCode}',
        type: 'web',
        openMode: 'embedded',
      },
    ]
    mocks.loadAllApiKeys.mockResolvedValue([apiKey(1), apiKey(2)])
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    const user = userEvent.setup()
    renderItem()

    const presetButton = screen.getByRole('button', { name: 'Canvas' })
    expect(presetButton.querySelector('svg')).toBeNull()
    presetButton.focus()
    await user.keyboard('{Enter}')
    await user.click(await screen.findByRole('button', { name: /Key 2/ }))

    await waitFor(() =>
      expect(mocks.navigate).toHaveBeenCalledWith({
        to: '/chat/$chatId',
        params: { chatId: '0' },
        search: { tokenId: 2 },
      })
    )
    expect(mocks.launchChatPreset).not.toHaveBeenCalled()
    expect(open).not.toHaveBeenCalled()
  })

  test('enters a protected embedded preset directly when only one Token is usable', async () => {
    mocks.chatPresets = [
      {
        id: '0',
        name: 'Canvas',
        url: 'https://canvas.example/#code={authCode}',
        type: 'web',
        openMode: 'embedded',
      },
    ]
    mocks.loadAllApiKeys.mockResolvedValue([apiKey(1)])
    const user = userEvent.setup()
    renderItem()

    await user.click(screen.getByRole('button', { name: 'Canvas' }))

    await waitFor(() =>
      expect(mocks.navigate).toHaveBeenCalledWith({
        to: '/chat/$chatId',
        params: { chatId: '0' },
        search: { tokenId: 1 },
      })
    )
    expect(screen.queryByText('Choose a Token')).not.toBeInTheDocument()
    expect(mocks.launchChatPreset).not.toHaveBeenCalled()
  })

  test('lets multiple Tokens choose the client import configuration', async () => {
    mocks.chatPresets = [
      {
        id: '0',
        name: 'Cherry Studio',
        url: 'cherrystudio://import?data={cherryConfig}',
        type: 'custom-protocol',
      },
    ]
    mocks.loadAllApiKeys.mockResolvedValue([apiKey(1), apiKey(2)])
    mocks.fetchChatKey.mockResolvedValue('sk-selected')
    vi.spyOn(window, 'open').mockReturnValue(popupWindow())
    const user = userEvent.setup()
    renderItem()

    await user.click(screen.getByRole('button', { name: /Cherry Studio/ }))
    await user.click(await screen.findByRole('button', { name: /Key 2/ }))

    await waitFor(() => expect(mocks.fetchChatKey).toHaveBeenCalledWith(2))
    expect(window.open).toHaveBeenCalledWith(
      expect.stringContaining('cherrystudio://import?data='),
      '_blank',
      'noopener'
    )
  })

  test('keeps the empty chooser open and offers the Token page when none are usable', async () => {
    mocks.chatPresets = [
      {
        id: '0',
        name: 'Canvas',
        url: 'https://canvas.example/#code={authCode}',
        type: 'web',
        openMode: 'embedded',
      },
    ]
    mocks.loadAllApiKeys.mockResolvedValue([])
    const user = userEvent.setup()
    renderItem()

    await user.click(screen.getByRole('button', { name: /Canvas/ }))

    expect(await screen.findByText('No usable Token')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Go to Tokens' })).toHaveAttribute(
      'href',
      '/keys'
    )
    expect(mocks.navigate).not.toHaveBeenCalled()
    expect(mocks.launchChatPreset).not.toHaveBeenCalled()
  })

  test('renders an embedded HTTP preset as an internal link with its configured icon', async () => {
    mocks.chatPresets = [
      {
        id: '0',
        name: 'Canvas',
        url: 'https://canvas.example',
        type: 'web',
        openMode: 'embedded',
        icon: 'Palette',
      },
    ]

    renderItem()

    const link = screen.getByRole('link', { name: 'Canvas' })
    expect(link).toHaveAttribute('href', '/chat/$chatId')
    expect(await screen.findByTestId('lucide-palette')).toBeInTheDocument()
  })

  test('does not render preset icons when entries are grouped', () => {
    mocks.chatPresets = Array.from({ length: 4 }, (_, index) => ({
      id: String(index),
      name: `Canvas ${index + 1}`,
      url: `https://canvas-${index + 1}.example`,
      type: 'web',
      openMode: 'embedded',
      icon: 'Palette',
    }))

    renderItem()

    expect(screen.queryByTestId('lucide-palette')).not.toBeInTheDocument()
  })
})
